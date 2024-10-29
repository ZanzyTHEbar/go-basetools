package gobasetools

import (
	"fmt"
	"sync"
)

type Observable[T interface{}] interface {
	Subscribe(id string) (<-chan T, error)
	Unsubscribe(id string) error
	Broadcast(value T)
	Notify(id string, value T) error
	Close()
}

type observable[T interface{}] struct {
	mutex       sync.RWMutex
	subscribers map[string]chan T
}

func NewObservable[T interface{}]() Observable[T] {
	return &observable[T]{
		subscribers: make(map[string]chan T),
	}
}

func (observ *observable[T]) Subscribe(id string) (<-chan T, error) {
	observ.mutex.Lock()
	defer observ.mutex.Unlock()

	if _, exists := observ.subscribers[id]; exists {
		return nil, fmt.Errorf("subscriber with id %s already exists", id)
	}

	subCh := make(chan T)
	observ.subscribers[id] = subCh
	return subCh, nil
}

func (observ *observable[T]) Unsubscribe(id string) error {
	observ.mutex.Lock()
	defer observ.mutex.Unlock()

	subCh, exists := observ.subscribers[id]
	if !exists {
		return fmt.Errorf("no subscriber with id %s", id)
	}

	close(subCh)
	delete(observ.subscribers, id)
	return nil
}

func (observ *observable[T]) Broadcast(value T) {
	observ.mutex.RLock()
	defer observ.mutex.RUnlock()

	for _, sub := range observ.subscribers {
		sub <- value
	}
}

func (observ *observable[T]) Close() {
	observ.mutex.Lock()
	defer observ.mutex.Unlock()

	for id, sub := range observ.subscribers {
		close(sub)
		delete(observ.subscribers, id)
	}
}

func (observ *observable[T]) Notify(id string, value T) error {
	observ.mutex.RLock()
	defer observ.mutex.RUnlock()

	sub, exists := observ.subscribers[id]
	if !exists {
		return fmt.Errorf("no subscriber with id %s", id)
	}

	sub <- value
	return nil
}
