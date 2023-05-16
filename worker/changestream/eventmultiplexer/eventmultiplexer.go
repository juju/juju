// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"context"
	"sync/atomic"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3/catacomb"
	"golang.org/x/sync/errgroup"

	"github.com/juju/juju/core/changestream"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Tracef(message string, args ...interface{})
	IsTraceEnabled() bool
}

// ChangeSet represents a set of changes.
type ChangeSet = []changestream.ChangeEvent

// Stream represents a way to get change events as set of terms.
type Stream interface {
	// Terms returns a channel for a given namespace (database) that returns
	// a set of terms. The notion of terms are a set of changes that can be
	// run one at a time asynchronously. Allowing changes within a given
	// term to be signaled of a change independently from one another.
	// Once a change within a term has been completed, only at that point
	// is another change processed, until all changes are exhausted.
	Terms() <-chan changestream.Term
}

type eventFilter struct {
	subscriptionID uint64
	changeMask     changestream.ChangeType
	filter         func(changestream.ChangeEvent) bool
}

// EventMultiplexer defines an event listener and dispatcher for db changes that
// can be multiplexed to subscriptions. The event queue allows consumers to
// subscribe via callbacks to the event queue. This is a lockless
// implementation, all subscriptions and changes are serialized in the main
// loop. Dispatching is randomized to ensure that subscriptions don't depend on
// ordering. The subscriptions can be associated with different subscription
// options, which provide filtering when dispatching. Unsubscribing is provided
// per subscription, which is done asynchronously.
type EventMultiplexer struct {
	catacomb catacomb.Catacomb
	stream   Stream
	logger   Logger
	clock    clock.Clock

	subscriptions      map[uint64]*subscription
	subscriptionsByNS  map[string][]*eventFilter
	subscriptionsAll   map[uint64]struct{}
	subscriptionsCount uint64

	// (un)subscription related channels to serialize adding and removing
	// subscriptions. This allows the queue to be lock less.
	subscriptionCh   chan subscriptionOpts
	unsubscriptionCh chan uint64
}

