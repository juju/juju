// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupsweeper

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"gopkg.in/tomb.v2"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/logger"
	environsconfig "github.com/juju/juju/environs/config"
)

// ModelConfigService provides the model configuration, used to resolve
// the backup directory.
type ModelConfigService interface {
	// ModelConfig returns the model config.
	ModelConfig(ctx context.Context) (*environsconfig.Config, error)
}

// WorkerConfig contains the configuration required to run the backup
// archive sweeper.
type WorkerConfig struct {
	// ModelConfigService provides the controller model's configuration.
	ModelConfigService ModelConfigService

	// Clock provides access to the current time and timers.
	Clock clock.Clock

	// Logger is used to log messages.
	Logger logger.Logger
}

// Validate validates the worker configuration.
func (cfg WorkerConfig) Validate() error {
	if cfg.ModelConfigService == nil {
		return errors.NotValidf("nil ModelConfigService")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Sweeper is a worker that periodically removes one-shot backup
// archives that have outlived their retention window.
type Sweeper struct {
	tomb tomb.Tomb

	modelConfig ModelConfigService
	clock       clock.Clock
	logger      logger.Logger
}

// NewWorker creates a new backup archive sweeper worker.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Sweeper{
		modelConfig: config.ModelConfigService,
		clock:       config.Clock,
		logger:      config.Logger,
	}
	w.tomb.Go(w.loop)
	return w, nil
}

// Kill stops the sweeper.
func (w *Sweeper) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the sweeper to stop.
func (w *Sweeper) Wait() error {
	return w.tomb.Wait()
}

func (w *Sweeper) loop() error {
	ctx := w.tomb.Context(context.Background())

	timer := w.clock.NewTimer(corebackups.SweepInterval)
	defer timer.Stop()

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-timer.Chan():
			timer.Reset(corebackups.SweepInterval)
			if err := w.sweep(ctx); err != nil {
				// A failed sweep is retried on the next tick: stale
				// archives staying on disk is diagnosable from the
				// log, and must not kill the worker.
				w.logger.Errorf(ctx, "sweeping expired backup archives: %v", err)
			}
		}
	}
}

// sweep removes expired one-shot archives from the backup directory.
// The backup directory and the retention window are resolved on every
// sweep so that model config changes take effect; stale files in a
// previously-configured directory are the operator's to clean.
//
// The retention window starts at archive creation (the staging rename
// preserves the ModTime), not at the last transfer attempt: a large
// archive over a slow link that fails mid-transfer still counts toward
// the same window from creation. Operators backing up over unreliable
// networks can widen the window via the backup-download-ttl model
// config attribute.
func (w *Sweeper) sweep(ctx context.Context) error {
	modelConfig, err := w.modelConfig.ModelConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	backupDir := corebackups.BackupDirToUse(modelConfig.BackupDir())
	if err := corebackups.CleanExpiredOneShotArchives(backupDir, modelConfig.BackupDownloadTTL(), w.clock.Now()); err != nil {
		return errors.Trace(err)
	}
	return nil
}
