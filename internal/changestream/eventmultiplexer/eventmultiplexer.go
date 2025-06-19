// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"
	"golang.org/x/sync/errgroup"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
)

// ChangeSet represents a set of changes.
type ChangeSet = []changestream.ChangeEvent

// Stream represents a way to get change events as set of terms.
type Stream interface {
	// Terms returns a channel from which terms for a given
	// namespace (database) will be received.
	// A term contains all records from the change_log table
	// since the last received term.
	Terms() <-chan changestream.Term

	// Dying returns a channel that is closed when the stream is dying.
	Dying() <-chan struct{}
}

// MetricsCollector represents the metrics methods called.
type MetricsCollector interface {
	SubscriptionsInc()
	SubscriptionsDec()
	DispatchDurationObserve(val float64, failed bool)
}

type eventFilter struct {
	subscriptionID uint64
	changeMask     changestream.ChangeType
	filter         func(changestream.ChangeEvent) bool
}

type reportRequest struct {
	data map[string]any
	done chan struct{}
}

// EventMultiplexer defines a way to receive streamed terms for changes that
// can be multiplexed to subscriptions. The event queue allows consumers to
// subscribe via callbacks to the event queue. This is a lockless
// implementation, all subscriptions and changes are serialized in the main
// loop. Dispatching is randomized to ensure that subscriptions don't depend on
// ordering. The subscriptions can be associated with different subscription
// options, which provide filtering when dispatching.
type EventMultiplexer struct {
	catacomb catacomb.Catacomb
	stream   Stream
	logger   logger.Logger
	clock    clock.Clock
	metrics  MetricsCollector

	subscriptions      map[uint64]*subscription
	subscriptionsByNS  map[string][]*eventFilter
	subscriptionsAll   map[uint64]struct{}
	subscriptionsCount uint64
	dispatchErrorCount int

	// subscriptionCh is a channel used to request new subscriptions.
	// This is used to sync subscription additions into the loop.
	subscriptionCh chan requestSubscription

	reportsCh chan reportRequest
}