// New creates a new EventMultiplexer that will use the Stream for events.
func New(stream Stream, clock clock.Clock, logger Logger) (*EventMultiplexer, error) {
	queue := &EventMultiplexer{
		stream:             stream,
		logger:             logger,
		clock:              clock,
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
func (e *EventMultiplexer) Subscribe(opts ...changestream.SubscriptionOption) (changestream.Subscription, error) {
	// Get a new subscription count without using any mutexes.
	subID := atomic.AddUint64(&e.subscriptionsCount, 1)

	sub := newSubscription(subID, func() { e.unsubscribe(subID) })
	if err := e.catacomb.Add(sub); err != nil {
		return nil, errors.Trace(err)
	}

	select {
	case <-e.catacomb.Dying():
		return nil, e.catacomb.ErrDying()
	case e.subscriptionCh <- subscriptionOpts{
		subscription: sub,
		opts:         opts,
	}:
	}

	return sub, nil
}

// Kill stops the event queue.
func (e *EventMultiplexer) Kill() {
	e.catacomb.Kill(nil)
}

// Wait waits for the event queue to stop.
func (e *EventMultiplexer) Wait() error {
	return e.catacomb.Wait()
}

func (e *EventMultiplexer) unsubscribe(subscriptionID uint64) {
	select {
	case <-e.catacomb.Dying():
		return
	case e.unsubscriptionCh <- subscriptionID:
	}
}

func (e *EventMultiplexer) loop() error {
	defer func() {
		for _, sub := range e.subscriptions {
			sub.close()
		}
		e.subscriptions = nil
		e.subscriptionsByNS = nil

		close(e.subscriptionCh)
		close(e.unsubscriptionCh)
	}()

	for {
		select {
		case <-e.catacomb.Dying():
			return e.catacomb.ErrDying()

		case term, ok := <-e.stream.Terms():
			// If the stream is closed, we expect that a new worker will come
			// again using the change stream worker infrastructure. In this case
			// just ignore and close out.
			if !ok {
				e.logger.Infof("change stream term channel is closed")
				return nil
			}

			changeSet := make(map[*subscription]ChangeSet)
			for _, change := range term.Changes() {
				for _, sub := range e.gatherSubscriptions(change) {
					changeSet[sub] = append(changeSet[sub], change)
				}
			}

			// Dispatch the set of changes, but do not cause the worker to
			// exit. Just log out the error and then mark the term as done.
			// There isn't anything we can do in this case.
			if err := e.dispatchSet(changeSet); err != nil {
				// TODO (stickupkid): We should expose this as either a metric
				// or some sort of feedback so we can see how ofter this is
				// actually happening?
				e.logger.Errorf("dispatching set: %v", err)
			}

			term.Done()

		case subOpt := <-e.subscriptionCh:
			sub := subOpt.subscription

			// Create a new subscription and assign a unique ID to it.
			e.subscriptions[sub.id] = sub

			// No options were supplied, just add it to the all bucket, so
			// they'll be included in every dispatch.
			if len(subOpt.opts) == 0 {
				e.subscriptionsAll[sub.id] = struct{}{}
				continue
			}

			// Register filters to route changes matching the subscription criteria to
			// the newly created subscription.
			for _, opt := range subOpt.opts {
				namespace := opt.Namespace()
				e.subscriptionsByNS[namespace] = append(e.subscriptionsByNS[namespace], &eventFilter{
					subscriptionID: sub.id,
					changeMask:     opt.ChangeMask(),
					filter:         opt.Filter(),
				})
				sub.topics[namespace] = struct{}{}
			}

		case subscriptionID := <-e.unsubscriptionCh:
			sub, found := e.subscriptions[subscriptionID]
			if !found {
				continue
			}

			for topic := range sub.topics {
				var updatedFilters []*eventFilter
				for _, filter := range e.subscriptionsByNS[topic] {
					if filter.subscriptionID == subscriptionID {
						continue
					}
					updatedFilters = append(updatedFilters, filter)
				}
				e.subscriptionsByNS[topic] = updatedFilters
			}

			delete(e.subscriptions, subscriptionID)
			delete(e.subscriptionsAll, subscriptionID)

			sub.close()
		}
	}
}

func (e *EventMultiplexer) gatherSubscriptions(ch changestream.ChangeEvent) []*subscription {
	subs := make(map[uint64]*subscription)

	for id := range e.subscriptionsAll {
		subs[id] = e.subscriptions[id]
	}

	traceEnabled := e.logger.IsTraceEnabled()
	for _, subOpt := range e.subscriptionsByNS[ch.Namespace()] {
		if _, ok := subs[subOpt.subscriptionID]; ok {
			continue
		}

		if (ch.Type() & subOpt.changeMask) == 0 {
			continue
		}

		if !subOpt.filter(ch) {
			if traceEnabled {
				e.logger.Tracef("filtering out change: %v", ch)
			}
			continue
		}

		if traceEnabled {
			e.logger.Tracef("dispatching change: %v", ch)
		}

		subs[subOpt.subscriptionID] = e.subscriptions[subOpt.subscriptionID]
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

// dispatchSet fans out the subscription requests against a given term of changes.
// Each subscription signals the change in a asynchronous fashion, allowing
// a subscription to not block another change within a given term.
func (e *EventMultiplexer) dispatchSet(changeSet map[*subscription]ChangeSet) error {
	grp, ctx := errgroup.WithContext(e.catacomb.Context(context.Background()))

	for sub, changes := range changeSet {
		sub, changes := sub, changes

		grp.Go(func() error {
			// Pass the context of the catacomb with the deadline to the
			// subscription. This allows the subscription to be cancelled
			// if the catacomb is dying or if the deadline is reached.
			return sub.signal(ctx, changes)
		})
	}

	return grp.Wait()
}
