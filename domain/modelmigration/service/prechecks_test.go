// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
)

// fullPrecheckArgs returns precheck arguments that exercise every check.
func fullPrecheckArgs(modelUUID string) modelmigration.ImportPrecheckArgs {
	return modelmigration.ImportPrecheckArgs{
		ModelUUID:      modelUUID,
		ModelName:      "prod-model",
		ModelQualifier: "prod",
		Cloud:          "my-cloud",
		CloudRegion:    "my-region",
		Users:          []string{"alice", "bob"},
		Credential: &modelmigration.ImportPrecheckCredential{
			Cloud: "my-cloud",
			Owner: "alice",
			Name:  "cred",
		},
		SecretBackend: "my-backend",
	}
}

// expectPrecheckPasses sets up the controller state mock so that every import
// precheck succeeds.
func (s *serviceSuite) expectPrecheckPasses(args modelmigration.ImportPrecheckArgs) {
	s.controllerState.EXPECT().CloudExists(gomock.Any(), args.Cloud).Return(true, nil)
	s.controllerState.EXPECT().CloudRegionExists(gomock.Any(), args.Cloud, args.CloudRegion).Return(true, nil)
	for _, u := range args.Users {
		s.controllerState.EXPECT().IsUserDisabled(gomock.Any(), u).Return(false, true, nil)
	}
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(false, true, nil)
	s.controllerState.EXPECT().SecretBackendExists(gomock.Any(), args.SecretBackend).Return(true, nil)
	s.controllerState.EXPECT().GetImportClaim(gomock.Any(), args.ModelUUID).Return(
		modelmigration.ImportClaim{}, modelmigrationerrors.ErrImportNotFound)
	s.controllerState.EXPECT().ModelExists(gomock.Any(), args.ModelUUID).Return(false, nil)
	s.controllerState.EXPECT().ModelNamespaceExists(gomock.Any(), args.ModelUUID).Return(false, nil)
	s.controllerState.EXPECT().ModelNameInUse(gomock.Any(), args.ModelName, args.ModelQualifier).Return(false, nil)
}

// TestPrecheckImportSuccess asserts that the import prechecks pass when the
// target controller can host the model.
func (s *serviceSuite) TestPrecheckImportSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.expectPrecheckPasses(args)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
}

// TestPrecheckImportCloudNotFound asserts the prechecks reject a model whose
// cloud does not exist on the target.
func (s *serviceSuite) TestPrecheckImportCloudNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CloudExists(gomock.Any(), args.Cloud).Return(false, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `.*cloud "my-cloud" not found on target controller.*`)
}

// TestPrecheckImportRegionNotFound asserts the prechecks reject a model whose
// cloud region is unknown to the target cloud.
func (s *serviceSuite) TestPrecheckImportRegionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CloudExists(gomock.Any(), args.Cloud).Return(true, nil)
	s.controllerState.EXPECT().CloudRegionExists(gomock.Any(), args.Cloud, args.CloudRegion).Return(false, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `.*cloud region "my-region" not valid for cloud "my-cloud".*`)
}

// TestPrecheckImportUserDisabled asserts the prechecks reject a model whose
// referenced user is disabled on the target.
func (s *serviceSuite) TestPrecheckImportUserDisabled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CloudExists(gomock.Any(), args.Cloud).Return(true, nil)
	s.controllerState.EXPECT().CloudRegionExists(gomock.Any(), args.Cloud, args.CloudRegion).Return(true, nil)
	s.controllerState.EXPECT().IsUserDisabled(gomock.Any(), "alice").Return(true, true, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `.*user "alice" is disabled on the target controller.*`)
}

// TestPrecheckImportMissingUserOK asserts that a user absent from the target is
// allowed (it is recreated on import).
func (s *serviceSuite) TestPrecheckImportMissingUserOK(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CloudExists(gomock.Any(), args.Cloud).Return(true, nil)
	s.controllerState.EXPECT().CloudRegionExists(gomock.Any(), args.Cloud, args.CloudRegion).Return(true, nil)
	// alice is missing (recreated on import), bob exists and is enabled.
	s.controllerState.EXPECT().IsUserDisabled(gomock.Any(), "alice").Return(false, false, nil)
	s.controllerState.EXPECT().IsUserDisabled(gomock.Any(), "bob").Return(false, true, nil)
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(false, true, nil)
	s.controllerState.EXPECT().SecretBackendExists(gomock.Any(), args.SecretBackend).Return(true, nil)
	s.controllerState.EXPECT().GetImportClaim(gomock.Any(), args.ModelUUID).Return(
		modelmigration.ImportClaim{}, modelmigrationerrors.ErrImportNotFound)
	s.controllerState.EXPECT().ModelExists(gomock.Any(), args.ModelUUID).Return(false, nil)
	s.controllerState.EXPECT().ModelNamespaceExists(gomock.Any(), args.ModelUUID).Return(false, nil)
	s.controllerState.EXPECT().ModelNameInUse(gomock.Any(), args.ModelName, args.ModelQualifier).Return(false, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
}

// TestPrecheckImportCredentialRevoked asserts the prechecks reject a model
// whose credential is revoked on the target and not revoked on the source.
func (s *serviceSuite) TestPrecheckImportCredentialRevoked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CloudExists(gomock.Any(), args.Cloud).Return(true, nil)
	s.controllerState.EXPECT().CloudRegionExists(gomock.Any(), args.Cloud, args.CloudRegion).Return(true, nil)
	for _, u := range args.Users {
		s.controllerState.EXPECT().IsUserDisabled(gomock.Any(), u).Return(false, true, nil)
	}
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(true, true, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `.*credential .* is revoked on the target controller.*`)
}

