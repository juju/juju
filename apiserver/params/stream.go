// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/juju/errors"
)

// StreamFormat is the "enum" for supported stream formats.
type StreamFormat string

// These are the supported log stream formats.
const (
	StreamFormatRaw  StreamFormat = ""
	StreamFormatJSON              = "json"
)

// StreamConfig holds common information needed to open a streaming
// connection to an API endpoint for reading.
type StreamConfig struct {
	// Format identifies the way in which the streamed data
	// should be formatted.
	Format StreamFormat
}

// Apply adjusts the provided query to match the config.
func (cfg StreamConfig) Apply(query url.Values) {
	if cfg.Format != "" {
		query.Set("format", string(cfg.Format))
	}
}

// GetStreamConfig extracts the generic stream config from the provided
// URL query.
func GetStreamConfig(query url.Values) (StreamConfig, error) {
	var cfg StreamConfig

	switch query.Get("format") {
	case string(StreamFormatRaw):
		cfg.Format = StreamFormatRaw
	case string(StreamFormatJSON):
		cfg.Format = StreamFormatJSON
	default:
		return cfg, errors.Errorf("unsupported stream format %q", query.Get("format"))
	}

	return cfg, nil
}

// TODO(ericsnow) At some point it would make sense to merge this code
// with Client.WatchDebugLog().

// LogStreamConfig holds all the information necessary to open a
// streaming connection to the API endpoint for reading log records.
type LogStreamConfig struct {
	StreamConfig

	// AllModels indicates whether logs for all the controller's models
	// should be included or just those of the connection's model.
	AllModels bool

	// StartTime, if set, determines where in the log history that
	// streaming should start.
	StartTime time.Time
}

// Endpoint returns the API endpoint path to use for log streaming.
func (cfg LogStreamConfig) Endpoint() string {
	return "/log"
}

// URLQuery converts the config into a URL query suitable for use when
// connecting to an API endpoint stream.
//func (cfg LogStreamConfig) URLQuery(format StreamFormat) url.Values {
//	query := make(url.Values)
//
//	return query
//}

// Apply adjusts the provided query to match the config.
func (cfg LogStreamConfig) Apply(query url.Values) {
	cfg.StreamConfig.Apply(query)

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

	basecfg, err := GetStreamConfig(query)
	if err != nil {
		return cfg, errors.Trace(err)
	}
	cfg.StreamConfig = basecfg

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
