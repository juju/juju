// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	internalworker "github.com/juju/juju/internal/worker"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
	stateChanged = "changed"
)

// ControllerNodeService is an interface that represents the service
// that provides information about the controller nodes. This is used to
// determine which nodes are available for remote API calls.
type ControllerNodeService interface {
	// GetAllAPIAddressesForAgents returns a map of controller IDs to their API
	// addresses that are available for agents. The map is keyed by controller
	// ID, and the values are slices of strings representing the API addresses
	// for each controller node.
	GetAllAPIAddressesForAgents(ctx context.Context) (map[string][]string, error)
	// WatchControllerNodes returns a watcher that observes changes to the
	// controller nodes.
	WatchControllerNodes(context.Context) (watcher.NotifyWatcher, error)
}

// WorkerConfig defines the configuration values that the pubsub worker needs
// to operate.
type WorkerConfig struct {
	Origin                names.Tag
	Clock                 clock.Clock
	ControllerNodeService ControllerNodeService
	Logger                logger.Logger

	APIInfo   *api.Info
	APIOpener api.OpenFunc
	NewRemote func(RemoteServerConfig) RemoteServer
}

// Validate checks that all the values have been set.
func (c *WorkerConfig) Validate() error {
	if c.Origin == nil {
		return errors.NotValidf("missing Origin")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if c.ControllerNodeService == nil {
		return errors.NotValidf("missing ControllerNodeService")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.APIInfo == nil {
		return errors.NotValidf("missing APIInfo")
	}
	if c.APIOpener == nil {
		return errors.NotValidf("missing APIOpener")
	}
	if c.NewRemote == nil {
		return errors.NotValidf("missing NewRemote")
	}
	return nil
}

type remoteWorker struct {
	internalStates chan string
	catacomb       catacomb.Catacomb

	cfg WorkerConfig

	runner *worker.Runner
}

// NewWorker exposes the remoteWorker as a Worker.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	return newWorker(config, nil)
}

func newWorker(cfg WorkerConfig, internalState chan string) (*remoteWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:  "remote-worker",
		Clock: cfg.Clock,
		IsFatal: func(err error) bool {
			// TODO (stickupkid): Handle specific errors here.
			return false
		},
		// Backoff for 5 seconds before restarting a worker.
		// This is a lifetime for the life of an API connection.
		RestartDelay: time.Second * 5,
		Logger:       internalworker.WrapLogger(cfg.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	w := &remoteWorker{
		cfg:            cfg,
		runner:         runner,
		internalStates: internalState,
	}

	err = catacomb.Invoke(catacomb.Plan{
		Name: "api-remote-worker",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.runner,
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// GetAPIRemotes returns the current API connections. It is expected that
// the caller will call this method just before making an API call to ensure
// that the connection is still valid. The caller must not cache the connections
// as they may change over time.
func (w *remoteWorker) GetAPIRemotes() ([]RemoteConnection, error) {
	workerNames := w.runner.WorkerNames()

	var remotes []RemoteConnection
	for _, name := range workerNames {
		worker, err := w.workerFromCache(name)
		if err != nil {
			return nil, errors.Trace(err)
		} else if worker == nil {
			// If the worker is not found, it means it was removed or not
			// started yet. This can happen if the worker is still starting
			// up or has been stopped.
			continue
		}

		remotes = append(remotes, worker)
	}
	return remotes, nil
}

// Kill is part of the worker.Worker interface.
func (w *remoteWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *remoteWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *remoteWorker) loop() error {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	ctx, cancel := w.scopedContext()
	defer cancel()

	watcher, err := w.cfg.ControllerNodeService.WatchControllerNodes(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-watcher.Changes():
			w.cfg.Logger.Debugf(ctx, "remoteWorker API server change")

			// Get the latest API addresses for all controller nodes.
			servers, err := w.cfg.ControllerNodeService.GetAllAPIAddressesForAgents(ctx)
			if err != nil {
				return errors.Trace(err)
			}

			// Locate the current workers, so we can remove any workers that are
			// no longer required.
			current := w.runner.WorkerNames()

			required := make(map[string]struct{})
			for target, addresses := range servers {
				if target == w.cfg.Origin.Id() {
					// We don't need a remote worker for the origin.
					continue
				}

				server, err := w.newRemoteServer(ctx, target, addresses)
				if err != nil {
					w.cfg.Logger.Errorf(ctx, "failed to start remote worker for %q: %v", target, err)
					return errors.Trace(err)
				}

				// Always update the server with the latest addresses, even if
				// it was just created. This is to ensure that the server is
				// always up to date with the latest addresses.
				server.UpdateAddresses(addresses)

				required[target] = struct{}{}
			}

			// Walk over the current workers and remove any that are no longer
			// required.
			for _, s := range current {
				if _, ok := required[s]; ok {
					continue
				}

				w.cfg.Logger.Debugf(ctx, "remote worker %q no longer required", s)
				if err := w.stopRemoteServer(ctx, s); err != nil {
					w.cfg.Logger.Errorf(ctx, "failed to stop remote worker %q: %v", s, err)
					continue
				}
			}

			w.cfg.Logger.Debugf(ctx, "remote workers updated: %v", required)

			w.reportInternalState(stateChanged)
		}
	}
}

func (w *remoteWorker) newRemoteServer(ctx context.Context, controllerID string, addresses []string) (RemoteServer, error) {
	// Create a new remote server APIInfo with the target and addresses.
	apiInfo := *w.cfg.APIInfo
	apiInfo.Addrs = addresses

	// Start a new worker with the target and addresses.
	err := w.runner.StartWorker(ctx, controllerID, func(ctx context.Context) (worker.Worker, error) {
		w.cfg.Logger.Debugf(ctx, "starting remote worker for %q", controllerID)
		return w.cfg.NewRemote(RemoteServerConfig{
			Clock:        w.cfg.Clock,
			Logger:       w.cfg.Logger,
			ControllerID: controllerID,
			APIInfo:      &apiInfo,
			APIOpener:    w.cfg.APIOpener,
		}), nil
	})
	if err != nil && !errors.Is(err, errors.AlreadyExists) {
		return nil, errors.Trace(err)
	}

	server, err := w.workerFromCache(controllerID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// This shouldn't happen, because we just started the worker and waited for
	// it with the workerFromCache call above.
	if server == nil {
		return nil, errors.NotFoundf("worker %q not found", controllerID)
	}

	return server, nil
}

func (w *remoteWorker) stopRemoteServer(ctx context.Context, controllerID string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	// Stop and remove the worker if it's no longer required.
	w.cfg.Logger.Debugf(ctx, "stopping remote worker for %q", controllerID)
	if err := w.runner.StopAndRemoveWorker(controllerID, ctx.Done()); errors.Is(err, context.DeadlineExceeded) {
		return errors.Errorf("failed to stop worker %q: timed out", controllerID)
	} else if err != nil {
		return errors.Errorf("failed to stop worker %q: %v", controllerID, err)
	}

	return nil
}

func (w *remoteWorker) workerFromCache(controllerID string) (RemoteServer, error) {
	// If the worker already exists, return the existing worker early.
	if tracked, err := w.runner.Worker(controllerID, w.catacomb.Dying()); err == nil {
		return tracked.(RemoteServer), nil
	} else if errors.Is(errors.Cause(err), worker.ErrDead) {
		// Handle the case where the DB runner is dead due to this worker dying.
		select {
		case <-w.catacomb.Dying():
			return nil, w.catacomb.ErrDying()
		default:
			return nil, errors.Trace(err)
		}
	} else if !errors.Is(errors.Cause(err), errors.NotFound) {
		// If it's not a NotFound error, return the underlying error.
		// We should only start a worker if it doesn't exist yet.
		return nil, errors.Trace(err)
	}

	// We didn't find the worker. Let the caller decide what to do.
	return nil, nil
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *remoteWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *remoteWorker) reportInternalState(state string) {
	select {
	case <-w.catacomb.Dying():
	case w.internalStates <- state:
	default:
	}
}
