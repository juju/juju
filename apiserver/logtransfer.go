// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/logsink"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type migrationLoggingStrategy struct {
	st         *state.State
	releaser   func()
	filePrefix string
	dbLogger   *state.DbLogger
	tracker    *logTracker
}

// newMigrationLogWriteCloserFunc returns a function that will create a
// logsink.LoggingStrategy given an *http.Request, that writes log
// messages to the state database and tracks their migration.
func newMigrationLogWriteCloserFunc(ctxt httpContext) logsink.NewLogWriteCloserFunc {
	return func(req *http.Request) (logsink.LogWriteCloser, error) {
		strategy := &migrationLoggingStrategy{}
		if err := strategy.init(ctxt, req); err != nil {
			return nil, errors.Annotate(err, "initialising migration logsink session")
		}
		return strategy, nil
	}
}

func (s *migrationLoggingStrategy) init(ctxt httpContext, req *http.Request) error {
	// Require MigrationModeNone because logtransfer happens after the
	// model proper is completely imported.
	st, releaser, err := ctxt.stateForMigration(req, state.MigrationModeNone)
	if err != nil {
		return errors.Trace(err)
	}

	// Here the log messages are expected to be coming from another
	// Juju controller, so the version number provided should be the
	// Juju version of the source controller. Require this to be
	// passed, even though we don't use it anywhere at the moment - it
	// provides future-proofing if we need to do some kind of
	// conversion of log messages from an old client.
	_, err = logsink.JujuClientVersionFromRequest(req)
	if err != nil {
		releaser()
		return errors.Trace(err)
	}

	s.releaser = releaser
	s.filePrefix = st.ModelUUID() + ":"
	s.dbLogger = state.NewDbLogger(st)
	s.tracker = newLogTracker(st)
	return nil
}

// WriteLog is part of the logsink.LogWriteCloser interface.
func (s *migrationLoggingStrategy) WriteLog(m params.LogRecord) error {
	level, _ := loggo.ParseLevel(m.Level)
	err := s.dbLogger.Log(m.Time, m.Entity, m.Module, m.Location, level, m.Message)
	if err == nil {
		err = s.tracker.Track(m.Time)
	}
	return errors.Annotate(err, "logging to DB failed")
}

// Close is part of the logsink.LogWriteCloser interface.
func (s *migrationLoggingStrategy) Close() error {
	s.dbLogger.Close()
	s.tracker.Close()
	s.releaser()
	return nil
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
