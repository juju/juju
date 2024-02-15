// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"strconv"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
)

const (
	defaultLoggerBufferSize    = 1000
	defaultLoggerFlushInterval = 2 * time.Second
)

func getLogSinkConfig(cfg agent.Config) (LogSinkConfig, error) {
	result := DefaultLogSinkConfig()
	var err error
	// TODO(debug-log) - these attributes are used for the mongo logger
	if v := cfg.Value(agent.LogSinkDBLoggerBufferSize); v != "" {
		result.LoggerBufferSize, err = strconv.Atoi(v)
		if err != nil {
			return result, errors.Annotatef(
				err, "parsing %s", agent.LogSinkDBLoggerBufferSize,
			)
		}
	}
	if v := cfg.Value(agent.LogSinkDBLoggerFlushInterval); v != "" {
		if result.LoggerFlushInterval, err = time.ParseDuration(v); err != nil {
			return result, errors.Annotatef(
				err, "parsing %s", agent.LogSinkDBLoggerFlushInterval,
			)
		}
	}
	return result, result.Validate()
}

// LogSinkConfig holds parameters to control the log sink's behaviour.
type LogSinkConfig struct {
	// LoggerBufferSize is the capacity of the log sink logger's buffer.
	LoggerBufferSize int

	// LoggerFlushInterval is the amount of time to allow a log record
	// to sit in the buffer before being flushed to the destination logger.
	LoggerFlushInterval time.Duration
}

// Validate validates the logsink endpoint configuration.
func (cfg LogSinkConfig) Validate() error {
	if cfg.LoggerBufferSize <= 0 || cfg.LoggerBufferSize > 1000 {
		return errors.NotValidf("LoggerBufferSize %d <= 0 or > 1000", cfg.LoggerBufferSize)
	}
	if cfg.LoggerFlushInterval <= 0 || cfg.LoggerFlushInterval > 10*time.Second {
		return errors.NotValidf("LoggerFlushInterval %s <= 0 or > 10 seconds", cfg.LoggerFlushInterval)
	}
	return nil
}

// DefaultLogSinkConfig returns a LogSinkConfig with default values.
func DefaultLogSinkConfig() LogSinkConfig {
	return LogSinkConfig{
		LoggerBufferSize:    defaultLoggerBufferSize,
		LoggerFlushInterval: defaultLoggerFlushInterval,
	}
}
