// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/juju/errors"
)

// These vars define how we rate limit incoming connections.
const (
	defaultLogSinkRateLimitBurst  = 1000
	defaultLogSinkRateLimitRefill = time.Millisecond
)

// LogSinkConfig holds parameters to control the API server's
// logsink endpoint behaviour.
type LogSinkConfig struct {
	// RateLimitBurst defines the number of log messages that will be let
	// through before we start rate limiting.
	RateLimitBurst int64

	// RateLimitRefill defines the rate at which log messages will be let
	// through once the initial burst amount has been depleted.
	RateLimitRefill time.Duration
}

// Validate validates the logsink endpoint configuration.
func (cfg LogSinkConfig) Validate() error {
	if cfg.RateLimitBurst <= 0 {
		return errors.NotValidf("RateLimitBurst %d <= 0", cfg.RateLimitBurst)
	}
	if cfg.RateLimitRefill <= 0 {
		return errors.NotValidf("RateLimitRefill %s <= 0", cfg.RateLimitRefill)
	}
	return nil
}

// DefaultLogSinkConfig returns a LogSinkConfig with default values.
func DefaultLogSinkConfig() LogSinkConfig {
	return LogSinkConfig{
		RateLimitBurst:  defaultLogSinkRateLimitBurst,
		RateLimitRefill: defaultLogSinkRateLimitRefill,
	}
}
