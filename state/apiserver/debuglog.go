// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/juju/loggo"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	_ "strconv"
	"strings"

	"code.google.com/p/go.net/websocket"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/utils/tailer"
)

// debugLogHandler takes requests to watch the debug log.
type debugLogHandler struct {
	state  *state.State
	logDir string
}

// ServeHTTP will serve up connections as a websocket.
// Args for the HTTP request are as follows:
//   includeAgent -> []string - lists agent tagsto include in the response
//      may finish with a '*' to match a prefix eg: unit-mysql-*, machine-2
//      - if none are set, then all lines are considered included
//   includeModule -> []string - lists logging modules to include in the response
//      - if none are set, then all lines are considered included
//   excludeAgent -> []string - lists agent tags to exclude from the response
//      as with include, it may finish with a '*'
//   excludeModule -> []string - lists logging modules to exclude from the response
//   limit -> int - show this many lines then exit
//   level -> string one of [TRACE, DEBUG, INFO, WARNING, ERROR]
//   replay -> string - one of [true, false], if true, start the file from the start
func (h *debugLogHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if err := h.authenticate(req); err != nil {
		h.authError(w)
		return
	}
	stream, err := newLogStream(req.URL.Query())
	if err != nil {
		h.sendError(w, http.StatusBadRequest, "%v", err)
		return
	}
	// Open log file.
	logLocation := filepath.Join(h.logDir, "all-machines.log")
	logFile, err := os.Open(logLocation)
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "cannot open log file: %v", err)
		return
	}
	defer logFile.Close()
	// Start streaming.
	wsServer := websocket.Server{
		Handler: func(wsConn *websocket.Conn) {
			stream.init(logFile, wsConn)
			go func() {
				defer stream.tomb.Done()
				defer wsConn.Close()
				stream.tomb.Kill(stream.loop())
			}()
			if err := stream.tomb.Wait(); err != nil {
				// TODO: possibly have a special error code for max lines
				// so we don't output noise
				logger.Errorf("debug-log handler error: %v", err)
			}
		},
	}
	wsServer.ServeHTTP(w, req)
}

func newLogStream(queryMap url.Values) (*logStream, error) {
	// Get the arguments of the request.

	// values :=
	// lines := 0
	// if linesAttr := values.Get("lines"); linesAttr != "" {
	// 	var err error
	// 	lines, err = strconv.Atoi(linesAttr)
	// 	if err != nil {

	// 	}
	// }
	// _ = lines
	// filter := values.Get("filter")
	// _, err := regexp.Compile(filter)
	// if err != nil {
	// 	h.sendError(w, http.StatusBadRequest, "cannot set filter: %v", err)
	// 	return
	// }

	//   includeAgent -> []string - lists agent tagsto include in the response
	//      may finish with a '*' to match a prefix eg: unit-mysql-*, machine-2
	//      - if none are set, then all lines are considered included
	//   includeModule -> []string - lists logging modules to include in the response
	//      - if none are set, then all lines are considered included
	//   excludeAgent -> []string - lists agent tags to exclude from the response
	//      as with include, it may finish with a '*'
	//   excludeModule -> []string - lists logging modules to exclude from the response
	//   limit -> int - show this many lines then exit
	//   lines -> int - up to this many lines in the past
	//   level -> string one of [TRACE, DEBUG, INFO, WARNING, ERROR]
	//   replay -> string - one of [true, false], if true, start the file from the start

	return &logStream{}, nil
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

type logLine struct {
	line   string
	agent  string
	level  loggo.Level
	module string
}

func parseLogLine(line string) *logLine {
	const (
		agentField  = 0
		levelField  = 3
		moduleField = 4
	)
	fields := strings.Fields(line)
	result := &logLine{
		line: line,
	}
	logger.Infof("%#v", fields)
	if len(fields) > agentField {
		agent := fields[agentField]
		logger.Infof("%q", agent)
		if strings.HasSuffix(agent, ":") {
			result.agent = agent[:len(agent)-1]
		}
	}
	if len(fields) > moduleField {
		if level, valid := loggo.ParseLevel(fields[levelField]); valid {
			result.level = level
			result.module = fields[moduleField]
		}
	}

	return result
}

// logStream runs the tailer to read a log file and stream
// it via a web socket.
type logStream struct {
	tomb          tomb.Tomb
	logTailer     *tailer.Tailer
	filterLevel   loggo.Level
	includeAgent  []string
	includeModule []string
	excludeAgent  []string
	excludeModule []string
	backlog       uint
	maxLines      uint
	lineCount     uint
	fromTheStart  bool
}

func (stream *logStream) init(logFile io.ReadSeeker, writer io.Writer) {
	if stream.fromTheStart {
		stream.logTailer = tailer.NewTailer(logFile, writer, stream.filterLine)
	} else {
		stream.logTailer = tailer.NewTailerBacktrack(logFile, writer, stream.backlog, stream.filterLine)
	}
}

// loop starts the tailer with the log file and the web socket.
func (stream *logStream) loop() error {
	select {
	case <-stream.logTailer.Dead():
		return stream.logTailer.Err()
	case <-stream.tomb.Dying():
		stream.logTailer.Stop()
	}
	return nil
}

// filterLine checks the received line for one of the confgured tags.
func (stream *logStream) filterLine(line []byte) bool {
	log := parseLogLine(string(line))
	result := stream.checkIncludeAgent(log) &&
		stream.checkIncludeModule(log) &&
		!stream.exclude(log) &&
		stream.checkLevel(log)
	if result && stream.maxLines > 0 {
		stream.lineCount++
		result = stream.lineCount <= stream.maxLines
		stream.tomb.Kill(fmt.Errorf("max lines reached"))
	}
	return result
}

func (stream *logStream) checkIncludeAgent(line *logLine) bool {
	if len(stream.includeAgent) == 0 {
		return true
	}
	for _, value := range stream.includeAgent {
		// special handling, if ends with '*', check prefix
		if strings.HasSuffix(value, "*") {
			if strings.HasPrefix(line.agent, value[:len(value)-1]) {
				return true
			}
		} else if line.agent == value {
			return true
		}
	}
	return false
}

func (stream *logStream) checkIncludeModule(line *logLine) bool {
	if len(stream.includeModule) == 0 {
		return true
	}
	for _, value := range stream.includeModule {
		if strings.HasPrefix(line.module, value) {
			return true
		}
	}
	return false
}

func (stream *logStream) exclude(line *logLine) bool {
	for _, value := range stream.excludeAgent {
		// special handling, if ends with '*', check prefix
		if strings.HasSuffix(value, "*") {
			if strings.HasPrefix(line.agent, value[:len(value)-1]) {
				return true
			}
		} else if line.agent == value {
			return true
		}
	}
	for _, value := range stream.excludeModule {
		if strings.HasPrefix(line.module, value) {
			return true
		}
	}
	return false
}

func (stream *logStream) checkLevel(line *logLine) bool {
	return line.level >= stream.filterLevel
}
