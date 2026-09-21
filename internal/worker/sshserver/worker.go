// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/core/watcher"
	internalworker "github.com/juju/juju/internal/worker"
)

// serverWorkerName is the runner ID and report key of the inner server worker.
const serverWorkerName = "ssh-server"

// ControllerConfigService is the interface that the worker uses to get the
// controller configuration.
type ControllerConfigService interface {
	// WatchControllerConfig returns a watcher that returns keys for any changes
	// to controller config.
	WatchControllerConfig(context.Context) (watcher.StringsWatcher, error)
	// ControllerConfig returns the current controller configuration.
	ControllerConfig(context.Context) (controller.Config, error)
}

// SSHService resolves controller host keys, user public keys, and terminating
// host keys for routed destinations.
type SSHService interface {
	// VirtualHostKey returns the terminating host key for a routed destination.
	VirtualHostKey(context.Context, virtualhostname.Info) (string, error)
	// ResolveK8sExecInfo resolves Kubernetes execution information for a routed
	// destination.
	ResolveK8sExecInfo(context.Context, virtualhostname.Info) (namespace, podName string, err error)
	// MachineForDestination resolves the machine for a routed destination.
	MachineForDestination(context.Context, virtualhostname.Info) (coremachine.Name, error)
	// SSHServerHostKey returns the controller's SSH server host key.
	SSHServerHostKey(context.Context) (string, error)
	// GetSSHServerPort returns the port the controller SSH jump server listens
	// on. The controller charm owns this value and pushes changes via the
	// control socket.
	GetSSHServerPort(context.Context) (int, error)
	// WatchSSHServerPort returns a watcher that notifies when the controller
	// SSH server port changes.
	WatchSSHServerPort(context.Context) (watcher.NotifyWatcher, error)
}

// ServerWrapperWorkerConfig holds the configuration required by the server wrapper worker.
type ServerWrapperWorkerConfig struct {
	ControllerConfigService ControllerConfigService
	SSHService              SSHService
	NewServerWorker         func(ServerWorkerConfig) (worker.Worker, error)
	Logger                  logger.Logger
	Authenticator           Authenticator
	Authorizer              Authorizer
	ServerFactory           *TerminatingServerFactory
	Metrics                 *Collector
	Clock                   clock.Clock
}

