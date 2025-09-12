// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestreampruner

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/changestream"
)

const (
	// defaultPruneInterval is the default interval at which the pruner will
	// run.
	defaultPruneInterval = time.Second * 5
)

// ChangeStreamService provides access to the changestream service.
type ChangeStreamService interface {
	// Prune prunes the change log up to the lowest watermark across all
	// controllers for the model.
	Prune(ctx context.Context, currentWindow changestream.Window) (changestream.Window, int64, error)
}

// WorkerConfig contains the configuration required to run a changestream
// pruner.
type WorkerConfig struct {
	// ChangeStreamService provides access to the changestream service.
	ChangeStreamService ChangeStreamService

	// Clock provides access to the current time and timers.
	Clock clock.Clock

	// Logger is used to log messages.
	Logger logger.Logger
}

// Validate validates the worker configuration.
func (cfg WorkerConfig) Validate() error {
	if cfg.ChangeStreamService == nil {
		return errors.NotValidf("nil ChangeStreamService")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Pruner is a worker that prunes the change log for a particular namespace.
type Pruner struct {
	tomb tomb.Tomb

	changeStreamService ChangeStreamService
	clock               clock.Clock
	logger              logger.Logger
}

// NewModelPruner creates a new Pruner for the given database and
// namespace.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Pruner{
		changeStreamService: config.ChangeStreamService,
		clock:               config.Clock,
		logger:              config.Logger,
	}

	w.tomb.Go(w.loop)
	return w, nil
}

// Kill stops the model pruner.
func (w *Pruner) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the model pruner to stop.
func (w *Pruner) Wait() error {
	return w.tomb.Wait()
}

func (w *Pruner) loop() error {
	ctx := w.tomb.Context(context.Background())

	timer := w.clock.NewTimer(defaultPruneInterval)
	defer timer.Stop()

	var window changestream.Window
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case <-timer.Chan():
			w.logger.Tracef(ctx, "running changestream pruner")

			newWindow, pruned, err := w.changeStreamService.Prune(ctx, window)
			if errors.Is(err, context.Canceled) {
				return tomb.ErrDying
			} else if err != nil {
				return errors.Trace(err)
			}

			if pruned > 0 {
				w.logger.Debugf(ctx, "pruned %d rows from change log", pruned)
			}

			window = newWindow

			timer.Reset(defaultPruneInterval)
		}
	}
}
