// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/api/meterstatus"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter/runner/context"
)

// connectedStatusHandler implements the NotifyWatchHandler interface.
type connectedStatusHandler struct {
	config ConnectedConfig

	code string
	info string
}

// ConnectedConfig contains all the dependencies required to create a new connected status worker.
type ConnectedConfig struct {
	Runner    HookRunner
	StateFile *StateFile
	Status    meterstatus.MeterStatusClient
}

// Validate validates the config structure and returns an error on failure.
func (c ConnectedConfig) Validate() error {
	if c.Runner == nil {
		return errors.New("hook runner not provided")
	}
	if c.StateFile == nil {
		return errors.New("state file not provided")
	}
	if c.Status == nil {
		return errors.New("meter status API client not provided")
	}
	return nil
}

// NewConnectedStatusWorker creates a new worker that monitors the meter status of the
// unit and runs the meter-status-changed hook appropriately.
func NewConnectedStatusWorker(cfg ConnectedConfig) (worker.Worker, error) {
	handler, err := NewConnectedStatusHandler(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: handler,
	})
}

// NewConnectedStatusHandler creates a new meter status handler for handling meter status
// changes as provided by the API.
func NewConnectedStatusHandler(cfg ConnectedConfig) (watcher.NotifyHandler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &connectedStatusHandler{
		config: cfg,
	}
	return w, nil
}

// SetUp is part of the worker.NotifyWatchHandler interface.
func (w *connectedStatusHandler) SetUp() (watcher.NotifyWatcher, error) {
	var err error
	w.code, w.info, _, err = w.config.StateFile.Read()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return w.config.Status.WatchMeterStatus()
}

// TearDown is part of the worker.NotifyWatchHandler interface.
func (w *connectedStatusHandler) TearDown() error {
	return nil
}

// Handle is part of the worker.NotifyWatchHandler interface.
func (w *connectedStatusHandler) Handle(abort <-chan struct{}) error {
	logger.Debugf("got meter status change signal from watcher")
	currentCode, currentInfo, err := w.config.Status.MeterStatus()
	if err != nil {
		return errors.Trace(err)
	}
	if currentCode == w.code && currentInfo == w.info {
		logger.Tracef("meter status (%q, %q) matches stored information (%q, %q), skipping", currentCode, currentInfo, w.code, w.info)
		return nil
	}
	w.applyStatus(currentCode, currentInfo, abort)
	w.code, w.info = currentCode, currentInfo
	err = w.config.StateFile.Write(w.code, w.info, nil)
	if err != nil {
		return errors.Annotate(err, "failed to record meter status worker state")
	}
	return nil
}

func (w *connectedStatusHandler) applyStatus(code, info string, abort <-chan struct{}) {
	logger.Tracef("applying meter status change: %q (%q)", code, info)
	err := w.config.Runner.RunHook(code, info, abort)
	cause := errors.Cause(err)
	switch {
	case context.IsMissingHookError(cause):
		logger.Infof("skipped %q hook (missing)", string(hooks.MeterStatusChanged))
	case err != nil:
		logger.Errorf("meter status worker encountered hook error: %v", err)
	}
}
