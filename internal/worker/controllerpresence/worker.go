// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerpresence

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/worker/apiremotecaller"
)

const (
	// BrokenConnection is returned when the connection to the remote API
	// server is broken.
	BrokenConnection = errors.ConstError("connection to remote API server is broken")
)

// StatusService is an interface that defines the status service required by the
// controller presence worker.
type StatusService interface {
	// DeleteMachinePresence removes the presence of the specified machine. If
	// the machine isn't found it ignores the error. The machine life is not
	// considered when making this query.
	DeleteMachinePresence(ctx context.Context, name machine.Name) error

	// DeleteUnitPresence removes the presence of the specified unit. If the
	// unit isn't found it ignores the error.
	// The unit life is not considered when making this query.
	DeleteUnitPresence(ctx context.Context, name coreunit.Name) error
}

// WorkerConfig defines the configuration values that the pubsub worker needs
// to operate.
type WorkerConfig struct {
	StatusService       StatusService
	APIRemoteSubscriber apiremotecaller.APIRemoteSubscriber
	Clock               clock.Clock
	Logger              logger.Logger
}

// Validate checks that all the values have been set.
func (c WorkerConfig) Validate() error {
	if c.StatusService == nil {
		return errors.New("missing StatusService not valid").Add(coreerrors.NotValid)
	}
	if c.APIRemoteSubscriber == nil {
		return errors.New("missing APIRemoteSubscriber not valid").Add(coreerrors.NotValid)
	}
	if c.Clock == nil {
		return errors.New("missing Clock not valid").Add(coreerrors.NotValid)
	}
	if c.Logger == nil {
		return errors.New("missing Logger not valid").Add(coreerrors.NotValid)
	}
	return nil
}

type controllerWorker struct {
	catacomb catacomb.Catacomb
	runner   *worker.Runner

	cfg WorkerConfig
}

// NewWorker exposes the remoteWorker as a Worker.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	return newWorker(config)
}

func newWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "controller-presence",
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: func(err error) bool {
			return errors.Is(err, BrokenConnection)
		},
		RestartDelay: time.Second * 5,
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	w := &controllerWorker{
		cfg:    config,
		runner: runner,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "controller-presence",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{runner},
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *controllerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *controllerWorker) Wait() error {
	return w.catacomb.Wait()
}

// Report returns a map of internal state for the controllerWorker.
func (w *controllerWorker) Report(ctx context.Context) map[string]any {
	report := make(map[string]any)
	report["runner"] = w.runner.Report(ctx)
	return report
}

func (w *controllerWorker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	subscriber, err := w.cfg.APIRemoteSubscriber.Subscribe()
	if err != nil {
		return errors.Capture(err)
	}
	defer subscriber.Close()

	if err := w.ensureConnectionTrackers(ctx); err != nil {
		return errors.Capture(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-subscriber.Changes():
			// Remove all existing tracking runner workers.
			for _, name := range w.runner.WorkerNames() {
				if err := w.runner.StopAndRemoveWorker(name, w.catacomb.Dying()); err != nil && !errors.Is(err, coreerrors.NotFound) {
					w.cfg.Logger.Debugf(ctx, "stopping connection tracker worker %q: %v", name, err)
					return errors.Capture(err)
				}
			}

			// Ensure we have connection trackers for all API remotes.
			if err := w.ensureConnectionTrackers(ctx); err != nil {
				w.cfg.Logger.Debugf(ctx, "ensuring connection trackers: %v", err)
				return errors.Capture(err)
			}
		}
	}
}

func (w *controllerWorker) ensureConnectionTrackers(ctx context.Context) error {
	callers, err := w.cfg.APIRemoteSubscriber.GetAPIRemotes()
	if err != nil {
		return errors.Capture(err)
	}

	for _, caller := range callers {
		// Start a runner to handle the connection, once it dies we need to
		// clean up presence information.
		workerName := "controller-" + caller.ControllerID()
		if err := w.runner.StartWorker(ctx, workerName, func(ctx context.Context) (worker.Worker, error) {
			return newConnectionTracker(caller.ControllerID(), caller, w.cfg.StatusService, w.cfg.Logger)
		}); err != nil && !errors.Is(err, coreerrors.AlreadyExists) {
			return errors.Capture(err)
		}
	}
	return nil
}

type connectionTracker struct {
	tomb tomb.Tomb

	controllerID  string
	connection    apiremotecaller.RemoteConnection
	statusService StatusService
	logger        logger.Logger

	connected atomic.Bool
}

func newConnectionTracker(controllerID string, conn apiremotecaller.RemoteConnection, statusService StatusService, logger logger.Logger) (worker.Worker, error) {
	w := &connectionTracker{
		controllerID:  controllerID,
		connection:    conn,
		statusService: statusService,
		logger:        logger,
	}

	w.tomb.Go(w.loop)

	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *connectionTracker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *connectionTracker) Wait() error {
	return w.tomb.Wait()
}

// Report returns a map of internal state for the connectionTracker.
func (w *connectionTracker) Report(ctx context.Context) map[string]any {
	report := make(map[string]any)
	report["controller-id"] = w.controllerID
	report["connected"] = w.connected.Load()
	return report
}

func (w *connectionTracker) loop() error {
	ctx := w.tomb.Context(context.Background())

	if err := w.connection.Connection(ctx, func(_ context.Context, c api.Connection) error {
		isBroken := c.IsBroken(ctx)
		w.connected.Store(!isBroken)
		if isBroken {
			return BrokenConnection
		}

		select {
		case <-w.tomb.Dying():
			return nil
		case <-c.Broken():
			w.connected.Store(false)
			return w.handleBrokenConnection(ctx)
		}
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (w *connectionTracker) handleBrokenConnection(ctx context.Context) error {
	// For a broken connection we want to remove any presence information for
	// machines and units that belong to this controller.
	w.logger.Debugf(ctx, "API remote caller connection to controller %q broken", w.controllerID)

	machineName := machine.Name(w.controllerID)
	if err := w.statusService.DeleteMachinePresence(ctx, machineName); err != nil {
		return errors.Errorf("deleting presence for machine %q: %w", machineName, err)
	}

	unitID, err := strconv.Atoi(w.controllerID)
	if err != nil {
		return errors.Errorf("parsing controller ID %q as unit number: %w", w.controllerID, err)
	}
	unitName, err := coreunit.NewNameFromParts(bootstrap.ControllerApplicationName, unitID)
	if err != nil {
		return errors.Errorf("creating unit name for controller ID %q: %w", w.controllerID, err)
	}
	if err := w.statusService.DeleteUnitPresence(ctx, unitName); err != nil {
		return errors.Errorf("deleting presence for unit %q: %w", unitName, err)
	}

	return nil
}
