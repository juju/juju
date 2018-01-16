// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"strconv"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver"
)

func getRateLimitConfig(cfg agent.Config) (apiserver.RateLimitConfig, error) {
	result := apiserver.DefaultRateLimitConfig()
	if v := cfg.Value(agent.AgentLoginRateLimit); v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			return apiserver.RateLimitConfig{}, errors.Annotatef(
				err, "parsing %s", agent.AgentLoginRateLimit,
			)
		}
		result.LoginRateLimit = val
	}
	if v := cfg.Value(agent.AgentLoginMinPause); v != "" {
		val, err := time.ParseDuration(v)
		if err != nil {
			return apiserver.RateLimitConfig{}, errors.Annotatef(
				err, "parsing %s", agent.AgentLoginMinPause,
			)
		}
		result.LoginMinPause = val
	}
	if v := cfg.Value(agent.AgentLoginMaxPause); v != "" {
		val, err := time.ParseDuration(v)
		if err != nil {
			return apiserver.RateLimitConfig{}, errors.Annotatef(
				err, "parsing %s", agent.AgentLoginMaxPause,
			)
		}
		result.LoginMaxPause = val
	}
	if v := cfg.Value(agent.AgentLoginRetryPause); v != "" {
		val, err := time.ParseDuration(v)
		if err != nil {
			return apiserver.RateLimitConfig{}, errors.Annotatef(
				err, "parsing %s", agent.AgentLoginRetryPause,
			)
		}
		result.LoginRetryPause = val
	}
	if v := cfg.Value(agent.AgentConnMinPause); v != "" {
		val, err := time.ParseDuration(v)
		if err != nil {
			return apiserver.RateLimitConfig{}, errors.Annotatef(
				err, "parsing %s", agent.AgentConnMinPause,
			)
		}
		result.ConnMinPause = val
	}
	if v := cfg.Value(agent.AgentConnMaxPause); v != "" {
		val, err := time.ParseDuration(v)
		if err != nil {
			return apiserver.RateLimitConfig{}, errors.Annotatef(
				err, "parsing %s", agent.AgentConnMaxPause,
			)
		}
		result.ConnMaxPause = val
	}
	if v := cfg.Value(agent.AgentConnLookbackWindow); v != "" {
		val, err := time.ParseDuration(v)
		if err != nil {
			return apiserver.RateLimitConfig{}, errors.Annotatef(
				err, "parsing %s", agent.AgentConnLookbackWindow,
			)
		}
		result.ConnLookbackWindow = val
	}
	if v := cfg.Value(agent.AgentConnLowerThreshold); v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			return apiserver.RateLimitConfig{}, errors.Annotatef(
				err, "parsing %s", agent.AgentConnLowerThreshold,
			)
		}
		result.ConnLowerThreshold = val
	}
	if v := cfg.Value(agent.AgentConnUpperThreshold); v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			return apiserver.RateLimitConfig{}, errors.Annotatef(
				err, "parsing %s", agent.AgentConnUpperThreshold,
			)
		}
		result.ConnUpperThreshold = val
	}
	return result, nil
}

func getLogSinkConfig(cfg agent.Config) (apiserver.LogSinkConfig, error) {
	result := apiserver.DefaultLogSinkConfig()
	var err error
	if v := cfg.Value(agent.LogSinkDBLoggerBufferSize); v != "" {
		result.DBLoggerBufferSize, err = strconv.Atoi(v)
		if err != nil {
			return result, errors.Annotatef(
				err, "parsing %s", agent.LogSinkDBLoggerBufferSize,
			)
		}
	}
	if v := cfg.Value(agent.LogSinkDBLoggerFlushInterval); v != "" {
		if result.DBLoggerFlushInterval, err = time.ParseDuration(v); err != nil {
			return result, errors.Annotatef(
				err, "parsing %s", agent.LogSinkDBLoggerFlushInterval,
			)
		}
	}
	if v := cfg.Value(agent.LogSinkRateLimitBurst); v != "" {
		result.RateLimitBurst, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return result, errors.Annotatef(
				err, "parsing %s", agent.LogSinkRateLimitBurst,
			)
		}
	}
	if v := cfg.Value(agent.LogSinkRateLimitRefill); v != "" {
		result.RateLimitRefill, err = time.ParseDuration(v)
		if err != nil {
			return result, errors.Annotatef(
				err, "parsing %s", agent.LogSinkRateLimitRefill,
			)
		}
	}
	return result, nil
}