// Validate validates the workers configuration is as expected.
func (c ServerWrapperWorkerConfig) Validate() error {
	if c.ControllerConfigService == nil {
		return errors.NotValidf("ControllerConfigService is required")
	}
	if c.Metrics == nil {
		return errors.NotValidf("missing Metrics")
	}
	if c.NewServerWorker == nil {
		return errors.NotValidf("NewSSHServer is required")
	}
	if c.SSHService == nil {
		return errors.NotValidf("SSHService is required")
	}
	if c.Logger == nil {
		return errors.NotValidf("Logger is required")
	}
	if c.Authenticator == nil {
		return errors.NotValidf("Authenticator is required")
	}
	if c.Authorizer == nil {
		return errors.NotValidf("Authorizer is required")
	}
	if c.ServerFactory == nil {
		return errors.NotValidf("ServerFactory is required")
	}
	if c.Clock == nil {
		return errors.NotValidf("Clock is required")
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

	// runner runs the inner server worker.
	runner *worker.Runner
}

// NewServerWrapperWorker returns a new worker that runs an ssh server worker
// internally, restarting it when its config changes.
func NewServerWrapperWorker(config ServerWrapperWorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:   "ssh-server",
		Clock:  config.Clock,
		Logger: internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &serverWrapperWorker{
		config: config,
		runner: runner,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "ssh-server",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{runner},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// TerminatingServerFactory returns the factory that builds per-destination
// terminating SSH servers, exposed through the manifold output for the
// apiserver's relay endpoint.
func (ssw *serverWrapperWorker) TerminatingServerFactory() *TerminatingServerFactory {
	return ssw.config.ServerFactory
}

// Kill implements worker.Worker.
func (ssw *serverWrapperWorker) Kill() {
	ssw.catacomb.Kill(nil)
}

// Wait implements worker.Worker.
func (ssw *serverWrapperWorker) Wait() error {
	return ssw.catacomb.Wait()
}

// Report returns the runner's report, which includes the server worker.
func (ssw *serverWrapperWorker) Report(ctx context.Context) map[string]any {
	return ssw.runner.Report(ctx)
}

// loop is the main loop of the server wrapper worker. It starts the server worker
// and listens for changes in the controller configuration.
func (ssw *serverWrapperWorker) loop() error {
	ctx := ssw.catacomb.Context(context.Background())

	// Watch for changes then acquire the latest controller configuration
	// to avoid starting the server with stale config values. Controller config
	// still owns the max concurrent connections value.
	controllerConfigWatcher, err := ssw.config.ControllerConfigService.WatchControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := ssw.catacomb.Add(controllerConfigWatcher); err != nil {
		return errors.Trace(err)
	}

	// The SSH server port is owned by the controller charm and pushed to the
	// SSH server service via the control socket. Watch it separately so the
	// server restarts when the charm changes the port.
	sshServerPortWatcher, err := ssw.config.SSHService.WatchSSHServerPort(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := ssw.catacomb.Add(sshServerPortWatcher); err != nil {
		return errors.Trace(err)
	}

	config, err := ssw.config.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	port, err := ssw.config.SSHService.GetSSHServerPort(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	maxConns := config.SSHMaxConcurrentConnections()
	jumpHostKey, err := ssw.config.SSHService.SSHServerHostKey(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	if err := ssw.startServer(ctx, jumpHostKey, port, maxConns); err != nil {
		return errors.Trace(err)
	}

	controllerConfigChanges := controllerConfigWatcher.Changes()
	sshServerPortChanges := sshServerPortWatcher.Changes()
	for {
		select {
		case <-ssw.catacomb.Dying():
			return ssw.catacomb.ErrDying()
		case _, ok := <-controllerConfigChanges:
			if !ok {
				return errors.New("controller config watcher closed")
			}

			config, err := ssw.config.ControllerConfigService.ControllerConfig(ctx)
			if err != nil {
				return errors.Trace(err)
			}
			if maxConns == config.SSHMaxConcurrentConnections() {
				continue
			}

			ssw.config.Logger.Debugf(ctx, "restarting ssh server with max concurrent connections %d",
				config.SSHMaxConcurrentConnections())

			// Wait for the old server so it releases the port before the
			// new one binds.
			err = ssw.runner.StopAndRemoveWorker(serverWorkerName, ssw.catacomb.Dying())
			if err != nil && !errors.Is(err, errors.NotFound) {
				return errors.Trace(err)
			}

			maxConns = config.SSHMaxConcurrentConnections()
			if err := ssw.startServer(ctx, jumpHostKey, port, maxConns); err != nil {
				return errors.Trace(err)
			}
		case _, ok := <-sshServerPortChanges:
			if !ok {
				return errors.New("ssh server port watcher closed")
			}

			currentPort, err := ssw.config.SSHService.GetSSHServerPort(ctx)
			if err != nil {
				return errors.Trace(err)
			}
			if port == currentPort {
				continue
			}

			ssw.config.Logger.Debugf(ctx, "restarting ssh server on port %d", currentPort)

			// Wait for the old server so it releases the port before the
			// new one binds.
			err = ssw.runner.StopAndRemoveWorker(serverWorkerName, ssw.catacomb.Dying())
			if err != nil && !errors.Is(err, errors.NotFound) {
				return errors.Trace(err)
			}

			port = currentPort
			if err := ssw.startServer(ctx, jumpHostKey, port, maxConns); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// startServer starts a jump server worker in the runner.
func (ssw *serverWrapperWorker) startServer(
	ctx context.Context, jumpHostKey string, port, maxConns int,
) error {
	serverConfig := ServerWorkerConfig{
		Logger:                   ssw.config.Logger,
		JumpHostKey:              jumpHostKey,
		Port:                     port,
		MaxConcurrentConnections: maxConns,
		SSHService:               ssw.config.SSHService,
		Authenticator:            ssw.config.Authenticator,
		Authorizer:               ssw.config.Authorizer,
		ServerFactory:            ssw.config.ServerFactory,
		Metrics:                  ssw.config.Metrics,
		Clock:                    ssw.config.Clock,
	}
	start := func(context.Context) (worker.Worker, error) {
		return ssw.config.NewServerWorker(serverConfig)
	}
	return errors.Trace(ssw.runner.StartWorker(ctx, serverWorkerName, start))
}
