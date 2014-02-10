// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"code.google.com/p/go.net/websocket"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/utils/tailer"
)

// defaultLogLocation is the location of the aggregated log in non-local
// environments.
const defaultLogLocation = "/var/log/juju/all-machines.log"

// debugLogHandler takes requests to watch the debug log.
type debugLogHandler struct {
	state *state.State
}

func (h *debugLogHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if err := h.authenticate(req); err != nil {
		h.authError(w)
		return
	}
	// Get the arguments of the request.
	values := req.URL.Query()
	lines := 0
	if linesAttr := values.Get("lines"); linesAttr != "" {
		var err error
		lines, err = strconv.Atoi(linesAttr)
		if err != nil {
			h.sendError(w, http.StatusBadRequest, "cannot parse number of lines: %v", err)
			return
		}
	}
	filter := values.Get("filter")
	filterRx, err := regexp.Compile(filter)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, "cannot set filter: %v", err)
		return
	}
	// Open log file.
	// TODO(mue) Add mechanism to get log location depending on provider.
	logFile, err := os.Open(defaultLogLocation)
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "cannot open log file: %v", err)
		return
	}
	defer logFile.Close()
	// Start streaming.
	wsServer := websocket.Server{
		Handler: func(wsConn *websocket.Conn) {
			stream := &logStream{filterRx: filterRx}
			go func() {
				defer stream.tomb.Done()
				defer wsConn.Close()
				stream.tomb.Kill(stream.loop(logFile, wsConn, lines))
			}()
			if err := stream.tomb.Wait(); err != nil {
				logger.Errorf("debug-log handler error: %v", err)
			}
		},
	}
	wsServer.ServeHTTP(w, req)
}

// sendError sends a JSON-encoded error response.
func (h *debugLogHandler) sendError(w http.ResponseWriter, statusCode int, message string, args ...interface{}) error {
	w.WriteHeader(statusCode)
	response := &params.SimpleError{Error: fmt.Sprintf(message, args...)}
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	w.Write(body)
	return nil
}

// authenticate parses HTTP basic authentication and authorizes the
// request by looking up the provided tag and password against state.
func (h *debugLogHandler) authenticate(r *http.Request) error {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || parts[0] != "Basic" {
		// Invalid header format or no header provided.
		return fmt.Errorf("invalid request format")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	// See RFC 2617, Section 2.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid request format")
	}
	tagPass := strings.SplitN(string(challenge), ":", 2)
	if len(tagPass) != 2 {
		return fmt.Errorf("invalid request format")
	}
	entity, err := checkCreds(h.state, params.Creds{
		AuthTag:  tagPass[0],
		Password: tagPass[1],
	})
	if err != nil {
		return err
	}
	// Only allow users, not agents.
	_, _, err = names.ParseTag(entity.Tag(), names.UserTagKind)
	if err != nil {
		return common.ErrBadCreds
	}
	return err
}

// authError sends an unauthorized error.
func (h *debugLogHandler) authError(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	h.sendError(w, http.StatusUnauthorized, "unauthorized")
}

// logStream runs the tailer to read a log file and stream
// it via a web socket.
type logStream struct {
	tomb     tomb.Tomb
	mux      sync.Mutex
	filterRx *regexp.Regexp
}

// loop starts the tailer with the log file and the web socket.
func (stream *logStream) loop(logFile io.ReadSeeker, wsConn *websocket.Conn, lines int) error {
	tailer := tailer.NewTailer(logFile, wsConn, lines, stream.filterLine)
	go stream.handleRequests(wsConn)
	select {
	case <-tailer.Dead():
		return tailer.Err()
	case <-stream.tomb.Dying():
		tailer.Stop()
	}
	return nil
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
func (stream *logStream) setFilter(filter string) {
	stream.mux.Lock()
	defer stream.mux.Unlock()
	filterRx, err := regexp.Compile(filter)
	if err != nil {
		// Keep filter untouched.
		logger.Errorf("error setting filter: %v", err)
		return
	}
	stream.filterRx = filterRx
}

// handleRequests allows the stream to handle requests, so far only
// the setting of the tags to filter.
func (stream *logStream) handleRequests(wsConn *websocket.Conn) {
	for {
		var req params.DebugLogRequest
		if err := websocket.JSON.Receive(wsConn, &req); err != nil {
			stream.tomb.Kill(fmt.Errorf("error receiving packet: %v", err))
			return
		}
		stream.setFilter(req.Filter)
	}
}
