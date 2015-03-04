// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"code.google.com/p/go.net/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type logSinkHandler struct {
	httpHandler
	st *state.State
}

// LogMessage is used to transmit log messages to the logsink API
// endpoint.  Single character field names are used for serialisation
// to keep the size down. These messages are going to be sent a lot.
type LogMessage struct {
	Time     time.Time   `json:"t"`
	Module   string      `json:"m"`
	Location string      `json:"l"`
	Level    loggo.Level `json:"v"`
	Message  string      `json:"x"`
}

// ServeHTTP implements the http.Handler interface.
func (h *logSinkHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	server := websocket.Server{
		Handler: func(socket *websocket.Conn) {
			defer socket.Close()
			// Validate before authenticate because the authentication is
			// dependent on the state connection that is determined during the
			// validation.
			stateWrapper, err := h.validateEnvironUUID(req)
			if err != nil {
				if errErr := h.sendError(socket, err); errErr != nil {
					// Log at DEBUG so that in a standard environment
					// logs cant't fill up with auth errors for
					// unauthenticated connections.
					logger.Debugf("error sending logsink error: %v", errErr)
				}
				return
			}
			defer stateWrapper.cleanup()

			tag, err := stateWrapper.authenticateAgent(req)
			if err != nil {
				if errErr := h.sendError(socket, errors.Errorf("auth failed: %v", err)); errErr != nil {
					// DEBUG used as above.
					logger.Debugf("error sending logsink error: %v", errErr)
				}
				return
			}

			// If we get to here, no more errors to report, so we report a nil
			// error.  This way the first line of the socket is always a json
			// formatted simple error.
			if err := h.sendError(socket, nil); err != nil {
				logger.Errorf("failed to send nil error at start of connection")
				return
			}

			dbLogger := state.NewDbLogger(stateWrapper.state, tag)
			defer dbLogger.Close()
			var m LogMessage
			for {
				if err := websocket.JSON.Receive(socket, &m); err != nil {
					if err != io.EOF {
						logger.Errorf("error while receiving logs: %v", err)
					}
					break
				}
				if err := dbLogger.Log(m.Time, m.Module, m.Location, m.Level, m.Message); err != nil {
					logger.Errorf("logging to DB failed: %v", err)
					break
				}
			}
		}}
	server.ServeHTTP(w, req)
}

// sendError sends a JSON-encoded error response.
func (h *logSinkHandler) sendError(w io.Writer, err error) error {
	response := &params.ErrorResult{}
	if err != nil {
		response.Error = &params.Error{Message: err.Error()}
	}
	message, err := json.Marshal(response)
	if err != nil {
		// If we are having trouble marshalling the error, we are in big trouble.
		logger.Errorf("failure to marshal SimpleError: %v", err)
		return errors.Trace(err)
	}
	message = append(message, []byte("\n")...)
	_, err = w.Write(message)
	return errors.Trace(err)
}
