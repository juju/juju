// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logstream

import (
	"io"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
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

// Next returns the next batch of log records from the server. The records are
// converted from the wire format into logfwd.Record. The first returned
// record will be the one after the last successfully sent record. If no
// records have been sent yet then it will be the oldest log record.
//
// An error indicates either the streaming connection is closed, the
// connection failed, or the data read from the connection is corrupted.
// In each of these cases the stream should be re-opened. It will start
// at the record after the one marked as successfully sent. So the
// the record at which Next() failed previously will be streamed again.
//
// That is only a problem when the same record is consistently streamed
// as invalid data. This will happen only if the record was invalid
// before being stored in the DB or if the DB on-disk storage for the
// record becomes corrupted. Both scenarios are highly unlikely and
// the respective systems are managed such that neither should happen.
func (ls *LogStream) Next() ([]logfwd.Record, error) {
	apiRecords, err := ls.next()
	if err != nil {
		return nil, errors.Trace(err)
	}
	records, err := recordsFromAPI(apiRecords, ls.controllerUUID)
	if err != nil {
		// This should only happen if the data got corrupted over the
		// network. Any other cause should be addressed by fixing the
		// code that resulted in the bad data that caused the error
		// here. If that code is between the DB on-disk storage and
		// the server-side stream then care should be taken to not
		// block on a consistently invalid record or to throw away
		// a record. The log stream needs to maintain a high level
		// of reliable delivery.
		return nil, errors.Trace(err)
	}
	return records, nil
}

func (ls *LogStream) next() (params.LogStreamRecords, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	var result params.LogStreamRecords
	if ls.stream == nil {
		return result, errors.Errorf("cannot read from closed stream")
	}

	err := ls.stream.ReadJSON(&result)
	if err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
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
func recordsFromAPI(apiRecords params.LogStreamRecords, controllerUUID string) ([]logfwd.Record, error) {
	result := make([]logfwd.Record, len(apiRecords.Records))
	for i, apiRec := range apiRecords.Records {
		rec, err := recordFromAPI(apiRec, controllerUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result[i] = rec
	}
	return result, nil
}

func recordFromAPI(apiRec params.LogStreamRecord, controllerUUID string) (logfwd.Record, error) {
	rec := logfwd.Record{
		ID:        apiRec.ID,
		Timestamp: apiRec.Timestamp,
		Message:   apiRec.Message,
	}

	origin, err := originFromAPI(apiRec, controllerUUID)
	if err != nil {
		return rec, errors.Trace(err)
	}
	rec.Origin = origin

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

func originFromAPI(apiRec params.LogStreamRecord, controllerUUID string) (logfwd.Origin, error) {
	var origin logfwd.Origin

	tag, err := names.ParseTag(apiRec.Entity)
	if err != nil {
		return origin, errors.Annotate(err, "invalid entity")
	}

	ver, err := version.Parse(apiRec.Version)
	if err != nil {
		return origin, errors.Annotatef(err, "invalid version %q", apiRec.Version)
	}

	switch tag := tag.(type) {
	case names.MachineTag:
		origin = logfwd.OriginForMachineAgent(tag, controllerUUID, apiRec.ModelUUID, ver)
	case names.UnitTag:
		origin = logfwd.OriginForUnitAgent(tag, controllerUUID, apiRec.ModelUUID, ver)
	default:
		origin, err = logfwd.OriginForJuju(tag, controllerUUID, apiRec.ModelUUID, ver)
		if err != nil {
			return origin, errors.Annotate(err, "could not extract origin")
		}
	}
	return origin, nil
}
