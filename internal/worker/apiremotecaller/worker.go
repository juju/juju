// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
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
	// GetAPIAddressesByControllerIDForAgents returns a map of controller IDs to
	// their API addresses that are available for agents. The map is keyed by
	// controller ID, and the values are slices of strings representing the API
	// addresses for each controller node.
	GetAPIAddressesByControllerIDForAgents(ctx context.Context) (map[string][]string, error)
	// WatchControllerAPIAddresses returns a watcher that observes changes to
	// the controller api address changes.
	WatchControllerAPIAddresses(context.Context) (watcher.NotifyWatcher, error)
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

	subscriptionID atomic.Int64
	subscribeCh    chan subscriber
	unsubscribeCh  chan int64
	notifyCh       chan struct{}
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

		subscribeCh:   make(chan subscriber),
		unsubscribeCh: make(chan int64),
		notifyCh:      make(chan struct{}),
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

// Subscribe creates a new subscription to be notified when the set of API
// remotes has changed. This is useful for callers that want to be notified when
// the set of remotes changes so they can update their internal state.
func (w *remoteWorker) Subscribe() (Subscription, error) {
	subscriber := newSubscriber(w.subscriptionID.Add(1), func(i int64) {
		select {
		case <-w.catacomb.Dying():
		case w.unsubscribeCh <- i:
		}
	})
	select {
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	case w.subscribeCh <- subscriber:
	}
	return subscriber, nil
}

// Kill is part of the worker.Worker interface.
func (w *remoteWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *remoteWorker) Wait() error {
	return w.catacomb.Wait()
}

// Report returns a map of internal state for the remoteWorker.
func (w *remoteWorker) Report(ctx context.Context) map[string]any {
	report := make(map[string]any)
	report["origin"] = w.cfg.Origin.Id()
	report["runner"] = w.runner.Report(ctx)
	return report
}

func (w *remoteWorker) loop() error {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	ctx, cancel := w.scopedContext()
	defer cancel()

	watcher, err := w.cfg.ControllerNodeService.WatchControllerAPIAddresses(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	subscribers := make(map[int64]subscriber)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-watcher.Changes():
			w.cfg.Logger.Debugf(ctx, "remoteWorker API server change")

			// Get the latest API addresses for all controller nodes.
			servers, err := w.cfg.ControllerNodeService.GetAPIAddressesByControllerIDForAgents(ctx)
			if errors.Is(err, controllernodeerrors.EmptyAPIAddresses) {
				// There should be at least one controller address available
				// (itself), so if we get an empty addresses error then we can't
				// proceed. Yet we shouldn't stop the worker coming up, so we
				// log the error and continue to wait for the next change.
				w.cfg.Logger.Warningf(ctx, "no API addresses available for remote workers: %v", err)
				continue
			} else if err != nil {
				return errors.Trace(err)
			}

			w.cfg.Logger.Tracef(ctx, "remoteWorker API servers: %v %v", servers, w.cfg.Origin.Id())

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

		case subscriber := <-w.subscribeCh:
			subscribers[subscriber.id] = subscriber

		case id := <-w.unsubscribeCh:
			delete(subscribers, id)

		case <-w.notifyCh:
			// Notify all subscribers of the change.
			for _, sub := range subscribers {
				select {
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				case sub.changes <- struct{}{}:
				}
			}
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
			APIOpener:    w.newConnection,
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

func (w *remoteWorker) newConnection(ctx context.Context, apiInfo *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
	conn, err := w.cfg.APIOpener(ctx, apiInfo, dialOpts)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// If the remote tracking worker changes the addresses and
	// restarts the connection, we need to notify the main worker
	// loop so that it can notify subscribers of the change. This
	// always ensures then that subscribers are notified when a
	// change occurs.
	// This is non-blocking, as we don't want to hold up the connection
	// establishment if the main loop is busy.
	go func() {
		select {
		case <-w.catacomb.Dying():
			return
		case <-ctx.Done():
			return
		case w.notifyCh <- struct{}{}:
			w.cfg.Logger.Tracef(ctx, "notified remote worker change")
		}
	}()

	return conn, nil
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

type subscriber struct {
	id          int64
	changes     chan struct{}
	unsubscribe func(int64)
}

func newSubscriber(id int64, unsubscribe func(int64)) subscriber {
	return subscriber{
		id:          id,
		changes:     make(chan struct{}),
		unsubscribe: unsubscribe,
	}
}

// Changes returns a channel that signals when the set of API remotes has
// changed.
func (s subscriber) Changes() <-chan struct{} {
	return s.changes
}

// Close closes the subscription.
func (s subscriber) Close() {
	s.unsubscribe(s.id)
}
