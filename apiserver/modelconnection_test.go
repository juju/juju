// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type modelConnectionSuite struct {
	modelService     *MockModelService
	migrationService *MockModelMigrationService

	modelUUID coremodel.UUID
}

func TestModelConnectionSuite(t *testing.T) {
	tc.Run(t, &modelConnectionSuite{})
}

func (s *modelConnectionSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelService = NewMockModelService(ctrl)
	s.migrationService = NewMockModelMigrationService(ctrl)
	s.modelUUID = coremodel.UUID(uuid.MustNewUUID().String())
	return ctrl
}

func (s *modelConnectionSuite) expectPresence(presence model.ModelPresence) {
	s.modelService.EXPECT().GetModelPresence(gomock.Any(), s.modelUUID).Return(presence, nil)
}

func (s *modelConnectionSuite) expectMode(mode modelmigration.MigrationMode) {
	s.migrationService.EXPECT().ModelMigrationMode(gomock.Any()).Return(mode, nil)
}

func (s *modelConnectionSuite) connection(c *tc.C) (modelConnection, error) {
	return modelConnectionFor(c.Context(), s.modelService, s.migrationService, s.modelUUID)
}

// TestActivatedModelIsConnectable verifies the ordinary case: an activated
// model is served, carrying its name, type and migration mode.
func (s *modelConnectionSuite) TestActivatedModelIsConnectable(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectPresence(model.ModelPresence{
		Name:      "prod",
		ModelType: coremodel.IAAS,
		Activated: true,
	})
	s.expectMode(modelmigration.MigrationModeNone)

	conn, err := s.connection(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn.connectable, tc.IsTrue)
	c.Check(conn.modelName, tc.Equals, "prod")
	c.Check(conn.modelType, tc.Equals, coremodel.IAAS)
	c.Check(conn.migrationMode, tc.Equals, modelmigration.MigrationModeNone)
}

// TestImportingModelIsConnectable is the case this whole seam exists for: a
// model a migration is still importing has not been activated yet, but its
// agents must be able to log in and validate against this controller during
// the migration's VALIDATION phase.
func (s *modelConnectionSuite) TestImportingModelIsConnectable(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectPresence(model.ModelPresence{
		Name:      "incoming",
		ModelType: coremodel.IAAS,
		Activated: false,
	})
	s.expectMode(modelmigration.MigrationModeImporting)

	conn, err := s.connection(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn.connectable, tc.IsTrue)
	c.Check(conn.modelName, tc.Equals, "incoming")
	c.Check(conn.modelType, tc.Equals, coremodel.IAAS)
	// The importing mode is what restrictAPIRootDuringMaintenance uses to keep
	// user logins out while agents are let through.
	c.Check(conn.migrationMode, tc.Equals, modelmigration.MigrationModeImporting)
}

// TestUnactivatedModelWithoutImportIsNotConnectable pins the narrow window: a
// half built model with no migration importing it - an add-model that never
// completed, say - stays invisible, exactly as before this seam existed.
func (s *modelConnectionSuite) TestUnactivatedModelWithoutImportIsNotConnectable(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectPresence(model.ModelPresence{
		Name:      "half-built",
		ModelType: coremodel.IAAS,
		Activated: false,
	})
	s.expectMode(modelmigration.MigrationModeNone)

	conn, err := s.connection(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn.connectable, tc.IsFalse)
}

// TestUnactivatedExportingModelIsNotConnectable verifies that only an import
// opens the window. Exporting is a source-side mode and never applies to a
// model that has not been activated here.
func (s *modelConnectionSuite) TestUnactivatedExportingModelIsNotConnectable(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectPresence(model.ModelPresence{ModelType: coremodel.IAAS, Activated: false})
	s.expectMode(modelmigration.MigrationModeExporting)

	conn, err := s.connection(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn.connectable, tc.IsFalse)
}

// TestAbsentModelIsNotConnectable verifies that a model with no row at all is
// reported as not connectable rather than as an error, so the caller can fall
// through to its redirect handling.
func (s *modelConnectionSuite) TestAbsentModelIsNotConnectable(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelService.EXPECT().GetModelPresence(gomock.Any(), s.modelUUID).
		Return(model.ModelPresence{}, modelerrors.NotFound)

	conn, err := s.connection(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn.connectable, tc.IsFalse)
}

