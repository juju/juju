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
	// DBLoggerBufferSize is the capacity of the database logger's buffer.
	DBLoggerBufferSize int

	// DBLoggerFlushInterval is the amount of time to allow a log record
	// to sit in the buffer before being flushed to the database.
	DBLoggerFlushInterval time.Duration

	// RateLimitBurst defines the number of log messages that will be let
	// through before we start rate limiting.
	RateLimitBurst int64

	// RateLimitRefill defines the rate at which log messages will be let
	// through once the initial burst amount has been depleted.
	RateLimitRefill time.Duration
}

// Validate validates the logsink endpoint configuration.
func (cfg LogSinkConfig) Validate() error {
	if cfg.DBLoggerBufferSize <= 0 || cfg.DBLoggerBufferSize > 1000 {
		return errors.NotValidf("DBLoggerBufferSize %d <= 0 or > 1000", cfg.DBLoggerBufferSize)
	}
	if cfg.DBLoggerFlushInterval <= 0 || cfg.DBLoggerFlushInterval > 10*time.Second {
		return errors.NotValidf("DBLoggerFlushInterval %s <= 0 or > 10 seconds", cfg.DBLoggerFlushInterval)
	}
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
		DBLoggerBufferSize:    defaultLoggerBufferSize,
		DBLoggerFlushInterval: defaultLoggerFlushInterval,
		RateLimitBurst:        defaultLogSinkRateLimitBurst,
		RateLimitRefill:       defaultLogSinkRateLimitRefill,
	}
}
