// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package pubsub

import (
	"sync"

	"github.com/juju/errors"
)

// Multiplexer allows multiple subscriptions to be made sharing a single
// message queue from the hub. This means that all the messages for the
// various subscriptions are called back in the order that the messages were
// published. If more than one handler is added to the Multiplexer that
// matches any given topic, the handlers are called back one after the other
// in the order that they were added.
type Multiplexer interface {
	Add(topic string, handler interface{}) error
	AddMatch(matcher func(string) bool, handler interface{}) error
	Unsubscribe()
}

type element struct {
	matcher  func(string) bool
	callback *structuredCallback
}

type multiplexer struct {
	logger       Logger
	mu           sync.Mutex
	outputs      []element
	marshaller   Marshaller
	unsubscriber func()
}

// NewMultiplexer creates a new multiplexer for the hub and subscribes it.
// Unsubscribing the multiplexer stops calls for all handlers added.
// Only structured hubs support multiplexer.
func (h *StructuredHub) NewMultiplexer() (Multiplexer, error) {
	mp := &multiplexer{logger: h.hub.logger, marshaller: h.marshaller}
	unsub, err := h.SubscribeMatch(mp.match, mp.callback)
	if err != nil {
		return nil, errors.Trace(err)
	}
	mp.unsubscriber = unsub
	return mp, nil
}

// Add a handler for a specific topic.
func (m *multiplexer) Add(topic string, handler interface{}) error {
	return m.AddMatch(equalTopic(topic), handler)
}

// AddMatch adds another handler for any topic that matches the matcher.
func (m *multiplexer) AddMatch(matcher func(string) bool, handler interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	callback, err := newStructuredCallback(m.logger, m.marshaller, handler)
	if err != nil {
		return errors.Trace(err)
	}
	m.outputs = append(m.outputs, element{matcher: matcher, callback: callback})
	return nil
}

// Add another topic matcher and handler to the multiplexer.
func (m *multiplexer) Unsubscribe() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.unsubscriber != nil {
		m.unsubscriber()
		m.unsubscriber = nil
	}
}

func (m *multiplexer) callback(topic string, data map[string]interface{}) {
	// Since the callback functions have arbitrary code, don't hold the
	// mutex for the duration of the calls.
	m.mu.Lock()
	outputs := make([]element, len(m.outputs))
	copy(outputs, m.outputs)
	m.mu.Unlock()

	for _, element := range outputs {
		if element.matcher(topic) {
			element.callback.handler(topic, data)
		}
	}
}

// If any of the topic matchers added for the handlers match the topic, the
// multiplexer matches.
func (m *multiplexer) match(topic string) bool {
	// Here we explicitly don't make a copy of the outputs as the match
	// function is going to be called much more often than the callback func.
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, element := range m.outputs {
		if element.matcher(topic) {
			return true
		}
	}
	return false
}
