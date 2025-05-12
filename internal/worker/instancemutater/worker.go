// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"context"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/instancemutater"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
)

type InstanceMutaterAPI interface {
	WatchModelMachines(ctx context.Context) (watcher.StringsWatcher, error)
	Machine(ctx context.Context, tag names.MachineTag) (instancemutater.MutaterMachine, error)
}

// Config represents the configuration required to run a new instance machineApi
// worker.
type Config struct {
	Facade InstanceMutaterAPI

	// Logger is the Logger for this worker.
	Logger logger.Logger

	Broker environs.LXDProfiler

	AgentConfig agent.Config

	// Tag is the current MutaterMachine tag
	Tag names.Tag

	// GetMachineWatcher allows the worker to watch different "machines"
	// depending on whether this work is running with an environ broker
	// or a container broker.
	GetMachineWatcher func(ctx context.Context) (watcher.StringsWatcher, error)

	// GetRequiredLXDProfiles provides a slice of strings representing the
	// lxd profiles to be included on every LXD machine used given the
	// current model name.
	GetRequiredLXDProfiles RequiredLXDProfilesFunc

	// GetRequiredContext provides a way to override the given context
	// Note: the following is required for testing purposes when we have an
	// error case and we want to know when it's valid to kill/clean the worker.
	GetRequiredContext RequiredMutaterContextFunc
}

type RequiredLXDProfilesFunc func(string) []string

type RequiredMutaterContextFunc func(MutaterContext) MutaterContext

// Validate checks for missing values from the configuration and checks that
// they conform to a given type.
func (config Config) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Broker == nil {
		return errors.NotValidf("nil Broker")
	}
	if config.AgentConfig == nil {
		return errors.NotValidf("nil AgentConfig")
	}
	if config.Tag == nil {
		return errors.NotValidf("nil Tag")
	}
	if _, ok := config.Tag.(names.MachineTag); !ok {
		if config.Tag.Kind() != names.ControllerAgentTagKind {
			// On K8s controllers, the controller agent has a ControllerAgentTagKind not a MachineKind
			// However, we shouldn't be running the InstanceMutater worker to track the state on the Controller
			// machine anyway. This is a hack for bug #1866623
			return errors.NotValidf("Tag of kind %v", config.Tag.Kind())
		}
		config.Logger.Debugf(context.Background(), "asked to start an instance mutator with Tag of kind %q", config.Tag.Kind())
	}
	if config.GetMachineWatcher == nil {
		return errors.NotValidf("nil GetMachineWatcher")
	}
	if config.GetRequiredLXDProfiles == nil {
		return errors.NotValidf("nil GetRequiredLXDProfiles")
	}
	if config.GetRequiredContext == nil {
		return errors.NotValidf("nil GetRequiredContext")
	}
	return nil
}

// NewEnvironWorker returns a worker that keeps track of
// the machines in the state and polls their instance
// for addition or removal changes.
func NewEnvironWorker(ctx context.Context, config Config) (worker.Worker, error) {
	config.GetMachineWatcher = config.Facade.WatchModelMachines
	config.GetRequiredLXDProfiles = func(modelName string) []string {
		return []string{"default", "juju-" + modelName}
	}
	config.GetRequiredContext = func(ctx MutaterContext) MutaterContext {
		return ctx
	}
	return newWorker(ctx, config)
}

// NewContainerWorker returns a worker that keeps track of
// the containers in the state for this machine agent and
// polls their instance for addition or removal changes.
func NewContainerWorker(ctx context.Context, config Config) (worker.Worker, error) {
	if _, ok := config.Tag.(names.MachineTag); !ok {
		config.Logger.Warningf(ctx, "cannot start a ContainerWorker on a %q, not restarting", config.Tag.Kind())
		return nil, dependency.ErrUninstall
	}
	m, err := config.Facade.Machine(ctx, config.Tag.(names.MachineTag))
	if err != nil {
		return nil, errors.Trace(err)
	}
	config.GetRequiredLXDProfiles = func(_ string) []string { return []string{"default"} }
	config.GetMachineWatcher = m.WatchContainers
	config.GetRequiredContext = func(ctx MutaterContext) MutaterContext {
		return ctx
	}
	return newWorker(ctx, config)
}

