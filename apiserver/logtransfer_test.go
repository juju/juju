// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"net/http/httptest"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	apiserverhttpcontext "github.com/juju/juju/apiserver/httpcontext"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/rpc/params"
)

type logWriterFunc func([]corelogger.LogRecord) error

func (f logWriterFunc) Log(records []corelogger.LogRecord) error {
	return f(records)
}

type recordingDomainServicesGetter struct {
	modelUUID coremodel.UUID
	err       error
}

func (g *recordingDomainServicesGetter) ServicesForModel(
	_ context.Context, modelUUID coremodel.UUID,
) (services.DomainServices, error) {
	g.modelUUID = modelUUID
	return nil, g.err
}

type migrationLoggingStrategySuite struct{}

func TestMigrationLoggingStrategySuite(t *stdtesting.T) {
	tc.Run(t, &migrationLoggingStrategySuite{})
}

func (s *migrationLoggingStrategySuite) newStrategy(
	c *tc.C,
	setter func(context.Context, time.Time) error,
	writer corelogger.LogWriter,
) *migrationLoggingStrategy {
	return &migrationLoggingStrategy{
		recordLogWriter:        writer,
		setLastLogTransferTime: setter,
		modelUUID:              coremodel.UUID("7220fe1f-96c8-4c1e-ba1a-9d0000000000"),
		requestContext:         c.Context(),
	}
}

func (s *migrationLoggingStrategySuite) recordAt(t time.Time) params.LogRecord {
	return params.LogRecord{
		Time:    t,
		Entity:  "unit-foo/0",
		Level:   "INFO",
		Module:  "juju.worker",
		Message: "a log message",
	}
}

func (s *migrationLoggingStrategySuite) TestInitUsesMigrationModelHeader(c *tc.C) {
	expectedModelUUID := coremodel.UUID("7220fe1f-96c8-4c1e-ba1a-9d0000000000")
	getter := &recordingDomainServicesGetter{err: errors.New("stop")}
	ctxt := httpContext{srv: &Server{shared: &sharedServerContext{
		domainServicesGetter: getter,
	}}}
	req := httptest.NewRequest("GET", "/migrate/logtransfer", nil)
	req.Header.Set(params.MigrationModelHTTPHeader, expectedModelUUID.String())
	req = req.WithContext(apiserverhttpcontext.SetContextModelUUID(
		req.Context(), "controller-model-uuid",
	))

	err := new(migrationLoggingStrategy).init(ctxt, req)
	c.Assert(err, tc.ErrorIs, getter.err)
	c.Check(getter.modelUUID, tc.Equals, expectedModelUUID)
}

func (s *migrationLoggingStrategySuite) TestWriteLogTracksEveryPeriod(c *tc.C) {
	var tracked []time.Time
	strategy := s.newStrategy(c, func(_ context.Context, t time.Time) error {
		tracked = append(tracked, t)
		return nil
	}, logWriterFunc(func([]corelogger.LogRecord) error { return nil }))

	base := time.Date(2026, 9, 27, 6, 23, 24, 0, time.UTC)
	for _, t := range []time.Time{
		base,
		base.Add(time.Minute),
		base.Add(2 * time.Minute),
	} {
		c.Assert(strategy.WriteLog(s.recordAt(t)), tc.ErrorIsNil)
	}

	c.Assert(tracked, tc.DeepEquals, []time.Time{
		base,
		base.Add(2 * time.Minute),
	})
}

func (s *migrationLoggingStrategySuite) TestWriteLogDoesNotTrackZeroTime(c *tc.C) {
	tracked := false
	strategy := s.newStrategy(c, func(context.Context, time.Time) error {
		tracked = true
		return nil
	}, logWriterFunc(func([]corelogger.LogRecord) error { return nil }))

	err := strategy.WriteLog(s.recordAt(time.Time{}))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tracked, tc.IsFalse)
}

func (s *migrationLoggingStrategySuite) TestWriteLogFailureDoesNotTrack(c *tc.C) {
	expected := errors.New("boom")
	tracked := false
	strategy := s.newStrategy(c, func(context.Context, time.Time) error {
		tracked = true
		return nil
	}, logWriterFunc(func([]corelogger.LogRecord) error { return expected }))

	err := strategy.WriteLog(s.recordAt(time.Now()))
	c.Assert(err, tc.ErrorIs, expected)
	c.Check(tracked, tc.IsFalse)
}

func (s *migrationLoggingStrategySuite) TestTrackingFailureFailsWrite(c *tc.C) {
	expected := errors.New("boom")
	strategy := s.newStrategy(c, func(context.Context, time.Time) error {
		return expected
	}, logWriterFunc(func([]corelogger.LogRecord) error { return nil }))

	err := strategy.WriteLog(s.recordAt(time.Now()))
	c.Assert(err, tc.ErrorIs, expected)
}
