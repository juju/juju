// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/worker/v3/catacomb"
	"golang.org/x/sync/errgroup"
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
func (q *EventMultiplexer) Subscribe(opts ...changestream.SubscriptionOption) (changestream.Subscription, error) {
	// Get a new subscription count without using any mutexes.
	subID := atomic.AddUint64(&q.subscriptionsCount, 1)

	sub := newSubscription(subID, func() { q.unsubscribe(subID) }, q.clock)
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
func (q *EventMultiplexer) Kill() {
	q.catacomb.Kill(nil)
}

// Wait waits for the event queue to stop.
func (q *EventMultiplexer) Wait() error {
	return q.catacomb.Wait()
}

func (q *EventMultiplexer) unsubscribe(subscriptionID uint64) {
	select {
	case <-q.catacomb.Dying():
		return
	case q.unsubscriptionCh <- subscriptionID:
	}
}

func (q *EventMultiplexer) loop() error {
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

		case term, ok := <-q.stream.Terms():
			// If the stream is closed, we expect that a new worker will come
			// again using the change stream worker infrastructure. In this case
			// just ignore and close out.
			if !ok {
				q.logger.Infof("change stream term channel is closed")
				return nil
			}

			changeSet := make(map[*subscription][]changestream.ChangeEvent)
			for _, change := range term.Changes() {
				subs := q.gatherSubscriptions(change)
				if len(subs) == 0 {
					continue
				}
				for _, sub := range subs {
					changeSet[sub] = append(changeSet[sub], change)
				}
			}

			q.dispatchSet(changeSet)

			term.Done()

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

func (q *EventMultiplexer) gatherSubscriptions(ch changestream.ChangeEvent) []*subscription {
	subs := make(map[uint64]*subscription)

	for id := range q.subscriptionsAll {
		subs[id] = q.subscriptions[id]
	}

	traceEnabled := q.logger.IsTraceEnabled()
	for _, subOpt := range q.subscriptionsByNS[ch.Namespace()] {
		if _, ok := subs[subOpt.subscriptionID]; ok {
			continue
		}

		if (ch.Type() & subOpt.changeMask) == 0 {
			continue
		}

		if !subOpt.filter(ch) {
			if traceEnabled {
				q.logger.Tracef("filtering out change: %v", ch)
			}
			continue
		}

		if traceEnabled {
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

// dispatchSet fans out the subscription requests against a given term of changes.
// Each subscription signals the change in a asynchronous fashion, allowing
// a subscription to not block another change within a given term.
func (q *EventMultiplexer) dispatchSet(changeSet map[*subscription]ChangeSet) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	grp, grpCtx := errgroup.WithContext(q.catacomb.Context(ctx))
	grp.SetLimit(len(changeSet))

	for sub, changes := range changeSet {
		sub, changes := sub, changes

		grp.Go(func() error {
			// Pass the context of the catacomb with the deadline to the
			// subscription. This allows the subscription to be cancelled
			// if the catacomb is dying or if the deadline is reached.
			return sub.signal(changes, grpCtx.Done(), ctx.Done())
		})
	}

	return grp.Wait()
}