func newWorker(ctx context.Context, config Config) (*mutaterWorker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	watcher, err := config.GetMachineWatcher(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w := &mutaterWorker{
		logger:                     config.Logger,
		facade:                     config.Facade,
		broker:                     config.Broker,
		machineWatcher:             watcher,
		getRequiredLXDProfilesFunc: config.GetRequiredLXDProfiles,
		getRequiredContextFunc:     config.GetRequiredContext,
	}
	// getRequiredContextFunc returns a MutaterContext, this is for overriding
	// during testing.
	err = catacomb.Invoke(catacomb.Plan{
		Name: "instance-mutater",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{watcher},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type mutaterWorker struct {
	catacomb catacomb.Catacomb

	logger                     logger.Logger
	broker                     environs.LXDProfiler
	facade                     InstanceMutaterAPI
	machineWatcher             watcher.StringsWatcher
	getRequiredLXDProfilesFunc RequiredLXDProfilesFunc
	getRequiredContextFunc     RequiredMutaterContextFunc
}

func (w *mutaterWorker) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	var wg sync.WaitGroup
	defer wg.Wait()
	m := &mutater{
		context:     w.getRequiredContextFunc(w),
		logger:      w.logger,
		wg:          &wg,
		machines:    make(map[names.MachineTag]chan struct{}),
		machineDead: make(chan instancemutater.MutaterMachine),
	}
	for {
		select {
		case <-m.context.dying():
			return m.context.errDying()
		case ids, ok := <-w.machineWatcher.Changes():
			if !ok {
				return errors.New("machines watcher closed")
			}
			tags := make([]names.MachineTag, len(ids))
			for i := range ids {
				tags[i] = names.NewMachineTag(ids[i])
			}
			if err := m.startMachines(ctx, tags); err != nil {
				return err
			}
		case d := <-m.machineDead:
			delete(m.machines, d.Tag())
		}
	}
}

// Kill implements worker.Worker.Kill.
func (w *mutaterWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (w *mutaterWorker) Wait() error {
	return w.catacomb.Wait()
}

// Stop stops the mutaterWorker and returns any
// error it encountered when running.
func (w *mutaterWorker) Stop() error {
	w.Kill()
	return w.Wait()
}

// newMachineContext is part of the mutaterContext interface.
func (w *mutaterWorker) newMachineContext() MachineContext {
	return w.getRequiredContextFunc(w)
}

// getMachine is part of the MachineContext interface.
func (w *mutaterWorker) getMachine(ctx context.Context, tag names.MachineTag) (instancemutater.MutaterMachine, error) {
	m, err := w.facade.Machine(ctx, tag)
	return m, err
}

// getBroker is part of the MachineContext interface.
func (w *mutaterWorker) getBroker() environs.LXDProfiler {
	return w.broker
}

// getRequiredLXDProfiles part of the MachineContext interface.
func (w *mutaterWorker) getRequiredLXDProfiles(modelName string) []string {
	return w.getRequiredLXDProfilesFunc(modelName)
}

// KillWithError is part of the lifetimeContext interface.
func (w *mutaterWorker) KillWithError(err error) {
	w.catacomb.Kill(err)
}

// dying is part of the lifetimeContext interface.
func (w *mutaterWorker) dying() <-chan struct{} {
	return w.catacomb.Dying()
}

// errDying is part of the lifetimeContext interface.
func (w *mutaterWorker) errDying() error {
	return w.catacomb.ErrDying()
}

// add is part of the lifetimeContext interface.
func (w *mutaterWorker) add(new worker.Worker) error {
	return w.catacomb.Add(new)
}

func (w *mutaterWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
