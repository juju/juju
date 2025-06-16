// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventmultiplexer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	internalworker "github.com/juju/juju/internal/worker"
)

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
// options, which provide filtering when dispatching. Unsubscribing is provided
// per subscription, which is done asynchronously.
type EventMultiplexer struct {
	catacomb catacomb.Catacomb
	stream   Stream
	logger   logger.Logger
	clock    clock.Clock
	metrics  MetricsCollector

	mutex              sync.Mutex
	subscriptions      map[uint64]*subscription
	subscriptionsByNS  map[string][]*eventFilter
	subscriptionsAll   map[uint64]struct{}
	subscriptionsCount uint64
	dispatchErrorCount int

	// (un)subscription related channels to serialize adding and removing
	// subscriptions. This allows the queue to be lock less.
	subscriptionCh   chan requestSubscription
	unsubscriptionCh chan uint64

	reportsCh chan reportRequest

	runner *worker.Runner
}

// New creates a new EventMultiplexer that will use the Stream for events.
func New(stream Stream, clock clock.Clock, metrics MetricsCollector, logger logger.Logger) (*EventMultiplexer, error) {
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "event-multiplexer",
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: func(err error) bool {
			return false
		},
		Clock:  clock,
		Logger: internalworker.WrapLogger(logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	queue := &EventMultiplexer{
		stream:  stream,
		logger:  logger,
		clock:   clock,
		metrics: metrics,

		subscriptions:      make(map[uint64]*subscription),
		subscriptionsByNS:  make(map[string][]*eventFilter),
		subscriptionsAll:   make(map[uint64]struct{}),
		subscriptionsCount: 0,
		dispatchErrorCount: 0,

		subscriptionCh:   make(chan requestSubscription),
		unsubscriptionCh: make(chan uint64),

		reportsCh: make(chan reportRequest),

		runner: runner,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "event-multiplexer",
		Site: &queue.catacomb,
		Work: queue.loop,
		Init: []worker.Worker{
			queue.runner,
		},
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

// Report returns a map of the current state of the event queue. This is
// used by the engine report.
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

	// We can't block the engine report, so we timeout after a second.
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

func (e *EventMultiplexer) unsubscribe(subscriptionID uint64) {
	select {
	case <-e.catacomb.Dying():
		return
	case <-e.stream.Dying():
		return
	case e.unsubscriptionCh <- subscriptionID:
	}
}

func (e *EventMultiplexer) loop() error {
	ctx, cancel := e.scopedContext()
	defer cancel()

	// The subscription channels need to handle the case where they're not
	// attempting
	go func() {
		for {
			select {
			case <-e.catacomb.Dying():
				return
			case request := <-e.subscriptionCh:
				// Get a new subscription count without using any mutexes.
				subID := atomic.AddUint64(&e.subscriptionsCount, 1)

				// Start the worker for the subscription.
				err := e.runner.StartWorker(ctx, e.subName(subID), func(ctx context.Context) (worker.Worker, error) {
					return newSubscription(subID, func() { e.unsubscribe(subID) }), nil
				})
				if errors.Is(err, e.catacomb.ErrDying()) {
					// This is fine since the caller should also be
					// handling the dying state of the multiplexer.
					return
				} else if err != nil {
					request.result <- requestSubscriptionResult{
						err: err,
					}
					continue
				}

				sub := e.processSubscription(ctx, subID, request)
				if sub == nil {
					// If the subscription was not created, then we should
					// signal the request with an error.
					request.result <- requestSubscriptionResult{
						err: errors.Errorf("subscription %d not found", subID),
					}
					continue
				}

				// We're up and running, so we can signal to the subscription that
				// it has been successfully created and is ready to receive changes.
				request.result <- requestSubscriptionResult{
					sub: sub,
				}
				continue

			case subscriptionID := <-e.unsubscriptionCh:

				e.processUnsubscription(ctx, subscriptionID)

			case r := <-e.reportsCh:

				e.mutex.Lock()
				r.data["subscriptions"] = len(e.runner.WorkerNames())
				r.data["subscriptions-by-ns"] = len(e.subscriptionsByNS)
				r.data["subscriptions-all"] = len(e.subscriptionsAll)
				r.data["dispatch-error-count"] = e.dispatchErrorCount

				// If the stream supports reporting, then include it in the report.
				if s, ok := e.stream.(reporter); ok {
					r.data["stream"] = s.Report()
				}
				e.mutex.Unlock()

				close(r.done)
			}
		}
	}()

	for {
		select {
		// If the catacomb is dying, then we should exit.
		case <-e.catacomb.Dying():
			return e.catacomb.ErrDying()

		// If the underlying stream is dying, then we should also exit.
		case <-e.stream.Dying():
			e.logger.Debugf(ctx, "change stream is dying, waiting for catacomb to die")

			<-e.catacomb.Dying()
			return e.catacomb.ErrDying()

		case term, ok := <-e.stream.Terms():
			// If the stream is closed, we expect that a new worker will come
			// again using the change stream worker infrastructure. In this case
			// just ignore and close out.
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
			dispatchErrs := e.dispatchSet(ctx, changeSet)
			failed := len(dispatchErrs) > 0
			if failed {
				e.logger.Errorf(ctx, "dispatching set: %v", dispatchErrs)
				e.dispatchErrorCount++
			}
			e.metrics.DispatchDurationObserve(e.clock.Now().Sub(begin).Seconds(), failed)

			// We should guarantee that the change set is not empty, so we
			// can force false here.
			term.Done(false, e.catacomb.Dying())

			// TODO (stickupkid): We might want to run runtime.Gosched()
			// here to allow other goroutines to run. This might end up
			// being a bottleneck if we have a lot of changes coming in.
		}
	}
}

func (e *EventMultiplexer) processSubscription(ctx context.Context, subscriptionID uint64, request requestSubscription) *subscription {
	sub, err := e.getSubscription(subscriptionID)
	if err != nil {
		e.logger.Debugf(ctx, "unsubscription for %d failed: %v", subscriptionID, err)
		return nil
	} else if sub == nil {
		return nil
	}

	e.mutex.Lock()

	// No options were supplied, just add it to the all bucket, so
	// they'll be included in every dispatch.
	e.subscriptions[sub.id] = sub

	if len(request.opts) == 0 {
		e.subscriptionsAll[sub.id] = struct{}{}
	} else {
		// Register filters to route changes matching the subscription criteria to
		// the newly created subscription.
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

	e.mutex.Unlock()

	e.metrics.SubscriptionsInc()

	return sub
}

func (e *EventMultiplexer) processUnsubscription(ctx context.Context, subscriptionID uint64) {
	sub, err := e.getSubscription(subscriptionID)
	if err != nil {
		return
	} else if sub == nil {
		return
	}

	e.mutex.Lock()

	for topic := range sub.topics {
		var updatedFilters []*eventFilter
		for _, filter := range e.subscriptionsByNS[topic] {
			if filter.subscriptionID == subscriptionID {
				continue
			}
			updatedFilters = append(updatedFilters, filter)
		}

		// If we don't have any more filters for this topic, remove it
		// otherwise we'll keep iterating over it.
		if len(updatedFilters) == 0 {
			delete(e.subscriptionsByNS, topic)
			continue
		}

		e.subscriptionsByNS[topic] = updatedFilters
	}

	delete(e.subscriptionsAll, subscriptionID)
	delete(e.subscriptions, subscriptionID)

	e.mutex.Unlock()

	e.metrics.SubscriptionsDec()

	// If the subscription errors out on a close, we don't want that
	// to bring down the entire multiplexer. Instead, just log it out
	// and continue.
	if err := sub.close(); err != nil {
		e.logger.Infof(ctx, "error closing subscription %d: %v", subscriptionID, err)
	}

	// Lastly, remove the worker from the runner. This will prevent the
	// report from including the subscription in the list of active
	// subscriptions.
	_ = e.runner.StopAndRemoveWorker(e.subName(subscriptionID), e.catacomb.Dying())
}

func (e *EventMultiplexer) getSubscription(subscriptionID uint64) (*subscription, error) {
	worker, err := e.runner.Worker(e.subName(subscriptionID), e.catacomb.Dying())
	if errors.Is(err, e.catacomb.ErrDying()) || errors.Is(err, errors.NotFound) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	return worker.(*subscription), nil
}

type reporter interface {
	Report() map[string]interface{}
}

func (e *EventMultiplexer) gatherSubscriptions(ctx context.Context, ch changestream.ChangeEvent) []*subscription {
	subs := make(map[uint64]*subscription)

	e.mutex.Lock()

	for id := range e.subscriptionsAll {
		subs[id] = e.subscriptions[id]
	}

	for _, subOpt := range e.subscriptionsByNS[ch.Namespace()] {
		if _, ok := subs[subOpt.subscriptionID]; ok {
			continue
		}

		if (ch.Type() & subOpt.changeMask) == 0 {
			continue
		}

		if !subOpt.filter(ch) {
			continue
		}

		subs[subOpt.subscriptionID] = e.subscriptions[subOpt.subscriptionID]
	}

	e.mutex.Unlock()

	// By collecting the subs within a map to ensure that a sub can be only
	// called once, we actually gain random ordering. This prevents subscribers
	// from depending on the order of dispatches.
	results := make([]*subscription, 0, len(subs))
	for _, sub := range subs {
		results = append(results, sub)
	}

	if e.logger.IsLevelEnabled(logger.TRACE) {
		e.logger.Tracef(ctx, "dispatching change: %v for: %v", ch, len(results))
	}
	return results
}

// dispatchSet fans out the subscription requests against a given term of
// changes. Each subscription signals the change in a asynchronous fashion,
// allowing a subscription to not block another change within a given term.
func (e *EventMultiplexer) dispatchSet(ctx context.Context, changeSet map[*subscription]ChangeSet) []error {
	var wg sync.WaitGroup

	errs := make(chan error, len(changeSet))
	for sub, changes := range changeSet {
		wg.Add(1)

		go func(sub *subscription, changes ChangeSet) {
			defer wg.Done()

			errs <- sub.dispatch(ctx, changes)
		}(sub, changes)
	}

	wg.Wait()
	close(errs)

	var results []error
	for err := range errs {
		if err != nil && !errors.Is(err, subscriptionClosed) {
			results = append(results, err)
		}
	}
	return results
}

func (e *EventMultiplexer) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(e.catacomb.Context(context.Background()))
}

func (e *EventMultiplexer) subName(id uint64) string {
	return fmt.Sprintf("sub-%d", id)
}
