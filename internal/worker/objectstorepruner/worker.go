package objectstorepruner

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
)

const (
	// defaultPruneMinInterval is the default minimum interval at which the
	// pruner will run.
	defaultPruneMinInterval = time.Minute
	// defaultPruneMaxInterval is the default maximum interval at which the
	// pruner will run.
	defaultPruneMaxInterval = time.Minute * 30
)

var (
	// backOffStrategy is the default backoff strategy used by the pruner.
	backOffStrategy = retry.ExpBackoff(defaultPruneMinInterval, defaultPruneMaxInterval, 1.5, false)
)

// WorkerConfig encapsulates the configuration options for the
// objectstore worker.
type WorkerConfig struct {
	ObjectStoreService ObjectStoreService
	ObjectStore        objectstore.ObjectStore
	Clock              clock.Clock
	Logger             logger.Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.ObjectStoreService == nil {
		return errors.NotValidf("missing ObjectStoreService")
	}
	if c.ObjectStore == nil {
		return errors.NotValidf("missing ObjectStore")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

// Pruner defines a worker that will truncate the change log.
type Pruner struct {
	tomb tomb.Tomb

	objectStore        objectstore.ObjectStore
	objectStoreService ObjectStoreService

	cfg WorkerConfig
}

// New creates a new Pruner.
func newWorker(cfg WorkerConfig) (*Pruner, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	pruner := &Pruner{
		objectStore:        cfg.ObjectStore,
		objectStoreService: cfg.ObjectStoreService,

		cfg: cfg,
	}

	pruner.tomb.Go(pruner.loop)

	return pruner, nil
}

// Kill is part of the worker.Worker interface.
func (w *Pruner) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Pruner) Wait() error {
	return w.tomb.Wait()
}

func (w *Pruner) loop() error {
	timer := w.cfg.Clock.NewTimer(defaultPruneMinInterval)
	defer timer.Stop()

	var attempts int
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case <-timer.Chan():
			// Attempt to prune, if there is any critical error, kill the
			// worker, which should force a restart.
			pruned, err := w.prune()
			if err != nil {
				return errors.Trace(err)
			}

			// If nothing was pruned, increment the attempts counter, otherwise
			// reset it. This should wind out the backoff strategy if there is
			// nothing to prune, thus reducing the frequency of the pruner.
			if pruned == 0 {
				attempts++
			} else {
				attempts = 0
			}

			timer.Reset(backOffStrategy(0, attempts))
		}
	}
}

func (w *Pruner) prune() (int, error) {
	ctx := w.tomb.Context(context.Background())

	metadata, err := w.objectStoreService.ListMetadata(ctx)
	if err != nil {
		return -1, errors.Trace(err)
	}

	files, err := w.objectStore.ListFiles(ctx)
	if err != nil {
		return -1, errors.Trace(err)
	}

}
