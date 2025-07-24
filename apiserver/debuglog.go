// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/apiserver/websocket"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/logtailer"
	"github.com/juju/juju/rpc/params"
)

// debugLogHandler takes requests to watch the debug log.
//
// It provides the underlying framework for the 2 debug-log
// variants. The supplied handle func allows for varied handling of
// requests.
type debugLogHandler struct {
	ctxt          httpContext
	authenticator authentication.HTTPAuthenticator
	authorizer    authentication.Authorizer
	handle        debugLogHandlerFunc
	logDir        string
}

type debugLogHandlerFunc func(
	clock.Clock,
	time.Duration,
	debugLogParams,
	debugLogSocket,
	logTailerFunc,
	<-chan struct{},
) error

func newDebugLogHandler(
	ctxt httpContext,
	authenticator authentication.HTTPAuthenticator,
	authorizer authentication.Authorizer,
	logDir string,
	handle debugLogHandlerFunc,
) *debugLogHandler {
	return &debugLogHandler{
		ctxt:          ctxt,
		authenticator: authenticator,
		authorizer:    authorizer,
		handle:        handle,
		logDir:        logDir,
	}
}

// ServeHTTP will serve up connections as a websocket for the
// debug-log API.
//
// The authentication and authorization have to be done after the http request
// has been upgraded to a websocket as we may be sending back a discharge
// required error. This error contains the macaroon that needs to be
// discharged by the user. In order for this error to be deserialized
// correctly any auth failure will come back in the initial error that is
// returned over the websocket. This is consumed by the ConnectStream function
// on the apiclient.
//
// Args for the HTTP request are as follows:
//
//	includeEntity -> []string - lists entity tags to include in the response
//	   - tags may finish with a '*' to match a prefix e.g.: unit-mysql-*, machine-2
//	   - if none are set, then all lines are considered included
//	includeModule -> []string - lists logging modules to include in the response
//	   - if none are set, then all lines are considered included
//	excludeEntity -> []string - lists entity tags to exclude from the response
//	   - as with include, it may finish with a '*'
//	excludeModule -> []string - lists logging modules to exclude from the response
//	limit -> uint - show *at most* this many lines
//	backlog -> uint
//	   - go back this many lines from the end before starting to filter
//	   - has no meaning if 'replay' is true
//	level -> string one of [TRACE, DEBUG, INFO, WARNING, ERROR]
//	replay -> string - one of [true, false], if true, start the file from the start
//	noTail -> string - one of [true, false], if true, existing logs are sent back,
//	   - but the command does not wait for new ones.
func (h *debugLogHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	handler := func(conn *websocket.Conn) {
		socket := &debugLogSocketImpl{conn: conn}
		defer conn.Close()

		// Authentication and authorization has to be done after the http
		// connection has been upgraded to a websocket.
		authInfo, err := h.authenticator.Authenticate(req)
		if err != nil {
			socket.sendError(errors.Annotate(err, "authentication failed"))
			return
		}
		if err := h.authorizer.Authorize(req.Context(), authInfo); err != nil {
			socket.sendError(errors.Annotate(err, "authorization failed"))
			return
		}

		modelUUID, _ := httpcontext.RequestModelUUID(req.Context())

		params, err := readDebugLogParams(req.URL.Query())
		if err != nil {
			socket.sendError(err)
			return
		}

		clock := h.ctxt.srv.clock
		maxDuration := h.ctxt.srv.shared.maxDebugLogDuration()

		logTailerFunc := func(p logtailer.LogTailerParams) (logtailer.LogTailer, error) {
			// TODO (stickupkid): This should come from the logsink directly, to
			// prevent unfettered access.
			logFile := filepath.Join(h.logDir, "logsink.log")
			if p.Firehose {
				modelUUID = ""
			}

			return logtailer.NewLogTailer(modelUUID, logFile, p)
		}

		// This should really use a tomb, then we don't have to do this song
		// and dance with the channel.
		done := make(chan struct{})
		go func() {
			defer close(done)

			select {
			case <-req.Context().Done():
			case <-h.ctxt.stop():
			}
		}()

		if err := h.handle(clock, maxDuration, params, socket, logTailerFunc, done); err != nil {
			if isBrokenPipe(err) {
				logger.Tracef(req.Context(), "debug-log handler stopped (client disconnected)")
			} else {
				logger.Errorf(req.Context(), "debug-log handler error: %v", err)
			}
		}
	}
	websocket.Serve(w, req, handler)
}

func isBrokenPipe(err error) bool {
	err = errors.Cause(err)
	if opErr, ok := err.(*net.OpError); ok {
		if sysCallErr, ok := opErr.Err.(*os.SyscallError); ok {
			return sysCallErr.Err == syscall.EPIPE
		}
		return opErr.Err == syscall.EPIPE
	}
	return false
}

// debugLogSocket describes the functionality required for the
// debuglog handlers to send logs to the client.
type debugLogSocket interface {
	// sendOk sends a nil error response, indicating there were no errors.
	sendOk()

	// sendError sends a JSON-encoded error response.
	sendError(err error)

	// sendLogRecord sends record JSON encoded.
	sendLogRecord(*params.LogMessage, int) error
}

