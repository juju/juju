// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"golang.org/x/net/websocket"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func newLogSinkHandler(h httpContext, logDir string) http.Handler {

	logPath := filepath.Join(logDir, "logsink.log")
	if err := primeLogFile(logPath); err != nil {
		// This isn't a fatal error so log and continue if priming
		// fails.
		logger.Errorf("Unable to prime %s (proceeding anyway): %v", logPath, err)
	}

	return &logSinkHandler{
		ctxt: h,
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
	ctxt       httpContext
	fileLogger io.WriteCloser
}

// ServeHTTP implements the http.Handler interface.
func (h *logSinkHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	server := websocket.Server{
		Handler: func(socket *websocket.Conn) {
			defer socket.Close()
			st, entity, err := h.ctxt.stateForRequestAuthenticatedAgent(req)
			if err != nil {
				h.sendError(socket, req, err)
				return
			}
			tag := entity.Tag()

			// If we get to here, no more errors to report, so we report a nil
			// error.  This way the first line of the socket is always a json
			// formatted simple error.
			h.sendError(socket, req, nil)

			filePrefix := st.EnvironUUID() + " " + tag.String() + ":"
			dbLogger := state.NewDbLogger(st, tag)
			defer dbLogger.Close()
			m := new(params.LogRecord)
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
func (h *logSinkHandler) sendError(w io.Writer, req *http.Request, err error) {
	if err != nil {
		logger.Errorf("returning error from %s %s: %s", req.Method, req.URL.Path, errors.Details(err))
	}
	sendJSON(w, &params.ErrorResult{
		Error: common.ServerError(err),
	})
}

// logToFile writes a single log message to the logsink log file.
func (h *logSinkHandler) logToFile(prefix string, m *params.LogRecord) error {
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
