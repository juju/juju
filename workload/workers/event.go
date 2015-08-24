// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

var logger = loggo.GetLogger("juju.workload.workers")

const (
	resEvents    = "events"
	resRunner    = "runner"
	resAPIClient = "apiclient"
)

// TODO(ericsnow) Switch handlers to...manifests? workers?

// TODO(ericsnow) Implement ManifoldConfig and Manifold() here?

// Events contains the events queued up for EventHandlers.
type Events struct {
	events chan []workload.Event
	tomb   tomb.Tomb
}

// NewEvents returns a new Events.
func NewEvents() *Events {
	e := &Events{
		events: make(chan []workload.Event),
	}
	go func() {
		select {
		case <-e.tomb.Dying():
			e.stop()
		}
	}()
	return e
}

func (e *Events) stop() {
	if e.events != nil {
		close(e.events)
	}
	e.tomb.Done()
}

// Close closes the Events.
func (e *Events) Close() error {
	e.tomb.Kill(nil)
	e.tomb.Wait()
	return nil
}

// AddEvents adds events to the list of events to be handled.
func (e *Events) AddEvents(events ...workload.Event) error {
	if len(events) == 0 {
		return nil
	}
	select {
	case <-e.tomb.Dying():
		return errors.Trace(workload.EventsClosed)
	case e.events <- events:
	}
	return nil
}

// EventHandlers orchestrates handling of events on workloads.
type EventHandlers struct {
	data eventHandlersData
}

// NewEventHandlers creates a new EventHandlers.
func NewEventHandlers() *EventHandlers {
	logger.Debugf("new event handler created")
	eh := EventHandlers{
		data: newEventHandlersData(nil),
	}
	return &eh
}

// Reset resets the event handlers.
func (eh *EventHandlers) Reset(apiClient context.APIClient) error {
	if err := eh.data.Close(); err != nil {
		return errors.Trace(err)
	}
	eh.data = newEventHandlersData(apiClient)
	return nil
}