// debugLogSocketImpl implements the debugLogSocket interface. It
// wraps a websocket.Conn and provides a few debug-log specific helper
// methods.
type debugLogSocketImpl struct {
	conn *websocket.Conn
}

// sendOk implements debugLogSocket.
func (s *debugLogSocketImpl) sendOk() {
	s.sendError(nil)
}

// sendError implements debugLogSocket.
func (s *debugLogSocketImpl) sendError(err error) {
	if sendErr := s.conn.SendInitialErrorV0(err); sendErr != nil {
		logger.Errorf(context.Background(), "closing websocket, %v", err)
		_ = s.conn.Close()
		return
	}
}

func (s *debugLogSocketImpl) sendLogRecord(record *params.LogMessage, version int) (err error) {
	if version == 1 {
		// Older clients expect just logger tags as an array.
		recordv1 := &params.LogMessageV1{
			Entity:    record.Entity,
			Timestamp: record.Timestamp,
			Severity:  record.Severity,
			Module:    record.Module,
			Location:  record.Location,
			Message:   record.Message,
		}
		if loggerTags, ok := record.Labels[loggo.LoggerTags]; ok {
			recordv1.Labels = strings.Split(loggerTags, ",")
		}
		return s.conn.WriteJSON(recordv1)
	}
	return s.conn.WriteJSON(record)
}

// debugLogParams contains the parsed debuglog API request parameters.
type debugLogParams struct {
	version       int
	startTime     time.Time
	fromTheStart  bool
	noTail        bool
	firehose      bool
	initialLines  uint
	filterLevel   corelogger.Level
	includeEntity []string
	excludeEntity []string
	includeModule []string
	excludeModule []string
	includeLabels map[string]string
	excludeLabels map[string]string
}

func readDebugLogParams(queryMap url.Values) (debugLogParams, error) {
	params := debugLogParams{version: 1}

	if value := queryMap.Get("version"); value != "" {
		vers, err := strconv.Atoi(value)
		if err != nil {
			return params, errors.Errorf("version value %q is not a valid number", value)
		}
		params.version = vers
	}

	if value := queryMap.Get("backlog"); value != "" {
		backLogVal, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return params, errors.Errorf("backlog value %q is not a valid unsigned number", value)
		}
		params.initialLines = uint(backLogVal)
	}

	if value := queryMap.Get("maxLines"); value != "" {
		maxLinesVal, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return params, errors.Errorf("maxLines value %q is not a valid unsigned number", value)
		}
		params.initialLines = uint(maxLinesVal)
	}

	if value := queryMap.Get("noTail"); value != "" {
		noTail, err := strconv.ParseBool(value)
		if err != nil {
			return params, errors.Errorf("noTail value %q is not a valid boolean", value)
		}
		params.noTail = noTail
	}

	if value := queryMap.Get("firehose"); value != "" {
		firehose, err := strconv.ParseBool(value)
		if err != nil {
			return params, errors.Errorf("firehose value %q is not a valid boolean", value)
		}
		params.firehose = firehose
	}

	if value := queryMap.Get("replay"); value != "" {
		replay, err := strconv.ParseBool(value)
		if err != nil {
			return params, errors.Errorf("replay value %q is not a valid boolean", value)
		}
		params.fromTheStart = replay
	}

	if value := queryMap.Get("level"); value != "" {
		var ok bool
		level, ok := corelogger.ParseLevelFromString(value)
		if !ok || level < corelogger.TRACE || level > corelogger.ERROR {
			return params, errors.Errorf("level value %q is not one of %q, %q, %q, %q, %q",
				value, corelogger.TRACE, corelogger.DEBUG, corelogger.INFO, corelogger.WARNING, corelogger.ERROR)
		}
		params.filterLevel = level
	}

	if value := queryMap.Get("startTime"); value != "" {
		startTime, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return params, errors.Errorf("start time %q is not a valid time in RFC3339 format", value)
		}
		params.startTime = startTime
	}

	params.includeEntity = queryMap["includeEntity"]
	params.excludeEntity = queryMap["excludeEntity"]
	params.includeModule = queryMap["includeModule"]
	params.excludeModule = queryMap["excludeModule"]

	params.includeLabels = make(map[string]string)
	if labels, ok := queryMap["includeLabels"]; ok {
		for _, label := range labels {
			parts := strings.Split(label, "=")
			if len(parts) < 2 {
				return debugLogParams{}, fmt.Errorf("include label key value %q %w", label, errors.NotValid)
			}
			params.includeLabels[parts[0]] = parts[1]
		}
	} else if loggerTags, ok := queryMap["includeLabel"]; ok {
		// For compatibility with older clients.
		params.includeLabels[loggo.LoggerTags] = strings.Join(loggerTags, ",")
	}
	params.excludeLabels = make(map[string]string)
	if labels, ok := queryMap["excludeLabels"]; ok {
		for _, label := range labels {
			parts := strings.Split(label, "=")
			if len(parts) < 2 {
				return debugLogParams{}, fmt.Errorf("exclude label key value %q %w", label, errors.NotValid)
			}
			params.excludeLabels[parts[0]] = parts[1]
		}
	} else if loggerTags, ok := queryMap["excludeLabel"]; ok {
		// For compatibility with older clients.
		params.excludeLabels[loggo.LoggerTags] = strings.Join(loggerTags, ",")
	}
	return params, nil
}
