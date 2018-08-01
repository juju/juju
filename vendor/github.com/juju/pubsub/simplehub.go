// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package pubsub

import "sync"

// SimpleHubConfig is the argument struct for NewSimpleHub.
type SimpleHubConfig struct {
	// Logger allows specifying a logging implementation for debug
	// and trace level messages emitted from the hub.
	Logger Logger
}

// NewSimpleHub returns a new SimpleHub instance.
//
// A simple hub does not touch the data that is passed through to Publish.
// This data is passed through to each Subscriber. Note that all subscribers
// are notified in parallel, and that no modification should be done to the
// data or data races will occur.
func NewSimpleHub(config *SimpleHubConfig) *SimpleHub {
	if config == nil {
		config = new(SimpleHubConfig)
	}

	logger := config.Logger
	if logger == nil {
		logger = noOpLogger{}
	}

	return &SimpleHub{
		logger: logger,
	}
}

// SimpleHub provides the base functionality of dealing with subscribers,
// and the notification of subscribers of events.
type SimpleHub struct {
	mutex       sync.Mutex
	subscribers []*subscriber
	idx         int
	logger      Logger
}

// Publish will notifiy all the subscribers that are interested by calling
// their handler function.
//
// The data is passed through to each Subscriber untouched. Note that all
// subscribers are notified in parallel, and that no modification should be
// done to the data or data races will occur.
//
// The channel return value is closed when all the subscribers have been
// notified of the event.
func (h *SimpleHub) Publish(topic string, data interface{}) <-chan struct{} {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	done := make(chan struct{})
	wait := sync.WaitGroup{}

	for _, s := range h.subscribers {
		if s.topicMatcher(topic) {
			wait.Add(1)
			s.notify(
				&handlerCallback{
					topic: topic,
					data:  data,
					wg:    &wait,
				})
		}
	}

	go func() {
		wait.Wait()
		close(done)
	}()

	return done
}

// Subscribe to a topic with a handler function. If the topic is the same
// as the published topic, the handler function is called with the
// published topic and the associated data.
//
// The return value is a function that will unsubscribe the caller from
// the hub, for this subscription.
func (h *SimpleHub) Subscribe(topic string, handler func(string, interface{})) func() {
	return h.SubscribeMatch(equalTopic(topic), handler)
}

// SubscribeMatch takes a function that determins whether the topic matches,
// and a handler function. If the matcher matches the published topic, the
// handler function is called with the published topic and the associated
// data.
//
// The return value is a function that will unsubscribe the caller from
// the hub, for this subscription.
func (h *SimpleHub) SubscribeMatch(matcher func(string) bool, handler func(string, interface{})) func() {
	if handler == nil || matcher == nil {
		// It is safe but useless.
		return func() {}
	}
	h.mutex.Lock()
	defer h.mutex.Unlock()

	sub := newSubscriber(matcher, handler, h.logger)
	sub.id = h.idx
	h.idx++
	h.subscribers = append(h.subscribers, sub)
	unsub := &handle{hub: h, id: sub.id}
	return unsub.Unsubscribe
}

func (h *SimpleHub) unsubscribe(id int) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	for i, sub := range h.subscribers {
		if sub.id == id {
			sub.close()
			h.subscribers = append(h.subscribers[0:i], h.subscribers[i+1:]...)
			return
		}
	}
}

type handle struct {
	hub *SimpleHub
	id  int
}

// Unsubscribe implements Unsubscriber.
func (h *handle) Unsubscribe() {
	h.hub.unsubscribe(h.id)
}

type handlerCallback struct {
	topic string
	data  interface{}
	wg    *sync.WaitGroup
}

func (h *handlerCallback) done() {
	h.wg.Done()
}
