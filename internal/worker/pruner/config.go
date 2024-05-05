// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pruner

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
)

// Logger defines the methods used by the pruner worker for logging.
type Logger interface {
	Infof(string, ...interface{})
}

// Config holds all necessary attributes to start a pruner worker.
type Config struct {
	Facade             Facade
	ModelConfigService ModelConfigService
	PruneInterval      time.Duration
	Clock              clock.Clock
	Logger             Logger
}

// Validate will err unless basic requirements for a valid
// config are met.
func (c *Config) Validate() error {
	if c.Facade == nil {
		return errors.New("missing Facade")
	}
	if c.ModelConfigService == nil {
		return errors.New("missing ModelConfigService")
	}
	if c.Clock == nil {
		return errors.New("missing Clock")
	}
	if c.Logger == nil {
		return errors.New("missing Logger")
	}
	return nil
}

// New returns a worker.Worker for history Pruner.
func New(conf Config) PrunerWorker {
	return PrunerWorker{
		config: conf,
	}
}
