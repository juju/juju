// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"sync"

	"github.com/juju/collections/deque"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// Config is an argument struct used to create a Worker.
type Config struct {
	Logger               Logger
	Backing              state.AllWatcherBacking
	PrometheusRegisterer prometheus.Registerer
	Cleanup              func()
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if config.Backing == nil {
		return errors.NotValidf("missing Backing")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("missing PrometheusRegisterer")
	}
	return nil
}

// Worker runs the primary goroutine for managing the multiwatchers.
type Worker struct {
	config Config

	tomb    tomb.Tomb
	metrics *Collector

	// store holds information about all known entities.
	store multiwatcher.Store

	// request receives requests from Multiwatcher clients.
	request chan *request

	// Each entry in the waiting map holds a linked list of Next requests
	// outstanding for the associated params.
	waiting map[*Watcher]*request

	mu           sync.Mutex
	watchers     []*Watcher
	restartCount int
	// remember the last five errors that caused us to restart the internal loop
	errors []error

	// The worker should not block incoming events from the watcher on the
	// processing of those events. Use a queue to store the events that are
	// needed to be processed.
	pending *deque.Deque
	data    chan struct{}
	closed  chan struct{}
}

// request holds a message from the Multiwatcher to the
// storeManager for some changes. The request will be
// replied to when some changes are available.
type request struct {
	// w holds the Multiwatcher that has originated the request.
	watcher *Watcher

	// reply receives a message when deltas are ready.  If reply is
	// nil, the Multiwatcher will be stopped.  If the reply is true,
	// the request has been processed; if false, the Multiwatcher
	// has been stopped,
	reply chan bool

	// noChanges receives a message when the manager checks for changes
	// and there are none.
	noChanges chan struct{}

	// changes is populated as part of the reply and will hold changes that have
	// occurred since the last replied-to Next request.
	changes []multiwatcher.Delta

	// next points to the next request in the list of outstanding
	// requests on a given watcher.  It is used only by the central
	// storeManager goroutine.
	next *request
}

// NewWorkerShim is a method used for hooking up the specific NewWorker
// to the manifold NewWorker config arg. This allows other tests to use
// the NewWorker to get something that acts as a multiwatcher.Factory
// without having to cast the worker.
func NewWorkerShim(config Config) (worker.Worker, error) {
	return NewWorker(config)
}

// NewWorker creates the worker and starts the loop goroutine.
func NewWorker(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	closed := make(chan struct{})
	close(closed)
	w := &Worker{
		config: config,
		// There always needs to be a valid request channel.
		request: make(chan *request),
		waiting: make(map[*Watcher]*request),
		store:   multiwatcher.NewStore(config.Logger),
		pending: deque.New(),
		data:    make(chan struct{}, 1),
		closed:  closed,
	}
	w.metrics = NewMetricsCollector(w)
	w.tomb.Go(w.loop)
	return w, nil
}

const (
	// Keys used in the Report method are used to retrieve from
	// the map in the metrics code, so define constants for the keys.
	reportWatcherKey = "num-watchers"
	reportStoreKey   = "store-size"
	reportRestartKey = "restart-count"
	reportErrorsKey  = "errors"
)

// Report is shown up in the engine report of the agent.
func (w *Worker) Report() map[string]interface{} {
	w.mu.Lock()
	count := len(w.watchers)
	store := w.store.Size()
	restart := w.restartCount
	var errors []string
	for _, err := range w.errors {
		errors = append(errors, err.Error())
	}
	w.mu.Unlock()

	report := map[string]interface{}{
		reportWatcherKey: count,
		reportStoreKey:   store,
		reportRestartKey: restart,
	}
	if len(errors) > 0 {
		report[reportErrorsKey] = errors
	}
	return report
}

// WatchController returns entity delta events for all models in the controller.
func (w *Worker) WatchController() multiwatcher.Watcher {
	return w.newWatcher(nil)
}