// TestPresenceErrorPropagates verifies that a real lookup failure is not
// silently turned into "not connectable", which would make a database problem
// look like a deleted model.
func (s *modelConnectionSuite) TestPresenceErrorPropagates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	boom := errors.New("boom")
	s.modelService.EXPECT().GetModelPresence(gomock.Any(), s.modelUUID).
		Return(model.ModelPresence{}, boom)

	_, err := s.connection(c)
	c.Assert(err, tc.ErrorIs, boom)
}

// TestMigrationModeErrorPropagates verifies the same for the migration mode
// lookup.
func (s *modelConnectionSuite) TestMigrationModeErrorPropagates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	boom := errors.New("boom")
	s.expectPresence(model.ModelPresence{ModelType: coremodel.IAAS, Activated: true})
	s.migrationService.EXPECT().ModelMigrationMode(gomock.Any()).
		Return(modelmigration.MigrationModeNone, boom)

	_, err := s.connection(c)
	c.Assert(err, tc.ErrorIs, boom)
}

// isModelAvailable is the pre-login gate: it decides whether the websocket is
// served at all, before Login is ever reached. These cases pin the behaviour
// it adds on top of modelConnectionFor - the redirect fall-through.

func (s *modelConnectionSuite) available(c *tc.C) error {
	return (&Server{}).isModelAvailable(
		c.Context(), s.modelService, s.migrationService, s.modelUUID)
}

// TestAvailableWhileImporting verifies that the connection is served for a
// model a migration is still importing. Without this the migration's
// VALIDATION phase fails: every agent is refused before it can even log in.
func (s *modelConnectionSuite) TestAvailableWhileImporting(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectPresence(model.ModelPresence{ModelType: coremodel.IAAS, Activated: false})
	s.expectMode(modelmigration.MigrationModeImporting)

	c.Assert(s.available(c), tc.ErrorIsNil)
}

// TestAvailableWhenActivated covers the ordinary case.
func (s *modelConnectionSuite) TestAvailableWhenActivated(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectPresence(model.ModelPresence{ModelType: coremodel.IAAS, Activated: true})
	s.expectMode(modelmigration.MigrationModeNone)

	c.Assert(s.available(c), tc.ErrorIsNil)
}

// TestNotAvailableWhenAbsent verifies an unknown model is still reported as
// not found, which is what turns into "unknown model" on the wire.
func (s *modelConnectionSuite) TestNotAvailableWhenAbsent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelService.EXPECT().GetModelPresence(gomock.Any(), s.modelUUID).
		Return(model.ModelPresence{}, modelerrors.NotFound)
	s.modelService.EXPECT().ModelRedirection(gomock.Any(), s.modelUUID).
		Return(model.ModelRedirection{}, modelerrors.ModelNotRedirected)

	c.Assert(s.available(c), tc.ErrorIs, modelerrors.NotFound)
}

// TestNotAvailableWhenHalfBuilt pins the narrow window at the connection gate:
// a model left unactivated with no import covering it is refused, so a failed
// add-model stays as invisible as it was before.
func (s *modelConnectionSuite) TestNotAvailableWhenHalfBuilt(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectPresence(model.ModelPresence{ModelType: coremodel.IAAS, Activated: false})
	s.expectMode(modelmigration.MigrationModeNone)
	s.modelService.EXPECT().ModelRedirection(gomock.Any(), s.modelUUID).
		Return(model.ModelRedirection{}, modelerrors.ModelNotRedirected)

	c.Assert(s.available(c), tc.ErrorIs, modelerrors.NotFound)
}

// TestAvailableWhenMigratedAway verifies the redirect fall-through still
// works: a model that left this controller is served so that Login can answer
// with a redirect rather than a flat refusal.
func (s *modelConnectionSuite) TestAvailableWhenMigratedAway(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelService.EXPECT().GetModelPresence(gomock.Any(), s.modelUUID).
		Return(model.ModelPresence{}, modelerrors.NotFound)
	s.modelService.EXPECT().ModelRedirection(gomock.Any(), s.modelUUID).
		Return(model.ModelRedirection{ControllerUUID: "other"}, nil)

	c.Assert(s.available(c), tc.ErrorIsNil)
}
