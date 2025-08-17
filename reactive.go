package gobasetools

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/sourcegraph/conc/pool"
	"golang.org/x/sync/errgroup"
)

// BackpressurePolicy defines how to behave when a subscriber's mailbox is full.
type BackpressurePolicy int

const (
	// BackpressureDropNewest drops the incoming value if the subscriber's mailbox is full (default).
	BackpressureDropNewest BackpressurePolicy = iota
	// BackpressureDropOldest removes one oldest pending value from the mailbox (if any) and enqueues the new one.
	BackpressureDropOldest
	// BackpressureBlock blocks the broadcaster until there is space in the subscriber's mailbox.
	// This may degrade real-time characteristics if any subscriber is slow.
	BackpressureBlock
)

const defaultBufferSize = 64

// SubConfig controls per-subscriber behavior.
type SubConfig[T any] struct {
	// Buffer is the per-subscriber mailbox depth (and outward channel depth).
	Buffer int
	// Policy determines backpressure behavior when the mailbox is full.
	Policy BackpressurePolicy
	// ReplayLatest, if true, immediately delivers the most recently broadcast value to the new subscriber (if any).
	ReplayLatest bool
	// DropCallback is invoked when a value is dropped for this subscriber due to backpressure.
	DropCallback func(id string, value T)
	// FlushOnUnsubscribe, if true, attempts to drain the mailbox to the outward channel before closing on unsubscribe.
	// Default false (fast close, drop remaining).
	FlushOnUnsubscribe bool
}

// SubOption modifies SubConfig.
type SubOption[T any] func(*SubConfig[T])

func WithBuffer[T any](n int) SubOption[T] {
	return func(c *SubConfig[T]) {
		if n < 0 {
			n = 0
		}
		c.Buffer = n
	}
}

func WithPolicy[T any](p BackpressurePolicy) SubOption[T] {
	return func(c *SubConfig[T]) { c.Policy = p }
}

func WithReplayLatest[T any](enable bool) SubOption[T] {
	return func(c *SubConfig[T]) { c.ReplayLatest = enable }
}

func WithDropCallback[T any](fn func(id string, value T)) SubOption[T] {
	return func(c *SubConfig[T]) { c.DropCallback = fn }
}

func WithFlushOnUnsubscribe[T any](enable bool) SubOption[T] {
	return func(c *SubConfig[T]) { c.FlushOnUnsubscribe = enable }
}

type Observable[T any] interface {
	// Subscribe registers a subscriber with id and returns a receive-only channel.
	// If ctx is canceled before subscription is established, returns ctx.Err.
	// An error is returned if the id already exists or the observable is closed.
	Subscribe(ctx context.Context, id string, opts ...SubOption[T]) (<-chan T, error)

	// Unsubscribe removes a subscriber by id and closes its channel.
	// Returns an error if no such subscriber exists.
	Unsubscribe(id string) error

	// Broadcast delivers value to all subscribers according to their backpressure policy.
	// This call is non-blocking per-subscriber by default (DropNewest).
	Broadcast(value T)

	// BroadcastCtx is like Broadcast but can return early if ctx is canceled while applying
	// blocking backpressure policies. For non-blocking policies, it behaves like Broadcast.
	BroadcastCtx(ctx context.Context, value T) error

	// Notify delivers value to a specific subscriber using that subscriber's backpressure policy.
	// Returns an error if the subscriber doesn't exist or if blocking is interrupted by ctx.
	Notify(id string, value T) error
	NotifyCtx(ctx context.Context, id string, value T) error

	// Close closes the observable and all subscriber channels. Further Subscribe returns an error,
	// and Broadcast/Notify become no-ops (or return errors for context variants).
	Close()

	// Closed reports whether the observable has been closed.
	Closed() bool

	// Len returns the number of current subscribers.
	Len() int
}

type subscriber[T any] struct {
	id      string
	out     chan T // outward-facing channel (receive-only to consumer)
	mailbox chan T // internal queue; writers: broadcaster; reader: worker
	done    chan struct{}
	cfg     SubConfig[T]
}

type observable[T any] struct {
	// Slow-path state protected by subsMu
	subsMu sync.RWMutex
	subs   map[string]*subscriber[T]

	// Fast-path snapshot for Broadcast: atomically swapped slice of subscribers.
	// Store type: []*subscriber[T]
	subsSnap atomic.Value

	// Closed flag (0 = open, 1 = closed)
	closed uint32

	// Most recent broadcast value (for ReplayLatest)
	// Stores *T; nil if none.
	last atomic.Value

	// Defaults for new subscribers
	def SubConfig[T]

	// Worker management with modern concurrency primitives
	workerPool *pool.Pool
}

