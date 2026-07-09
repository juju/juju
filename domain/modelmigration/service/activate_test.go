// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/controller"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/uuid"
)

// TestGetImportClaim asserts the claim is read through and returned unchanged.
func (s *serviceSuite) TestGetImportClaim(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	claim := modelmigration.ImportClaim{Phase: modelmigration.ImportPhaseActivating}
	s.controllerState.EXPECT().GetImportClaim(gomock.Any(), modelUUID.String()).Return(claim, nil)

	got, err := s.service(c).GetImportClaim(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, claim)
}

// TestGetImportClaimInvalidModelUUID asserts an invalid model UUID is rejected
// before any state call.
func (s *serviceSuite) TestGetImportClaimInvalidModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service(c).GetImportClaim(c.Context(), coremodel.UUID("not-a-uuid"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestSetImportPhaseActivating asserts the transition is delegated to state.
func (s *serviceSuite) TestSetImportPhaseActivating(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.controllerState.EXPECT().SetImportPhaseActivating(gomock.Any(), modelUUID.String()).Return(nil)

	err := s.service(c).SetImportPhaseActivating(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetImportPhaseActivatingInvalidModelUUID asserts an invalid model UUID is
// rejected before any state call.
func (s *serviceSuite) TestSetImportPhaseActivatingInvalidModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).SetImportPhaseActivating(c.Context(), coremodel.UUID("not-a-uuid"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestDeleteActivatedImport asserts the delete is delegated to state.
func (s *serviceSuite) TestDeleteActivatedImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.controllerState.EXPECT().DeleteActivatedImport(gomock.Any(), modelUUID.String()).Return(nil)

	err := s.service(c).DeleteActivatedImport(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// TestDeleteActivatedImportInvalidModelUUID asserts an invalid model UUID is
// rejected before any state call.
func (s *serviceSuite) TestDeleteActivatedImportInvalidModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).DeleteActivatedImport(c.Context(), coremodel.UUID("not-a-uuid"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestEnsureSourceControllerExists asserts the service generates one valid
// address row UUID per address before delegating to state.
func (s *serviceSuite) TestEnsureSourceControllerExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerUUID := tc.Must(c, controller.NewUUID)
	addrs := []string{"10.0.0.1:17070", "10.0.0.2:17070"}
	consumedModels := []string{tc.Must(c, coremodel.NewUUID).String()}

	s.controllerState.EXPECT().EnsureSourceControllerExists(
		gomock.Any(), controllerUUID.String(), "alias", "ca-cert", addrs, gomock.Any(), consumedModels,
	).DoAndReturn(func(
		_ context.Context, _, _, _ string, gotAddrs, gotAddrUUIDs, _ []string,
	) error {
		c.Check(gotAddrUUIDs, tc.HasLen, len(gotAddrs))
		for _, u := range gotAddrUUIDs {
			c.Check(uuid.IsValidUUIDString(u), tc.IsTrue)
		}
		return nil
	})

	err := s.service(c).EnsureSourceControllerExists(
		c.Context(), controllerUUID, "alias", "ca-cert", addrs, consumedModels,
	)
	c.Assert(err, tc.ErrorIsNil)
}

// TestEnsureSourceControllerExistsInvalidControllerUUID asserts an invalid
// controller UUID is rejected before any state call.
func (s *serviceSuite) TestEnsureSourceControllerExistsInvalidControllerUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).EnsureSourceControllerExists(
		c.Context(), controller.UUID("not-a-uuid"), "alias", "ca-cert", nil, nil,
	)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestExternalControllerModelsForImport asserts the mappings are read through
// and returned unchanged.
func (s *serviceSuite) TestExternalControllerModelsForImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	models := []coremodelmigration.OffererModel{{
		ModelUUID:      tc.Must(c, coremodel.NewUUID).String(),
		ControllerUUID: tc.Must(c, coremodel.NewUUID).String(),
	}}
	s.controllerState.EXPECT().ExternalControllerModelsForImport(gomock.Any(), modelUUID.String()).Return(models, nil)

	got, err := s.service(c).ExternalControllerModelsForImport(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, models)
}

// TestExternalControllerModelsForImportInvalidModelUUID asserts an invalid
// model UUID is rejected before any state call.
func (s *serviceSuite) TestExternalControllerModelsForImportInvalidModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service(c).ExternalControllerModelsForImport(c.Context(), coremodel.UUID("not-a-uuid"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetControllerTargetVersion asserts the version is read through state.
func (s *serviceSuite) TestGetControllerTargetVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetControllerTargetVersion(gomock.Any()).Return("4.1.1", nil)

	got, err := s.service(c).GetControllerTargetVersion(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, "4.1.1")
}

// TestDeleteModelImportingStatus asserts the gate delete is delegated to the
// model state.
func (s *serviceSuite) TestDeleteModelImportingStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().DeleteModelImportingStatus(gomock.Any()).Return(nil)

	err := s.service(c).DeleteModelImportingStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

// TestGetModelTargetAgentVersion asserts the model version is read through the
// model state.
func (s *serviceSuite) TestGetModelTargetAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return("4.0.0", nil)

	got, err := s.service(c).GetModelTargetAgentVersion(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, "4.0.0")
}

// TestSetModelTargetAgentVersion asserts the precondition and target version
// are delegated to the model state.
func (s *serviceSuite) TestSetModelTargetAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().SetModelTargetAgentVersion(gomock.Any(), "4.0.0", "4.1.1").Return(nil)

	err := s.service(c).SetModelTargetAgentVersion(c.Context(), "4.0.0", "4.1.1")
	c.Assert(err, tc.ErrorIsNil)
}
