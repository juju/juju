// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/httpcontext"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modelmigration"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/rpc/params"
)

// recordingLogWriter captures the records handed to it so a test can assert on
// how they were stamped.
type recordingLogWriter struct {
	records []corelogger.LogRecord
}

// Log implements [corelogger.LogWriter].
func (w *recordingLogWriter) Log(records []corelogger.LogRecord) error {
	w.records = append(w.records, records...)
	return nil
}

// migrationLogTransferSuite covers how a log transfer request establishes which
// model the migrated log records belong to, and which requests are refused.
//
// /migrate/logtransfer is a controller-scoped route, so the request context
// carries the *controller* model's UUID; the model being migrated is named by
// the migration header instead. Records filed under the wrong model are not
// lost, only invisible to `juju debug-log -m <controller>:<model>`, so nothing
// downstream catches the mistake.
type migrationLogTransferSuite struct {
	modelService         *MockLogTransferModelService
	migrationModeService *MockLogTransferMigrationModeService
}

// TestMigrationLogTransferSuite runs all of the tests that are a part of the
// [migrationLogTransferSuite].
func TestMigrationLogTransferSuite(t *testing.T) {
	tc.Run(t, &migrationLogTransferSuite{})
}

func (s *migrationLogTransferSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelService = NewMockLogTransferModelService(ctrl)
	s.migrationModeService = NewMockLogTransferMigrationModeService(ctrl)

	c.Cleanup(func() {
		s.modelService = nil
		s.migrationModeService = nil
	})
	return ctrl
}

// newLogTransferRequest builds a log transfer request carrying the given value
// in the migration model header. An empty value omits the header entirely.
func newLogTransferRequest(modelUUID string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/migrate/logtransfer", nil)
	if modelUUID != "" {
		req.Header.Set(params.MigrationModelHTTPHeader, modelUUID)
	}
	return req
}

// TestModelUUIDMissingHeader checks that a request which does not name a model
// is refused rather than falling back to the model implied by the route, which
// for this endpoint is the controller model.
func (s *migrationLogTransferSuite) TestModelUUIDMissingHeader(c *tc.C) {
	_, err := logTransferModelUUID(newLogTransferRequest(""))
	tc.Check(c, err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestModelUUIDMalformed checks that a header value which is not a UUID is
// rejected. This matters because the value goes on to key a logger worker, so
// accepting arbitrary strings would let a caller spawn an unbounded number of
// them.
func (s *migrationLogTransferSuite) TestModelUUIDMalformed(c *tc.C) {
	for _, value := range []string{"not-a-uuid", "  ", "deadbeef"} {
		c.Run(value, func(c *testing.T) {
			_, err := logTransferModelUUID(newLogTransferRequest(value))
			tc.Check(c, err, tc.ErrorIs, errors.BadRequest)
		})
	}
}

// TestModelUUIDFromHeader checks that the model is taken from the migration
// header.
func (s *migrationLogTransferSuite) TestModelUUIDFromHeader(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)

	got, err := logTransferModelUUID(newLogTransferRequest(modelUUID.String()))
	tc.Assert(c, err, tc.ErrorIsNil)
	tc.Check(c, got, tc.Equals, modelUUID)
}

// TestModelUUIDPrefersHeaderOverContext checks that the migration header names
// the model even when the request context carries a different one, which on
// this route it always does: ControllerModelHandler populates the context with
// the controller model's UUID.
func (s *migrationLogTransferSuite) TestModelUUIDPrefersHeaderOverContext(c *tc.C) {
	migratedModel := tc.Must(c, coremodel.NewUUID)
	controllerModel := tc.Must(c, coremodel.NewUUID)

	req := newLogTransferRequest(migratedModel.String())
	req = req.WithContext(httpcontext.SetContextModelUUID(req.Context(), controllerModel))

	got, err := logTransferModelUUID(req)
	tc.Assert(c, err, tc.ErrorIsNil)
	tc.Check(c, got, tc.Equals, migratedModel)
	tc.Check(c, got, tc.Not(tc.Equals), controllerModel)
}

// TestValidateUnknownModel checks that a request naming a model which does not
// exist on this controller is refused, and that the migration mode is not even
// consulted for it.
func (s *migrationLogTransferSuite) TestValidateUnknownModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.modelService.EXPECT().CheckModelExists(gomock.Any(), modelUUID).Return(false, nil)

	err := validateLogTransferTarget(c.Context(), modelUUID, s.modelService, s.migrationModeService)
	tc.Check(c, err, tc.ErrorIs, errors.NotFound)
}

