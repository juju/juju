// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"io"
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type migrationLoggingStrategy struct {
	ctxt       httpContext
	st         *state.State
	releaser   func()
	filePrefix string
	dbLogger   *state.DbLogger
	tracker    *logTracker
	fileLogger io.Writer
}

func newMigrationLoggingStrategy(ctxt httpContext, fileLogger io.Writer) LoggingStrategy {
	return &migrationLoggingStrategy{ctxt: ctxt, fileLogger: fileLogger}
}

// Authenticate checks that the user is a controller superuser and
// that the requested model is migrating. Part of LoggingStrategy.
func (s *migrationLoggingStrategy) Authenticate(req *http.Request) error {
	// Require MigrationModeNone because logtransfer happens after the
	// model proper is completely imported.
	st, releaser, err := s.ctxt.stateForMigration(req, state.MigrationModeNone)
	if err != nil {
		return errors.Trace(err)
	}

	// Here the log messages are expected to be coming from another
	// Juju controller, so the version number provided should be the
	// Juju version of the source controller. Require this to be
	// passed, even though we don't use it anywhere at the moment - it
	// provides future-proofing if we need to do some kind of
	// conversion of log messages from an old client.
	_, err = jujuClientVersionFromReq(req)
	if err != nil {
		releaser()
		return errors.Trace(err)
	}
	s.st = st
	s.releaser = releaser
	return nil
}

// Start creates the destination DB logger. Part of LoggingStrategy.
func (s *migrationLoggingStrategy) Start() {
	s.filePrefix = s.st.ModelUUID() + ":"
	s.dbLogger = state.NewDbLogger(s.st)
	s.tracker = newLogTracker(s.st)
}

// Log writes the given record to the DB and to the backup file
// logger. Part of LoggingStrategy.
func (s *migrationLoggingStrategy) Log(m params.LogRecord) bool {
	level, _ := loggo.ParseLevel(m.Level)
	dbErr := s.dbLogger.Log(m.Time, m.Entity, m.Module, m.Location, level, m.Message)
	if dbErr == nil {
		dbErr = s.tracker.Track(m.Time)
	}
	if dbErr != nil {
		logger.Errorf("logging to DB failed: %v", dbErr)
	}

	fileErr := logToFile(s.fileLogger, s.filePrefix, m)
	if fileErr != nil {
		logger.Errorf("logging to file logger failed: %v", fileErr)
	}

	return dbErr == nil && fileErr == nil
}

// Stop imdicates that there are no more log records coming, so we can
// release resources and close loggers. Part of LoggingStrategy.
func (s *migrationLoggingStrategy) Stop() {
	s.dbLogger.Close()
	s.tracker.Close()
	s.releaser()
	// Perhaps clear s.st and s.releaser?
}

const trackingPeriod = 2 * time.Minute

func newLogTracker(st *state.State) *logTracker {
	return &logTracker{tracker: state.NewLastSentLogTracker(st, st.ModelUUID(), "migration-logtransfer")}
}

// logTracker assumes that log messages are sent in time order (which
// is how they come from debug-log). If not, this won't give
// meaningful values, and transferring logs could produce large
// numbers of duplicates if restarted.
type logTracker struct {
	tracker     *state.LastSentLogTracker
	trackedTime time.Time
	seenTime    time.Time
}

func (l *logTracker) Track(t time.Time) error {
	l.seenTime = t
	if t.Sub(l.trackedTime) < trackingPeriod {
		return nil
	}
	l.trackedTime = t
	return errors.Trace(l.tracker.Set(0, t.UnixNano()))
}

func (l *logTracker) Close() error {
	err := l.tracker.Set(0, l.seenTime.UnixNano())
	if err != nil {
		l.tracker.Close()
		return errors.Trace(err)
	}
	return errors.Trace(l.tracker.Close())
}
