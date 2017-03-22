// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/featureflag"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
)

// LoggingStrategy handles the authentication and logging details for
// a particular logsink handler.
type LoggingStrategy interface {
	// Authenticate should check that the request identifies the kind
	// of client that is expected to be talking to this endpoint.
	Authenticate(*http.Request) error

	// Start prepares any underlying loggers before sending them
	// messages. This should only be called once.
	Start()

	// Log writes out the given record to any backing loggers for the strategy.
	Log(params.LogRecord) bool

	// Stop tells the strategy that there are no more log messages
	// coming, so it can clean up any resources it holds and close any
	// loggers. Once Stop has been called no more log messages can be
	// written.
	Stop()
}

type agentLoggingStrategy struct {
	ctxt       httpContext
	st         *state.State
	releaser   func()
	version    version.Number
	entity     names.Tag
	filePrefix string
	dbLogger   *state.EntityDbLogger
	fileLogger io.Writer
}

func newAgentLoggingStrategy(ctxt httpContext, fileLogger io.Writer) LoggingStrategy {
	return &agentLoggingStrategy{ctxt: ctxt, fileLogger: fileLogger}
}

// Authenticate checks that this is request is from a machine
// agent. Part of LoggingStrategy.
func (s *agentLoggingStrategy) Authenticate(req *http.Request) error {
	st, releaser, entity, err := s.ctxt.stateForRequestAuthenticatedAgent(req)
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
		releaser()
		return errors.Trace(err)
	}
	s.st = st
	s.releaser = releaser
	s.version = ver
	s.entity = entity.Tag()
	return nil
}

// Start creates the underlying DB logger. Part of LoggingStrategy.
func (s *agentLoggingStrategy) Start() {
	s.filePrefix = s.st.ModelUUID() + ":"
	s.dbLogger = state.NewEntityDbLogger(s.st, s.entity, s.version)
}

// Log writes the record to the file and entity loggers. Part of
// LoggingStrategy.
func (s *agentLoggingStrategy) Log(m params.LogRecord) bool {
	level, _ := loggo.ParseLevel(m.Level)
	dbErr := s.dbLogger.Log(m.Time, m.Module, m.Location, level, m.Message)
	if dbErr != nil {
		logger.Errorf("logging to DB failed: %v", dbErr)
	}
	m.Entity = s.entity.String()
	fileErr := logToFile(s.fileLogger, s.filePrefix, m)
	if fileErr != nil {
		logger.Errorf("logging to logsink.log failed: %v", fileErr)
	}
	return dbErr == nil && fileErr == nil
}

