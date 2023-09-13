// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/api/agent/machineactions"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

var logger = loggo.GetLogger("juju.worker.machineactions")

// Facade defines the capabilities required by the worker from the API.
type Facade interface {
	WatchActionNotifications(agent names.MachineTag) (watcher.StringsWatcher, error)
	RunningActions(agent names.MachineTag) ([]params.ActionResult, error)

	Action(names.ActionTag) (*machineactions.Action, error)
	ActionBegin(names.ActionTag) error
	ActionFinish(tag names.ActionTag, status string, results map[string]any, message string) error
}

// WorkerConfig defines the worker's dependencies.
type WorkerConfig struct {
	Facade       Facade
	MachineTag   names.MachineTag
	MachineLock  machinelock.Lock
	HandleAction func(name string, params map[string]any) (results map[string]any, err error)
}

// Validate returns an error if the configuration is not complete.
func (c WorkerConfig) Validate() error {
	if c.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if c.MachineTag == (names.MachineTag{}) {
		return errors.NotValidf("unspecified MachineTag")
	}
	if c.HandleAction == nil {
		return errors.NotValidf("nil HandleAction")
	}
	return nil
}

// NewMachineActionsWorker returns a worker.Worker that watches for actions
// enqueued on this machine and tries to execute them.
func NewMachineActionsWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	swConfig := watcher.StringsConfig{
		Handler: &handler{config: config, limiter: make(chan struct{}, maxConcurrency)},
	}
	return watcher.NewStringsWorker(swConfig)
}

// At most 100 actions can run simultaneously.
const maxConcurrency = 100

// handler implements watcher.StringsHandler
type handler struct {
	config   WorkerConfig
	wait     sync.WaitGroup
	limiter  chan struct{}
	inflight int64
}

// SetUp is part of the watcher.StringsHandler interface.
func (h *handler) SetUp() (watcher.StringsWatcher, error) {
	actions, err := h.config.Facade.RunningActions(h.config.MachineTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We try to cancel any running action before starting up so actions don't linger around
	// We *should* really have only one action coming up here if the execution is serial but
	// this is best effort anyway.
	for _, action := range actions {
		tag, err := names.ParseActionTag(action.Action.Tag)
		if err != nil {
			logger.Infof("tried to cancel action %s but failed with error %v", action.Action.Tag, err)
			continue
		}
		err = h.config.Facade.ActionFinish(tag, params.ActionFailed, nil, "action cancelled")
		if err != nil {
			logger.Infof("tried to cancel action %s but failed with error %v", action.Action.Tag, err)
		}
	}
	return h.config.Facade.WatchActionNotifications(h.config.MachineTag)
}

// Handle is part of the watcher.StringsHandler interface.
// It should give us any actions currently enqueued for this machine.
// We try to execute every action before returning
func (h *handler) Handle(abort <-chan struct{}, actionsSlice []string) error {
	for _, actionId := range actionsSlice {
		ok := names.IsValidAction(actionId)
		if !ok {
			return errors.Errorf("got invalid action id %s", actionId)
		}

		actionTag := names.NewActionTag(actionId)
		action, err := h.config.Facade.Action(actionTag)
		if err != nil {
			// If there is an error attempting to get the action, then don't bounce
			// the worker. We can't remove the action notification directly, as that
			// requires the action to exist.
			// TODO (stickupkid) As a follow up, we should have a new method that
			// allows the removal of a action notification without an action present.
			logger.Infof("unable to retrieve action %s: %v", actionId, err)
			continue
		}

		// Acquire concurrency slot.
		select {
		case h.limiter <- struct{}{}:
		case <-abort:
			// The associated strings watcher has been aborted, so there isn't
			// anything we can do here but give up.
			logger.Debugf("action %q aborted waiting in queue", actionTag.ID)
			return nil
		}
		h.wait.Add(1)
		atomic.AddInt64(&h.inflight, 1)

		// Run the action.
		go h.runAction(actionTag, *action, abort)
	}
	return nil
}

// TearDown is part of the watcher.NotifyHandler interface.
func (h *handler) TearDown() error {
	// Wait for any running actions to finish.
	// TODO (stickupkid): This wait group could wait for ever if any of actions hang.
	// Instead we should be much more clever and wait for a limited time before marking
	// any outstanding actions as failed.
	inflight := atomic.LoadInt64(&h.inflight)
	if inflight > 0 {
		logger.Infof("Waiting for %d running actions...", inflight)
	}
	h.wait.Wait()
	if inflight > 0 {
		logger.Infof("Done waiting for actions.")
	}
	return nil
}

func (h *handler) runAction(actionTag names.ActionTag, action machineactions.Action, abort <-chan struct{}) {
	var results map[string]any
	var actionErr error
	defer func() {
		// The result returned from handling the action is sent through using ActionFinish.
		var finishErr error
		if actionErr != nil {
			finishErr = h.config.Facade.ActionFinish(actionTag, params.ActionFailed, nil, actionErr.Error())
		} else {
			finishErr = h.config.Facade.ActionFinish(actionTag, params.ActionCompleted, results, "")
		}
		if finishErr != nil &&
			!params.IsCodeAlreadyExists(finishErr) &&
			!params.IsCodeNotFoundOrCodeUnauthorized(finishErr) {
			logger.Errorf("could not finish action %s: %v", action.Name(), finishErr)
		}

		// Release concurrency slot.
		select {
		case <-h.limiter:
		case <-abort:
			logger.Debugf("action %q aborted waiting to enqueue", actionTag)
		}
		atomic.AddInt64(&h.inflight, -1)
		h.wait.Done()
	}()

	if !action.Parallel() || action.ExecutionGroup() != "" {
		group := "exec-command"
		worker := "machine exec command runner"
		if g := action.ExecutionGroup(); g != "" {
			group = fmt.Sprintf("%s-%s", group, g)
			worker = fmt.Sprintf("%s (exec group=%s)", worker, g)
		}
		spec := machinelock.Spec{
			Cancel:  abort,
			Worker:  worker,
			Comment: fmt.Sprintf("action %s", action.ID()),
			Group:   group,
		}
		releaser, err := h.config.MachineLock.Acquire(spec)
		if err != nil {
			actionErr = errors.Annotatef(err, "could not acquire machine execution lock for exec action %s", action.Name())
			return
		}
		defer releaser()
	}

	if err := h.config.Facade.ActionBegin(actionTag); err != nil {
		actionErr = errors.Annotatef(err, "could not begin action %s", action.Name())
		return
	}
	results, actionErr = h.config.HandleAction(action.Name(), action.Params())
}
