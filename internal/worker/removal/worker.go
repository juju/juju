// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import (
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
)

// Config holds configuration required to run the removal worker.
type Config struct {

	// RemovalService supplies the removal domain logic to the worker.
	RemovalService RemovalService

	// Logger logs stuff.
	Logger logger.Logger
}

// Validate ensures that the configuration is
// correctly populated for worker operation.
func (config Config) Validate() error {
	if config.RemovalService == nil {
		return errors.New("nil RemovalService not valid").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.New("nil Logger not valid").Add(coreerrors.NotValid)
	}
	return nil
}

type removalWorker struct {
	catacomb catacomb.Catacomb

	cfg Config
}

// NewWorker starts a new removal worker based
// on the input configuration and returns it.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	w := &removalWorker{
		cfg: config,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

func (w *removalWorker) loop() (err error) {
	<-w.catacomb.Dying()
	return w.catacomb.Err()
}

// Kill (worker.Worker) tells the worker to stop and return from its loop.
func (w *removalWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait (worker.Worker) waits for the worker to stop,
// and returns the error with which it exited.
func (w *removalWorker) Wait() error {
	return w.catacomb.Wait()
}
