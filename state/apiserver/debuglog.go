// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"code.google.com/p/go.net/websocket"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils/tailer"
)

// debugLogHandler takes requests to watch the debug log.
type debugLogHandler struct {
	httpHandler
	logDir string
}

var maxLinesReached = fmt.Errorf("max lines reached")

// ServeHTTP will serve up connections as a websocket.
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
		Handler: func(socket *websocket.Conn) {
			logger.Infof("debug log handler starting")
			if err := h.authenticate(req); err != nil {
				h.sendError(socket, fmt.Errorf("auth failed: %v", err))
				socket.Close()
				return
			}
			stream, err := newLogStream(req.URL.Query())
			if err != nil {
				h.sendError(socket, err)
				socket.Close()
				return
			}
			// Open log file.
			logLocation := filepath.Join(h.logDir, "all-machines.log")
			logFile, err := os.Open(logLocation)
			if err != nil {
				h.sendError(socket, fmt.Errorf("cannot open log file: %v", err))
				socket.Close()
				return
			}
			defer logFile.Close()
			if err := stream.positionLogFile(logFile); err != nil {
				h.sendError(socket, fmt.Errorf("cannot position log file: %v", err))
				socket.Close()
				return
			}

			// If we get to here, no more errors to report, so we report a nil
			// error.  This way the first line of the socket is always a json
			// formatted simple error.
			if err := h.sendError(socket, nil); err != nil {
				logger.Errorf("could not send good log stream start")
				socket.Close()
				return
			}

			stream.start(logFile, socket)
			go func() {
				defer stream.tomb.Done()
				defer socket.Close()
				stream.tomb.Kill(stream.loop())
			}()
			if err := stream.tomb.Wait(); err != nil {
				if err != maxLinesReached {
					logger.Errorf("debug-log handler error: %v", err)
				}
			}
		}}
	server.ServeHTTP(w, req)
}

func newLogStream(queryMap url.Values) (*logStream, error) {
	maxLines := uint(0)
	if value := queryMap.Get("maxLines"); value != "" {
		num, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("maxLines value %q is not a valid unsigned number", value)
		}
		maxLines = uint(num)
	}

	fromTheStart := false
	if value := queryMap.Get("replay"); value != "" {
		replay, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("replay value %q is not a valid boolean", value)
		}
		fromTheStart = replay
	}

	backlog := uint(0)
	if value := queryMap.Get("backlog"); value != "" {
		num, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("backlog value %q is not a valid unsigned number", value)
		}
		backlog = uint(num)
	}

	level := loggo.UNSPECIFIED
	if value := queryMap.Get("level"); value != "" {
		var ok bool
		level, ok = loggo.ParseLevel(value)
		if !ok || level < loggo.TRACE || level > loggo.ERROR {
			return nil, fmt.Errorf("level value %q is not one of %q, %q, %q, %q, %q",
				value, loggo.TRACE, loggo.DEBUG, loggo.INFO, loggo.WARNING, loggo.ERROR)
		}
	}

	return &logStream{
		includeEntity: queryMap["includeEntity"],
		includeModule: queryMap["includeModule"],
		excludeEntity: queryMap["excludeEntity"],
		excludeModule: queryMap["excludeModule"],
		maxLines:      maxLines,
		fromTheStart:  fromTheStart,
		backlog:       backlog,
		filterLevel:   level,
	}, nil
}

// sendError sends a JSON-encoded error response.
func (h *debugLogHandler) sendError(w io.Writer, err error) error {
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
	_, err = w.Write(message)
	return err
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
	if len(fields) > agentField {
		agent := fields[agentField]
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
	includeEntity []string
	includeModule []string
	excludeEntity []string
	excludeModule []string
	backlog       uint
	maxLines      uint
	lineCount     uint
	fromTheStart  bool
}

// positionLogFile will update the internal read position of the logFile to be
// at the end of the file or somewhere in the middle if backlog has been specified.
func (stream *logStream) positionLogFile(logFile io.ReadSeeker) error {
	// Seek to the end, or lines back from the end if we need to.
	if !stream.fromTheStart {
		return tailer.SeekLastLines(logFile, stream.backlog, stream.filterLine)
	}
	return nil
}

// start the tailer listening to the logFile, and sending the matching
// lines to the writer.
func (stream *logStream) start(logFile io.ReadSeeker, writer io.Writer) {
	stream.logTailer = tailer.NewTailer(logFile, writer, stream.countedFilterLine)
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
	return stream.checkIncludeEntity(log) &&
		stream.checkIncludeModule(log) &&
		!stream.exclude(log) &&
		stream.checkLevel(log)
}

// countedFilterLine checks the received line for one of the confgured tags,
// and also checks to make sure the stream doesn't send more than the
// specified number of lines.
func (stream *logStream) countedFilterLine(line []byte) bool {
	result := stream.filterLine(line)
	if result && stream.maxLines > 0 {
		stream.lineCount++
		result = stream.lineCount <= stream.maxLines
		if stream.lineCount == stream.maxLines {
			stream.tomb.Kill(maxLinesReached)
		}
	}
	return result
}

func (stream *logStream) checkIncludeEntity(line *logLine) bool {
	if len(stream.includeEntity) == 0 {
		return true
	}
	for _, value := range stream.includeEntity {
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
	for _, value := range stream.excludeEntity {
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
