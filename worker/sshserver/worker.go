// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/state"
)

// SystemState holds methods on a state that has been retrieved
// via state.SystemState().
type SystemState interface {
	WatchControllerConfig() state.NotifyWatcher
}

// ServerWrapperWorkerConfig holds the configuration required by the server wrapper worker.
type ServerWrapperWorkerConfig struct {
	SystemState     SystemState
	NewServerWorker func(ServerWorkerConfig) (worker.Worker, error)
	Logger          Logger
}

// Validate validates the workers configuration is as expected.
func (c ServerWrapperWorkerConfig) Validate() error {
	if c.SystemState == nil {
		return errors.NotValidf("SystemState is required")
	}
	if c.NewServerWorker == nil {
		return errors.NotValidf("NewSSHServer is required")
	}
	if c.Logger == nil {
		return errors.NotValidf("Logger is required")
	}
	return nil
}

// serverWrapperWorker is a worker that runs an ssh server worker.
type serverWrapperWorker struct {
	// catacomb holds the catacomb responsible for running the server worker
	// and configuration watcher workers.
	catacomb catacomb.Catacomb

	// config holds the configuration required by the server wrapper worker.
	config ServerWrapperWorkerConfig
}

// NewServerWrapperWorker returns a new worker that runs an ssh server worker internally.
// This worker will listen for changes in the controller configuration and restart the
// server worker when the port or max concurrent connections changes.
func NewServerWrapperWorker(config ServerWrapperWorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &serverWrapperWorker{
		config: config,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// Kill implements worker.Worker.
func (ssw *serverWrapperWorker) Kill() {
	ssw.catacomb.Kill(nil)
}

// Wait implements worker.Worker.
func (ssw *serverWrapperWorker) Wait() error {
	return ssw.catacomb.Wait()
}

// loop is the main loop of the server wrapper worker. It starts the server worker
// and listens for changes in the controller configuration.
func (ssw *serverWrapperWorker) loop() error {
	srv, err := ssw.config.NewServerWorker(ServerWorkerConfig{
		Logger: ssw.config.Logger,
	})
	if err != nil {
		return errors.Trace(err)
	}

	if err := ssw.catacomb.Add(srv); err != nil {
		return errors.Trace(err)
	}

	controllerConfig := ssw.config.SystemState.WatchControllerConfig()
	if err := ssw.catacomb.Add(controllerConfig); err != nil {
		return errors.Trace(err)
	}

	changesChan := controllerConfig.Changes()
	for {
		select {
		case <-ssw.catacomb.Dying():
			return ssw.catacomb.ErrDying()
		case <-changesChan:
			// TODO(ale8k): Once the configuration PR is merged, get the max conns & port
			// from controller config. Get the HostKey from server info, and feed them through
			// to NewServerWorker.

			// Restart the server worker.
			srv.Kill()
			if err := srv.Wait(); err != nil {
				return errors.Trace(err)
			}

			// Start the server again.
			srv, err = ssw.config.NewServerWorker(ServerWorkerConfig{
				Logger: ssw.config.Logger,
			})
			if err != nil {
				return errors.Trace(err)
			}

			// Re-add it to the catacomb with the newly updated configuration.
			if err := ssw.catacomb.Add(srv); err != nil {
				return errors.Trace(err)
			}
		}
	}
}