// WatchModel returns entity delta events just for the specified model.
func (w *Worker) WatchModel(modelUUID string) multiwatcher.Watcher {
	return w.newWatcher(
		func(in []multiwatcher.Delta) []multiwatcher.Delta {
			// Returns an empty slice if there is nothing to match with the
			// implementation of the Watcher for noChanges. Both could potentially
			// be updated to return a nil slice.
			result := make([]multiwatcher.Delta, 0, len(in))
			for _, delta := range in {
				if delta.Entity.EntityID().ModelUUID == modelUUID {
					result = append(result, delta)
				}
			}
			return result
		})
}

func (w *Worker) newWatcher(filter func([]multiwatcher.Delta) []multiwatcher.Delta) *Watcher {
	watcher := &Watcher{
		request: w.request,
		control: &w.tomb,
		logger:  w.config.Logger,
		// Buffered err channel as if there is a fetch error on the all watcher backing
		// the error is passed to the watcher.
		err:    make(chan error, 1),
		filter: filter,
	}
	w.mu.Lock()
	w.watchers = append(w.watchers, watcher)
	w.mu.Unlock()
	return watcher
}

func (w *Worker) loop() error {
	w.config.Logger.Tracef("worker loop started")
	defer w.config.Logger.Tracef("worker loop completed")
	defer func() {
		if w.config.Cleanup != nil {
			w.config.Cleanup()
		}
	}()

	_ = w.config.PrometheusRegisterer.Register(w.metrics)
	defer w.config.PrometheusRegisterer.Unregister(w.metrics)

	for {
		err := w.inner()
		select {
		case <-w.tomb.Dying():
			return nil
		default:
			w.mu.Lock()
			w.restartCount++
			w.errors = append(w.errors, err)
			if len(w.errors) > 5 {
				// Remembering the last five errors is somewhat of an arbitrary number,
				// but we want more than just the last one, and we do want a cap.
				// Since we only ever add one at a time, we know that removing just
				// the first one will get us back to five.
				w.errors = w.errors[1:]
			}
			w.store = multiwatcher.NewStore(w.config.Logger)
			w.request = make(chan *request)
			w.waiting = make(map[*Watcher]*request)
			// Since the worker itself isn't dying, we need to manually stop all
			// the watchers.
			for _, watcher := range w.watchers {
				watcher.err <- err
			}
			w.watchers = nil
			w.mu.Unlock()
		}
	}
}

// We don't want to restart the worker just because the backing has raised an error.
// If it does, we record the error, and start again with a new store.
func (w *Worker) inner() error {
	w.config.Logger.Tracef("worker inner started")
	defer w.config.Logger.Tracef("worker inner completed")

	// Create the wait group, and set up the defer before the watching
	// the backing, as we want the backing unwatch to happen before the
	// waitgroup wait call. This is to ensure we aren't blocking the
	// backing event generator.
	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()

	backing := w.config.Backing
	in := make(chan watcher.Change)
	backing.Watch(in)
	defer backing.Unwatch(in)

	processError := make(chan error)

	go func() {
		err := w.process(backing, w.tomb.Dying())
		select {
		case <-w.tomb.Dying():
		case processError <- err:
		}
		wg.Done()
	}()

	for {
		select {
		case <-w.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case err := <-processError:
			return errors.Trace(err)
		case change := <-in:
			w.mu.Lock()
			w.pending.PushBack(&change)
			if w.pending.Len() == 1 {
				select {
				// In all normal cases, we can push something onto sm.data
				// as it is a buffered channel. And if the length is one,
				// then data should be empty. However paranoia and all that.
				case w.data <- struct{}{}:
				default:
				}
			}
			w.mu.Unlock()
		}
	}
}

