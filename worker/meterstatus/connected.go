// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/api/agent/meterstatus"
	"github.com/juju/juju/core/watcher"
)

// connectedStatusHandler implements the NotifyWatchHandler interface.
type connectedStatusHandler struct {
	config ConnectedConfig

	st *State
}

// ConnectedConfig contains all the dependencies required to create a new connected status worker.
type ConnectedConfig struct {
	Runner          HookRunner
	Status          meterstatus.MeterStatusClient
	StateReadWriter StateReadWriter
	Logger          Logger
}

// Validate validates the config structure and returns an error on failure.
func (c ConnectedConfig) Validate() error {
	if c.Runner == nil {
		return errors.NotValidf("missing Runner")
	}
	if c.StateReadWriter == nil {
		return errors.NotValidf("missing StateReadWriter")
	}
	if c.Status == nil {
		return errors.NotValidf("missing Status")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
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
func (w *connectedStatusHandler) SetUp(_ context.Context) (watcher.NotifyWatcher, error) {
	var err error
	if w.st, err = w.config.StateReadWriter.Read(); err != nil {
		if !errors.Is(err, errors.NotFound) {
			return nil, errors.Trace(err)
		}

		// Start with blank state
		w.st = new(State)
	}

	return w.config.Status.WatchMeterStatus()
}

// TearDown is part of the worker.NotifyWatchHandler interface.
func (w *connectedStatusHandler) TearDown() error {
	return nil
}

// Handle is part of the worker.NotifyWatchHandler interface.
func (w *connectedStatusHandler) Handle(ctx context.Context) error {
	w.config.Logger.Debugf("got meter status change signal from watcher")
	currentCode, currentInfo, err := w.config.Status.MeterStatus()
	if err != nil {
		return errors.Trace(err)
	}
	if currentCode == w.st.Code && currentInfo == w.st.Info {
		w.config.Logger.Tracef("meter status (%q, %q) matches stored information (%q, %q), skipping", currentCode, currentInfo, w.st.Code, w.st.Info)
		return nil
	}
	w.applyStatus(currentCode, currentInfo, ctx.Done())
	w.st.Code, w.st.Info = currentCode, currentInfo
	if err = w.config.StateReadWriter.Write(w.st); err != nil {
		return errors.Annotate(err, "failed to record meter status worker state")
	}
	return nil
}

func (w *connectedStatusHandler) applyStatus(code, info string, abort <-chan struct{}) {
	w.config.Logger.Tracef("applying meter status change: %q (%q)", code, info)
	w.config.Runner.RunHook(code, info, abort)
}
