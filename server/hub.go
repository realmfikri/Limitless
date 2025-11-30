package main

import "sync"

type subscription[T any] struct {
	ch chan T
}

type hub[T any] struct {
	mu   sync.RWMutex
	subs map[*subscription[T]]struct{}
}

func newHub[T any]() *hub[T] {
	return &hub[T]{subs: make(map[*subscription[T]]struct{})}
}

func (h *hub[T]) Subscribe(buffer int) *subscription[T] {
	sub := &subscription[T]{ch: make(chan T, buffer)}
	h.mu.Lock()
	h.subs[sub] = struct{}{}
	h.mu.Unlock()
	return sub
}

func (h *hub[T]) Unsubscribe(sub *subscription[T]) {
	h.mu.Lock()
	delete(h.subs, sub)
	h.mu.Unlock()
	close(sub.ch)
}

func (h *hub[T]) Broadcast(value T) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for sub := range h.subs {
		select {
		case sub.ch <- value:
		default:
		}
	}
}
