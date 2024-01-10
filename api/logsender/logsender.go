// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	"context"
	"io"
	"net/url"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// LogWriter is the interface that allows sending log
// messages to the server for storage.
type LogWriter interface {
	// WriteLog writes the given log record.
	WriteLog(*params.LogRecord) error

	io.Closer
}

// API provides access to the LogSender API.
type API struct {
	connector base.StreamConnector
}

// NewAPI creates a new client-side logsender API.
func NewAPI(connector base.StreamConnector) *API {
	return &API{connector: connector}
}

// LogWriter returns a new log writer interface value
// which must be closed when finished with.
func (api *API) LogWriter(ctx context.Context) (LogWriter, error) {
	attrs := make(url.Values)
	// Version 1 does ping/pong handling.
	attrs.Set("version", "1")
	conn, err := api.connector.ConnectStream(ctx, "/logsink", attrs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot connect to /logsink")
	}
	logWriter := newWriter(conn)
	return logWriter, nil
}

type writer struct {
	conn     base.Stream
	readErrs chan error
}

func newWriter(conn base.Stream) *writer {
	w := &writer{
		conn:     conn,
		readErrs: make(chan error, 1),
	}

	go w.readLoop()
	return w
}

// readLoop is necessary for the client to process websocket control messages.
// If we get an error, enqueue it so that if a subsequent call to WriteLog
// fails do to our closure of the socket, we can enhance the resulting error.
// Close() is safe to call concurrently.
func (w *writer) readLoop() {
	for {
		if _, _, err := w.conn.NextReader(); err != nil {
			select {
			case w.readErrs <- err:
			default:
			}

			_ = w.conn.Close()
			break
		}
	}
}

// WriteLog streams the log record as JSON to the logsink endpoint.
// Upon error, check to see if there is an enqueued read error that
// we can use to enhance the output.
func (w *writer) WriteLog(m *params.LogRecord) error {
	// Note: due to the fire-and-forget nature of the logsink API,
	// it is possible that when the connection dies, any logs that
	// were "in-flight" will not be recorded on the server side.
	if err := w.conn.WriteJSON(m); err != nil {
		var readErr error
		select {
		case readErr, _ = <-w.readErrs:
		default:
		}

		if readErr != nil {
			err = errors.Annotate(err, readErr.Error())
		}
		return errors.Annotate(err, "sending log message")
	}
	return nil
}

func (w writer) Close() error {
	return w.conn.Close()
}
