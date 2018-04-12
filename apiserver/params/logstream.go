// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"
)

// LogStreamRecord contains a slice of LogStreamRecords.
type LogStreamRecords struct {
	Records []LogStreamRecord `json:"records"`
}

// LogStreamRecord describes a single log record being streamed from
// the server.
type LogStreamRecord struct {
	ID        int64     `json:"id"`
	ModelUUID string    `json:"mid"`
	Entity    string    `json:"ent"`
	Version   string    `json:"ver,omitempty"`
	Timestamp time.Time `json:"ts"`
	Module    string    `json:"mod"`
	Location  string    `json:"lo"`
	Level     string    `json:"lv"`
	Message   string    `json:"msg"`
}

// LogStreamConfig holds all the information necessary to open a
// streaming connection to the API endpoint for reading log records.
//
// The field tags relate to the following 2 libraries:
//   github.com/google/go-querystring/query (encoding)
//   github.com/gorilla/schema (decoding)
//
// See apiserver/debuglog.go:debugLogParams for additional things we
// may consider supporting here.
type LogStreamConfig struct {
	// Sink identifies the target to which log records will be streamed.
	// This is used as a bookmark for where to start the next time logs
	// are streamed for the same sink.
	Sink string `schema:"sink" url:"sink,omitempty"`

	// MaxLookbackDuration is the maximum time duration from the past to stream.
	// It must be a valid time duration string.
	MaxLookbackDuration string `schema:"maxlookbackduration" url:"maxlookbackduration,omitempty"`

	// MaxLookbackRecords is the maximum number of log records to stream from the past.
	MaxLookbackRecords int `schema:"maxlookbackrecords" url:"maxlookbackrecords,omitempty"`
}
