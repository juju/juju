// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventqueue

import (
	"sync"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
)

// Logger represents the logging methods called.
type Logger interface {
	Tracef(message string, args ...interface{})
	IsTraceEnabled() bool
}

// Stream represents a way to get change events.
type Stream interface {
	// Changes returns a channel for a given namespace (database).
	Changes() <-chan changestream.ChangeEvent
}

type subscription struct {
	id      int
	changes chan changestream.ChangeEvent
	topics  map[string]struct{}

	unsubscriber func()
}

// Unsubscribe removes the subscription from the event queue.
func (s *subscription) Unsubscribe() {
	s.unsubscriber()
}

// Changes returns a channel that will receive events from the event queue
// for the subscription.
func (s *subscription) Changes() <-chan changestream.ChangeEvent {
	return s.changes
}

type eventFilter struct {
	subscriptionID int
	changeMask     changestream.ChangeType
	filter         func(changestream.ChangeEvent) bool
}

// EventQueue is a queue of events that can be subscribed to.
type EventQueue struct {
	tomb   tomb.Tomb
	stream Stream
	logger Logger

	mutex             sync.Mutex
	subscriptions     map[int]*subscription
	subscriptionsByNS map[string][]*eventFilter
}

// New creates a new EventQueue that will use the Stream for events.
func New(stream Stream, logger Logger) *EventQueue {
	queue := &EventQueue{
		stream:            stream,
		logger:            logger,
		subscriptions:     make(map[int]*subscription),
		subscriptionsByNS: make(map[string][]*eventFilter),
	}

	queue.tomb.Go(queue.loop)

	return queue
}

// Subscribe creates a new subscription to the event queue.
func (s *EventQueue) Subscribe(opts ...changestream.SubscriptionOption) (changestream.Subscription, error) {
	if len(opts) == 0 {
		return nil, errors.Errorf("no subscription options specified")
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Create a new subscription and assign a unique ID to it.
	subID := len(s.subscriptions)
	sub := &subscription{
		id:           subID,
		changes:      make(chan changestream.ChangeEvent),
		topics:       make(map[string]struct{}),
		unsubscriber: func() { s.unsubscribe(subID) },
	}
	s.subscriptions[sub.id] = sub

	// Register filters to route changes matching the subscription criteria to the newly crated subscription.
	for _, opt := range opts {
		namespace := opt.Namespace()
		s.subscriptionsByNS[namespace] = append(s.subscriptionsByNS[namespace], &eventFilter{
			subscriptionID: sub.id,
			changeMask:     opt.ChangeMask(),
			filter:         opt.Filter(),
		})
		sub.topics[namespace] = struct{}{}
	}

	return sub, nil
}

// Kill stops the event queue.
func (s *EventQueue) Kill() {
	s.tomb.Kill(nil)
}

// Wait waits for the event queue to stop.
func (s *EventQueue) Wait() error {
	return s.tomb.Wait()
}

func (s *EventQueue) unsubscribe(subscriptionID int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	sub, found := s.subscriptions[subscriptionID]
	if !found {
		return
	}

	for topic := range sub.topics {
		var updatedFilters []*eventFilter
		for _, filter := range s.subscriptionsByNS[topic] {
			if filter.subscriptionID == subscriptionID {
				continue
			}
			updatedFilters = append(updatedFilters, filter)
		}
		s.subscriptionsByNS[topic] = updatedFilters
	}

	delete(s.subscriptions, subscriptionID)
}

func (s *EventQueue) loop() error {
	defer func() {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		for _, sub := range s.subscriptions {
			close(sub.changes)
		}
		s.subscriptions = make(map[int]*subscription)
	}()

	for {
		var (
			ch changestream.ChangeEvent
			ok bool
		)
		select {
		case <-s.tomb.Dying():
			return tomb.ErrDying
		case ch, ok = <-s.stream.Changes():
			if !ok {
				return nil // EOF
			}
		}

		// The dispatch is done in two stages to avoid holding the lock for the
		// duration of the dispatch.
		subs := s.gatherSubscriptions(ch)
		if err := s.dispatchChange(subs, ch); err != nil {
			return errors.Trace(err)
		}
	}
}

func (s *EventQueue) gatherSubscriptions(ch changestream.ChangeEvent) []*subscription {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var subs []*subscription
	for _, subOpt := range s.subscriptionsByNS[ch.Namespace()] {
		if (ch.Type() & subOpt.changeMask) == 0 {
			if s.logger.IsTraceEnabled() {
				s.logger.Tracef("ignoring change: %v", ch)
			}
			continue
		}

		if !subOpt.filter(ch) {
			if s.logger.IsTraceEnabled() {
				s.logger.Tracef("filtering out change: %v", ch)
			}
			continue
		}

		if s.logger.IsTraceEnabled() {
			s.logger.Tracef("dispatching change: %v", ch)
		}

		subs = append(subs, s.subscriptions[subOpt.subscriptionID])
	}
	return subs
}

func (s *EventQueue) dispatchChange(subs []*subscription, ch changestream.ChangeEvent) error {
	for _, sub := range subs {
		select {
		case <-s.tomb.Dying():
			return tomb.ErrDying
		case sub.changes <- ch:
			// pushed change.
		}
	}
	return nil
}
