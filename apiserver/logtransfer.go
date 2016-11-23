// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type migrationLoggingStrategy struct {
	ctxt       httpContext
	st         *state.State
	filePrefix string
	dbLogger   *state.DbLogger
	fileLogger io.Writer
}

func newMigrationLoggingStrategy(ctxt httpContext, fileLogger io.Writer) LoggingStrategy {
	return &migrationLoggingStrategy{ctxt: ctxt, fileLogger: fileLogger}
}

// Authenticate checks that the user is a controller superuser and
// that the requested model is migrating. Part of LoggingStrategy.
func (s *migrationLoggingStrategy) Authenticate(req *http.Request) error {
	st, err := s.ctxt.stateForMigration(req)
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
		return errors.Trace(err)
	}
	s.st = st
	return nil
}

// Start creates the destination DB logger. Part of LoggingStrategy.
func (s *migrationLoggingStrategy) Start() {
	s.filePrefix = s.st.ModelUUID() + ":"
	s.dbLogger = state.NewDbLogger(s.st)
}

// Log writes the given record to the DB and to the backup file
// logger. Part of LoggingStrategy.
func (s *migrationLoggingStrategy) Log(m params.LogRecord) bool {
	level, _ := loggo.ParseLevel(m.Level)
	dbErr := s.dbLogger.Log(m.Time, m.Entity, m.Module, m.Location, level, m.Message)
	if dbErr != nil {
		logger.Errorf("logging to DB failed: %v", dbErr)
	}
	fileErr := logToFile(s.fileLogger, s.filePrefix, m)
	if fileErr != nil {
		logger.Errorf("logging to logsink.log failed: %v", fileErr)
	}
	return dbErr == nil && fileErr == nil
}

// Stop imdicates that there are no more log records coming, so we can
// release resources and close loggers. Part of LoggingStrategy.
func (s *migrationLoggingStrategy) Stop() {
	s.dbLogger.Close()
	s.ctxt.release(s.st)
}
