package eventwatcher

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
)

const (
	defaultUnsubscribeTimeout = time.Second
)

var (
	// The backoff strategy is used to back-off when we get no changes
	// from the database. This is used to prevent the worker from polling
	// the database too frequently and allow us to attempt to coalesce
	// changes when there is less activity.
	backOffStrategy = retry.ExpBackoff(time.Millisecond*10, time.Millisecond*250, 1.5, false)
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
	IsTraceEnabled() bool
}

type Watcher interface {
	worker.Worker
	Changes() <-chan []changestream.ChangeEvent
	Unsubscribe()
}

// EventWatcher takes an EventSource and allows you to watch all the events
// that are emitted from it. All the events are run asynchronously to prevent
// blocking the caller.
type EventWatcher struct {
	catacomb catacomb.Catacomb
	source   changestream.EventSource

	clock  clock.Clock
	logger Logger

	watcherRunner  *worker.Runner
	addRequests    chan chan Watcher
	removeRequests chan string
}

// New returns a new EventWatcher.
func New(source changestream.EventSource, clock clock.Clock, logger Logger) (*EventWatcher, error) {
	w := &EventWatcher{
		source:         source,
		clock:          clock,
		logger:         logger,
		addRequests:    make(chan chan Watcher),
		removeRequests: make(chan string),
		watcherRunner: worker.NewRunner(worker.RunnerParams{
			Clock: clock,
			IsFatal: func(err error) bool {
				return false
			},
			RestartDelay: time.Second * 10,
			Logger:       logger,
		}),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Kill stops the event watcher.
func (e *EventWatcher) Kill() {
	e.catacomb.Kill(nil)
}

// Wait waits for the event watcher to stop.
func (e *EventWatcher) Wait() error {
	return e.catacomb.Wait()
}

// Watch returns a Watcher that can be used to watch for events.
func (e *EventWatcher) Watch() (changestream.Watcher, error) {
	request := make(chan Watcher)
	select {
	case <-e.catacomb.Dying():
		return nil, changestream.ErrWatcherDying
	case e.addRequests <- request:
	}

	select {
	case <-e.catacomb.Dying():
		return nil, changestream.ErrWatcherDying
	case watchable := <-request:
		return watchable, nil
	}
}

func (w *EventWatcher) loop() error {
	sub, err := w.source.Subscribe()
	if err != nil {
		return errors.Trace(err)
	}
	// Ensure that we unsubscribe from the source when we are done.
	defer sub.Unsubscribe()

	ctx, cancel := w.scopedContext()
	defer cancel()

	events := make([][]changestream.ChangeEvent, 0)

	var id int
	var attempt int
	var overflow []string
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case changes := <-sub.Changes():
			w.logger.Tracef("received %d changes", len(changes))

			events = append(events, changes)

		case <-sub.Done():
			return nil

		case res := <-w.addRequests:
			id++
			namespace := fmt.Sprintf("watchable-%d", id)

			if err := w.watcherRunner.StartWorker(namespace, func() (worker.Worker, error) {
				return newWatchable(namespace, w.removeRequests), nil
			}); err != nil {
				return errors.Trace(err)
			}

			worker, err := w.watcherRunner.Worker(namespace, ctx.Done())
			if err != nil {
				return errors.Trace(err)
			}

			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case res <- worker.(Watcher):
			}

		case namespace := <-w.removeRequests:
			if err := w.watcherRunner.StopWorker(namespace); err != nil && !errors.Is(err, errors.NotFound) {
				return errors.Trace(err)
			}
			// Remove the namespace from the overflow list, it's pointless
			// attempting to dispatch to it if it's gone.
			var names []string
			for _, name := range overflow {
				if name == namespace {
					continue
				}
				names = append(names, name)
			}
			overflow = names

		default:
			if len(events) == 0 {
				attempt++
				select {
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				case <-w.clock.After(backOffStrategy(0, attempt)):
					continue
				}
			}

			// We have some events.
			attempt = 0

			// We allow for 100 milliseconds to process the events. If those
			// events are not processed within that time, we will try again
			// later.
			local, localCancel := context.WithTimeout(ctx, time.Millisecond*100)

			// We have overflow names to dispatch to, so use them, before
			// falling back to the worker runner.
			var names []string
			if len(overflow) > 0 {
				names = overflow
			} else {
				names = w.watcherRunner.WorkerNames()
			}

			var dispatched []string
			for _, name := range names {
				worker, err := w.watcherRunner.Worker(name, local.Done())
				if err != nil {
					if errors.Is(err, errors.NotFound) {
						// If the worker is not found, then we can continue.
						// Likely from the overflowed names.
						continue
					}
					localCancel()
					return errors.Trace(err)
				}

				watchable := worker.(*watchable)
				watchable.dispatch(events[0])

				dispatched = append(dispatched, name)
			}

			// Work out the overflow names to dispatch to. This can happen if
			// we didn't dispatch to all the workers.
			overflow = set.NewStrings(names...).Difference(set.NewStrings(dispatched...)).Values()

			// If the overflowed names are empty, then we can remove the first
			// event and continue.
			if len(overflow) == 0 {
				events = events[1:]
			}

			localCancel()
		}
	}
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *EventWatcher) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}

type watchable struct {
	tomb      tomb.Tomb
	changes   chan []changestream.ChangeEvent
	namespace string
	unsub     chan<- string
}

func newWatchable(namespace string, unsub chan<- string) *watchable {
	w := &watchable{
		namespace: namespace,
		unsub:     unsub,
	}
	w.tomb.Go(w.loop)

	return w
}

// Kill stops the event watcher.
func (w *watchable) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the event watcher to stop.
func (w *watchable) Wait() error {
	return w.tomb.Wait()
}

func (w *watchable) Changes() <-chan []changestream.ChangeEvent {
	return w.changes
}

func (w *watchable) Unsubscribe() {
	select {
	case <-w.tomb.Dying():
	case w.unsub <- w.namespace:
	case <-time.After(defaultUnsubscribeTimeout):
	}
}

func (w *watchable) loop() error {
	<-w.tomb.Dying()
	return tomb.ErrDying
}

func (w *watchable) dispatch(events []changestream.ChangeEvent) {
	w.tomb.Go(func() error {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case w.changes <- events:
		}
		return nil
	})
}
