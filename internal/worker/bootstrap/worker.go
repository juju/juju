// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/flags"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/worker/gate"
)

const (
	// States which report the state of the worker.
	stateStarted   = "started"
	stateCompleted = "completed"
)

// Operation performs the controller work required before bootstrap can be marked
// complete. It must honour cancellation and be safe to retry after failure.
// On success it may return a cleanup function for staged artefacts. The worker
// calls cleanup only after persisting the completion flag, so it must not be used
// for releasing resources that also need to be released on failure.
type Operation func(context.Context) (func(), error)

// WorkerConfig contains the operation and its lifecycle dependencies.
type WorkerConfig struct {
	Operation           Operation
	FlagService         FlagService
	BootstrapUnlocker   gate.Unlocker
	ControllerModelUUID coremodel.UUID
	StatusHistory       StatusHistory
	Logger              logger.Logger
	Clock               clock.Clock
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.Operation == nil {
		return jujuerrors.NotValidf("nil Operation")
	}
	if c.FlagService == nil {
		return jujuerrors.NotValidf("nil FlagService")
	}
	if c.BootstrapUnlocker == nil {
		return jujuerrors.NotValidf("nil BootstrapUnlocker")
	}
	if err := c.ControllerModelUUID.Validate(); err != nil {
		return errors.Errorf("controller model id: %w", err)
	}
	if c.StatusHistory == nil {
		return jujuerrors.NotValidf("nil StatusHistory")
	}
	if c.Logger == nil {
		return jujuerrors.NotValidf("nil Logger")
	}
	if c.Clock == nil {
		return jujuerrors.NotValidf("nil Clock")
	}
	return nil
}

type bootstrapWorker struct {
	internalStates chan string
	cfg            WorkerConfig
	logger         logger.Logger
	tomb           tomb.Tomb
}

// NewWorker creates a new bootstrap worker.
func NewWorker(cfg WorkerConfig) (*bootstrapWorker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg WorkerConfig, internalStates chan string) (*bootstrapWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	w := &bootstrapWorker{
		internalStates: internalStates,
		cfg:            cfg,
		logger:         cfg.Logger.Child("worker"),
	}
	w.tomb.Go(w.loop)
	return w, nil
}

// Kill stops the worker.
func (w *bootstrapWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the worker to stop and then returns the reason it was killed.
func (w *bootstrapWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *bootstrapWorker) loop() error {
	w.reportInternalState(stateStarted)

	ctx, cancel := w.scopedContext()
	defer cancel()

	cleanup, err := w.cfg.Operation(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	// Persist completion before deleting staged artefacts or opening the gate.
	if err := w.cfg.FlagService.SetFlag(ctx, flags.BootstrapFlag, true, flags.BootstrapFlagDescription); err != nil {
		return errors.Capture(err)
	}
	if cleanup != nil {
		cleanup()
	}

	w.reportInternalState(stateCompleted)
	w.cfg.BootstrapUnlocker.Unlock()

	// Write the domain status history for the model. We don't care if this
	// fails, but we also are trying to ensure it's written only once. By
	// writing it here, we know that at the very least that the bootstrap
	// worker hasn't been restarted.
	modelUUID := w.cfg.ControllerModelUUID
	if err := w.cfg.StatusHistory.RecordStatus(ctx, status.ModelNamespace.WithID(modelUUID.String()), corestatus.StatusInfo{
		Status: corestatus.Available,
		Since:  new(w.cfg.Clock.Now()),
	}); err != nil {
		w.logger.Warningf(ctx, "recording status for model %q: %v", modelUUID, err)
	}

	return nil
}

func (w *bootstrapWorker) reportInternalState(state string) {
	select {
	case <-w.tomb.Dying():
	case w.internalStates <- state:
	default:
	}
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action completes.
func (w *bootstrapWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.tomb.Context(context.Background()))
}
