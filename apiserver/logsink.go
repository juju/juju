// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func newLogSinkHandler(h httpHandler, logDir string) http.Handler {

	logPath := filepath.Join(logDir, "logsink.log")
	if err := primeLogFile(logPath); err != nil {
		// This isn't a fatal error so log and continue if priming
		// fails.
		logger.Errorf("Unable to prime %s (proceeding anyway): %v", logPath, err)
	}

	return &logSinkHandler{
		httpHandler: h,
		fileLogger: &lumberjack.Logger{
			Filename:   logPath,
			MaxSize:    500, // MB
			MaxBackups: 1,
		},
	}
}

// primeLogFile ensures the logsink log file is created with the
// correct mode and ownership.
func primeLogFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return errors.Trace(err)
	}
	f.Close()
	err = utils.ChownPath(path, "syslog")
	return errors.Trace(err)
}

type logSinkHandler struct {
	httpHandler
	fileLogger io.WriteCloser
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

			st := stateWrapper.state
			filePrefix := st.EnvironUUID() + " " + tag.String() + ":"
			dbLogger := state.NewDbLogger(st, tag)
			defer dbLogger.Close()
			m := new(LogMessage)
			for {
				if err := websocket.JSON.Receive(socket, m); err != nil {
					if err != io.EOF {
						logger.Errorf("error while receiving logs: %v", err)
					}
					break
				}

				fileErr := h.logToFile(filePrefix, m)
				if fileErr != nil {
					logger.Errorf("logging to logsink.log failed: %v", fileErr)
				}

				dbErr := dbLogger.Log(m.Time, m.Module, m.Location, m.Level, m.Message)
				if dbErr != nil {
					logger.Errorf("logging to DB failed: %v", err)
				}

				if fileErr != nil || dbErr != nil {
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

// logToFile writes a single log message to the logsink log file.
func (h *logSinkHandler) logToFile(prefix string, m *LogMessage) error {
	_, err := h.fileLogger.Write([]byte(strings.Join([]string{
		prefix,
		m.Time.In(time.UTC).Format("2006-01-02 15:04:05"),
		m.Level.String(),
		m.Module,
		m.Location,
		m.Message,
	}, " ") + "\n"))
	return err
}
