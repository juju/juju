package pruner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
)

// Config holds all necessary attributes to start a pruner worker.
type Config struct {
	Facade        Facade
	PruneInterval time.Duration
	Clock         clock.Clock
}

// Validate will err unless basic requirements for a valid
// config are met.
func (c *Config) Validate() error {
	if c.Facade == nil {
		return errors.New("missing Facade")
	}
	if c.Clock == nil {
		return errors.New("missing Clock")
	}
	return nil
}

// New returns a worker.Worker for history Pruner.
func New(conf Config) PrunerWorker {
	return PrunerWorker{
		config: conf,
	}
}