// NewObservable creates a new observable with sensible defaults:
// - Buffer: 64
// - Policy: DropNewest
func NewObservable[T any]() Observable[T] {
	o := &observable[T]{
		subs: make(map[string]*subscriber[T]),
		def: SubConfig[T]{
			Buffer:             defaultBufferSize,
			Policy:             BackpressureDropNewest,
			ReplayLatest:       false,
			DropCallback:       nil,
			FlushOnUnsubscribe: false,
		},
		workerPool: pool.New(),
	}
	o.subsSnap.Store([]*subscriber[T]{})
	o.last.Store((*T)(nil))
	return o
}

// NewObservableWithDefaults allows overriding defaults applied to all new subscribers.
func NewObservableWithDefaults[T any](defaults SubConfig[T]) Observable[T] {
	// Normalize defaults
	if defaults.Buffer < 0 {
		defaults.Buffer = 0
	}
	o := &observable[T]{
		subs:       make(map[string]*subscriber[T]),
		def:        defaults,
		workerPool: pool.New(),
	}
	o.subsSnap.Store([]*subscriber[T]{})
	o.last.Store((*T)(nil))
	return o
}

func (o *observable[T]) isClosed() bool {
	return atomic.LoadUint32(&o.closed) == 1
}

func (o *observable[T]) Subscribe(ctx context.Context, id string, opts ...SubOption[T]) (<-chan T, error) {
	if id == "" {
		return nil, errors.New("subscriber id must not be empty")
	}
	if o.isClosed() {
		return nil, errors.New("observable is closed")
	}

	// Build config from defaults + options
	cfg := o.def
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.Buffer < 0 {
		cfg.Buffer = 0
	}

	sub := &subscriber[T]{
		id:      id,
		out:     make(chan T, cfg.Buffer),
		mailbox: make(chan T, cfg.Buffer),
		done:    make(chan struct{}),
		cfg:     cfg,
	}

	// Register under lock
	o.subsMu.Lock()
	if o.isClosed() {
		o.subsMu.Unlock()
		return nil, errors.New("observable is closed")
	}
	if _, exists := o.subs[id]; exists {
		o.subsMu.Unlock()
		return nil, fmt.Errorf("subscriber with id %s already exists", id)
	}
	o.subs[id] = sub

	// Rebuild snapshot for fast-path broadcast
	snap := make([]*subscriber[T], 0, len(o.subs))
	for _, s := range o.subs {
		snap = append(snap, s)
	}
	o.subsSnap.Store(snap)
	o.subsMu.Unlock()

	// Start worker to forward mailbox -> out using the managed pool
	// but maintain the original asynchronous timing semantics
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Handle any panics in worker goroutines gracefully
				// If the pool is closed, just run the worker directly
				if sub.cfg.FlushOnUnsubscribe {
					o.forwardWithFlush(sub)
				} else {
					o.forwardDropOnClose(sub)
				}
			}
		}()
		
		if o.isClosed() {
			// If already closed, run directly
			if sub.cfg.FlushOnUnsubscribe {
				o.forwardWithFlush(sub)
			} else {
				o.forwardDropOnClose(sub)
			}
			return
		}
		
		o.workerPool.Go(func() {
			if sub.cfg.FlushOnUnsubscribe {
				o.forwardWithFlush(sub)
			} else {
				o.forwardDropOnClose(sub)
			}
		})
	}()

	// Optional immediate replay of latest broadcasted value
	if cfg.ReplayLatest {
		if ptr, ok := o.last.Load().(*T); ok && ptr != nil {
			o.enqueue(nil, sub, *ptr, nil) // nil ctx => non-blocking per policy (except Block which will block)
		}
	}

	// Respect context cancellation before handing out the channel
	select {
	case <-ctx.Done():
		// Undo subscription
		_ = o.Unsubscribe(id)
		return nil, ctx.Err()
	default:
	}

	return sub.out, nil
}

func (o *observable[T]) Unsubscribe(id string) error {
	o.subsMu.Lock()
	sub, exists := o.subs[id]
	if !exists {
		o.subsMu.Unlock()
		return fmt.Errorf("no subscriber with id %s", id)
	}
	delete(o.subs, id)

	// Rebuild fast-path snapshot
	snap := make([]*subscriber[T], 0, len(o.subs))
	for _, s := range o.subs {
		snap = append(snap, s)
	}
	o.subsSnap.Store(snap)
	o.subsMu.Unlock()

	// Signal worker to stop; outward channel will be closed by worker.
	close(sub.done)
	return nil
}

func (o *observable[T]) Broadcast(value T) {
	_ = o.BroadcastCtx(context.Background(), value)
}

