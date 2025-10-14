package main

import (
	"sync"
)

type Broker struct {
	mu      sync.RWMutex
	clients map[chan IdentifiedEvent]struct{}
}

func NewBroker() *Broker {
	return &Broker{clients: make(map[chan IdentifiedEvent]struct{})}
}

func (b *Broker) Subscribe() (ch chan IdentifiedEvent, unsubscribe func()) {
	ch = make(chan IdentifiedEvent, 8) // small buffer to avoid head-of-line blocking
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.clients, ch)
		b.mu.Unlock()
		close(ch)
	}
}
func (b *Broker) Publish(msg IdentifiedEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
			// client too slow; drop the message for this client
		}
	}
}
