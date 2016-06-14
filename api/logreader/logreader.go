// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logreader

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common/stream"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/logfwd"
	"github.com/juju/juju/version"
)

// JSONReadCloser provides the functionality to send JSON-serialized
// values over a streaming connection.
type JSONReadCloser interface {
	io.Closer

	// ReadJSON decodes the next JSON value from the connection and
	// sets the value at the provided pointer to that newly decoded one.
	ReadJSON(interface{}) error
}

// LogRecordReader is a worker that provides log records it gets over
// a streaming connection. After getting each record, it converts them
// from params.LogRecord to logfwd.Record. These are then available
// through the reader's channel.
type LogRecordReader struct {
	tomb tomb.Tomb

	conn           JSONReadCloser
	out            chan logfwd.Record
	controllerUUID string
}

// OpenLogRecordReader opens a stream to the API's /log endpoint and
// returns a reader that wraps that stream.
//
// Note that the caller is responsible for stopping the reader, e.g. by
// passing it to worker.Stop().
func OpenLogRecordReader(conn base.StreamConnector, cfg params.LogStreamConfig, controllerUUID string) (*LogRecordReader, error) {
	wsStream, err := stream.Open(conn, cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	lrr := newLogRecordReader(wsStream, controllerUUID)
	return lrr, nil
}

// newLogRecordReader starts a new reader and returns it. The provided
// connection is the one from which the reader will stream log records.
func newLogRecordReader(conn JSONReadCloser, controllerUUID string) *LogRecordReader {
	out := make(chan logfwd.Record)
	lrr := &LogRecordReader{
		conn:           conn,
		out:            out,
		controllerUUID: controllerUUID,
	}
	go func() {
		defer lrr.tomb.Done()
		defer close(lrr.out)
		defer lrr.conn.Close()
		lrr.tomb.Kill(lrr.loop())
	}()
	return lrr
}

// Channel returns a channel that can be used to receive log records.
func (lrr *LogRecordReader) Channel() <-chan logfwd.Record {
	return lrr.out
}

func (lrr *LogRecordReader) loop() error {
	for {
		var apiRecord params.LogStreamRecord
		err := lrr.conn.ReadJSON(&apiRecord)
		if err != nil {
			return err
		}

		record, err := api2record(apiRecord, lrr.controllerUUID)
		if err != nil {
			return errors.Trace(err)
		}

		select {
		case <-lrr.tomb.Dying():
			return tomb.ErrDying
		case lrr.out <- record:
		}
	}
}

// Kill implements Worker.Kill()
func (lrr *LogRecordReader) Kill() {
	lrr.tomb.Kill(nil)
}

// Wait implements Worker.Wait()
func (lrr *LogRecordReader) Wait() error {
	return lrr.tomb.Wait()
}

// See the counterpart in apiserver/logstream.go.
func api2record(apiRec params.LogStreamRecord, controllerUUID string) (logfwd.Record, error) {
	rec := logfwd.Record{
		Origin: logfwd.Origin{
			ControllerUUID: controllerUUID,
			ModelUUID:      apiRec.ModelUUID,
			JujuVersion:    version.Current,
		},
		Timestamp: apiRec.Timestamp,
		Message:   apiRec.Message,
	}

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
