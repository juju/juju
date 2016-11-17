// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/version"
	"golang.org/x/net/websocket"
	"gopkg.in/juju/names.v2"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type LoggingStrategy interface {
	Authenticate(*http.Request) error
	Start()
	Log(params.LogRecord) bool
	Stop()
}

type AgentLoggingStrategy struct {
	ctxt       httpContext
	st         *state.State
	version    version.Number
	entity     names.Tag
	filePrefix string
	dbLogger   *state.EntityDbLogger
	fileLogger io.Writer
}

func (s *AgentLoggingStrategy) Authenticate(req *http.Request) error {
	st, entity, err := s.ctxt.stateForRequestAuthenticatedAgent(req)
	if err != nil {
		return errors.Trace(err)
	}
	// Note that this endpoint is agent-only. Thus the only
	// callers will necessarily provide their Juju version.
	//
	// This would be a problem if non-Juju clients (e.g. the
	// GUI) could use this endpoint since we require that the
	// *Juju* version be provided as part of the request. Any
	// attempt to open this endpoint to broader access must
	// address this caveat appropriately.
	ver, err := jujuClientVersionFromReq(req)
	if err != nil {
		return errors.Trace(err)
	}
	s.st = st
	s.version = ver
	s.entity = entity.Tag()
	return nil
}

func (s *AgentLoggingStrategy) Start() {
	s.filePrefix = s.st.ModelUUID() + " " + s.entity.String() + ":"
	s.dbLogger = state.NewEntityDbLogger(s.st, s.entity, s.version)
}

func (s *AgentLoggingStrategy) Log(m params.LogRecord) bool {
	level, _ := loggo.ParseLevel(m.Level)
	dbErr := s.dbLogger.Log(m.Time, m.Module, m.Location, level, m.Message)
	if dbErr != nil {
		logger.Errorf("logging to DB failed: %v", dbErr)
	}
	fileErr := logToFile(s.fileLogger, s.filePrefix, m)
	if fileErr != nil {
		logger.Errorf("logging to logsink.log failed: %v", fileErr)
	}
	return dbErr == nil && fileErr == nil
}

func (s *AgentLoggingStrategy) Stop() {
	s.dbLogger.Close()
	s.ctxt.release(s.st)
}

func newAgentLoggingStrategy(ctxt httpContext, fileLogger io.Writer) LoggingStrategy {
	return &AgentLoggingStrategy{ctxt: ctxt, fileLogger: fileLogger}
}

func newLogSinkHandler(h httpContext, w io.Writer, newStrategy func(httpContext, io.Writer) LoggingStrategy) http.Handler {
	return &logSinkHandler{ctxt: h, fileLogger: w, newStrategy: newStrategy}
}

func newLogSinkWriter(logPath string) (io.WriteCloser, error) {
	if err := primeLogFile(logPath); err != nil {
		// This isn't a fatal error so log and continue if priming fails.
		logger.Warningf("Unable to prime %s (proceeding anyway): %v", logPath, err)
	}

	return &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    300, // MB
		MaxBackups: 2,
	}, nil
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
	ctxt        httpContext
	newStrategy func(httpContext, io.Writer) LoggingStrategy
	fileLogger  io.Writer
}

// ServeHTTP implements the http.Handler interface.
func (h *logSinkHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	server := websocket.Server{
		Handler: func(socket *websocket.Conn) {
			defer socket.Close()
			strategy := h.newStrategy(h.ctxt, h.fileLogger)
			err := strategy.Authenticate(req)
			if err != nil {
				h.sendError(socket, req, err)
				return
			}
			strategy.Start()
			defer strategy.Stop()

			// If we get to here, no more errors to report, so we report a nil
			// error.  This way the first line of the socket is always a json
			// formatted simple error.
			h.sendError(socket, req, nil)

			logCh := h.receiveLogs(socket)
			for {
				select {
				case <-h.ctxt.stop():
					return
				case m, ok := <-logCh:
					if !ok {
						return
					}
					success := strategy.Log(m)
					if !success {
						return
					}
				}
			}
		},
	}
	server.ServeHTTP(w, req)
}

func jujuClientVersionFromReq(req *http.Request) (version.Number, error) {
	verStr := req.URL.Query().Get("jujuclientversion")
	if verStr == "" {
		return version.Zero, errors.New(`missing "jujuclientversion" in URL query`)
	}
	ver, err := version.Parse(verStr)
	if err != nil {
		return version.Zero, errors.Annotatef(err, "invalid jujuclientversion %q", verStr)
	}
	return ver, nil
}

func (h *logSinkHandler) receiveLogs(socket *websocket.Conn) <-chan params.LogRecord {
	logCh := make(chan params.LogRecord)

	go func() {
		// Close the channel to signal ServeHTTP to finish. Otherwise
		// we leak goroutines on client disconnect, because the server
		// isn't shutting down so h.ctxt.stop() is never closed.
		defer close(logCh)
		var m params.LogRecord
		for {
			// Receive() blocks until data arrives but will also be
			// unblocked when the API handler calls socket.Close as it
			// finishes.
			if err := websocket.JSON.Receive(socket, &m); err != nil {
				logger.Debugf("logsink receive error: %v", err)
				return
			}

			// Send the log message.
			select {
			case <-h.ctxt.stop():
				return
			case logCh <- m:
			}
		}
	}()

	return logCh
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
func logToFile(logger io.Writer, prefix string, m params.LogRecord) error {
	_, err := logger.Write([]byte(strings.Join([]string{
		prefix,
		m.Time.In(time.UTC).Format("2006-01-02 15:04:05"),
		m.Level,
		m.Module,
		m.Location,
		m.Message,
	}, " ") + "\n"))
	return err
}
