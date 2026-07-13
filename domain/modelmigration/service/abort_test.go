// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modelmigration"
)

// TestSetImportPhaseAborting asserts the transition is delegated to state.
func (s *serviceSuite) TestSetImportPhaseAborting(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.controllerState.EXPECT().SetImportPhaseAborting(gomock.Any(), modelUUID.String()).Return(nil)

	err := s.service(c).SetImportPhaseAborting(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetImportPhaseAbortingInvalidModelUUID asserts an invalid model UUID is
// rejected before any state call.
func (s *serviceSuite) TestSetImportPhaseAbortingInvalidModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).SetImportPhaseAborting(c.Context(), coremodel.UUID("not-a-uuid"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestFinalizeAbortedImport asserts the finalize is delegated to state.
func (s *serviceSuite) TestFinalizeAbortedImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.controllerState.EXPECT().FinalizeAbortedImport(gomock.Any(), modelUUID.String()).Return(nil)

	err := s.service(c).FinalizeAbortedImport(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// TestFinalizeAbortedImportInvalidModelUUID asserts an invalid model UUID is
// rejected before any state call.
func (s *serviceSuite) TestFinalizeAbortedImportInvalidModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).FinalizeAbortedImport(c.Context(), coremodel.UUID("not-a-uuid"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestStageAbortedModelDatabaseDeletion asserts the staging is delegated to
// state.
func (s *serviceSuite) TestStageAbortedModelDatabaseDeletion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.controllerState.EXPECT().StageAbortedModelDatabaseDeletion(gomock.Any(), modelUUID.String()).Return(nil)

	err := s.service(c).StageAbortedModelDatabaseDeletion(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// TestStageAbortedModelDatabaseDeletionInvalidModelUUID asserts an invalid model
// UUID is rejected before any state call.
func (s *serviceSuite) TestStageAbortedModelDatabaseDeletionInvalidModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service(c).StageAbortedModelDatabaseDeletion(c.Context(), coremodel.UUID("not-a-uuid"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetAllImportClaims asserts the scan is delegated to state and returned
// unchanged.
func (s *serviceSuite) TestGetAllImportClaims(c *tc.C) {
	defer s.setupMocks(c).Finish()

	claims := []modelmigration.ImportClaimStatus{{
		ModelUUID: tc.Must(c, coremodel.NewUUID).String(),
		Phase:     modelmigration.ImportPhaseAborting,
	}}
	s.controllerState.EXPECT().GetAllImportClaims(gomock.Any()).Return(claims, nil)

	got, err := s.service(c).GetAllImportClaims(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, claims)
}

// TestIsImportNamespaceRegistered asserts the predicate is delegated to state.
func (s *serviceSuite) TestIsImportNamespaceRegistered(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := tc.Must(c, coremodel.NewUUID)
	s.controllerState.EXPECT().IsImportNamespaceRegistered(gomock.Any(), modelUUID.String()).Return(true, nil)

	got, err := s.service(c).IsImportNamespaceRegistered(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.IsTrue)
}

// TestIsImportNamespaceRegisteredInvalidModelUUID asserts an invalid model UUID
// is rejected before any state call.
func (s *serviceSuite) TestIsImportNamespaceRegisteredInvalidModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service(c).IsImportNamespaceRegistered(c.Context(), coremodel.UUID("not-a-uuid"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}
