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

// subscription represents a subscriber in the event queue. It holds a tomb, so
// that we can tie the lifecycle of a subscription to the event queue.
type subscription struct {
	tomb tomb.Tomb
	id   uint64

	topics map[string]struct{}

	changes chan changestream.ChangeEvent
	active  chan struct{}

	unsubscribeFn func()
}

func newSubscription(id uint64, unsubscribeFn func()) *subscription {
	sub := &subscription{
		id:            id,
		changes:       make(chan changestream.ChangeEvent),
		topics:        make(map[string]struct{}),
		active:        make(chan struct{}),
		unsubscribeFn: unsubscribeFn,
	}

	sub.tomb.Go(sub.loop)

	return sub
}

// Unsubscribe removes the subscription from the event queue asynchronously.
// This ensures that all unsubscriptions can be serialized. No unsubscribe will
// actually never happen inside a dispatch call. If you attempt to unsubscribe
// whilst the dispatch signalling, the unsubscribe will happen after all
// dispatches have been called.
func (s *subscription) Unsubscribe() {
	s.unsubscribeFn()
}

// Changes returns the channel that the subscription will receive events on.
func (s *subscription) Changes() <-chan changestream.ChangeEvent {
	return s.changes
}

// Done provides a way to know from the consumer side if the underlying
// subscription has been terminated. This is useful to know if the event queue
// has been closed.
func (s *subscription) Done() <-chan struct{} {
	return s.active
}

// Kill implements worker.Worker.
func (s *subscription) Kill() {
	s.tomb.Kill(nil)
}

// Wait implements worker.Worker.
func (s *subscription) Wait() error {
	return s.tomb.Wait()
}

func (s *subscription) loop() error {
	select {
	case <-s.tomb.Dying():
		return tomb.ErrDying
	case <-s.active:
		return nil
	}
}

// signal will dispatch a change event to the subscription. If the subscription
// is not active, the change will be dropped.
func (s *subscription) signal(change changestream.ChangeEvent) {
	select {
	case <-s.tomb.Dying():
		return
	case <-s.active:
		return
	case s.changes <- change:
	}
}

// close closes the active channel, which will signal to the consumer that the
// subscription is no longer active.
func (s *subscription) close() {
	close(s.active)
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

// EventQueue defines an event listener and dispatcher for db changes that can
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
func (q *EventQueue) Subscribe(opts ...changestream.SubscriptionOption) (changestream.Subscription, error) {
	// Get a new subscription count without using any mutexes.
	subID := atomic.AddUint64(&q.subscriptionsCount, 1)

	sub := newSubscription(subID, func() { q.unsubscribe(subID) })
	if err := q.catacomb.Add(sub); err != nil {
		return nil, errors.Trace(err)
	}

	select {
	case <-q.catacomb.Dying():
		return nil, q.catacomb.ErrDying()
	case q.subscriptionCh <- subscriptionOpts{
		subscription: sub,
		opts:         opts,
	}:
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
	select {
	case <-q.catacomb.Dying():
		return
	case q.unsubscriptionCh <- subscriptionID:
	}
}

func (q *EventQueue) loop() error {
	defer func() {
		for _, sub := range q.subscriptions {
			sub.close()
		}
		q.subscriptions = nil
		q.subscriptionsByNS = nil

		close(q.subscriptionCh)
		close(q.unsubscriptionCh)
	}()

	for {
		select {
		case <-q.catacomb.Dying():
			return q.catacomb.ErrDying()

		case event, ok := <-q.stream.Changes():
			// If the stream is closed, we expect that a new worker will come
			// again using the change stream worker infrastructure. In this case
			// just ignore and close out.
			if !ok {
				q.logger.Infof("change stream change channel is closed")
				return nil
			}

			subs := q.gatherSubscriptions(event)
			for _, sub := range subs {
				sub.signal(event)
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
			// the newly created subscription.
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

			sub.close()
		}
	}
}

func (q *EventQueue) gatherSubscriptions(ch changestream.ChangeEvent) []*subscription {
	subs := make(map[uint64]*subscription)

	for id := range q.subscriptionsAll {
		subs[id] = q.subscriptions[id]
	}

	for _, subOpt := range q.subscriptionsByNS[ch.Namespace()] {
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
