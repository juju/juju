// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/logsink"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/syslogger"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

const (
	defaultLoggerBufferSize    = 1000
	defaultLoggerFlushInterval = 2 * time.Second
)

// RecordLogger defines an interface for logging a singular log record.
type RecordLogger interface {
	Log([]corelogger.LogRecord) error
}

// apiServerLoggers contains a map of loggers, either a DB or syslog. Both
// DB and syslog are wrapped by a buffered logger to prevent flooding of
// the DB or syslog in TRACE level mode.
// When one of the logging strategies requires a logger, it uses this to get it.
// When the State corresponding to the logger is removed from the
// state pool, the strategies must call the apiServerLoggers.removeLogger
// method.
type apiServerLoggers struct {
	syslogger           syslogger.SysLogger
	loggingOutputs      []string
	clock               clock.Clock
	loggerBufferSize    int
	loggerFlushInterval time.Duration
	mu                  sync.Mutex
	loggers             map[*state.State]*bufferedLogger
}

func (d *apiServerLoggers) getLogger(st *state.State) RecordLogger {
	d.mu.Lock()
	defer d.mu.Unlock()
	if l, ok := d.loggers[st]; ok {
		return l
	}
	if d.loggers == nil {
		d.loggers = make(map[*state.State]*bufferedLogger)
	}

	outputs := d.getLoggers(st)

	bufferedLogger := &bufferedLogger{
		BufferedLogger: corelogger.NewBufferedLogger(
			outputs,
			d.loggerBufferSize,
			d.loggerFlushInterval,
			d.clock,
		),
		closer: outputs,
	}
	d.loggers[st] = bufferedLogger
	return bufferedLogger
}

func (d *apiServerLoggers) removeLogger(st *state.State) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if l, ok := d.loggers[st]; ok {
		l.Close()
		delete(d.loggers, st)
	}
}

func (d *apiServerLoggers) getLoggers(st *state.State) corelogger.LoggerCloser {
	// If the logging output is empty, then send it to state.
	if len(d.loggingOutputs) == 0 {
		return state.NewDbLogger(st)
	}

	return corelogger.MakeLoggers(d.loggingOutputs, corelogger.LoggersConfig{
		SysLogger: func() corelogger.Logger {
			return d.syslogger
		},
		DBLogger: func() corelogger.Logger {
			return state.NewDbLogger(st)
		},
	})
}

// dispose closes all apiServerLoggers in the map, and clears the memory. This
// must not be called concurrently with any other apiServerLoggers methods.
func (d *apiServerLoggers) dispose() {
	for _, l := range d.loggers {
		_ = l.Close()
	}
	d.loggers = nil
}

type bufferedLogger struct {
	*corelogger.BufferedLogger
	closer io.Closer
}

func (b *bufferedLogger) Close() error {
	err := errors.Trace(b.BufferedLogger.Flush())
	_ = b.closer.Close()
	return err
}

type agentLoggingStrategy struct {
	apiServerLoggers *apiServerLoggers
	fileLogger       io.Writer

	recordLogger RecordLogger
	releaser     func()
	version      version.Number
	entity       string
	modelUUID    string
}

// newAgentLogWriteCloserFunc returns a function that will create a
// logsink.LoggingStrategy given an *http.Request, that writes log
// messages to the given writer and also to the state database.
func newAgentLogWriteCloserFunc(
	ctxt httpContext,
	fileLogger io.Writer,
	apiServerLoggers *apiServerLoggers,
) logsink.NewLogWriteCloserFunc {
	return func(req *http.Request) (logsink.LogWriteCloser, error) {
		strategy := &agentLoggingStrategy{
			apiServerLoggers: apiServerLoggers,
			fileLogger:       fileLogger,
		}
		if err := strategy.init(ctxt, req); err != nil {
			return nil, errors.Annotate(err, "initialising agent logsink session")
		}
		return strategy, nil
	}
}

func (s *agentLoggingStrategy) init(ctxt httpContext, req *http.Request) error {
	st, entity, err := ctxt.stateForRequestAuthenticatedAgent(req)
	if err != nil {
		return errors.Trace(err)
	}
	// Note that this endpoint is agent-only. Thus the only
	// callers will necessarily provide their Juju version.
	//
	// This would be a problem if non-Juju clients could use
	// this endpoint since we require that the *Juju* version
	// be provided as part of the request. Any attempt to open
	// this endpoint to broader access must address this caveat
	// appropriately.
	ver, err := common.JujuClientVersionFromRequest(req)
	if err != nil {
		st.Release()
		return errors.Trace(err)
	}
	s.version = ver
	s.entity = entity.Tag().String()
	s.modelUUID = st.ModelUUID()
	s.recordLogger = s.apiServerLoggers.getLogger(st.State)
	s.releaser = func() {
		if removed := st.Release(); removed {
			s.apiServerLoggers.removeLogger(st.State)
		}
	}
	return nil
}

func (s *agentLoggingStrategy) filePrefix() string {
	return s.modelUUID + ":"
}

// Close is part of the logsink.LogWriteCloser interface.
//
// Close releases the StatePool entry, closing the DB logger
// if the State is closed/removed. The file logger is owned
// by the apiserver, so it is not closed.
func (s *agentLoggingStrategy) Close() error {
	s.releaser()
	return nil
}

// WriteLog is part of the logsink.LogWriteCloser interface.
func (s *agentLoggingStrategy) WriteLog(m params.LogRecord) error {
	level, _ := loggo.ParseLevel(m.Level)
	dbErr := errors.Annotate(s.recordLogger.Log([]corelogger.LogRecord{{
		Time:      m.Time,
		Entity:    s.entity,
		Version:   s.version,
		Module:    m.Module,
		Location:  m.Location,
		Level:     level,
		Message:   m.Message,
		Labels:    m.Labels,
		ModelUUID: s.modelUUID,
	}}), "logging to DB failed")

	// If the log entries cannot be inserted to the DB log out an error
	// to let users know. See LP1930899.
	if dbErr != nil {
		// If this fails then the next logToFile will fail as well; no
		// need to check for errors here.
		_ = logToFile(s.fileLogger, s.filePrefix(), params.LogRecord{
			Time:    time.Now(),
			Module:  "juju.apiserver",
			Level:   loggo.ERROR.String(),
			Message: errors.Annotate(dbErr, "unable to persist log entry to the database").Error(),
		})
	}

	m.Entity = s.entity
	fileErr := errors.Annotate(
		logToFile(s.fileLogger, s.filePrefix(), m),
		"logging to logsink.log failed",
	)
	err := dbErr
	if err == nil {
		err = fileErr
	} else if fileErr != nil {
		err = errors.Errorf("%s; %s", dbErr, fileErr)
	}
	return err
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
		strings.Join(m.Labels, ","),
	}, " ") + "\n"))
	return err
}
