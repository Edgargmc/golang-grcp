package main

import (
	"sync"

	pb "grpc-todo/gen"
)

// EventPublisher es todo lo que necesitan los métodos CRUD: avisar que algo pasó.
type EventPublisher interface {
	Publish(event *pb.TodoEvent)
}

// EventSubscriber es todo lo que necesita WatchTodos: sumarse y sacarse de la lista de escucha.
type EventSubscriber interface {
	Subscribe() (id int, events <-chan *pb.TodoEvent)
	Unsubscribe(id int)
}

// broadcaster implementa ambas interfaces. Es la única pieza que sabe
// que "avisar" significa mandar por un canal de Go.
type broadcaster struct {
	mu        sync.Mutex
	nextID    int
	listeners map[int]chan *pb.TodoEvent
}

func newBroadcaster() *broadcaster {
	return &broadcaster{
		listeners: make(map[int]chan *pb.TodoEvent),
	}
}

func (b *broadcaster) Subscribe() (int, <-chan *pb.TodoEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++
	ch := make(chan *pb.TodoEvent, 8)
	b.listeners[id] = ch

	return id, ch
}

// listenerCount es solo para tests: no forma parte de EventPublisher ni
// EventSubscriber a propósito, para no ensuciar el contrato de producción.
func (b *broadcaster) listenerCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.listeners)
}

func (b *broadcaster) Unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.listeners[id]; ok {
		close(ch)
		delete(b.listeners, id)
	}
}

func (b *broadcaster) Publish(event *pb.TodoEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.listeners {
		select {
		case ch <- event:
		default:
			// El listener está lento y no vació su buffer: descartamos
			// este evento para no bloquear al que está publicando.
		}
	}
}
