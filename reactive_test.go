package gobasetools

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestSubscribe ensures that subscribers can subscribe and receive messages.
func TestSubscribe(t *testing.T) {
	obs := NewObservable[string]()
	defer obs.Close()

	ctx := context.Background()
	sub, err := obs.Subscribe(ctx, "testID")
	if err != nil {
		t.Fatalf("unexpected error on subscribe: %v", err)
	}

	go obs.Broadcast("Test message")

	select {
	case msg := <-sub:
		if msg != "Test message" {
			t.Errorf("expected 'Test message', got '%s'", msg)
		}
	case <-time.After(1 * time.Second):
		t.Error("did not receive message in time")
	}
}

// TestNotify tests sending a message to a specific subscriber only.
func TestNotify(t *testing.T) {
	obs := NewObservable[string]()
	defer obs.Close()

	ctx := context.Background()
	subA, _ := obs.Subscribe(ctx, "A")
	subB, _ := obs.Subscribe(ctx, "B")

	go obs.Notify("A", "Hello, A only!")

	select {
	case msg := <-subA:
		if msg != "Hello, A only!" {
			t.Errorf("expected 'Hello, A only!', got '%s'", msg)
		}
	case <-subB:
		t.Error("Subscriber B should not have received the message")
	case <-time.After(1 * time.Second):
		t.Error("Subscriber A did not receive the message in time")
	}
}

// TestUnsubscribe verifies that unsubscribing stops a subscriber from receiving further messages.
func TestUnsubscribe(t *testing.T) {
	obs := NewObservable[string]()
	defer obs.Close()

	ctx := context.Background()
	sub, _ := obs.Subscribe(ctx, "testID")
	obs.Unsubscribe("testID")

	// Give some time for the worker to shut down
	time.Sleep(10 * time.Millisecond)

	go obs.Broadcast("No message")

	select {
	case _, open := <-sub:
		if open {
			t.Error("subscriber channel should be closed after unsubscribing")
		}
		// Channel is closed, which is expected
	case <-time.After(100 * time.Millisecond):
		t.Error("expected subscriber channel to be closed after unsubscribing")
	}
}