// Close cleans up the handler's resources.
func (eh *EventHandlers) Close() error {
	if err := eh.data.Close(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// RegisterHandler adds a handler to the list of handlers used when new
// events are processed.
func (eh *EventHandlers) RegisterHandler(handler func([]workload.Event, context.APIClient, Runner) error) {
	logger.Debugf("registering handler: %#v", handler)
	eh.data.Handlers = append(eh.data.Handlers, handler)
}

// AddEvents adds events to the list of events to be handled.
func (eh *EventHandlers) AddEvents(events ...workload.Event) error {
	if err := eh.data.Events.AddEvents(events...); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (eh *EventHandlers) handleEvents(events []workload.Event) error {
	apiClient := eh.data.APIClient
	runner := eh.data.Engine.Runner
	logger.Debugf("handling %d events", len(events))
	for _, handleEvents := range eh.data.Handlers {
		if err := handleEvents(events, apiClient, runner); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (eh *EventHandlers) loop(stopCh <-chan struct{}) error {
	done := false
	for !done {
		select {
		case <-stopCh:
			done = true
		case events, alive := <-eh.data.Events.events:
			if !alive {
				done = true
			} else if err := eh.handleEvents(events); err != nil {
				return errors.Trace(err)
			}
		}
	}
	// TODO(ericsnow) Call eh.Close() here?
	return nil
}

// StartEngine creates a new dependency engine and starts it.
func (eh *EventHandlers) StartEngine() (worker.Worker, error) {
	if eh.data.Engine != nil {
		return nil, errors.Errorf("engine already started")
	}

	engine, err := newEventHandlersEngine(eh)
	if err != nil {
		return nil, errors.Trace(err)
	}
	eh.data.Engine = engine

	return engine.Engine, nil
}

type eventHandlersEngine struct {
	Handlers *EventHandlers
	Engine   dependency.Engine
	// Runner is used in lieu of Engine due to support for stopping.
	Runner worker.Runner
}

func newEventHandlersEngine(handlers *EventHandlers) (*eventHandlersEngine, error) {
	ehe := &eventHandlersEngine{
		Handlers: handlers,
	}

	engine, err := newEngine()
	if err != nil {
		return nil, errors.Trace(err)
	}

	manifolds := ehe.manifolds()
	// TODO(ericsnow) Move the following to a helper in worker/dependency or worker/util?
	if err := dependency.Install(engine, manifolds); err != nil {
		if err := worker.Stop(engine); err != nil {
			logger.Errorf("while stopping engine with bad manifolds: %v", err)
		}
		return nil, errors.Trace(err)
	}

	ehe.Engine = engine
	ehe.Runner = newRunner()
	return ehe, nil
}

func (ehe *eventHandlersEngine) Close() error {
	ehe.Runner.Kill()
	if err := ehe.Runner.Wait(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// manifolds returns the set of manifolds that should be added for the unit.
func (ehe *eventHandlersEngine) manifolds() dependency.Manifolds {
	manifolds := dependency.Manifolds{
		resEvents:    ehe.eventsManifold(),
		resAPIClient: ehe.apiManifold(),
	}
	if ehe.Runner != nil {
		manifolds[resRunner] = ehe.runnerManifold()
	}
	return manifolds
}

func (ehe *eventHandlersEngine) runnerManifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Start: func(dependency.GetResourceFunc) (worker.Worker, error) {
			return util.NewValueWorker(ehe.Runner)
		},
		Output: util.ValueWorkerOutput,
	}
}

func (ehe *eventHandlersEngine) apiManifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Start: func(dependency.GetResourceFunc) (worker.Worker, error) {
			return util.NewValueWorker(ehe.Handlers.data.APIClient)
		},
		Output: util.ValueWorkerOutput,
	}
}

func (ehe *eventHandlersEngine) eventsManifold() dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Start: func(dependency.GetResourceFunc) (worker.Worker, error) {
			// Pull all existing from State (via API) and add an event for each.
			events, err := InitialEvents(ehe.Handlers.data.APIClient)
			if err != nil {
				return nil, errors.Trace(err)
			}

			logger.Debugf("starting new worker")
			w := worker.NewSimpleWorker(ehe.Handlers.loop)
			// These must be added *after* the worker is started.
			ehe.Handlers.data.Events.AddEvents(events...)
			return w, nil
		},
	}
}

// InitialEvents returns the events that correspond to the current Juju state.
func InitialEvents(apiClient context.APIClient) ([]workload.Event, error) {
	// TODO(ericsnow) Use an API call that returns all of them at once,
	// rather than using a Get call for each?

	hctx, err := context.NewContextAPI(apiClient, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ids, err := hctx.List()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var events []workload.Event
	for _, id := range ids {
		info, err := hctx.Get(id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// TODO(wwitzel3) (Upgrade/Restart broken) during a restart of the
		// worker, the Plugin loses its absPath for the executable.
		plugin, err := hctx.Plugin(info)
		if err != nil {
			return nil, errors.Trace(err)
		}

		events = append(events, workload.Event{
			Kind:     workload.EventKindTracked,
			ID:       info.ID(),
			Plugin:   plugin,
			PluginID: info.Details.ID,
		})
	}
	return events, nil
}

type eventHandlersData struct {
	Events   *Events
	Handlers []func([]workload.Event, context.APIClient, Runner) error

	APIClient context.APIClient
	Engine    *eventHandlersEngine
}

func newEventHandlersData(apiClient context.APIClient) eventHandlersData {
	data := eventHandlersData{
		Events:    NewEvents(),
		APIClient: apiClient,
	}
	return data
}

// Close cleans up the handler's resources.
func (data *eventHandlersData) Close() error {
	if err := data.Events.Close(); err != nil {
		// TODO(ericsnow) Stop the engine anyway?
		return errors.Trace(err)
	}
	if data.Engine != nil {
		if err := data.Engine.Close(); err != nil {
			return errors.Trace(err)
		}
		// TODO(ericsnow) Set data.Engine to nil?
	}
	return nil
}