// New creates a new EventMultiplexer that will use the Stream for events.
func New(stream Stream, clock clock.Clock, metrics MetricsCollector, logger logger.Logger) (*EventMultiplexer, error) {
	queue := &EventMultiplexer{
		stream:             stream,
		logger:             logger,
		clock:              clock,
		metrics:            metrics,
		subscriptions:      make(map[uint64]*subscription),
		subscriptionsByNS:  make(map[string][]*eventFilter),
		subscriptionsAll:   make(map[uint64]struct{}),
		subscriptionsCount: 0,
		dispatchErrorCount: 0,

		subscriptionCh: make(chan requestSubscription),

		reportsCh: make(chan reportRequest),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "event-multiplexer",
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
	result := make(chan requestSubscriptionResult)
	select {
	case <-e.catacomb.Dying():
		return nil, database.ErrEventMultiplexerDying
	case e.subscriptionCh <- requestSubscription{
		opts:   opts,
		result: result,
	}:
	}

	select {
	case <-e.catacomb.Dying():
		return nil, database.ErrEventMultiplexerDying
	case res := <-result:
		return res.sub, errors.Trace(res.err)
	}
}

// Kill stops the event queue.
func (e *EventMultiplexer) Kill() {
	e.catacomb.Kill(nil)
}

// Wait waits for the event queue to stop.
func (e *EventMultiplexer) Wait() error {
	return e.catacomb.Wait()
}

// Report returns the current state of the event queue.
// This is used by the engine report.
func (e *EventMultiplexer) Report() map[string]any {
	ctx, cancel := e.scopedContext()
	defer cancel()

	r := reportRequest{
		data: make(map[string]any),
		done: make(chan struct{}),
	}
	select {
	case <-e.catacomb.Dying():
		return nil
	case <-e.stream.Dying():
		return nil

	// We can't block the engine report, so we time out after a second.
	// This can happen if we're in the middle of a dispatch and the term
	// channel is blocked.
	case <-e.clock.After(time.Second):
		e.logger.Errorf(ctx, "report request timed out")
		return nil
	case e.reportsCh <- r:
	}

	select {
	case <-e.catacomb.Dying():
		return nil
	case <-e.stream.Dying():
		return nil
	case <-r.done:
		return r.data
	}
}

func (e *EventMultiplexer) loop() error {
	ctx, cancel := e.scopedContext()
	defer cancel()

	defer func() {
		for _, sub := range e.subscriptions {
			sub.Kill()
		}
		e.subscriptions = nil
		e.subscriptionsByNS = nil
		e.subscriptionsAll = nil
	}()

	for {
		e.cleanupDeadSubscriptions(ctx)

		select {
		case <-e.catacomb.Dying():
			return e.catacomb.ErrDying()

		case <-e.stream.Dying():
			e.logger.Infof(ctx, "change stream is dying, waiting for catacomb to die")

			<-e.catacomb.Dying()
			return e.catacomb.ErrDying()

		case term, ok := <-e.stream.Terms():
			// If the stream is closed, we expect that a new worker will come
			// again using the change stream worker infrastructure.
			// Just exit the loop.
			if !ok {
				e.logger.Infof(ctx, "change stream term channel is closed")
				return nil
			}

			changeSet := make(map[*subscription]ChangeSet)
			for _, change := range term.Changes() {
				subs := e.gatherSubscriptions(ctx, change)
				if len(subs) == 0 {
					continue
				}

				for _, sub := range subs {
					changeSet[sub] = append(changeSet[sub], change)
				}
			}

			// Nothing to do here, just mark the term as done.
			if len(changeSet) == 0 {
				term.Done(true, e.catacomb.Dying())
				continue
			}

			begin := e.clock.Now()
			// Dispatch the set of changes, but do not cause the worker to
			// exit. Just log out the error and then mark the term as done.
			// There isn't anything we can do in this case.
			err := e.dispatchSet(changeSet)
			if err != nil {
				e.logger.Errorf(ctx, "dispatching set: %v", err)
				e.dispatchErrorCount++
			}
			e.metrics.DispatchDurationObserve(e.clock.Now().Sub(begin).Seconds(), err != nil)

			// We should guarantee that the change set is not empty,
			// so we can force false here.
			term.Done(false, e.catacomb.Dying())

		case request := <-e.subscriptionCh:
			sub := newSubscription(atomic.AddUint64(&e.subscriptionsCount, 1))

			if err := e.catacomb.Add(sub); err != nil {
				sub.Kill()

				if errors.Is(err, e.catacomb.ErrDying()) {
					return err
				}

				select {
				case <-e.catacomb.Dying():
					return e.catacomb.ErrDying()
				case request.result <- requestSubscriptionResult{err: err}:
					continue
				}
			}

			e.metrics.SubscriptionsInc()

			// Create a new subscription and assign a unique ID to it.
			e.subscriptions[sub.id] = sub

			// No options were supplied, just add it to the all-subs bucket,
			// so they'll be included in every dispatch.
			if len(request.opts) == 0 {
				e.subscriptionsAll[sub.id] = struct{}{}
			} else {
				// Register filters to route changes matching the subscription
				// criteria to the newly created subscription.
				for _, opt := range request.opts {
					namespace := opt.Namespace()
					e.subscriptionsByNS[namespace] = append(e.subscriptionsByNS[namespace], &eventFilter{
						subscriptionID: sub.id,
						changeMask:     opt.ChangeMask(),
						filter:         opt.Filter(),
					})
					sub.topics[namespace] = struct{}{}
				}
			}

			select {
			case <-e.catacomb.Dying():
				return e.catacomb.ErrDying()
			case request.result <- requestSubscriptionResult{sub: sub}:
			}

		case r := <-e.reportsCh:
			r.data["subscriptions"] = len(e.subscriptions)
			r.data["subscriptions-by-ns"] = len(e.subscriptionsByNS)
			r.data["subscriptions-all"] = len(e.subscriptionsAll)
			r.data["dispatch-error-count"] = e.dispatchErrorCount

			// If the stream supports reporting, then include it in the report.
			if s, ok := e.stream.(reporter); ok {
				r.data["stream"] = s.Report()
			}
			close(r.done)
		}
	}
}

type reporter interface {
	Report() map[string]interface{}
}

func (e *EventMultiplexer) gatherSubscriptions(ctx context.Context, ch changestream.ChangeEvent) []*subscription {
	subs := make(map[uint64]*subscription)

	for id := range e.subscriptionsAll {
		subs[id] = e.subscriptions[id]
	}

	traceEnabled := e.logger.IsLevelEnabled(logger.TRACE)
	for _, subOpt := range e.subscriptionsByNS[ch.Namespace()] {
		if _, ok := subs[subOpt.subscriptionID]; ok {
			continue
		}

		if (ch.Type() & subOpt.changeMask) == 0 {
			continue
		}

		if !subOpt.filter(ch) {
			if traceEnabled {
				e.logger.Tracef(ctx, "filtering out change: %v", ch)
			}
			continue
		}

		if traceEnabled {
			e.logger.Tracef(ctx, "dispatching change: %v", ch)
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
			// subscription. This allows the subscription to be cancelled if the
			// catacomb is dying or if the deadline is reached.
			// If the subscription has been killed or did not dequeue in time,
			// mark it bad. It will be removed at the top of the next loop.
			return sub.dispatch(ctx, changes)
		})
	}

	return grp.Wait()
}

func (e *EventMultiplexer) cleanupDeadSubscriptions(ctx context.Context) {
	for id, sub := range e.subscriptions {
		select {
		case <-sub.Done():
			e.logger.Debugf(ctx, "removing dead subscription %d", id)
			e.unsubscribe(id, sub)
		default:
		}
	}
}

func (e *EventMultiplexer) unsubscribe(subID uint64, sub *subscription) {
	for topic := range sub.topics {
		var updatedFilters []*eventFilter
		for _, filter := range e.subscriptionsByNS[topic] {
			if filter.subscriptionID == subID {
				continue
			}
			updatedFilters = append(updatedFilters, filter)
		}

		// If we don't have any more filters for this topic,
		// remove it so we don't keep iterating over it.
		if len(updatedFilters) == 0 {
			delete(e.subscriptionsByNS, topic)
			continue
		}
		e.subscriptionsByNS[topic] = updatedFilters
	}

	delete(e.subscriptions, subID)
	delete(e.subscriptionsAll, subID)

	e.metrics.SubscriptionsDec()
}

func (e *EventMultiplexer) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(e.catacomb.Context(context.Background()))
}
