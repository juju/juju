// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

// LogStreamRecord describes a single log record being streamed from
// the server.
//
// Single character field names are used for serialisation to keep the
// size down. These messages are going to be sent a lot.
type LogStreamRecord struct {
	ModelUUID string    `json:"o"`
	Time      time.Time `json:"t"`
	Module    string    `json:"m"`
	Location  string    `json:"l"`
	Level     string    `json:"v"`
	Message   string    `json:"x"`
}

// LoggoLevel converts the level string to a loggo.Level.
func (rec LogStreamRecord) LoggoLevel() loggo.Level {
	level, ok := loggo.ParseLevel(rec.Level)
	if !ok {
		return loggo.UNSPECIFIED
	}
	return level
}

// TODO(ericsnow) At some point it would make sense to merge this code
// with Client.WatchDebugLog().

// LogStreamConfig holds all the information necessary to open a
// streaming connection to the API endpoint for reading log records.
type LogStreamConfig struct {
	// AllModels indicates whether logs for all the controller's models
	// should be included or just those of the connection's model.
	AllModels bool

	// StartTime, if set, determines where in the log history that
	// streaming should start.
	StartTime time.Time
}

// Endpoint returns the API endpoint path to use for log streaming.
func (cfg LogStreamConfig) Endpoint() string {
	return "/logstream"
}

// Apply adjusts the provided query to match the config.
func (cfg LogStreamConfig) Apply(query url.Values) {
	if cfg.AllModels {
		query.Set("all", fmt.Sprint(true))
	}

	if !cfg.StartTime.IsZero() {
		query.Set("start", fmt.Sprint(cfg.StartTime.Unix()))
	}
}

// GetLogStreamConfig returns the config that corresponds to the
// provided URL query.
func GetLogStreamConfig(query url.Values) (LogStreamConfig, error) {
	var cfg LogStreamConfig

	if value := query.Get("all"); value != "" {
		allModels, err := strconv.ParseBool(value)
		if err != nil {
			return cfg, errors.Errorf("all value %q is not a valid boolean", value)
		}
		cfg.AllModels = allModels
	}

	if value := query.Get("start"); value != "" {
		unix, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return cfg, errors.Errorf("start value %q is not a valid unix timestamp", value)
		}
		// 1 second granularity is good enough.
		cfg.StartTime = time.Unix(int64(unix), 0)
	}

	return cfg, nil
}
