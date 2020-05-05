// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/api/machineactions"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
)

var logger = loggo.GetLogger("juju.worker.machineactions")

// Facade defines the capabilities required by the worker from the API.
type Facade interface {
	WatchActionNotifications(agent names.MachineTag) (watcher.StringsWatcher, error)
	RunningActions(agent names.MachineTag) ([]params.ActionResult, error)

	Action(names.ActionTag) (*machineactions.Action, error)
	ActionBegin(names.ActionTag) error
	ActionFinish(tag names.ActionTag, status string, results map[string]interface{}, message string) error
}

// WorkerConfig defines the worker's dependencies.
type WorkerConfig struct {
	Facade       Facade
	MachineTag   names.MachineTag
	HandleAction func(name string, params map[string]interface{}) (results map[string]interface{}, err error)
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
		Handler: &handler{config},
	}
	return watcher.NewStringsWorker(swConfig)
}

// handler implements watcher.StringsHandler
type handler struct {
	config WorkerConfig
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
func (h *handler) Handle(_ <-chan struct{}, actionsSlice []string) error {
	for _, actionId := range actionsSlice {
		ok := names.IsValidAction(actionId)
		if !ok {
			return errors.Errorf("got invalid action id %s", actionId)
		}

		actionTag := names.NewActionTag(actionId)
		action, err := h.config.Facade.Action(actionTag)
		if err != nil {
			return errors.Annotatef(err, "could not retrieve action %s", actionId)
		}

		err = h.config.Facade.ActionBegin(actionTag)
		if err != nil {
			return errors.Annotatef(err, "could not begin action %s", action.Name())
		}

		// We try to handle the action. The result returned from handling the action is
		// sent through using ActionFinish. We only stop the loop if ActionFinish fails.
		var finishErr error
		results, err := h.config.HandleAction(action.Name(), action.Params())
		if err != nil {
			finishErr = h.config.Facade.ActionFinish(actionTag, params.ActionFailed, nil, err.Error())
		} else {
			finishErr = h.config.Facade.ActionFinish(actionTag, params.ActionCompleted, results, "")
		}
		if finishErr != nil {
			return errors.Trace(finishErr)
		}
	}
	return nil
}

// TearDown is part of the watcher.NotifyHandler interface.
func (h *handler) TearDown() error {
	// Nothing to cleanup, only state is the watcher
	return nil
}
