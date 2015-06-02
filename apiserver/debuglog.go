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
	"regexp"
	"strconv"
	"strings"

	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/tailer"
	"golang.org/x/net/websocket"
	"launchpad.net/tomb"

	"github.com/juju/juju/apiserver/params"
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
			// Validate before authenticate because the authentication is
			// dependent on the state connection that is determined during the
			// validation.
			stateWrapper, err := h.validateEnvironUUID(req)
			if err != nil {
				h.sendError(socket, err)
				socket.Close()
				return
			}
			defer stateWrapper.cleanup()
			// TODO (thumper): We need to work out how we are going to filter
			// logging information based on environment.
			if err := stateWrapper.authenticateUser(req); err != nil {
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
			if req.URL.Query().Get("cat") != "" {
				io.Copy(socket, logFile)
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
	line      string
	agentTag  string
	agentName string
	level     loggo.Level
	module    string
}

func parseLogLine(line string) *logLine {
	const (
		agentTagIndex = 0
		levelIndex    = 3
		moduleIndex   = 4
	)
	fields := strings.Fields(line)
	result := &logLine{
		line: line,
	}
	if len(fields) > agentTagIndex {
		agentTag := fields[agentTagIndex]
		// Drop mandatory trailing colon (:).
		// Since colon is mandatory, agentTag without it is invalid and will be empty ("").
		if strings.HasSuffix(agentTag, ":") {
			result.agentTag = agentTag[:len(agentTag)-1]
		}
		/*
		 Drop unit suffix.
		 In logs, unit information may be prefixed with either a unit_tag by itself or a unit_tag[nnnn].
		 The code below caters for both scenarios.
		*/
		if bracketIndex := strings.Index(agentTag, "["); bracketIndex != -1 {
			result.agentTag = agentTag[:bracketIndex]
		}
		// If, at this stage, result.agentTag is empty,  we could not deduce the tag. No point getting the name...
		if result.agentTag != "" {
			// Entity Name deduced from entity tag
			entityTag, err := names.ParseTag(result.agentTag)
			if err != nil {
				/*
				 Logging error but effectively swallowing it as there is no where to propogate.
				 We don't expect ParseTag to fail since the tag was generated by juju in the first place.
				*/
				logger.Errorf("Could not deduce name from tag %q: %v\n", result.agentTag, err)
			}
			result.agentName = entityTag.Id()
		}
	}
	if len(fields) > moduleIndex {
		if level, valid := loggo.ParseLevel(fields[levelIndex]); valid {
			result.level = level
			result.module = fields[moduleIndex]
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

// filterLine checks the received line for one of the configured tags.
func (stream *logStream) filterLine(line []byte) bool {
	log := parseLogLine(string(line))
	return stream.checkIncludeEntity(log) &&
		stream.checkIncludeModule(log) &&
		!stream.exclude(log) &&
		stream.checkLevel(log)
}

// countedFilterLine checks the received line for one of the configured tags,
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
		if agentMatchesFilter(line, value) {
			return true
		}
	}
	return false
}

// agentMatchesFilter checks if agentTag tag or agentTag name match given filter
func agentMatchesFilter(line *logLine, aFilter string) bool {
	return hasMatch(line.agentName, aFilter) || hasMatch(line.agentTag, aFilter)
}

// hasMatch determines if value contains filter using regular expressions.
// All wildcard occurrences are changed to `.*`
// Currently, all match exceptions are logged and not propagated.
func hasMatch(value, aFilter string) bool {
	/* Special handling: out of 12 regexp metacharacters \^$.|?+()[*{
	   only asterix (*) can be legally used as a wildcard in this context.
	   Both machine and unit tag and name specifications do not allow any other metas.
	   Consequently, if aFilter contains wildcard (*), do not escape it -
	   transform it into a regexp "any character(s)" sequence.
	*/
	aFilter = strings.Replace(aFilter, "*", `.*`, -1)
	matches, err := regexp.MatchString("^"+aFilter+"$", value)
	if err != nil {
		// logging errors here... but really should they be swallowed?
		logger.Errorf("\nCould not match filter %q and regular expression %q\n.%v\n", value, aFilter, err)
	}
	return matches
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
		if agentMatchesFilter(line, value) {
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