func (o *observable[T]) BroadcastCtx(ctx context.Context, value T) error {
	if o.isClosed() {
		return nil
	}

	// Store latest for replay-latest subscribers.
	valCopy := value
	o.last.Store(&valCopy)

	// Read atomic snapshot
	raw := o.subsSnap.Load()
	if raw == nil {
		return nil
	}
	subs := raw.([]*subscriber[T])
	
	// For large subscriber lists, use parallel processing with errgroup for better error handling
	if len(subs) > 10 {
		g, gctx := errgroup.WithContext(ctx)
		for _, sub := range subs {
			sub := sub // capture loop variable
			g.Go(func() error {
				select {
				case <-gctx.Done():
					return gctx.Err()
				default:
					o.enqueue(gctx, sub, value, nil)
					return nil
				}
			})
		}
		return g.Wait()
	} else {
		// For small subscriber lists, sequential processing preserves timing semantics
		for _, sub := range subs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				o.enqueue(ctx, sub, value, nil)
			}
		}
	}
	
	return ctx.Err()
}

func (o *observable[T]) Notify(id string, value T) error {
	return o.NotifyCtx(context.Background(), id, value)
}

func (o *observable[T]) NotifyCtx(ctx context.Context, id string, value T) error {
	if o.isClosed() {
		return fmt.Errorf("observable is closed")
	}
	// Read subscriber under read lock
	o.subsMu.RLock()
	sub, exists := o.subs[id]
	o.subsMu.RUnlock()
	if !exists {
		return fmt.Errorf("no subscriber with id %s", id)
	}
	o.enqueue(ctx, sub, value, &id)
	return ctx.Err()
}

func (o *observable[T]) Close() {
	if !atomic.CompareAndSwapUint32(&o.closed, 0, 1) {
		return
	}

	// Snapshot and clear map
	o.subsMu.Lock()
	subs := make([]*subscriber[T], 0, len(o.subs))
	for _, s := range o.subs {
		subs = append(subs, s)
	}
	o.subs = make(map[string]*subscriber[T])
	o.subsSnap.Store([]*subscriber[T]{})
	o.subsMu.Unlock()

	// Stop all workers
	for _, s := range subs {
		close(s.done)
	}
	
	// Wait for all workers to complete using the pool's wait functionality
	o.workerPool.Wait()

	// Clear replay
	o.last.Store((*T)(nil))
}

func (o *observable[T]) Closed() bool {
	return o.isClosed()
}

func (o *observable[T]) Len() int {
	o.subsMu.RLock()
	n := len(o.subs)
	o.subsMu.RUnlock()
	return n
}

// forwardDropOnClose drops remaining mailbox items on close, then closes outward channel.
func (o *observable[T]) forwardDropOnClose(s *subscriber[T]) {
	defer close(s.out)
	for {
		select {
		case v := <-s.mailbox:
			// forward to outward channel unless unsubscribed
			select {
			case s.out <- v:
			case <-s.done:
				return
			}
		case <-s.done:
			// drop remaining
			for {
				select {
				case <-s.mailbox:
				default:
					return
				}
			}
		}
	}
}

// forwardWithFlush flushes remaining mailbox items to outward channel on close (may block briefly).
func (o *observable[T]) forwardWithFlush(s *subscriber[T]) {
	defer close(s.out)
	for {
		select {
		case v := <-s.mailbox:
			select {
			case s.out <- v:
			case <-s.done:
				// attempt best-effort flush
				for {
					select {
					case x := <-s.mailbox:
						select {
						case s.out <- x:
						default:
							// receiver is gone; stop trying
							return
						}
					default:
						return
					}
				}
			}
		case <-s.done:
			// flush remaining
			for {
				select {
				case v := <-s.mailbox:
					select {
					case s.out <- v:
					default:
						// receiver is gone; stop trying
						return
					}
				default:
					return
				}
			}
		}
	}
}

// enqueue delivers v to sub according to its backpressure policy.
// If ctx is non-nil and Policy==Block, it respects ctx cancellation.
func (o *observable[T]) enqueue(ctx context.Context, sub *subscriber[T], v T, _ *string) {
	switch sub.cfg.Policy {
	case BackpressureBlock:
		if ctx == nil {
			// No context provided; block until space or unsubscribed
			select {
			case sub.mailbox <- v:
			case <-sub.done:
			}
			return
		}
		select {
		case sub.mailbox <- v:
		case <-sub.done:
		case <-ctx.Done():
			// allow caller to observe ctx.Err()
		}
	case BackpressureDropOldest:
		// First, try fast path.
		select {
		case sub.mailbox <- v:
			return
		default:
		}
		// Mailbox appears full; drop one oldest item (if possible), then retry once.
		select {
		case <-sub.mailbox:
		default:
		}
		select {
		case sub.mailbox <- v:
			return
		default:
			// Still cannot enqueue; drop newest.
			if sub.cfg.DropCallback != nil {
				sub.cfg.DropCallback(sub.id, v)
			}
			return
		}
	case BackpressureDropNewest:
		fallthrough
	default:
		select {
		case sub.mailbox <- v:
		default:
			if sub.cfg.DropCallback != nil {
				sub.cfg.DropCallback(sub.id, v)
			}
			return
		}
	}
}
