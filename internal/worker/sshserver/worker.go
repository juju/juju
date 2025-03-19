// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
)

// ServerWrapperWorkerConfig holds the configuration required by the server wrapper worker.
type ServerWrapperWorkerConfig struct {
	NewServerWorker      func(ServerWorkerConfig) (worker.Worker, error)
	Logger               Logger
	FacadeClient         FacadeClient
	NewSSHServerListener func(net.Listener, time.Duration) net.Listener
}

// Validate validates the workers configuration is as expected.
func (c ServerWrapperWorkerConfig) Validate() error {
	if c.NewServerWorker == nil {
		return errors.NotValidf("NewSSHServer is required")
	}
	if c.Logger == nil {
		return errors.NotValidf("Logger is required")
	}
	if c.FacadeClient == nil {
		return errors.NotValidf("FacadeClient is required")
	}
	if c.NewSSHServerListener == nil {
		return errors.NotValidf("NewSSHServerListener is required")
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

func (ssw *serverWrapperWorker) getLatestControllerConfig() (port, maxConns int, err error) {
	ctrlCfg, err := ssw.config.FacadeClient.ControllerConfig()
	if err != nil {
		return port, maxConns, errors.Trace(err)
	}

	return ctrlCfg.SSHServerPort(), ctrlCfg.SSHMaxConcurrentConnections(), nil
}

// loop is the main loop of the server wrapper worker. It starts the server worker
// and listens for changes in the controller configuration.
func (ssw *serverWrapperWorker) loop() error {
	jumpHostKey, err := ssw.config.FacadeClient.SSHServerHostKey()
	if err != nil {
		return errors.Trace(err)
	}
	if jumpHostKey == "" {
		return errors.New("jump host key is empty")
	}

	controllerConfigWatcher, err := ssw.config.FacadeClient.WatchControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}

	if err := ssw.catacomb.Add(controllerConfigWatcher); err != nil {
		return errors.Trace(err)
	}

	port, maxConns, err := ssw.getLatestControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}

	srv, err := ssw.config.NewServerWorker(ServerWorkerConfig{
		Logger:                   ssw.config.Logger,
		JumpHostKey:              jumpHostKey,
		Port:                     port,
		MaxConcurrentConnections: maxConns,
		NewSSHServerListener:     ssw.config.NewSSHServerListener,
	})
	if err != nil {
		return errors.Trace(err)
	}

	if err := ssw.catacomb.Add(srv); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-ssw.catacomb.Dying():
			return ssw.catacomb.ErrDying()
		case <-controllerConfigWatcher.Changes():
			newPort, newMaxConnections, err := ssw.getLatestControllerConfig()
			if err != nil {
				return errors.Trace(err)
			}
			if port == newPort && newMaxConnections == maxConns {
				ssw.config.Logger.Debugf("controller configuration changed, but nothing changed for the ssh server.")
				continue
			}
			return errors.New("changes detected, stopping SSH server worker")
		}
	}
}
