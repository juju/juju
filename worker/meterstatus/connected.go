// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/api/meterstatus"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter/runner/context"
)

var (
	newSocketListener = func(path string, handler spool.ConnectionHandler) (stopper, error) {
		return spool.NewSocketListener(path, handler)
	}
)

type stopper interface {
	Stop()
}

// connectedStatusHandler implements the NotifyWatchHandler interface.
type connectedStatusHandler struct {
	config ConnectedConfig

	code string
	info string

	stopListener func()
}

// ConnectedConfig contains all the dependencies required to create a new connected status worker.
type ConnectedConfig struct {
	Runner     HookRunner
	StateFile  *StateFile
	Status     meterstatus.MeterStatusClient
	SocketPath string
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
	if cfg.SocketPath != "" {
		statusHandler := handler.(*connectedStatusHandler)
		listener, err := newSocketListener(cfg.SocketPath, &socketHandler{handler: statusHandler})
		if err != nil {
			return nil, errors.Trace(err)
		}
		statusHandler.stopListener = listener.Stop
	}
	return worker.NewNotifyWorker(handler), nil
}

// NewConnectedStatusHandler creates a new meter status handler for handling meter status
// changes as provided by the API.
func NewConnectedStatusHandler(cfg ConnectedConfig) (worker.NotifyWatchHandler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &connectedStatusHandler{
		config: cfg,
	}
	return w, nil
}

// SetUp is part of the worker.NotifyWatchHandler interface.
func (w *connectedStatusHandler) SetUp() (apiwatcher.NotifyWatcher, error) {
	var err error
	w.code, w.info, _, err = w.config.StateFile.Read()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return w.config.Status.WatchMeterStatus()
}

// TearDown is part of the worker.NotifyWatchHandler interface.
func (w *connectedStatusHandler) TearDown() error {
	if w.stopListener != nil {
		w.stopListener()
	}
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

type meterStatus struct {
	Code string `json:"code"`
	Info string `json:"info"`
}

type socketHandler struct {
	handler *connectedStatusHandler
}

// Handle implements the spool.ConnectionHandler interface.
func (h *socketHandler) Handle(c net.Conn) (err error) {
	defer func() {
		io.Copy(ioutil.Discard, c)
		if err != nil {
			fmt.Fprintf(c, "%v\n", err)
		} else {
			fmt.Fprintf(c, "ok\n")
		}
		c.Close()
	}()
	err = c.SetDeadline(time.Now().Add(spool.DefaultTimeout))
	if err != nil {
		return errors.Annotate(err, "failed to set the deadline")
	}
	var status meterStatus
	decoder := json.NewDecoder(c)
	err = decoder.Decode(&status)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Errorf("XXX SETTING STATUS %#v", status)
	h.handler.applyStatus(status.Code, status.Info, nil)
	return nil
}
