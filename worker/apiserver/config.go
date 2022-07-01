// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"strconv"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/v3/agent"
	"github.com/juju/juju/v3/apiserver"
)

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
