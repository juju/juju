// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
)

// TestBeginImportSuccess asserts that the service generates a claim UUID and
// returns it unchanged.
func (s *serviceSuite) TestBeginImportSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.controllerState.EXPECT().BeginImport(gomock.Any(), modelUUID.String(), gomock.Any(), "source-uuid").Return(
		modelmigration.ImportClaim{Phase: modelmigration.ImportPhaseImporting}, nil)

	claimUUID, err := s.service().BeginImport(c.Context(), modelUUID, "source-uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claimUUID, tc.Not(tc.Equals), "")
}

// TestBeginImportInvalidModelUUID asserts that an invalid model UUID is
// rejected before any state call.
func (s *serviceSuite) TestBeginImportInvalidModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service().BeginImport(c.Context(), coremodel.UUID("not-a-uuid"), "source-uuid")
	c.Assert(err, tc.ErrorMatches, "validating model uuid.*")
}

// TestBeginImportDuplicateImporting asserts that a duplicate claim still in
// the importing phase reports a plain AlreadyExists error, using the claim
// returned by state without a second read.
func (s *serviceSuite) TestBeginImportDuplicateImporting(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.controllerState.EXPECT().BeginImport(gomock.Any(), modelUUID.String(), gomock.Any(), "source-uuid").Return(
		modelmigration.ImportClaim{Phase: modelmigration.ImportPhaseImporting},
		modelmigrationerrors.ErrImportClaimExists)

	_, err := s.service().BeginImport(c.Context(), modelUUID, "source-uuid")
	c.Assert(err, tc.ErrorIs, coreerrors.AlreadyExists)
	c.Check(err, tc.ErrorMatches, "model import for .*")
}

// TestBeginImportDuplicateActivating asserts that a duplicate claim that has
// crossed into activating reports activation-in-progress wording.
func (s *serviceSuite) TestBeginImportDuplicateActivating(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.controllerState.EXPECT().BeginImport(gomock.Any(), modelUUID.String(), gomock.Any(), "source-uuid").Return(
		modelmigration.ImportClaim{Phase: modelmigration.ImportPhaseActivating},
		modelmigrationerrors.ErrImportClaimExists)

	_, err := s.service().BeginImport(c.Context(), modelUUID, "source-uuid")
	c.Assert(err, tc.ErrorIs, coreerrors.AlreadyExists)
	c.Check(err, tc.ErrorMatches, ".*activation in progress.*")
}

// TestBeginImportDuplicateAborting asserts that a duplicate claim mid-cleanup
// reports cleanup-in-progress wording.
func (s *serviceSuite) TestBeginImportDuplicateAborting(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.controllerState.EXPECT().BeginImport(gomock.Any(), modelUUID.String(), gomock.Any(), "source-uuid").Return(
		modelmigration.ImportClaim{Phase: modelmigration.ImportPhaseAborting},
		modelmigrationerrors.ErrImportClaimExists)

	_, err := s.service().BeginImport(c.Context(), modelUUID, "source-uuid")
	c.Assert(err, tc.ErrorIs, coreerrors.AlreadyExists)
	c.Check(err, tc.ErrorMatches, ".*cleanup in progress.*")
}

// TestAssertImportingPassesThrough asserts the service delegates directly to
// the controller state.
func (s *serviceSuite) TestAssertImportingPassesThrough(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.controllerState.EXPECT().AssertImporting(gomock.Any(), modelUUID.String()).Return(nil)

	err := s.service().AssertImporting(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportOfferPermissionsPassesThrough asserts the service delegates
// directly to the controller state.
func (s *serviceSuite) TestImportOfferPermissionsPassesThrough(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	offerUUIDs := []string{"offer-1", "offer-2"}
	s.controllerState.EXPECT().ImportOfferPermissions(gomock.Any(), modelUUID.String(), "claim-uuid", offerUUIDs).Return(nil)

	err := s.service().ImportOfferPermissions(c.Context(), modelUUID, "claim-uuid", offerUUIDs)
	c.Assert(err, tc.ErrorIsNil)
}

// TestEnsureExternalControllerExistsGeneratesAddressUUIDs asserts the service
// generates address UUIDs before handing the reference to the state layer.
func (s *serviceSuite) TestEnsureExternalControllerExistsGeneratesAddressUUIDs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ref := coremodelmigration.ExternalController{
		UUID:      "ext-uuid",
		Addresses: []string{"10.0.0.1:17070"},
	}
	s.controllerState.EXPECT().EnsureExternalControllerExists(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, got modelmigrationinternal.ExternalController) error {
			c.Check(got.UUID, tc.Equals, "ext-uuid")
			c.Assert(got.Addresses, tc.HasLen, 1)
			c.Check(got.Addresses[0].Address, tc.Equals, "10.0.0.1:17070")
			c.Check(got.Addresses[0].UUID, tc.Not(tc.Equals), "")
			return nil
		})

	err := s.service().EnsureExternalControllerExists(c.Context(), ref)
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportExternalControllersPassesThrough asserts the service translates
// the references and delegates to the controller state.
func (s *serviceSuite) TestImportExternalControllersPassesThrough(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	refs := []coremodelmigration.ExternalController{{UUID: "ext-uuid"}}
	stateRefs := []modelmigrationinternal.ExternalController{{
		UUID:      "ext-uuid",
		Addresses: []modelmigrationinternal.ExternalControllerAddress{},
	}}
	s.controllerState.EXPECT().ImportExternalControllers(
		gomock.Any(), modelUUID.String(), "claim-uuid", stateRefs).Return(nil)

	err := s.service().ImportExternalControllers(c.Context(), modelUUID, "claim-uuid", refs)
	c.Assert(err, tc.ErrorIsNil)
}
