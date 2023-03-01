// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventqueue

import (
	"sync/atomic"

	"gopkg.in/tomb.v2"

	"github.com/juju/errors"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/worker/v3/catacomb"
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
	tomb    tomb.Tomb
	id      uint64
	handler changestream.Handler
	topics  map[string]struct{}
	done    <-chan struct{}

	unsubscriber func()
}

// Unsubscribe removes the subscription from the event queue asynchronously.
// This ensures that all unsubscriptions can be serialized. No unsubscribe will
// actually never happen inside a dispatch call. If you attempt to unsubscribe
// whilst the dispatch signalling, the unsubscribe will happen after all
// dispatches have been called.
func (s *subscription) Unsubscribe() {
	s.unsubscriber()
}

// Done provides a way to know from the consumer side if the underlying
// subscription has been terminated. This is useful to know if the event queue
// has been killed. Ultimately this is tied to the event queue tomb.
func (s *subscription) Done() <-chan struct{} {
	return s.done
}

func (s *subscription) Kill() {
	s.tomb.Kill(nil)
}

func (s *subscription) Wait() error {
	return s.tomb.Wait()
}

func (s *subscription) loop() error {
	for {
		select {
		case <-s.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

type subscriptionOpts struct {
	*subscription
	opts []changestream.SubscriptionOption
}

type eventFilter struct {
	subscriptionID uint64
	changeMask     changestream.ChangeType
	filter         func(changestream.ChangeEvent) bool
}

// EventQueue defines a event listener and dispatcher for db changes that can
// be multiplexed to subscriptions. The event queue allows consumers to
// subscribe via callbacks to the event queue. This is a lockless
// implementation, all subscriptions and changes are serialized in the main
// loop. Dispatching is randomized to ensure that subscriptions don't depend on
// ordering. The subscriptions can be associated with different subscription
// options, which provide filtering when dispatching. Unsubscribing is provided
// per subscription, which is done asynchronously.
type EventQueue struct {
	catacomb catacomb.Catacomb
	stream   Stream
	logger   Logger

	subscriptions      map[uint64]*subscription
	subscriptionsByNS  map[string][]*eventFilter
	subscriptionsAll   map[uint64]struct{}
	subscriptionsCount uint64

	// (un)subscription related channels to serialize adding and removing
	// subscriptions. This allows the queue to be lock less.
	subscriptionCh   chan subscriptionOpts
	unsubscriptionCh chan uint64
}

// New creates a new EventQueue that will use the Stream for events.
func New(stream Stream, logger Logger) (*EventQueue, error) {
	queue := &EventQueue{
		stream:             stream,
		logger:             logger,
		subscriptions:      make(map[uint64]*subscription),
		subscriptionsByNS:  make(map[string][]*eventFilter),
		subscriptionsAll:   make(map[uint64]struct{}),
		subscriptionsCount: 0,

		subscriptionCh:   make(chan subscriptionOpts),
		unsubscriptionCh: make(chan uint64),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &queue.catacomb,
		Work: queue.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return queue, nil
}

// Subscribe creates a new subscription to the event queue. Options can be
// provided to allow filter during the dispatching phase.
func (q *EventQueue) Subscribe(handler changestream.Handler, opts ...changestream.SubscriptionOption) (changestream.Subscription, error) {
	subID := atomic.AddUint64(&q.subscriptionsCount, 1)
	sub := &subscription{
		id:           subID,
		handler:      handler,
		topics:       make(map[string]struct{}),
		unsubscriber: func() { q.unsubscribe(subID) },
	}

	sub.tomb.Go(sub.loop)
	sub.done = sub.tomb.Dying()

	if err := q.catacomb.Add(sub); err != nil {
		return nil, errors.Trace(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		select {
		case <-q.catacomb.Dying():
			return
		case q.subscriptionCh <- subscriptionOpts{
			subscription: sub,
			opts:         opts,
		}:
		}
	}()

	select {
	case <-done:
	case <-q.catacomb.Dying():
		return nil, q.catacomb.ErrDying()
	}

	return sub, nil
}

// Kill stops the event queue.
func (q *EventQueue) Kill() {
	q.catacomb.Kill(nil)
}

// Wait waits for the event queue to stop.
func (q *EventQueue) Wait() error {
	return q.catacomb.Wait()
}

func (q *EventQueue) unsubscribe(subscriptionID uint64) {
	go func() {
		select {
		case <-q.catacomb.Dying():
			return
		case q.unsubscriptionCh <- subscriptionID:
		}
	}()
}

func (q *EventQueue) loop() error {
	defer func() {
		q.subscriptions = nil
		q.subscriptionsByNS = nil
	}()

	for {
		select {
		case <-q.catacomb.Dying():
			return q.catacomb.ErrDying()

		case ch, ok := <-q.stream.Changes():
			// If the stream is closed, we expect that a new worker will come
			// again using the change stream worker infrastructure. In this case
			// just ignore and close out.
			if !ok {
				q.logger.Infof("change stream change channel is closed")
				return nil
			}

			subs := q.gatherSubscriptions(ch)
			for _, sub := range subs {
				// Ensure we check tomb dying as the handling logic is
				// synchronous and blocking. This is to effectively handle
				// back pressure on the stream.
				select {
				case <-q.catacomb.Dying():
					return q.catacomb.ErrDying()
				default:
				}
				sub.handler(ch)
			}

		case subOpt := <-q.subscriptionCh:
			sub := subOpt.subscription

			// Create a new subscription and assign a unique ID to it.
			q.subscriptions[sub.id] = sub

			// No options were supplied, just add it to the all bucket, so
			// they'll be included in every dispatch.
			if len(subOpt.opts) == 0 {
				q.subscriptionsAll[sub.id] = struct{}{}
				continue
			}

			// Register filters to route changes matching the subscription criteria to
			// the newly crated subscription.
			for _, opt := range subOpt.opts {
				namespace := opt.Namespace()
				q.subscriptionsByNS[namespace] = append(q.subscriptionsByNS[namespace], &eventFilter{
					subscriptionID: sub.id,
					changeMask:     opt.ChangeMask(),
					filter:         opt.Filter(),
				})
				sub.topics[namespace] = struct{}{}
			}

		case subscriptionID := <-q.unsubscriptionCh:
			sub, found := q.subscriptions[subscriptionID]
			if !found {
				continue
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
			delete(q.subscriptionsAll, subscriptionID)

			sub.Kill()
		}
	}
}

func (q *EventQueue) gatherSubscriptions(ch changestream.ChangeEvent) []*subscription {
	subs := make(map[uint64]*subscription)

	for id := range q.subscriptionsAll {
		subs[id] = q.subscriptions[id]
	}

	for _, subOpt := range q.subscriptionsByNS[ch.Namespace()] {
		// TODO (stickupkid): If the subscription has already been added, do we
		// want to recall the filter to ensure consistency?
		if _, ok := subs[subOpt.subscriptionID]; ok {
			continue
		}

		if (ch.Type() & subOpt.changeMask) == 0 {
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

		subs[subOpt.subscriptionID] = q.subscriptions[subOpt.subscriptionID]
	}

	// By collecting the subs within a map to ensure that a sub can be only
	// called once, we actually gain random ordering. This prevents subscribers
	// from depending on the order of dispatches.
	results := make([]*subscription, 0, len(subs))
	for _, sub := range subs {
		results = append(results, sub)
	}
	return results
}
