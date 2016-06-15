// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logstream

import (
	"io"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common/stream"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/logfwd"
)

// jsonReadCloser provides the functionality to send JSON-serialized
// values over a streaming connection.
type jsonReadCloser interface {
	io.Closer

	// ReadJSON decodes the next JSON value from the connection and
	// sets the value at the provided pointer to that newly decoded one.
	ReadJSON(interface{}) error
}

// LogStream streams log entries of the /logstream API endpoint over
// a websocket connection.
type LogStream struct {
	mu             sync.Mutex
	stream         jsonReadCloser
	controllerUUID string
}

// Open opens a websocket to the API's /logstream endpoint and returns
// a stream of log records from that connection.
func Open(conn base.StreamConnector, cfg params.LogStreamConfig, controllerUUID string) (*LogStream, error) {
	wsStream, err := stream.Open(conn, "/logstream", &cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ls := &LogStream{
		stream:         wsStream,
		controllerUUID: controllerUUID,
	}
	return ls, nil
}

// Next returns the next log record from the server. The records are
// coverted from the wire format into logfwd.Record.
func (ls *LogStream) Next() (logfwd.Record, error) {
	var record logfwd.Record
	apiRecord, err := ls.next()
	if err != nil {
		return record, errors.Trace(err)
	}
	record, err = api2record(apiRecord, ls.controllerUUID)
	if err != nil {
		return record, errors.Trace(err)
	}
	return record, nil
}

func (ls *LogStream) next() (params.LogStreamRecord, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	var apiRec params.LogStreamRecord
	if ls.stream == nil {
		return apiRec, errors.Errorf("cannot read from closed stream")
	}

	err := ls.stream.ReadJSON(&apiRec)
	if err != nil {
		return apiRec, errors.Trace(err)
	}
	return apiRec, nil
}

// Close closes the stream.
func (ls *LogStream) Close() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.stream == nil {
		return nil
	}
	if err := ls.stream.Close(); err != nil {
		return errors.Trace(err)
	}
	ls.stream = nil
	return nil
}

// See the counterpart in apiserver/logstream.go.
func api2record(apiRec params.LogStreamRecord, controllerUUID string) (logfwd.Record, error) {
	rec := logfwd.Record{
		Origin: logfwd.Origin{
			ControllerUUID: controllerUUID,
			ModelUUID:      apiRec.ModelUUID,
		},
		Timestamp: apiRec.Timestamp,
		Message:   apiRec.Message,
	}

	ver, err := version.Parse(apiRec.Version)
	if err != nil {
		return rec, errors.Annotatef(err, "invalid version %q", apiRec.Version)
	}
	rec.Origin.JujuVersion = ver

	loc, err := logfwd.ParseLocation(apiRec.Module, apiRec.Location)
	if err != nil {
		return rec, errors.Trace(err)
	}
	rec.Location = loc

	level, ok := loggo.ParseLevel(apiRec.Level)
	if !ok {
		return rec, errors.Errorf("unrecognized log level %q", apiRec.Level)
	}
	rec.Level = level

	if err := rec.Validate(); err != nil {
		return rec, errors.Trace(err)
	}

	return rec, nil
}
