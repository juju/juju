// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"

	"github.com/juju/juju/api/agent/machineactions"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/watcher"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

var logger = internallogger.GetLogger("juju.worker.machineactions")

// Facade defines the capabilities required by the worker from the API.
type Facade interface {
	WatchActionNotifications(ctx context.Context, agent names.MachineTag) (watcher.StringsWatcher, error)
	RunningActions(ctx context.Context, agent names.MachineTag) ([]params.ActionResult, error)

	Action(context.Context, names.ActionTag) (*machineactions.Action, error)
	ActionBegin(context.Context, names.ActionTag) error
	ActionFinish(ctx context.Context, tag names.ActionTag, status string, results map[string]any, message string) error
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
		Handler: newHandler(config),
	}
	return watcher.NewStringsWorker(swConfig)
}

// At most 100 actions can run simultaneously.
const maxConcurrency = 100

var tearDownWait = 30 * time.Second

// handler implements watcher.StringsHandler
type handler struct {
	config  WorkerConfig
	limiter chan struct{}

	mu       sync.Mutex
	inflight int
	idle     chan struct{}
}

func newHandler(config WorkerConfig) *handler {
	idle := make(chan struct{})
	close(idle)
	return &handler{
		config:  config,
		limiter: make(chan struct{}, maxConcurrency),
		idle:    idle,
	}
}

// SetUp is part of the watcher.StringsHandler interface.
func (h *handler) SetUp(ctx context.Context) (watcher.StringsWatcher, error) {
	actions, err := h.config.Facade.RunningActions(ctx, h.config.MachineTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We try to cancel any running action before starting up so actions don't linger around
	// We *should* really have only one action coming up here if the execution is serial but
	// this is best effort anyway.
	for _, action := range actions {
		tag, err := names.ParseActionTag(action.Action.Tag)
		if err != nil {
			logger.Infof(ctx, "tried to cancel action %s but failed with error %v", action.Action.Tag, err)
			continue
		}
		err = h.config.Facade.ActionFinish(ctx, tag, params.ActionFailed, nil, "action cancelled")
		if err != nil {
			logger.Infof(ctx, "tried to cancel action %s but failed with error %v", action.Action.Tag, err)
		}
	}
	return h.config.Facade.WatchActionNotifications(ctx, h.config.MachineTag)
}

// Handle is part of the watcher.StringsHandler interface.
// It should give us any actions currently enqueued for this machine.
// We try to execute every action before returning
func (h *handler) Handle(ctx context.Context, actionsSlice []string) error {
	for _, actionId := range actionsSlice {
		ok := names.IsValidAction(actionId)
		if !ok {
			return errors.Errorf("got invalid action id %s", actionId)
		}

		actionTag := names.NewActionTag(actionId)
		action, err := h.config.Facade.Action(ctx, actionTag)
		if err != nil {
			// If there is an error attempting to get the action, then don't bounce
			// the worker. We can't remove the action notification directly, as that
			// requires the action to exist.
			// TODO (stickupkid) As a follow up, we should have a new method that
			// allows the removal of a action notification without an action present.
			logger.Infof(ctx, "unable to retrieve action %s: %v", actionId, err)
			continue
		}

		// Acquire concurrency slot.
		select {
		case h.limiter <- struct{}{}:
		case <-ctx.Done():
			// The associated strings watcher has been aborted, so there isn't
			// anything we can do here but give up.
			logger.Debugf(ctx, "action %q aborted waiting in queue", actionTag.ID)
			return nil
		}
		h.startAction()

		// Run the action.
		go h.runAction(ctx, actionTag, *action)
	}
	return nil
}

// TearDown is part of the watcher.NotifyHandler interface.
func (h *handler) TearDown() error {
	// Wait for any running actions to finish.
	inflight, idle := h.waitState()
	if inflight > 0 {
		logger.Infof(context.Background(), "Waiting for %d running actions...", inflight)
	}

	if inflight == 0 {
		return nil
	}

	select {
	case <-idle:
		logger.Infof(context.Background(), "Done waiting for actions.")
	case <-time.After(tearDownWait):
		logger.Warningf(
			context.Background(),
			"timed out waiting for %d running actions, continuing shutdown",
			inflight,
		)
	}
	return nil
}

func (h *handler) runAction(ctx context.Context, actionTag names.ActionTag, action machineactions.Action) {
	var results map[string]any
	var actionErr error
	defer func() {
		// The result returned from handling the action is sent through using ActionFinish.
		var finishErr error
		if actionErr != nil {
			finishErr = h.config.Facade.ActionFinish(ctx, actionTag, params.ActionFailed, nil, actionErr.Error())
		} else {
			finishErr = h.config.Facade.ActionFinish(ctx, actionTag, params.ActionCompleted, results, "")
		}
		if finishErr != nil &&
			!params.IsCodeAlreadyExists(finishErr) &&
			!params.IsCodeNotFoundOrCodeUnauthorized(finishErr) {
			logger.Errorf(ctx, "could not finish action %s: %v", action.Name(), finishErr)
		}

		// Release concurrency slot.
		select {
		case <-h.limiter:
		case <-ctx.Done():
			logger.Debugf(ctx, "action %q aborted waiting to enqueue", actionTag)
		}
		h.finishAction()
	}()

	if !action.Parallel() || action.ExecutionGroup() != "" {
		group := "exec-command"
		worker := "machine exec command runner"
		if g := action.ExecutionGroup(); g != "" {
			group = fmt.Sprintf("%s-%s", group, g)
			worker = fmt.Sprintf("%s (exec group=%s)", worker, g)
		}
		spec := machinelock.Spec{
			Cancel:  ctx.Done(),
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

	if err := h.config.Facade.ActionBegin(ctx, actionTag); err != nil {
		actionErr = errors.Annotatef(err, "could not begin action %s", action.Name())
		return
	}
	results, actionErr = h.config.HandleAction(action.Name(), action.Params())
}

func (h *handler) startAction() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.inflight == 0 {
		h.idle = make(chan struct{})
	}
	h.inflight++
}

func (h *handler) finishAction() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.inflight == 0 {
		return
	}
	h.inflight--
	if h.inflight == 0 {
		close(h.idle)
	}
}

func (h *handler) waitState() (int, <-chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.inflight, h.idle
}