// TestPrecheckImportSecretBackendNotFound asserts the prechecks reject a model
// whose secret backend does not exist on the target.
func (s *serviceSuite) TestPrecheckImportSecretBackendNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CloudExists(gomock.Any(), args.Cloud).Return(true, nil)
	s.controllerState.EXPECT().CloudRegionExists(gomock.Any(), args.Cloud, args.CloudRegion).Return(true, nil)
	for _, u := range args.Users {
		s.controllerState.EXPECT().IsUserDisabled(gomock.Any(), u).Return(false, true, nil)
	}
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(false, true, nil)
	s.controllerState.EXPECT().SecretBackendExists(gomock.Any(), args.SecretBackend).Return(false, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `.*secret backend "my-backend" not found on target controller.*`)
}

// TestPrecheckImportClaimExists asserts the prechecks report an existing import
// claim for the model UUID.
func (s *serviceSuite) TestPrecheckImportClaimExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CloudExists(gomock.Any(), args.Cloud).Return(true, nil)
	s.controllerState.EXPECT().CloudRegionExists(gomock.Any(), args.Cloud, args.CloudRegion).Return(true, nil)
	for _, u := range args.Users {
		s.controllerState.EXPECT().IsUserDisabled(gomock.Any(), u).Return(false, true, nil)
	}
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(false, true, nil)
	s.controllerState.EXPECT().SecretBackendExists(gomock.Any(), args.SecretBackend).Return(true, nil)
	s.controllerState.EXPECT().GetImportClaim(gomock.Any(), args.ModelUUID).Return(
		modelmigration.ImportClaim{
			SourceMigrationUUID: "src",
			Phase:               modelmigration.ImportPhaseImporting,
		}, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `.*already has an import claim on this controller.*`)
}

// TestPrecheckImportModelUUIDCollision asserts the prechecks reject a model
// whose UUID already exists on the target without an import claim.
func (s *serviceSuite) TestPrecheckImportModelUUIDCollision(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CloudExists(gomock.Any(), args.Cloud).Return(true, nil)
	s.controllerState.EXPECT().CloudRegionExists(gomock.Any(), args.Cloud, args.CloudRegion).Return(true, nil)
	for _, u := range args.Users {
		s.controllerState.EXPECT().IsUserDisabled(gomock.Any(), u).Return(false, true, nil)
	}
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(false, true, nil)
	s.controllerState.EXPECT().SecretBackendExists(gomock.Any(), args.SecretBackend).Return(true, nil)
	s.controllerState.EXPECT().GetImportClaim(gomock.Any(), args.ModelUUID).Return(
		modelmigration.ImportClaim{}, modelmigrationerrors.ErrImportNotFound)
	s.controllerState.EXPECT().ModelExists(gomock.Any(), args.ModelUUID).Return(true, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `.*model with same UUID already exists.*`)
}

// TestPrecheckImportNameInUse asserts the prechecks reject a model whose
// name/qualifier already exists on the target.
func (s *serviceSuite) TestPrecheckImportNameInUse(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.expectPrecheckPassesUntilName(args)
	s.controllerState.EXPECT().ModelNameInUse(gomock.Any(), args.ModelName, args.ModelQualifier).Return(true, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `.*model named "prod-model" already exists.*`)
}

// expectPrecheckPassesUntilName sets up every precheck up to (but not
// including) the model name/qualifier collision check to succeed.
func (s *serviceSuite) expectPrecheckPassesUntilName(args modelmigration.ImportPrecheckArgs) {
	s.controllerState.EXPECT().CloudExists(gomock.Any(), args.Cloud).Return(true, nil)
	s.controllerState.EXPECT().CloudRegionExists(gomock.Any(), args.Cloud, args.CloudRegion).Return(true, nil)
	for _, u := range args.Users {
		s.controllerState.EXPECT().IsUserDisabled(gomock.Any(), u).Return(false, true, nil)
	}
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(false, true, nil)
	s.controllerState.EXPECT().SecretBackendExists(gomock.Any(), args.SecretBackend).Return(true, nil)
	s.controllerState.EXPECT().GetImportClaim(gomock.Any(), args.ModelUUID).Return(
		modelmigration.ImportClaim{}, modelmigrationerrors.ErrImportNotFound)
	s.controllerState.EXPECT().ModelExists(gomock.Any(), args.ModelUUID).Return(false, nil)
	s.controllerState.EXPECT().ModelNamespaceExists(gomock.Any(), args.ModelUUID).Return(false, nil)
}
