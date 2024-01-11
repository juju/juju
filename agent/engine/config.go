// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent/errors"
)

// EngineErrorDelay is the amount of time the dependency engine waits
// between getting an error from a worker, and restarting it. It is exposed
// here so tests can make it smaller.
var EngineErrorDelay = 3 * time.Second

// DependencyEngineConfig returns a dependency engine config.
func DependencyEngineConfig(metrics dependency.Metrics, logger dependency.Logger) dependency.EngineConfig {
	return dependency.EngineConfig{
		IsFatal:          errors.IsFatal,
		WorstError:       errors.MoreImportantError,
		ErrorDelay:       EngineErrorDelay,
		BounceDelay:      10 * time.Millisecond,
		BackoffFactor:    1.2,
		BackoffResetTime: 1 * time.Minute,
		MaxDelay:         2 * time.Minute,
		Clock:            clock.WallClock,
		Metrics:          metrics,
		Logger:           logger,
	}
}
