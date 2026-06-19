// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
)

// TestBeginImportSuccess asserts that a fresh claim is returned unchanged.
func (s *serviceSuite) TestBeginImportSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID).String()
	s.controllerState.EXPECT().BeginImport(gomock.Any(), modelUUID, "source-uuid").Return("claim-uuid", nil)

	claimUUID, err := s.service().BeginImport(c.Context(), modelUUID, "source-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claimUUID, tc.Equals, "claim-uuid")
}

// TestBeginImportDuplicateImporting asserts that a duplicate claim still in
// the importing phase reports a plain AlreadyExists error.
func (s *serviceSuite) TestBeginImportDuplicateImporting(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID).String()
	s.controllerState.EXPECT().BeginImport(gomock.Any(), modelUUID, "source-uuid").Return(
		"", modelmigrationerrors.ErrImportClaimExists)
	s.controllerState.EXPECT().GetImportClaim(gomock.Any(), modelUUID).Return(
		modelmigration.ImportClaim{Phase: modelmigration.ImportPhaseImporting}, nil)

	_, err := s.service().BeginImport(c.Context(), modelUUID, "source-uuid")
	c.Assert(err, tc.ErrorIs, coreerrors.AlreadyExists)
	c.Check(err, tc.ErrorMatches, "model import for .*")
}

// TestBeginImportDuplicateActivating asserts that a duplicate claim that has
// crossed into activating reports activation-in-progress wording.
func (s *serviceSuite) TestBeginImportDuplicateActivating(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID).String()
	s.controllerState.EXPECT().BeginImport(gomock.Any(), modelUUID, "source-uuid").Return(
		"", modelmigrationerrors.ErrImportClaimExists)
	s.controllerState.EXPECT().GetImportClaim(gomock.Any(), modelUUID).Return(
		modelmigration.ImportClaim{Phase: modelmigration.ImportPhaseActivating}, nil)

	_, err := s.service().BeginImport(c.Context(), modelUUID, "source-uuid")
	c.Assert(err, tc.ErrorIs, coreerrors.AlreadyExists)
	c.Check(err, tc.ErrorMatches, ".*activation in progress.*")
}

// TestBeginImportDuplicateAborting asserts that a duplicate claim mid-cleanup
// reports cleanup-in-progress wording.
func (s *serviceSuite) TestBeginImportDuplicateAborting(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID).String()
	s.controllerState.EXPECT().BeginImport(gomock.Any(), modelUUID, "source-uuid").Return(
		"", modelmigrationerrors.ErrImportClaimExists)
	s.controllerState.EXPECT().GetImportClaim(gomock.Any(), modelUUID).Return(
		modelmigration.ImportClaim{Phase: modelmigration.ImportPhaseAborting}, nil)

	_, err := s.service().BeginImport(c.Context(), modelUUID, "source-uuid")
	c.Assert(err, tc.ErrorIs, coreerrors.AlreadyExists)
	c.Check(err, tc.ErrorMatches, ".*cleanup in progress.*")
}

// TestAssertImportingPassesThrough asserts the service delegates directly to
// the controller state.
func (s *serviceSuite) TestAssertImportingPassesThrough(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID).String()
	s.controllerState.EXPECT().AssertImporting(gomock.Any(), modelUUID).Return(nil)

	err := s.service().AssertImporting(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportOfferPermissionsPassesThrough asserts the service delegates
// directly to the controller state.
func (s *serviceSuite) TestImportOfferPermissionsPassesThrough(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID).String()
	offerUUIDs := []string{"offer-1", "offer-2"}
	s.controllerState.EXPECT().ImportOfferPermissions(gomock.Any(), modelUUID, "claim-uuid", offerUUIDs).Return(nil)

	err := s.service().ImportOfferPermissions(c.Context(), modelUUID, "claim-uuid", offerUUIDs)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportExternalControllersPassesThrough asserts the service delegates
// directly to the controller state.
func (s *serviceSuite) TestImportExternalControllersPassesThrough(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID).String()
	refs := []coremodelmigration.ExternalController{{UUID: "ext-uuid"}}
	s.controllerState.EXPECT().ImportExternalControllers(gomock.Any(), modelUUID, "claim-uuid", refs).Return(nil)

	err := s.service().ImportExternalControllers(c.Context(), modelUUID, "claim-uuid", refs)
	c.Assert(err, tc.ErrorIsNil)
}
