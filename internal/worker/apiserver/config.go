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

func getLogSinkConfig(cfg agent.Config) (apiserver.LogSinkConfig, error) {
	result := apiserver.DefaultLogSinkConfig()
	var err error
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
