// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pruner

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/environs/config"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead pass one through as config to the worker.
type logger interface{}

var (
	_ logger = struct{}{}
)

// Facade represents an API that implements status history pruning.
type Facade interface {
	Prune(time.Duration, int) error
}

// PrunerWorker prunes status history or action records at regular intervals.
type PrunerWorker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill is defined on worker.Worker.
func (w *PrunerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker.
func (w *PrunerWorker) Wait() error {
	return w.catacomb.Wait()
}

// Catacomb returns the prune worker's catacomb.
func (w *PrunerWorker) Catacomb() *catacomb.Catacomb {
	return &w.catacomb
}

// Config return the prune worker's config.
func (w *PrunerWorker) Config() *Config {
	return &w.config
}

// Work is the main body of generic pruner loop.
func (w *PrunerWorker) Work(getPrunerConfig func(*config.Config) (time.Duration, uint)) error {
	modelConfigService := w.config.ModelConfigService
	modelConfigWatcher, err := modelConfigService.Watch()
	if err != nil {
		return errors.Trace(err)
	}

	ctx, cancel := w.scopedContext()
	defer cancel()

	if err := w.addWatcher(ctx, modelConfigWatcher); err != nil {
		return errors.Trace(err)
	}

	var (
		maxAge          time.Duration
		maxCollectionMB uint

		timer   clock.Timer
		timerCh <-chan time.Time
	)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case _, ok := <-modelConfigWatcher.Changes():
			if !ok {
				return errors.New("model configuration watcher closed")
			}
			modelConfig, err := modelConfigService.ModelConfig(ctx)
			if err != nil {
				return errors.Annotate(err, "cannot load model configuration")
			}

			newMaxAge, newMaxCollectionMB := getPrunerConfig(modelConfig)

			if newMaxAge != maxAge || newMaxCollectionMB != maxCollectionMB {
				w.config.Logger.Infof("pruner config: max age: %v, max collection size %dM for %s (%s)",
					newMaxAge, newMaxCollectionMB, modelConfig.Name(), modelConfig.UUID())
				maxAge = newMaxAge
				maxCollectionMB = newMaxCollectionMB
			}
			if timer == nil {
				timer = w.config.Clock.NewTimer(w.config.PruneInterval)
				timerCh = timer.Chan()
			}

		case <-timerCh:
			err := w.config.Facade.Prune(maxAge, int(maxCollectionMB))
			if err != nil {
				return errors.Trace(err)
			}
			timer.Reset(w.config.PruneInterval)
		}
	}
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *PrunerWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.catacomb.Context(ctx), cancel
}

func (w *PrunerWorker) addWatcher(ctx context.Context, watcher eventsource.Watcher[[]string]) error {
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	// Consume the initial events from the watchers. The watcher will
	// dispatch an initial event when it is created, so we need to consume
	// that event before we can start watching.
	if _, err := eventsource.ConsumeInitialEvent[[]string](ctx, watcher); err != nil {
		return errors.Trace(err)
	}

	return nil
}