// TestClose ensures all subscribers stop receiving messages after closing.
func TestClose(t *testing.T) {
	obs := NewObservable[string]()
	ctx := context.Background()
	subA, _ := obs.Subscribe(ctx, "A")
	subB, _ := obs.Subscribe(ctx, "B")

	obs.Close()

	select {
	case _, open := <-subA:
		if open {
			t.Error("expected subA to be closed after Close()")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected subA to be closed after Close()")
	}

	select {
	case _, open := <-subB:
		if open {
			t.Error("expected subB to be closed after Close()")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected subB to be closed after Close()")
	}
}

// TestBackpressureDropNewest tests the default backpressure policy.
func TestBackpressureDropNewest(t *testing.T) {
	// This test verifies that the drop callback is called when backpressure occurs
	obs := NewObservable[string]()
	defer obs.Close()

	var droppedCount int
	var mu sync.Mutex

	dropCallback := func(id string, value string) {
		mu.Lock()
		droppedCount++
		mu.Unlock()
	}

	ctx := context.Background()
	// Use buffer size 0 to force immediate backpressure
	_, err := obs.Subscribe(ctx, "test", WithBuffer[string](0), WithDropCallback[string](dropCallback))
	if err != nil {
		t.Fatalf("unexpected error on subscribe: %v", err)
	}

	// Send multiple messages without reading - should trigger drops
	obs.Broadcast("msg1")
	obs.Broadcast("msg2") // This should be dropped
	obs.Broadcast("msg3") // This should be dropped

	// Wait for drops to be processed
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if droppedCount == 0 {
		t.Error("expected some messages to be dropped")
	}
	mu.Unlock()
}

// TestBackpressureDropOldest tests dropping oldest values when buffer is full.
func TestBackpressureDropOldest(t *testing.T) {
	// This test verifies that the drop callback is called with DropOldest policy
	obs := NewObservable[string]()
	defer obs.Close()

	var droppedCount int
	var mu sync.Mutex

	dropCallback := func(id string, value string) {
		mu.Lock()
		droppedCount++
		mu.Unlock()
	}

	ctx := context.Background()
	// Use buffer size 0 to force immediate backpressure
	_, err := obs.Subscribe(ctx, "test", 
		WithBuffer[string](0), 
		WithPolicy[string](BackpressureDropOldest),
		WithDropCallback[string](dropCallback))
	if err != nil {
		t.Fatalf("unexpected error on subscribe: %v", err)
	}

	// Send multiple messages without reading - should trigger drops
	obs.Broadcast("msg1")
	obs.Broadcast("msg2") // Should attempt to drop oldest and then drop this
	obs.Broadcast("msg3") // Should attempt to drop oldest and then drop this

	// Wait for drops to be processed
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if droppedCount == 0 {
		t.Error("expected some messages to be dropped")
	}
	mu.Unlock()
}

// TestReplayLatest tests that new subscribers receive the latest broadcast value.
func TestReplayLatest(t *testing.T) {
	obs := NewObservable[string]()
	defer obs.Close()

	// Broadcast before any subscribers
	obs.Broadcast("initial message")

	ctx := context.Background()
	// Subscribe with replay enabled
	sub, err := obs.Subscribe(ctx, "test", WithReplayLatest[string](true))
	if err != nil {
		t.Fatalf("unexpected error on subscribe: %v", err)
	}

	// Should immediately receive the initial message
	select {
	case msg := <-sub:
		if msg != "initial message" {
			t.Errorf("expected 'initial message', got '%s'", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected to receive initial message via replay")
	}
}

// TestContextCancellation tests that Subscribe respects context cancellation.
func TestContextCancellation(t *testing.T) {
	obs := NewObservable[string]()
	defer obs.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := obs.Subscribe(ctx, "test")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

// TestBroadcastCtx tests context-aware broadcasting.
func TestBroadcastCtx(t *testing.T) {
	obs := NewObservable[string]()
	defer obs.Close()

	ctx := context.Background()
	sub, _ := obs.Subscribe(ctx, "test")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := obs.BroadcastCtx(ctx, "test message")
	if err != nil {
		t.Errorf("unexpected error from BroadcastCtx: %v", err)
	}

	select {
	case msg := <-sub:
		if msg != "test message" {
			t.Errorf("expected 'test message', got '%s'", msg)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("expected to receive message")
	}
}

// TestNotifyCtx tests context-aware notification.
func TestNotifyCtx(t *testing.T) {
	obs := NewObservable[string]()
	defer obs.Close()

	ctx := context.Background()
	sub, _ := obs.Subscribe(ctx, "test")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := obs.NotifyCtx(ctx, "test", "notify message")
	if err != nil {
		t.Errorf("unexpected error from NotifyCtx: %v", err)
	}

	select {
	case msg := <-sub:
		if msg != "notify message" {
			t.Errorf("expected 'notify message', got '%s'", msg)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("expected to receive notification")
	}
}

// TestDropCallback tests the drop callback functionality.
func TestDropCallback(t *testing.T) {
	obs := NewObservable[string]()
	defer obs.Close()

	var droppedMsgs []string
	var mu sync.Mutex

	dropCallback := func(id string, value string) {
		mu.Lock()
		droppedMsgs = append(droppedMsgs, value)
		mu.Unlock()
	}

	ctx := context.Background()
	sub, err := obs.Subscribe(ctx, "test", 
		WithBuffer[string](1), 
		WithDropCallback[string](dropCallback))
	if err != nil {
		t.Fatalf("unexpected error on subscribe: %v", err)
	}

	// Block the consumer to let the mailbox fill up
	var receivedMsgs []string
	done := make(chan struct{})

	go func() {
		defer close(done)
		// Don't read immediately - let messages accumulate to trigger drops
		time.Sleep(50 * time.Millisecond)
		for msg := range sub {
			receivedMsgs = append(receivedMsgs, msg)
		}
	}()

	// Fill buffer and overflow
	obs.Broadcast("msg1")
	obs.Broadcast("msg2") // This should be dropped and trigger callback

	// Wait for drops to be processed
	time.Sleep(20 * time.Millisecond)

	// Check that callback was called
	mu.Lock()
	droppedCount := len(droppedMsgs)
	mu.Unlock()

	if droppedCount == 0 {
		t.Error("expected drop callback to be called")
	}

	// Clean up
	obs.Unsubscribe("test")
	<-done
}

// TestLen tests the subscriber count functionality.
func TestLen(t *testing.T) {
	obs := NewObservable[string]()
	defer obs.Close()

	if obs.Len() != 0 {
		t.Errorf("expected 0 subscribers, got %d", obs.Len())
	}

	ctx := context.Background()
	_, _ = obs.Subscribe(ctx, "sub1")
	if obs.Len() != 1 {
		t.Errorf("expected 1 subscriber, got %d", obs.Len())
	}

	_, _ = obs.Subscribe(ctx, "sub2")
	if obs.Len() != 2 {
		t.Errorf("expected 2 subscribers, got %d", obs.Len())
	}

	obs.Unsubscribe("sub1")
	time.Sleep(10 * time.Millisecond) // Allow cleanup
	if obs.Len() != 1 {
		t.Errorf("expected 1 subscriber after unsubscribe, got %d", obs.Len())
	}
}

// TestClosed tests the Closed() method.
func TestClosed(t *testing.T) {
	obs := NewObservable[string]()

	if obs.Closed() {
		t.Error("expected observable to not be closed initially")
	}

	obs.Close()

	if !obs.Closed() {
		t.Error("expected observable to be closed after Close()")
	}
}

// TestSubscribeToClosedObservable tests that subscribing to a closed observable returns an error.
func TestSubscribeToClosedObservable(t *testing.T) {
	obs := NewObservable[string]()
	obs.Close()

	ctx := context.Background()
	_, err := obs.Subscribe(ctx, "test")
	if err == nil {
		t.Error("expected error when subscribing to closed observable")
	}
}
