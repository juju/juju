// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/pubsub/apiserver"
	internalworker "github.com/juju/juju/internal/worker"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
	stateChanged = "changed"
)

// WorkerConfig defines the configuration values that the pubsub worker needs
// to operate.
type WorkerConfig struct {
	Origin names.Tag
	Clock  clock.Clock
	Hub    *pubsub.StructuredHub
	Logger logger.Logger

	APIInfo   *api.Info
	APIOpener api.OpenFunc
	NewRemote func(RemoteServerConfig) RemoteServer
}

// Validate checks that all the values have been set.
func (c *WorkerConfig) Validate() error {
	if c.Origin == nil {
		return errors.NotValidf("missing origin")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing clock")
	}
	if c.Hub == nil {
		return errors.NotValidf("missing hub")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	if c.APIInfo == nil {
		return errors.NotValidf("missing api info")
	}
	if c.APIOpener == nil {
		return errors.NotValidf("missing api opener")
	}
	if c.NewRemote == nil {
		return errors.NotValidf("missing new remote")
	}
	return nil
}

type serverChanges struct {
	servers map[string][]string
}

type remoteWorker struct {
	internalStates chan string
	catacomb       catacomb.Catacomb

	cfg WorkerConfig

	runner  *worker.Runner
	changes chan serverChanges

	mu         sync.Mutex
	apiRemotes []RemoteConnection

	unsubServerDetails func()
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
		changes:        make(chan serverChanges),
		internalStates: internalState,
		apiRemotes:     make([]RemoteConnection, 0),
	}

	w.unsubServerDetails, err = cfg.Hub.Subscribe(apiserver.DetailsTopic, w.apiServerChanges)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Ask for the current server details now that we're subscribed.
	detailsRequest := apiserver.DetailsRequest{
		Requester: "api-remote-worker",
		LocalOnly: true,
	}
	if _, err := cfg.Hub.Publish(apiserver.DetailsRequestTopic, detailsRequest); err != nil {
		return nil, errors.Trace(err)
	}

	err = catacomb.Invoke(catacomb.Plan{
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
func (w *remoteWorker) GetAPIRemotes() []RemoteConnection {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.apiRemotes
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

	defer w.unsubServerDetails()

	ctx, cancel := w.scopedContext()
	defer cancel()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case change := <-w.changes:
			w.cfg.Logger.Debugf(ctx, "remoteWorker API server change: %v", change)

			// Locate the current workers, so we can remove any workers that are
			// no longer required.
			current := w.runner.WorkerNames()

			required := make(map[string]RemoteConnection)
			for target, addresses := range change.servers {

				server, err := w.newRemoteServer(ctx, target, addresses)
				if err != nil {
					w.cfg.Logger.Errorf(ctx, "failed to start remote worker for %q: %v", target, err)
					return errors.Trace(err)
				}

				// Always update the server with the latest addresses, even if
				// it was just created. This is to ensure that the server is
				// always up to date with the latest addresses.
				server.UpdateAddresses(addresses)

				required[target] = server
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

			var remotes []RemoteConnection
			for _, server := range required {
				if server == nil {
					continue
				}
				remotes = append(remotes, server)
			}

			w.mu.Lock()
			w.apiRemotes = remotes
			w.mu.Unlock()

			w.reportInternalState(stateChanged)
		}
	}
}

func (w *remoteWorker) apiServerChanges(topic string, details apiserver.Details, err error) {
	ctx, cancel := w.scopedContext()
	defer cancel()

	if err != nil {
		w.cfg.Logger.Errorf(ctx, "remoteWorker callback error: %v", err)
		return
	}

	w.cfg.Logger.Debugf(ctx, "remoteWorker API server changes: %v", details)

	changes := make(map[string][]string)

	for id, apiServer := range details.Servers {
		// The target is constructed from an id, and the tag type
		// needs to match that of the origin tag.
		var target names.Tag
		switch w.cfg.Origin.Kind() {
		case names.MachineTagKind:
			target = names.NewMachineTag(id)
		case names.ControllerAgentTagKind:
			target = names.NewControllerAgentTag(id)
		default:
			w.cfg.Logger.Errorf(ctx, "unknown remoteWorker origin tag: %s", id)
			continue
		}

		// If the target is the origin, we don't need a connection to ourselves.
		if target == w.cfg.Origin {
			continue
		}

		// TODO: always use the internal address?
		addresses := apiServer.Addresses
		if apiServer.InternalAddress != "" {
			addresses = []string{apiServer.InternalAddress}
		}

		changes[target.Id()] = addresses
	}

	// We must dispatch every time we get a change, as the API server might
	// actually be moving from HA to non-HA and we need to update the workers
	// accordingly. This involves the clearing out of the old workers.
	select {
	case <-w.catacomb.Dying():
		return
	case w.changes <- serverChanges{
		servers: changes,
	}:
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