// TestValidateModelLookupError checks that a failure to determine whether the
// model exists is surfaced rather than treated as "does not exist".
func (s *migrationLogTransferSuite) TestValidateModelLookupError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	boom := errors.New("boom")
	s.modelService.EXPECT().CheckModelExists(gomock.Any(), modelUUID).Return(false, boom)

	err := validateLogTransferTarget(c.Context(), modelUUID, s.modelService, s.migrationModeService)
	tc.Check(c, err, tc.ErrorMatches, ".*boom.*")
}

// TestValidateWrongMigrationMode checks that log transfer is refused while the
// model is still mid-migration. Log transfer runs after the model proper has
// been imported, so a model in any other mode is not ready to receive records.
func (s *migrationLogTransferSuite) TestValidateWrongMigrationMode(c *tc.C) {
	modes := []modelmigration.MigrationMode{
		modelmigration.MigrationModeImporting,
		modelmigration.MigrationModeExporting,
	}
	for _, mode := range modes {
		ctrl := s.setupMocks(c)

		modelUUID := tc.Must(c, coremodel.NewUUID)
		s.modelService.EXPECT().CheckModelExists(gomock.Any(), modelUUID).Return(true, nil)
		s.migrationModeService.EXPECT().ModelMigrationMode(gomock.Any()).Return(mode, nil)

		err := validateLogTransferTarget(c.Context(), modelUUID, s.modelService, s.migrationModeService)
		tc.Check(c, err, tc.ErrorIs, errors.BadRequest)

		ctrl.Finish()
	}
}

// TestValidateModeError checks that a failure to read the migration mode is
// surfaced.
func (s *migrationLogTransferSuite) TestValidateModeError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.modelService.EXPECT().CheckModelExists(gomock.Any(), modelUUID).Return(true, nil)
	s.migrationModeService.EXPECT().ModelMigrationMode(gomock.Any()).Return(
		modelmigration.MigrationModeNone, errors.New("boom"))

	err := validateLogTransferTarget(c.Context(), modelUUID, s.modelService, s.migrationModeService)
	tc.Check(c, err, tc.ErrorMatches, ".*boom.*")
}

// TestValidateAcceptsImportedModel checks the case a migration actually
// produces: the model exists on this controller and has finished importing.
func (s *migrationLogTransferSuite) TestValidateAcceptsImportedModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.modelService.EXPECT().CheckModelExists(gomock.Any(), modelUUID).Return(true, nil)
	s.migrationModeService.EXPECT().ModelMigrationMode(gomock.Any()).Return(
		modelmigration.MigrationModeNone, nil)

	err := validateLogTransferTarget(c.Context(), modelUUID, s.modelService, s.migrationModeService)
	tc.Check(c, err, tc.ErrorIsNil)
}

// TestWriteLogStampsMigratedModel checks that records are stamped with the
// model the strategy was initialised for. The record's ModelUUID is what
// `juju debug-log -m <model>` filters on, so a wrong value makes the migrated
// history invisible.
func (s *migrationLogTransferSuite) TestWriteLogStampsMigratedModel(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	writer := &recordingLogWriter{}
	strategy := &migrationLoggingStrategy{
		modelUUID:       modelUUID,
		recordLogWriter: writer,
	}

	err := strategy.WriteLog(params.LogRecord{
		Level:   "INFO",
		Module:  "juju.worker.apicaller",
		Message: "connected",
	})
	tc.Assert(c, err, tc.ErrorIsNil)

	tc.Assert(c, len(writer.records), tc.Equals, 1)
	tc.Check(c, writer.records[0].ModelUUID, tc.Equals, modelUUID.String())
}

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

func TestMigrationLoggingStrategySuite(t *testing.T) {
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
	getter := &recordingDomainServicesGetter{err: internalerrors.New("stop")}
	ctxt := httpContext{srv: &Server{shared: &sharedServerContext{
		domainServicesGetter: getter,
	}}}
	req := httptest.NewRequest("GET", "/migrate/logtransfer", nil)
	req.Header.Set(params.MigrationModelHTTPHeader, expectedModelUUID.String())
	req.Header.Set(params.JujuClientVersion, "5.0.0")
	req = req.WithContext(httpcontext.SetContextModelUUID(
		req.Context(), coremodel.UUID("controller-model-uuid"),
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
	expected := internalerrors.New("boom")
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
	expected := internalerrors.New("boom")
	strategy := s.newStrategy(c, func(context.Context, time.Time) error {
		return expected
	}, logWriterFunc(func([]corelogger.LogRecord) error { return nil }))

	err := strategy.WriteLog(s.recordAt(time.Now()))
	c.Assert(err, tc.ErrorIs, expected)
}
