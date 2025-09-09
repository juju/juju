// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operationpruner

import (
	"context"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/retry"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
	// Watch returns a watcher that returns keys for any changes to model
	// config.
	Watch(ctx context.Context) (watcher.StringsWatcher, error)
}

// OperationService provides access to operations
type OperationService interface {
	// PruneOperations removes operations older than maxAge or larger than maxSizeMB.
	PruneOperations(context context.Context, maxAge time.Duration, maxSizeMB int) error
}

// Config is the configuration for the operation pruner.
type Config struct {
	Clock            clock.Clock
	ModelConfig      ModelConfigService
	OperationService OperationService
	Logger           logger.Logger

	// PruneInterval is the interval at which the pruner will run.
	PruneInterval time.Duration
}

// Validate checks whether the worker configuration settings are valid.
func (config Config) Validate() error {
	if config.Clock == nil {
		return errors.Errorf("nil clock.Clock").Add(coreerrors.NotValid)
	}
	if config.ModelConfig == nil {
		return errors.Errorf("nil ModelConfigService").Add(coreerrors.NotValid)
	}
	if config.OperationService == nil {
		return errors.Errorf("nil OperationService").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.Errorf("nil Logger").Add(coreerrors.NotValid)
	}
	if config.PruneInterval <= 0 {
		return errors.Errorf("prune interval must be positive").Add(coreerrors.NotValid)
	}
	return nil
}

// prunerWorker is a worker that prunes operations.
type prunerWorker struct {
	config   Config
	catacomb catacomb.Catacomb

	// mu guards the fields below it.
	mu sync.Mutex

	maxAge     time.Duration
	maxSizeMB  int
	lastUpdate time.Time
	lastPrune  time.Time
}

// NewWorker returns a new pruner worker.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	w := &prunerWorker{
		config: config,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "operation-pruner",
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Capture(err)
}

// Kill is part of the worker.Worker interface.
func (w *prunerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *prunerWorker) Wait() error {
	return w.catacomb.Wait()
}

// Report shows up in the dependency engine report.
func (w *prunerWorker) Report() map[string]interface{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	return map[string]interface{}{
		"max-age":     w.maxAge,
		"max-size-mb": w.maxSizeMB,
		"last-update": w.lastUpdate,
		"last-prune":  w.lastPrune,
	}
}

// jitter returns a random duration around the given period, between 0.5 and 1.5
// times the period.
func jitter(period time.Duration) time.Duration {
	half := period / 2
	return retry.ExpBackoff(half, period+half, 2, true)(0, 1)
}

// loop is the worker's main loop.
//   - It watches for changes to the model configuration to get up-to-date values
//     for the pruning interval and the maximum size of operation results.
//   - It periodically prunes operations.
func (w *prunerWorker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	watch, err := w.config.ModelConfig.Watch(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	if err := w.catacomb.Add(watch); err != nil {
		return errors.Capture(err)
	}

	initCfg, err := w.config.ModelConfig.ModelConfig(ctx)
	if err != nil {
		return errors.Errorf("getting model config: %w", err)
	}

	w.updateConfig(ctx, initCfg)
	var (
		pruneTimer = w.config.Clock.NewTimer(w.nextPruneInterval(ctx))
	)
	defer pruneTimer.Stop()
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case keys, ok := <-watch.Changes():
			if !ok {
				return errors.New("model config watcher closed")
			}
			changes := set.NewStrings(keys...)
			if !changes.Contains(config.MaxActionResultsSize) &&
				!changes.Contains(config.MaxActionResultsAge) {
				continue
			}

			cfg, err := w.config.ModelConfig.ModelConfig(ctx)
			if err != nil {
				return errors.Errorf("getting model config: %w", err)
			}
			w.updateConfig(ctx, cfg)
			err = w.doPrune(ctx, pruneTimer)
			if err != nil {
				return errors.Capture(err)
			}
		case <-pruneTimer.Chan():
			err = w.doPrune(ctx, pruneTimer)
			if err != nil {
				return errors.Capture(err)
			}
		}
	}
}

// doPrune prunes operations.
func (w *prunerWorker) doPrune(ctx context.Context, pruneTimer clock.Timer) error {
	maxAge, maxSizeMB := w.getPruneArgs()
	err := w.config.OperationService.PruneOperations(ctx, maxAge, maxSizeMB)
	if err != nil {
		return errors.Errorf("pruning operations: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastPrune = w.config.Clock.Now()
	pruneTimer.Reset(w.nextPruneInterval(ctx))
	return nil
}

// nextPruneInterval returns a jittered duration for the next prune interval.
func (w *prunerWorker) nextPruneInterval(ctx context.Context) time.Duration {
	jittered := jitter(w.config.PruneInterval)
	w.config.Logger.Debugf(ctx, "jittered prune interval: %v", jittered)
	return jittered
}

// getPruneArgs returns the current prune arguments. The returned values are
// guarded by w.mu to avoid races
func (w *prunerWorker) getPruneArgs() (time.Duration, int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.maxAge, w.maxSizeMB
}

// updateConfig updates the pruner's configuration. It is guarded by w.mu to
// avoid races.
func (w *prunerWorker) updateConfig(ctx context.Context, initCfg *config.Config) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.maxAge = initCfg.MaxActionResultsAge()
	w.maxSizeMB = int(initCfg.MaxActionResultsSizeMB())
	w.lastUpdate = w.config.Clock.Now()
	w.config.Logger.Debugf(ctx, "config updated: max-age=%v, max-size-mb=%v", w.maxAge, w.maxSizeMB)
}
