// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/httpcontext"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modelmigration"
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
// carries the *controller* model's UUID. The model being migrated is named by
// the migration header instead. Getting this wrong is not loud: it silently
// files a migrated model's entire log history under the controller model, where
// `juju debug-log -m <controller>:<model>` will never show it.
type migrationLogTransferSuite struct {
	modelService *MockMigrationLogModelService
	modeService  *MockMigrationLogModeService
}

// TestMigrationLogTransferSuite runs all of the tests that are a part of the
// [migrationLogTransferSuite].
func TestMigrationLogTransferSuite(t *testing.T) {
	tc.Run(t, &migrationLogTransferSuite{})
}

func (s *migrationLogTransferSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelService = NewMockMigrationLogModelService(ctrl)
	s.modeService = NewMockMigrationLogModeService(ctrl)

	c.Cleanup(func() {
		s.modelService = nil
		s.modeService = nil
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
// is refused rather than silently falling back to the model implied by the
// route, which for this endpoint is the controller model.
func (s *migrationLogTransferSuite) TestModelUUIDMissingHeader(c *tc.C) {
	_, err := migrationLogModelUUID(newLogTransferRequest(""))
	tc.Check(c, err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestModelUUIDMalformed checks that a header value which is not a UUID is
// rejected. This matters because the value goes on to key a logger worker, so
// accepting arbitrary strings would let a caller spawn an unbounded number of
// them.
//
// Note that [migrationLogModelUUID] takes no services at all, so a malformed
// UUID structurally cannot reach the model lookup.
func (s *migrationLogTransferSuite) TestModelUUIDMalformed(c *tc.C) {
	for _, value := range []string{"not-a-uuid", "  ", "deadbeef"} {
		c.Run(value, func(c *testing.T) {
			_, err := migrationLogModelUUID(newLogTransferRequest(value))
			tc.Check(c, err, tc.ErrorIs, errors.BadRequest)
		})
	}
}

// TestModelUUIDFromHeader checks that the model is taken from the migration
// header.
func (s *migrationLogTransferSuite) TestModelUUIDFromHeader(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)

	got, err := migrationLogModelUUID(newLogTransferRequest(modelUUID.String()))
	tc.Assert(c, err, tc.ErrorIsNil)
	tc.Check(c, got, tc.Equals, modelUUID)
}

// TestModelUUIDPrefersHeaderOverContext is the regression test for the defect
// this endpoint had: the request context is populated by ControllerModelHandler
// with the *controller* model's UUID, and reading it instead of the migration
// header filed every migrated record under the controller model.
//
// The header and the context are deliberately set to different models here, and
// the header must win.
func (s *migrationLogTransferSuite) TestModelUUIDPrefersHeaderOverContext(c *tc.C) {
	migratedModel := tc.Must(c, coremodel.NewUUID)
	controllerModel := tc.Must(c, coremodel.NewUUID)

	req := newLogTransferRequest(migratedModel.String())
	req = req.WithContext(httpcontext.SetContextModelUUID(req.Context(), controllerModel))

	got, err := migrationLogModelUUID(req)
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

	err := validateMigrationLogTarget(c.Context(), modelUUID, s.modelService, s.modeService)
	tc.Check(c, err, tc.ErrorIs, errors.NotFound)
}

// TestValidateModelLookupError checks that a failure to determine whether the
// model exists is surfaced rather than treated as "does not exist".
func (s *migrationLogTransferSuite) TestValidateModelLookupError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	boom := errors.New("boom")
	s.modelService.EXPECT().CheckModelExists(gomock.Any(), modelUUID).Return(false, boom)

	err := validateMigrationLogTarget(c.Context(), modelUUID, s.modelService, s.modeService)
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
		s.modeService.EXPECT().ModelMigrationMode(gomock.Any()).Return(mode, nil)

		err := validateMigrationLogTarget(c.Context(), modelUUID, s.modelService, s.modeService)
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
	s.modeService.EXPECT().ModelMigrationMode(gomock.Any()).Return(
		modelmigration.MigrationModeNone, errors.New("boom"))

	err := validateMigrationLogTarget(c.Context(), modelUUID, s.modelService, s.modeService)
	tc.Check(c, err, tc.ErrorMatches, ".*boom.*")
}

// TestValidateAcceptsImportedModel checks the case a migration actually
// produces: the model exists on this controller and has finished importing.
func (s *migrationLogTransferSuite) TestValidateAcceptsImportedModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.modelService.EXPECT().CheckModelExists(gomock.Any(), modelUUID).Return(true, nil)
	s.modeService.EXPECT().ModelMigrationMode(gomock.Any()).Return(
		modelmigration.MigrationModeNone, nil)

	err := validateMigrationLogTarget(c.Context(), modelUUID, s.modelService, s.modeService)
	tc.Check(c, err, tc.ErrorIsNil)
}

// TestWriteLogStampsMigratedModel checks that records are filed under the model
// named by the migration header. This is the property the whole endpoint exists
// to get right: the record's ModelUUID is what `juju debug-log -m <model>`
// filters on, so a wrong value makes the migrated history invisible.
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