// Stop closes the DB logger and releases the state. It doesn't close
// the file logger because that lives longer than one request. Once it
// has been called then it can't be restarted unless Authenticate has
// been called again. Part of LoggingStrategy.
func (s *agentLoggingStrategy) Stop() {
	s.dbLogger.Close()
	s.releaser()
	// Should we clear out s.st, s.releaser, s.entity here?
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

// Since the logsink only recieves messages, it is possible for the other end
// to disappear without the server noticing. To fix this, we use the
// underlying websocket control messages ping/pong. Periodically the server
// writes a ping, and the other end replies with a pong. Now the tricky bit is
// that it appears in all the examples found on the interweb that it is
// possible for the control message to be sent successfully to something that
// isn't entirely alive, which is why relying on an error return from the
// write call is insufficient to mark the connection as dead. Instead the
// write and read deadlines inherent in the underlying Go networking libraries
// are used to force errors on timeouts. However the underlying network
// libraries use time.Now() to determine whether or not to send errors, so
// using a testing clock here isn't going to work. So we rely on manual
// testing, and what is defined as good practice by the library authors.
//
// Now, in theory, we should be using this ping/pong across all the websockets,
// but that is a little outside the scope of this piece of work.

const (
	// pongDelay is how long the server will wait for a pong to be sent
	// before the websocket is considered broken.
	pongDelay = 60 * time.Second

	// pingPeriod is how often ping messages are sent. This should be shorter
	// than the pongDelay, but not by too much.
	pingPeriod = (pongDelay * 9) / 10

	// writeWait is how long the write call can take before it errors out.
	writeWait = 10 * time.Second

	// For endpoints that don't support ping/pong (i.e. agents prior to 2.2-beta1)
	// we will time out their connections after six hours of inactivity.
	vZeroDelay = 6 * time.Hour
)

// ServeHTTP implements the http.Handler interface.
func (h *logSinkHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	handler := func(socket *websocket.Conn) {
		defer socket.Close()
		strategy := h.newStrategy(h.ctxt, h.fileLogger)
		err := strategy.Authenticate(req)
		if err != nil {
			h.sendError(socket, req, err)
			return
		}
		endpointVersion, err := h.getVersion(req)
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

		// Older versions did not respond to ping control messages, so don't try.
		doPingPong := endpointVersion > 0

		// Here we configure the ping/pong handling for the websocket so
		// the server can notice when the client goes away.
		var tickChannel <-chan time.Time
		if doPingPong {
			socket.SetReadDeadline(time.Now().Add(pongDelay))
			socket.SetPongHandler(func(string) error {
				logger.Tracef("pong logsink %p", socket)
				socket.SetReadDeadline(time.Now().Add(pongDelay))
				return nil
			})
			ticker := time.NewTicker(pingPeriod)
			defer ticker.Stop()
			tickChannel = ticker.C
		} else {
			socket.SetReadDeadline(time.Now().Add(vZeroDelay))
		}

		logCh := h.receiveLogs(socket, endpointVersion)
		for {
			select {
			case <-h.ctxt.stop():
				return
			case <-tickChannel:
				deadline := time.Now().Add(writeWait)
				logger.Tracef("ping logsink %p", socket)
				if err := socket.WriteControl(websocket.PingMessage, []byte{}, deadline); err != nil {
					// This error is expected if the other end goes away. By
					// returning we clean up the strategy and close the socket
					// through the defer calls.
					logger.Debugf("failed to write ping: %s", err)
					return
				}
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
	}
	websocketServer(w, req, handler)
}

func (h *logSinkHandler) getVersion(req *http.Request) (int, error) {
	verStr := req.URL.Query().Get("version")
	switch verStr {
	case "":
		return 0, nil
	case "1":
		return 1, nil
	default:
		return 0, errors.Errorf("unknown version %q", verStr)
	}
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

func (h *logSinkHandler) receiveLogs(socket *websocket.Conn, endpointVersion int) <-chan params.LogRecord {
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
			if err := socket.ReadJSON(&m); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					logger.Debugf("logsink receive error: %v", err)
				} else {
					logger.Debugf("disconnected, %p", socket)
				}
				// Try to tell the other end we are closing. If the other end
				// has already disconnected from us, this will fail, but we don't
				// care that much.
				socket.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Send the log message.
			select {
			case <-h.ctxt.stop():
				return
			case logCh <- m:
				// If the remote end does not support ping/pong, we bump
				// the read deadline everytime a message is recieved.
				if endpointVersion == 0 {
					socket.SetReadDeadline(time.Now().Add(vZeroDelay))
				}
			}
		}
	}()

	return logCh
}

// sendError sends a JSON-encoded error response.
func (h *logSinkHandler) sendError(ws *websocket.Conn, req *http.Request, err error) {
	// There is no need to log the error for normal operators as there is nothing
	// they can action. This is for developers.
	if err != nil && featureflag.Enabled(feature.DeveloperMode) {
		logger.Errorf("returning error from %s %s: %s", req.Method, req.URL.Path, errors.Details(err))
	}
	if sendErr := sendInitialErrorV0(ws, err); sendErr != nil {
		logger.Errorf("closing websocket, %v", err)
		ws.Close()
	}
}

// logToFile writes a single log message to the logsink log file.
func logToFile(writer io.Writer, prefix string, m params.LogRecord) error {
	_, err := writer.Write([]byte(strings.Join([]string{
		prefix,
		m.Entity,
		m.Time.In(time.UTC).Format("2006-01-02 15:04:05"),
		m.Level,
		m.Module,
		m.Location,
		m.Message,
	}, " ") + "\n"))
	return err
}
