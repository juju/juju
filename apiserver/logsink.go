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
	"github.com/juju/loggo"
	"github.com/juju/version"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/logsink"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/logdb"
)

const (
	defaultDBLoggerBufferSize    = 1000
	defaultDBLoggerFlushInterval = 2 * time.Second
)

type agentLoggingStrategy struct {
	dbloggers  *dbloggers
	fileLogger io.Writer

	dblogger   recordLogger
	releaser   func()
	version    version.Number
	entity     string
	filePrefix string
}

type recordLogger interface {
	Log([]state.LogRecord) error
}

// dbloggers contains a map of buffered DB loggers. When one of the
// logging strategies requires a DB logger, it uses this to get it.
// When the State corresponding to the DB logger is removed from the
// state pool, the strategies must call the dbloggers.remove method.
type dbloggers struct {
	clock                 clock.Clock
	dbLoggerBufferSize    int
	dbLoggerFlushInterval time.Duration
	mu                    sync.Mutex
	loggers               map[*state.State]*bufferedDbLogger
}

func (d *dbloggers) get(st *state.State) recordLogger {
	d.mu.Lock()
	defer d.mu.Unlock()
	if l, ok := d.loggers[st]; ok {
		return l
	}
	if d.loggers == nil {
		d.loggers = make(map[*state.State]*bufferedDbLogger)
	}
	dbl := state.NewDbLogger(st)
	l := &bufferedDbLogger{dbl, logdb.NewBufferedLogger(
		dbl,
		d.dbLoggerBufferSize,
		d.dbLoggerFlushInterval,
		d.clock,
	)}
	d.loggers[st] = l
	return l
}

func (d *dbloggers) remove(st *state.State) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if l, ok := d.loggers[st]; ok {
		l.Close()
		delete(d.loggers, st)
	}
}

// dispose closes all dbloggers in the map, and clears the memory. This
// must not be called concurrently with any other dbloggers methods.
func (d *dbloggers) dispose() {
	for _, l := range d.loggers {
		l.Close()
	}
	d.loggers = nil
}

type bufferedDbLogger struct {
	dbl *state.DbLogger
	*logdb.BufferedLogger
}

func (b *bufferedDbLogger) Close() error {
	err := errors.Trace(b.Flush())
	b.dbl.Close()
	return err
}

// newAgentLogWriteCloserFunc returns a function that will create a
// logsink.LoggingStrategy given an *http.Request, that writes log
// messages to the given writer and also to the state database.
func newAgentLogWriteCloserFunc(
	ctxt httpContext,
	fileLogger io.Writer,
	dbloggers *dbloggers,
) logsink.NewLogWriteCloserFunc {
	return func(req *http.Request) (logsink.LogWriteCloser, error) {
		strategy := &agentLoggingStrategy{
			dbloggers:  dbloggers,
			fileLogger: fileLogger,
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
	// This would be a problem if non-Juju clients (e.g. the
	// GUI) could use this endpoint since we require that the
	// *Juju* version be provided as part of the request. Any
	// attempt to open this endpoint to broader access must
	// address this caveat appropriately.
	ver, err := common.JujuClientVersionFromRequest(req)
	if err != nil {
		st.Release()
		return errors.Trace(err)
	}
	s.version = ver
	s.entity = entity.Tag().String()
	s.filePrefix = st.ModelUUID() + ":"
	s.dblogger = s.dbloggers.get(st.State)
	s.releaser = func() {
		if removed := st.Release(); removed {
			s.dbloggers.remove(st.State)
		}
	}
	return nil
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
	dbErr := errors.Annotate(s.dblogger.Log([]state.LogRecord{{
		Time:     m.Time,
		Entity:   s.entity,
		Version:  s.version,
		Module:   m.Module,
		Location: m.Location,
		Level:    level,
		Message:  m.Message,
	}}), "logging to DB failed")

	m.Entity = s.entity
	fileErr := errors.Annotate(
		logToFile(s.fileLogger, s.filePrefix, m),
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
	}, " ") + "\n"))
	return err
}