func (w *Worker) process(backing state.AllWatcherBacking, done <-chan struct{}) error {
	// We have no idea what changes the watcher might be trying to
	// send us while getAll proceeds, but we don't mind, because
	// backing.Changed is idempotent with respect to both updates
	// and removals.
	if err := backing.GetAll(w.store); err != nil {
		return errors.Trace(err)
	}
	var next <-chan struct{}

	for {
		select {
		case <-done:
			return nil
		case <-w.data:
			// Has new data been pushed on?
			w.config.Logger.Tracef("new data pushed on queue")
		case <-next:
			// If there was already data, next is a closed channel.
			// Otherwise it is nil, so won't pass through.
			w.config.Logger.Tracef("process data on queue")
		case req := <-w.request:
			// If we get a watcher request to handle while we are
			// waiting for changes, handle it, and respond.
			w.config.Logger.Tracef("handle request: %#v", req)
			w.handle(req)
		}
		change, empty := w.popOne()
		if empty {
			next = nil
		} else {
			next = w.closed
		}
		if change != nil {
			if err := backing.Changed(w.store, *change); err != nil {
				return errors.Trace(err)
			}
		}
		w.respond()
	}
}

func (w *Worker) popOne() (*watcher.Change, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	val, ok := w.pending.PopFront()
	if !ok {
		// nothing to do
		return nil, true
	}
	empty := w.pending.Len() == 0
	return val.(*watcher.Change), empty
}

// Kill implements worker.Worker.Kill.
func (w *Worker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (w *Worker) Wait() error {
	return errors.Trace(w.tomb.Wait())
}

// handle processes a request from a Multiwatcher.
func (w *Worker) handle(req *request) {
	w.config.Logger.Tracef("start handle")
	defer w.config.Logger.Tracef("finish handle")
	if req.watcher.stopped {
		w.config.Logger.Tracef("watcher %p is stopped", req.watcher)
		// The watcher has previously been stopped.
		if req.reply != nil {
			select {
			case req.reply <- false:
			case <-w.tomb.Dying():
			}
		}
		return
	}
	if req.reply == nil {
		w.config.Logger.Tracef("request to stop watcher %p", req.watcher)
		// This is a request to stop the watcher.
		for req := w.waiting[req.watcher]; req != nil; req = req.next {
			select {
			case req.reply <- false:
			case <-w.tomb.Dying():
			}
		}
		delete(w.waiting, req.watcher)
		req.watcher.stopped = true
		w.store.DecReference(req.watcher.revno)
		return
	}
	// Add request to head of list.
	w.config.Logger.Tracef("add watcher %p request to waiting", req.watcher)
	req.next = w.waiting[req.watcher]
	w.waiting[req.watcher] = req
}

// respond responds to all outstanding requests that are satisfiable.
func (w *Worker) respond() {
	w.config.Logger.Tracef("start respond")
	defer w.config.Logger.Tracef("finish respond")
	for watcher, req := range w.waiting {
		revno := watcher.revno
		changes, latestRevno := w.store.ChangesSince(revno)
		w.config.Logger.Tracef("%d changes since %d for watcher %p", len(changes), revno, watcher)
		if len(changes) == 0 {
			if req.noChanges != nil {
				w.config.Logger.Tracef("sending down noChanges for watcher %p", watcher)
				select {
				case req.noChanges <- struct{}{}:
				case <-w.tomb.Dying():
					return
				}

				w.removeWaitingReq(watcher, req)
			}
			continue
		}

		req.changes = changes
		watcher.revno = latestRevno

		w.config.Logger.Tracef("sending changes down reply channel for watcher %p", watcher)
		select {
		case req.reply <- true:
		case <-w.tomb.Dying():
			return
		}

		w.removeWaitingReq(watcher, req)
		w.store.AddReference(revno)
	}
}

func (w *Worker) removeWaitingReq(watcher *Watcher, req *request) {
	if next := req.next; next == nil {
		// Last request for this watcher.
		delete(w.waiting, watcher)
	} else {
		w.waiting[watcher] = next
	}
}
