// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"

	"code.google.com/p/go.net/websocket"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils/tailer"
	"launchpad.net/tomb"
)

const logLocation = "/var/log/juju/all-machines.log"

// logHandler takes requests to watch the debug log.
type logHandler struct {
	commonHandler
}

// newLogHandler creates a new log handler.
func newLogHandler(state *state.State) *logHandler {
	return &logHandler{commonHandler{state}}
}

func (h *logHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if err := h.authenticate(req); err != nil {
		h.sendAuthError(h, w)
		return
	}
	// Get environment tag.
	env, err := h.state.Environment()
	if err != nil {
		h.sendError(h, w, http.StatusInternalServerError, "cannot retrieve environment tag: %v", err)
		return
	}
	envTag := env.Tag()
	// Open log file.
	logFile, err := os.Open(logLocation)
	if err != nil {
		h.sendError(h, w, http.StatusInternalServerError, "cannot open log file: %v", err)
		return
	}
	defer logFile.Close()
	// Get the arguments of the request.
	values := req.URL.Query()
	lines, err := strconv.Atoi(values["lines"][0])
	if err != nil {
		h.sendError(h, w, http.StatusInternalServerError, "cannot parse number of lines: %v", err)
		return
	}
	entities := values["entity"]
	// Start streaming.
	wsServer := websocket.Server{
		Handler: func(wsConn *websocket.Conn) {
			stream := &logStream{envTag: envTag}
			go stream.loop(logFile, wsConn, lines, entities)
			stream.tomb.Wait()
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
	envTag   string
	noFilter bool
	prefixes [][]byte
}

// loop starts the tailer with the log file and the web socket.
func (stream *logStream) loop(logFile io.ReadSeeker, wsConn *websocket.Conn, lines int, entities []string) {
	defer stream.tomb.Done()
	stream.setFilter(entities)
	tailer := tailer.NewTailer(logFile, wsConn, lines, stream.filter)
	go stream.handleRequests(wsConn)
	select {
	case <-tailer.Dead():
		stream.tomb.Kill(tailer.Err())
	case <-stream.tomb.Dying():
		tailer.Stop()
	}
}

// filter checks the received line for one of the confgured tags.
func (stream *logStream) filter(line []byte) bool {
	stream.mux.Lock()
	defer stream.mux.Unlock()
	// Check if no filter due to whole environment.
	if stream.noFilter {
		return true
	}
	// Check all prefixes.
	for _, prefix := range stream.prefixes {
		if bytes.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// setFilter configures the stream filtering by setting the
// tags to filter.
func (stream *logStream) setFilter(entities []string) {
	stream.mux.Lock()
	defer stream.mux.Unlock()
	stream.prefixes = make([][]byte, len(entities))
	for i, entity := range entities {
		if entity == stream.envTag {
			// In case of environment tag no filtering.
			stream.noFilter = true
			break
		}
		// Add prefix for filter.
		stream.prefixes[i] = []byte(entity + ":")
	}
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
		stream.setFilter(req.Entities)
	}
}
