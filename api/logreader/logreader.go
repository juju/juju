// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package logreader implements the API for
// retrieving log messages from the API server.
package logreader

import (
	"fmt"
	"net/url"
	"time"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/logfwd"
	"github.com/juju/juju/version"
)

// API provides access to the LogsReader API.
type API struct {
	connector base.StreamConnector
}

// NewAPI creates a new client-side logsender client.
func NewAPI(api base.APICaller) *API {
	return &API{
		connector: api,
	}
}

// LogsReader supports reading log messages transmitted by the server.
// The caller is responsible for closing the reader.
func (api *API) LogsReader(start time.Time) (*LogsReader, error) {
	attrs := url.Values{
		"format": []string{"json"},
		"all":    []string{"true"},
	}
	if !start.IsZero() {
		attrs.Set("start", fmt.Sprint(start.Unix()))
	}

	conn, err := api.connector.ConnectStream("/log", attrs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot connect to /log")
	}
	return newLogsReader(conn), nil
}

type LogsReader struct {
	tomb tomb.Tomb

	conn base.Stream
	out  chan logfwd.Record
}

func newLogsReader(conn base.Stream) *LogsReader {
	out := make(chan logfwd.Record)
	w := &LogsReader{
		conn: conn,
		out:  out,
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		defer w.conn.Close()
		w.tomb.Kill(w.loop())
	}()
	return w
}

// ReadLogs returns a channel that can be used to receive log records.
func (r *LogsReader) ReadLogs() <-chan logfwd.Record {
	return r.out
}

func (r *LogsReader) loop() error {
	for {
		var apiRecord params.LogRecord
		err := r.conn.ReadJSON(&apiRecord)
		if err != nil {
			return err
		}

		record, err := api2record(apiRecord)
		if err != nil {
			return errors.Trace(err)
		}

		select {
		case <-r.tomb.Dying():
			return tomb.ErrDying
		case r.out <- record:
		}
	}
}

// Kill implements Worker.Kill()
func (r *LogsReader) Kill() {
	r.tomb.Kill(nil)
}

// Wait implements Worker.Wait()
func (r *LogsReader) Wait() error {
	return r.tomb.Wait()
}

func api2record(apiRec params.LogRecord) (logfwd.Record, error) {
	rec := logfwd.Record{
		Origin: logfwd.Origin{
			ControllerUUID: apiRec.ControllerUUID,
			ModelUUID:      apiRec.ModelUUID,
			JujuVersion:    version.Current,
		},
		Timestamp: apiRec.Time,
		Level:     apiRec.Level,
		Message:   apiRec.Message,
	}

	loc, err := logfwd.ParseLocation(apiRec.Module, apiRec.Location)
	if err != nil {
		return rec, errors.Trace(err)
	}
	rec.Location = loc

	if err := rec.Validate(); err != nil {
		return rec, errors.Trace(err)
	}

	return rec, nil
}
