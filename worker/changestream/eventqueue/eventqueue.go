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
	Infof(message string, args ...interface{})
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

	mutex              sync.Mutex
	subscriptions      map[int]*subscription
	subscriptionsByNS  map[string][]*eventFilter
	subscriptionsCount int

	actions chan func()
}

// New creates a new EventQueue that will use the Stream for events.
func New(stream Stream, logger Logger) *EventQueue {
	queue := &EventQueue{
		stream:             stream,
		logger:             logger,
		subscriptions:      make(map[int]*subscription),
		subscriptionsByNS:  make(map[string][]*eventFilter),
		subscriptionsCount: 0,
		actions:            make(chan func()),
	}

	queue.tomb.Go(queue.loop)

	return queue
}

// Subscribe creates a new subscription to the event queue.
func (q *EventQueue) Subscribe(opts ...changestream.SubscriptionOption) (changestream.Subscription, error) {
	if len(opts) == 0 {
		return nil, errors.Errorf("no subscription options specified")
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	// Create a new subscription and assign a unique ID to it.
	q.subscriptionsCount++
	sub := &subscription{
		id:           q.subscriptionsCount,
		changes:      make(chan changestream.ChangeEvent),
		topics:       make(map[string]struct{}),
		unsubscriber: func() { q.unsubscribe(q.subscriptionsCount) },
	}
	q.subscriptions[sub.id] = sub

	// Register filters to route changes matching the subscription criteria to the newly crated subscription.
	for _, opt := range opts {
		namespace := opt.Namespace()
		q.subscriptionsByNS[namespace] = append(q.subscriptionsByNS[namespace], &eventFilter{
			subscriptionID: sub.id,
			changeMask:     opt.ChangeMask(),
			filter:         opt.Filter(),
		})
		sub.topics[namespace] = struct{}{}
	}

	return sub, nil
}

// Kill stops the event queue.
func (q *EventQueue) Kill() {
	q.tomb.Kill(nil)
}

// Wait waits for the event queue to stop.
func (q *EventQueue) Wait() error {
	return q.tomb.Wait()
}

func (q *EventQueue) unsubscribe(subscriptionID int) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	sub, found := q.subscriptions[subscriptionID]
	if !found {
		return
	}

	for topic := range sub.topics {
		var updatedFilters []*eventFilter
		for _, filter := range q.subscriptionsByNS[topic] {
			if filter.subscriptionID == subscriptionID {
				continue
			}
			updatedFilters = append(updatedFilters, filter)
		}
		q.subscriptionsByNS[topic] = updatedFilters
	}

	delete(q.subscriptions, subscriptionID)

	// Close the subscription channel after a dispatch. If a unsubscription
	// happens during a dispatch, then it causes the event queue to potentially
	// panic. Removal of the subscription from the queue is synchronous, so
	// the subscription can no longer be used when another dispatch occurs. The
	// closing of the channel then becomes asynchronous and can be closed during
	// the next selection.
	q.tomb.Go(func() error {
		// Ensure that we give up if the tomb is dying.
		select {
		case <-q.tomb.Dying():
			return tomb.ErrDying
		case q.actions <- func() {
			close(sub.changes)
		}:
		}
		return nil
	})
}

func (q *EventQueue) loop() error {
	defer func() {
		q.mutex.Lock()
		defer q.mutex.Unlock()

		for _, sub := range q.subscriptions {
			close(sub.changes)
		}
		q.subscriptions = make(map[int]*subscription)
	}()

	for {
		var (
			ch changestream.ChangeEvent
			ok bool
		)
		select {
		case <-q.tomb.Dying():
			return tomb.ErrDying
		case action := <-q.actions:
			action()
			continue
		case ch, ok = <-q.stream.Changes():
			// If the stream is closed, we expect that a new worker will come
			// again using the change stream worker infrastructure. In this case
			// just ignore and close out.
			if !ok {
				q.logger.Infof("change stream change channel is closed")
				return nil
			}
		}

		// The dispatch is done in two stages to avoid holding the lock for the
		// duration of the dispatch.
		subs := q.gatherSubscriptions(ch)
		if err := q.dispatchChange(subs, ch); err != nil {
			return errors.Trace(err)
		}
	}
}

func (q *EventQueue) gatherSubscriptions(ch changestream.ChangeEvent) []*subscription {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	var subs []*subscription
	for _, subOpt := range q.subscriptionsByNS[ch.Namespace()] {
		if (ch.Type() & subOpt.changeMask) == 0 {
			if q.logger.IsTraceEnabled() {
				q.logger.Tracef("ignoring change: %v", ch)
			}
			continue
		}

		if !subOpt.filter(ch) {
			if q.logger.IsTraceEnabled() {
				q.logger.Tracef("filtering out change: %v", ch)
			}
			continue
		}

		if q.logger.IsTraceEnabled() {
			q.logger.Tracef("dispatching change: %v", ch)
		}

		subs = append(subs, q.subscriptions[subOpt.subscriptionID])
	}
	return subs
}

func (q *EventQueue) dispatchChange(subs []*subscription, ch changestream.ChangeEvent) error {
	for _, sub := range subs {
		select {
		case <-q.tomb.Dying():
			return tomb.ErrDying
		case sub.changes <- ch:
			// pushed change.
		}
	}
	return nil
}
