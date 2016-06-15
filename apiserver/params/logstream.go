// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"
)

// LogStreamRecord describes a single log record being streamed from
// the server.
type LogStreamRecord struct {
	ModelUUID string    `json:"id"`
	Version   string    `json:"ver"`
	Timestamp time.Time `json:"ts"`
	Module    string    `json:"mod"`
	Location  string    `json:"lo"`
	Level     string    `json:"lv"`
	Message   string    `json:"msg"`
}

// TODO(ericsnow) At some point it would make sense to merge this code
// with Client.WatchDebugLog().

// LogStreamConfig holds all the information necessary to open a
// streaming connection to the API endpoint for reading log records.
//
// The field tags relate to the following 2 libraries:
//   github.com/google/go-querystring/query (encoding)
//   github.com/gorilla/schema (decoding)
type LogStreamConfig struct {
	// AllModels indicates whether logs for all the controller's models
	// should be included or just those of the connection's model.
	AllModels bool `schema:"all" url:"all,omitempty"`

	// StartTime, if set, determines where in the log history that
	// streaming should start.
	StartTime time.Time `schema:"start" url:"start,omitempty"`
}
