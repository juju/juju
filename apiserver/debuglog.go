// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"syscall"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"golang.org/x/net/websocket"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// debugLogHandler takes requests to watch the debug log.
//
// It provides the underlying framework for the 2 debug-log
// variants. The supplied handle func allows for varied handling of
// requests.
type debugLogHandler struct {
	httpHandler
	stop   <-chan struct{}
	handle debugLogHandlerFunc
}

type debugLogHandlerFunc func(
	state.LoggingState,
	*debugLogParams,
	debugLogSocket,
	<-chan struct{},
) error

func newDebugLogHandler(
	statePool *state.StatePool,
	stop <-chan struct{},
	handle debugLogHandlerFunc,
) *debugLogHandler {
	return &debugLogHandler{
		httpHandler: httpHandler{statePool: statePool},
		stop:        stop,
		handle:      handle,
	}
}

// ServeHTTP will serve up connections as a websocket for the
// debug-log API.
//
// Args for the HTTP request are as follows:
//   includeEntity -> []string - lists entity tags to include in the response
//      - tags may finish with a '*' to match a prefix e.g.: unit-mysql-*, machine-2
//      - if none are set, then all lines are considered included
//   includeModule -> []string - lists logging modules to include in the response
//      - if none are set, then all lines are considered included
//   excludeEntity -> []string - lists entity tags to exclude from the response
//      - as with include, it may finish with a '*'
//   excludeModule -> []string - lists logging modules to exclude from the response
//   limit -> uint - show *at most* this many lines
//   backlog -> uint
//      - go back this many lines from the end before starting to filter
//      - has no meaning if 'replay' is true
//   level -> string one of [TRACE, DEBUG, INFO, WARNING, ERROR]
//   replay -> string - one of [true, false], if true, start the file from the start
func (h *debugLogHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	server := websocket.Server{
		Handler: func(conn *websocket.Conn) {
			socket := &debugLogSocketImpl{conn}
			defer socket.Close()

			logger.Infof("debug log handler starting")
			// Validate before authenticate because the authentication is
			// dependent on the state connection that is determined during the
			// validation.
			stateWrapper, err := h.validateEnvironUUID(req)
			if err != nil {
				socket.sendError(err)
				return
			}
			if err := stateWrapper.authenticateUser(req); err != nil {
				socket.sendError(fmt.Errorf("auth failed: %v", err))
				return
			}

			params, err := readDebugLogParams(req.URL.Query())
			if err != nil {
				socket.sendError(err)
				return
			}

			if err := h.handle(stateWrapper.state, params, socket, h.stop); err != nil {
				if isBrokenPipe(err) {
					logger.Tracef("debug-log handler stopped (client disconnected)")
				} else {
					logger.Errorf("debug-log handler error: %v", err)
				}
			}
		}}
	server.ServeHTTP(w, req)
}

func isBrokenPipe(err error) bool {
	err = errors.Cause(err)
	if opErr, ok := err.(*net.OpError); ok {
		return opErr.Err == syscall.EPIPE
	}
	return false
}

// debugLogSocket describes the functionality required for the
// debuglog handlers to send logs to the client.
type debugLogSocket interface {
	io.Writer

	// sendOk sends a nil error response, indicating there were no errors.
	sendOk() error

	// sendError sends a JSON-encoded error response.
	sendError(err error) error
}

// debugLogSocketImpl implements the debugLogSocket interface. It
// wraps a websocket.Conn and provides a few debug-log specific helper
// methods.
type debugLogSocketImpl struct {
	*websocket.Conn
}

// sendOK implements debugLogSocket.
func (s *debugLogSocketImpl) sendOk() error {
	return s.sendError(nil)
}

// sendErr implements debugLogSocket.
func (s *debugLogSocketImpl) sendError(err error) error {
	response := &params.ErrorResult{}
	if err != nil {
		response.Error = &params.Error{Message: fmt.Sprint(err)}
	}
	message, err := json.Marshal(response)
	if err != nil {
		// If we are having trouble marshalling the error, we are in big trouble.
		logger.Errorf("failure to marshal SimpleError: %v", err)
		return err
	}
	message = append(message, []byte("\n")...)
	_, err = s.Conn.Write(message)
	return err
}

// debugLogParams contains the parsed debuglog API request parameters.
type debugLogParams struct {
	maxLines      uint
	fromTheStart  bool
	backlog       uint
	filterLevel   loggo.Level
	includeEntity []string
	excludeEntity []string
	includeModule []string
	excludeModule []string
}

func readDebugLogParams(queryMap url.Values) (*debugLogParams, error) {
	params := new(debugLogParams)

	if value := queryMap.Get("maxLines"); value != "" {
		num, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return nil, errors.Errorf("maxLines value %q is not a valid unsigned number", value)
		}
		params.maxLines = uint(num)
	}

	if value := queryMap.Get("replay"); value != "" {
		replay, err := strconv.ParseBool(value)
		if err != nil {
			return nil, errors.Errorf("replay value %q is not a valid boolean", value)
		}
		params.fromTheStart = replay
	}

	if value := queryMap.Get("backlog"); value != "" {
		num, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return nil, errors.Errorf("backlog value %q is not a valid unsigned number", value)
		}
		params.backlog = uint(num)
	}

	if value := queryMap.Get("level"); value != "" {
		var ok bool
		level, ok := loggo.ParseLevel(value)
		if !ok || level < loggo.TRACE || level > loggo.ERROR {
			return nil, errors.Errorf("level value %q is not one of %q, %q, %q, %q, %q",
				value, loggo.TRACE, loggo.DEBUG, loggo.INFO, loggo.WARNING, loggo.ERROR)
		}
		params.filterLevel = level
	}

	params.includeEntity = queryMap["includeEntity"]
	params.excludeEntity = queryMap["excludeEntity"]
	params.includeModule = queryMap["includeModule"]
	params.excludeModule = queryMap["excludeModule"]

	return params, nil
}
