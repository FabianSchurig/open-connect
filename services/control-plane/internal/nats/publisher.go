// Package nats provides the publisher abstraction (Epic D / arc42 §5.2).
//
// The interface lets unit tests use an in-memory implementation while
// integration tests / production use a real `nats-server` connection. The
// real adapter is added in a follow-up PR; nothing else in this package
// imports `nats.go` so the control-plane builds without that dependency
// during MVP scaffolding.
package nats

import (
	"fmt"
	"sync"
)

// Publisher publishes a payload on a NATS subject. Implementations MUST be
// safe for concurrent use.
type Publisher interface {
	Publish(subject string, payload []byte) error
}

// Requester performs a NATS request-reply.
type Requester interface {
	Request(subject string, payload []byte) ([]byte, error)
}

// MemBus is an in-process implementation used by tests.
type MemBus struct {
	mu        sync.Mutex
	Published []Message
	Replies   map[string]func([]byte) []byte
}

type Message struct {
	Subject string
	Payload []byte
}

func NewMemBus() *MemBus {
	return &MemBus{Replies: map[string]func([]byte) []byte{}}
}

func (m *MemBus) Publish(subject string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Published = append(m.Published, Message{Subject: subject, Payload: append([]byte(nil), payload...)})
	return nil
}

func (m *MemBus) Request(subject string, payload []byte) ([]byte, error) {
	m.mu.Lock()
	reply, ok := m.Replies[subject]
	m.mu.Unlock()
	if !ok {
		// Distinguish "no responder registered" from a real nil payload so
		// that test bugs surface instead of silently returning empty data.
		return nil, fmt.Errorf("nats mem-bus: no responder for subject %q", subject)
	}
	return reply(payload), nil
}

// PublishedOn returns a copy of the messages published on the given subject.
func (m *MemBus) PublishedOn(subject string) []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Message, 0)
	for _, msg := range m.Published {
		if msg.Subject == subject {
			out = append(out, msg)
		}
	}
	return out
}
