// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

const (
	// TODO(ale8k): Use generated hostkey from initialise()
	// As of right now, the generated host key is in mongo.
	// The initialisation logic needs migrating over to DQLite and then
	// a domain service should call the method to retrieve the generated
	// host key here. For now, we're hardcoding it it to stop the server bouncing.
	temporaryJumpHostKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz
c2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7VHoJY7LZ7yXzuWlSVYAAA
AIiZq0wRmatMEQAAAAtzc2gtZWQyNTUxOQAAACBT8UidoqUmpUFFCGEhZhHWGE7V
HoJY7LZ7yXzuWlSVYAAAAEBYRsJTytYJUidtOuv3s3tdjyDA+4TSdCz9+hFKjyqz
v1PxSJ2ipSalQUUIYSFmEdYYTtUegljstnvJfO5aVJVgAAAAAAECAwQF
-----END OPENSSH PRIVATE KEY-----
`
)

// ControllerConfigService is the interface that the worker uses to get the
// controller configuration.
type ControllerConfigService interface {
	// WatchControllerConfig returns a watcher that returns keys for any changes
	// to controller config.
	WatchControllerConfig() (watcher.StringsWatcher, error)
	// ControllerConfig returns the current controller configuration.
	ControllerConfig(context.Context) (controller.Config, error)
}

// ServerWrapperWorkerConfig holds the configuration required by the server wrapper worker.
type ServerWrapperWorkerConfig struct {
	ControllerConfigService ControllerConfigService
	NewServerWorker         func(ServerWorkerConfig) (worker.Worker, error)
	Logger                  logger.Logger
	NewSSHServerListener    func(net.Listener, time.Duration) net.Listener
}

// Validate validates the workers configuration is as expected.
func (c ServerWrapperWorkerConfig) Validate() error {
	if c.ControllerConfigService == nil {
		return errors.NotValidf("ControllerConfigService is required")
	}
	if c.NewServerWorker == nil {
		return errors.NotValidf("NewSSHServer is required")
	}
	if c.Logger == nil {
		return errors.NotValidf("Logger is required")
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

	// workerReporters holds the maps of worker reporters.
	workerReporters map[string]worker.Reporter

	// m holds the mutex to gate access to the workerReporters map
	m sync.RWMutex
}

// NewServerWrapperWorker returns a new worker that runs an ssh server worker internally.
// This worker will listen for changes in the controller configuration and restart the
// server worker when the port or max concurrent connections changes.
func NewServerWrapperWorker(config ServerWrapperWorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &serverWrapperWorker{
		config:          config,
		workerReporters: map[string]worker.Reporter{},
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

// Report calls the report methods in the workerReporters map to collect and return
// report maps to the inspection worker.
func (ssw *serverWrapperWorker) Report() map[string]any {
	ssw.m.RLock()
	defer ssw.m.RUnlock()
	reports := map[string]any{}
	for name, reporter := range ssw.workerReporters {
		reports[name] = reporter.Report()
	}
	return map[string]any{
		"workers": reports,
	}
}

// loop is the main loop of the server wrapper worker. It starts the server worker
// and listens for changes in the controller configuration.
func (ssw *serverWrapperWorker) loop() error {
	// Watch for changes then acquire the latest controller configuration
	// to avoid starting the server with stale config values.
	controllerConfigWatcher, err := ssw.config.ControllerConfigService.WatchControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}
	if err := ssw.catacomb.Add(controllerConfigWatcher); err != nil {
		return errors.Trace(err)
	}
	ssw.addWorkerReporter("controller-watcher", controllerConfigWatcher)

	ctx := ssw.catacomb.Context(context.Background())
	config, err := ssw.config.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	port := config.SSHServerPort()
	maxConns := config.SSHMaxConcurrentConnections()

	srv, err := ssw.config.NewServerWorker(ServerWorkerConfig{
		Logger:                   ssw.config.Logger,
		JumpHostKey:              temporaryJumpHostKey,
		Port:                     port,
		MaxConcurrentConnections: maxConns,
		NewSSHServerListener:     ssw.config.NewSSHServerListener,
	})
	ssw.addWorkerReporter("ssh-server", srv)
	if err != nil {
		return errors.Trace(err)
	}

	if err := ssw.catacomb.Add(srv); err != nil {
		return errors.Trace(err)
	}

	changesChan := controllerConfigWatcher.Changes()
	for {
		select {
		case <-ssw.catacomb.Dying():
			return ssw.catacomb.ErrDying()
		case <-changesChan:
			config, err := ssw.config.ControllerConfigService.ControllerConfig(ctx)
			if err != nil {
				return errors.Trace(err)
			}
			if port == config.SSHServerPort() && maxConns == config.SSHMaxConcurrentConnections() {
				ssw.config.Logger.Debugf(context.Background(), "controller configuration changed, but nothing changed for the ssh server.")
				continue
			}
			return errors.New("changes detected, stopping SSH server worker")
		}
	}
}

// addWorkerReporter adds the worker to the workerReporters map if the type assertion
// to worker.Reporter is successful.
func (ssw *serverWrapperWorker) addWorkerReporter(name string, w worker.Worker) {
	ssw.m.Lock()
	defer ssw.m.Unlock()
	reporter, ok := w.(worker.Reporter)
	if ok {
		ssw.workerReporters[name] = reporter
	}
}
