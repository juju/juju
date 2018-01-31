// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/core/auditlog"
)

// These vars define how we rate limit incoming connections.
const (
	defaultLoginRateLimit         = 10 // concurrent login operations
	defaultLoginMinPause          = 100 * time.Millisecond
	defaultLoginMaxPause          = 1 * time.Second
	defaultLoginRetryPause        = 5 * time.Second
	defaultConnMinPause           = 0 * time.Millisecond
	defaultConnMaxPause           = 5 * time.Second
	defaultConnLookbackWindow     = 1 * time.Second
	defaultConnLowerThreshold     = 1000   // connections per second
	defaultConnUpperThreshold     = 100000 // connections per second
	defaultLogSinkRateLimitBurst  = 1000
	defaultLogSinkRateLimitRefill = time.Millisecond
)

// RateLimitConfig holds parameters to control
// aspects of rate limiting connections and logins.
type RateLimitConfig struct {
	LoginRateLimit     int
	LoginMinPause      time.Duration
	LoginMaxPause      time.Duration
	LoginRetryPause    time.Duration
	ConnMinPause       time.Duration
	ConnMaxPause       time.Duration
	ConnLookbackWindow time.Duration
	ConnLowerThreshold int
	ConnUpperThreshold int
}

// DefaultRateLimitConfig returns a RateLimtConfig struct with
// all attributes set to their default values.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		LoginRateLimit:     defaultLoginRateLimit,
		LoginMinPause:      defaultLoginMinPause,
		LoginMaxPause:      defaultLoginMaxPause,
		LoginRetryPause:    defaultLoginRetryPause,
		ConnMinPause:       defaultConnMinPause,
		ConnMaxPause:       defaultConnMaxPause,
		ConnLookbackWindow: defaultConnLookbackWindow,
		ConnLowerThreshold: defaultConnLowerThreshold,
		ConnUpperThreshold: defaultConnUpperThreshold,
	}
}

// Validate validates the rate limit configuration.
// We apply arbitrary but sensible upper limits to prevent
// typos from introducing obviously bad config.
func (c RateLimitConfig) Validate() error {
	if c.LoginRateLimit <= 0 || c.LoginRateLimit > 100 {
		return errors.NotValidf("login-rate-limit %d <= 0 or > 100", c.LoginRateLimit)
	}
	if c.LoginMinPause < 0 || c.LoginMinPause > 100*time.Millisecond {
		return errors.NotValidf("login-min-pause %d < 0 or > 100ms", c.LoginMinPause)
	}
	if c.LoginMaxPause < 0 || c.LoginMaxPause > 5*time.Second {
		return errors.NotValidf("login-max-pause %d < 0 or > 5s", c.LoginMaxPause)
	}
	if c.LoginRetryPause < 0 || c.LoginRetryPause > 10*time.Second {
		return errors.NotValidf("login-retry-pause %d < 0 or > 10s", c.LoginRetryPause)
	}
	if c.ConnMinPause < 0 || c.ConnMinPause > 100*time.Millisecond {
		return errors.NotValidf("conn-min-pause %d < 0 or > 100ms", c.ConnMinPause)
	}
	if c.ConnMaxPause < 0 || c.ConnMaxPause > 10*time.Second {
		return errors.NotValidf("conn-max-pause %d < 0 or > 10s", c.ConnMaxPause)
	}
	if c.ConnLookbackWindow < 0 || c.ConnLookbackWindow > 5*time.Second {
		return errors.NotValidf("conn-lookback-window %d < 0 or > 5s", c.ConnMaxPause)
	}
	return nil
}

// AuditLogConfig holds parameters to control audit logging.
type AuditLogConfig struct {
	Enabled bool

	// CaptureAPIArgs says whether to capture API method args (command
	// line args will always be captured).
	CaptureAPIArgs bool

	// MaxSizeMB defines the maximum log file size.
	MaxSizeMB int

	// MaxBackups determines how many files back to keep.
	MaxBackups int

	// ExcludeMethods is a set of facade.method names that we
	// shouldn't consider to be interesting: if a conversation only
	// consists of these method calls we won't log it.
	ExcludeMethods set.Strings

	// Target is the AuditLog entries should be written to.
	Target auditlog.AuditLog
}

// Validate checks the audit logging configuration.
func (cfg AuditLogConfig) Validate() error {
	if cfg.Enabled && cfg.Target == nil {
		return errors.NewNotValid(nil, "logging enabled but no target provided")
	}
	return nil
}

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
		DBLoggerBufferSize:    defaultDBLoggerBufferSize,
		DBLoggerFlushInterval: defaultDBLoggerFlushInterval,
		RateLimitBurst:        defaultLogSinkRateLimitBurst,
		RateLimitRefill:       defaultLogSinkRateLimitRefill,
	}
}
