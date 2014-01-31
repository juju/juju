// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"

	"code.google.com/p/go.net/websocket"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils/tailer"
	"launchpad.net/tomb"
)

const (
	// defaultLogLocation is the location of the aggregated log in non-local
	// environments.
	defaultLogLocation = "/var/log/juju/all-machines.log"

	// localLogLocation is the template for the log location in local
	// environments. It needs the callers Juju home and the environment
	// name.
	localLogLocation = "%s/%s/log/all-machines.log"
)

// logHandler takes requests to watch the debug log.
type logHandler struct {
	commonHandler
}

// newLogHandler returns a new http.Handler
// that handles debug-log HTTP requests.
func newLogHandler(state *state.State) *logHandler {
	return &logHandler{commonHandler{state}}
}

func (h *logHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if err := h.authenticate(req); err != nil {
		h.sendAuthError(h, w)
		return
	}
	// Get the arguments of the request.
	values := req.URL.Query()
	jujuHome := values.Get("juju-home")
	lines := 0
	if linesAttr := values.Get("lines"); linesAttr != "" {
		var err error
		lines, err = strconv.Atoi(linesAttr)
		if err != nil {
			h.sendError(h, w, http.StatusInternalServerError, "cannot parse number of lines: %v", err)
			return
		}
	}
	filter := values.Get("filter")
	// Open log file.
	logLoc, err := logLocation(h.state, jujuHome)
	if err != nil {
		h.sendError(h, w, http.StatusInternalServerError, "cannot find log file: %v", err)
		return
	}
	logFile, err := os.Open(logLoc)
	if err != nil {
		h.sendError(h, w, http.StatusInternalServerError, "cannot open log file: %v", err)
		return
	}
	defer logFile.Close()
	// Start streaming.
	wsServer := websocket.Server{
		Handler: func(wsConn *websocket.Conn) {
			stream := &logStream{}
			go stream.loop(logFile, wsConn, lines, filter)
			if err := stream.tomb.Wait(); err != nil {
				logger.Errorf("debug-log handler error: %v", err)
			}
		},
	}
	wsServer.ServeHTTP(w, req)
}

// errorResponse wraps the message for an error response.
func (h *logHandler) errorResponse(message string) interface{} {
	return &params.EntityLogResponse{Error: message}
}

// logStream runs the tailer to read a log file and stream
// it via a web socket.
type logStream struct {
	tomb     tomb.Tomb
	mux      sync.Mutex
	filter   string
	filterRx *regexp.Regexp
}

// loop starts the tailer with the log file and the web socket.
func (stream *logStream) loop(logFile io.ReadSeeker, wsConn *websocket.Conn, lines int, filter string) {
	defer stream.tomb.Done()
	if err := stream.setFilter(filter); err != nil {
		stream.tomb.Kill(err)
		return
	}
	tailer := tailer.NewTailer(logFile, wsConn, lines, stream.filterLine)
	go stream.handleRequests(wsConn)
	select {
	case <-tailer.Dead():
		stream.tomb.Kill(tailer.Err())
	case <-stream.tomb.Dying():
		tailer.Stop()
	}
}

// filterLine checks the received line for one of the confgured tags.
func (stream *logStream) filterLine(line []byte) bool {
	stream.mux.Lock()
	defer stream.mux.Unlock()
	// Check if no filter.
	if stream.filterRx == nil {
		return true
	}
	// Check if the filter matches.
	return stream.filterRx.Match(line)
}

// setFilter configures the stream filtering by setting the
// tags to filter.
func (stream *logStream) setFilter(filter string) (err error) {
	stream.mux.Lock()
	defer stream.mux.Unlock()
	stream.filterRx, err = regexp.Compile(filter)
	return
}

// handleRequests allows the stream to handle requests, so far only
// the setting of the tags to filter.
func (stream *logStream) handleRequests(wsConn *websocket.Conn) {
	for {
		var req params.EntityLogRequest
		if err := websocket.JSON.Receive(wsConn, &req); err != nil {
			stream.tomb.Kill(fmt.Errorf("error receiving packet: %v", err))
			return
		}
		if err := stream.setFilter(req.Filter); err != nil {
			stream.tomb.Kill(fmt.Errorf("error setting filter: %v", err))
			return
		}
	}
}

// logLocation tries to get the location of the log file.
func logLocation(st *state.State, jujuHome string) (string, error) {
	_, err := os.Stat(defaultLogLocation)
	if err == nil {
		// Non-local environment.
		return defaultLogLocation, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("error looking for log file: %v", err)
	}
	// Not found, so maybe local environment.
	env, err := st.Environment()
	if err != nil {
		return "", err
	}
	envName := env.Name()
	localLogLoc := fmt.Sprintf(localLogLocation, jujuHome, envName)
	_, err = os.Stat(localLogLoc)
	if err == nil {
		// Found it.
		return localLogLoc, nil
	}
	return "", fmt.Errorf("error looking for log file: %v", err)
}
