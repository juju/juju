// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modelmigration"
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
	s.controllerState.EXPECT().CheckCloudRegion(
		gomock.Any(), args.Cloud, args.CloudRegion,
	).Return(true, true, nil)
	s.controllerState.EXPECT().GetDisabledUsers(gomock.Any(), args.Users).Return(nil, nil)
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(false, true, nil)
	s.controllerState.EXPECT().SecretBackendExists(gomock.Any(), args.SecretBackend).Return(true, nil)
	s.controllerState.EXPECT().CheckImportModelCollision(
		gomock.Any(), args.ModelUUID, args.ModelName, args.ModelQualifier,
	).Return(modelmigration.ImportModelCollision{}, nil)
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
	s.controllerState.EXPECT().CheckCloudRegion(
		gomock.Any(), args.Cloud, args.CloudRegion,
	).Return(false, false, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Check(err, tc.ErrorMatches, `.*cloud "my-cloud" not found on target controller.*`)
}

// TestPrecheckImportRegionNotFound asserts the prechecks reject a model whose
// cloud region is unknown to the target cloud.
func (s *serviceSuite) TestPrecheckImportRegionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CheckCloudRegion(
		gomock.Any(), args.Cloud, args.CloudRegion,
	).Return(true, false, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Check(err, tc.ErrorMatches, `.*cloud region "my-region" not valid for cloud "my-cloud".*`)
}

// TestPrecheckImportUserDisabled asserts the prechecks reject a model whose
// referenced user is disabled on the target.
func (s *serviceSuite) TestPrecheckImportUserDisabled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CheckCloudRegion(
		gomock.Any(), args.Cloud, args.CloudRegion,
	).Return(true, true, nil)
	s.controllerState.EXPECT().GetDisabledUsers(gomock.Any(), args.Users).Return([]string{"alice"}, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Check(err, tc.ErrorMatches, `.*users "alice" are disabled on the target controller.*`)
}

// TestPrecheckImportMissingUserOK asserts that a user absent from the target is
// allowed (it is recreated on import).
func (s *serviceSuite) TestPrecheckImportMissingUserOK(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CheckCloudRegion(
		gomock.Any(), args.Cloud, args.CloudRegion,
	).Return(true, true, nil)
	s.controllerState.EXPECT().GetDisabledUsers(gomock.Any(), args.Users).Return(nil, nil)
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(false, true, nil)
	s.controllerState.EXPECT().SecretBackendExists(gomock.Any(), args.SecretBackend).Return(true, nil)
	s.controllerState.EXPECT().CheckImportModelCollision(
		gomock.Any(), args.ModelUUID, args.ModelName, args.ModelQualifier,
	).Return(modelmigration.ImportModelCollision{}, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
}

// TestPrecheckImportCredentialRevoked asserts the prechecks reject a model
// whose credential is revoked on the target and not revoked on the source.
func (s *serviceSuite) TestPrecheckImportCredentialRevoked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CheckCloudRegion(
		gomock.Any(), args.Cloud, args.CloudRegion,
	).Return(true, true, nil)
	s.controllerState.EXPECT().GetDisabledUsers(gomock.Any(), args.Users).Return(nil, nil)
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(true, true, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Check(err, tc.ErrorMatches, `.*credential .* is revoked on the target controller.*`)
}

// TestPrecheckImportSecretBackendNotFound asserts the prechecks reject a model
// whose secret backend does not exist on the target.
func (s *serviceSuite) TestPrecheckImportSecretBackendNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CheckCloudRegion(
		gomock.Any(), args.Cloud, args.CloudRegion,
	).Return(true, true, nil)
	s.controllerState.EXPECT().GetDisabledUsers(gomock.Any(), args.Users).Return(nil, nil)
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(false, true, nil)
	s.controllerState.EXPECT().SecretBackendExists(gomock.Any(), args.SecretBackend).Return(false, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Check(err, tc.ErrorMatches, `.*secret backend "my-backend" not found on target controller.*`)
}

// TestPrecheckImportInProgress asserts the prechecks report a model already
// being imported for the model UUID.
func (s *serviceSuite) TestPrecheckImportInProgress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CheckCloudRegion(
		gomock.Any(), args.Cloud, args.CloudRegion,
	).Return(true, true, nil)
	s.controllerState.EXPECT().GetDisabledUsers(gomock.Any(), args.Users).Return(nil, nil)
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(false, true, nil)
	s.controllerState.EXPECT().SecretBackendExists(gomock.Any(), args.SecretBackend).Return(true, nil)
	s.controllerState.EXPECT().CheckImportModelCollision(
		gomock.Any(), args.ModelUUID, args.ModelName, args.ModelQualifier,
	).Return(modelmigration.ImportModelCollision{Importing: true}, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Check(err, tc.ErrorMatches, `.*already exists on this controller \(currently importing\).*`)
}

// TestPrecheckImportModelUUIDCollision asserts the prechecks reject a model
// whose UUID already exists on the target without an import claim.
func (s *serviceSuite) TestPrecheckImportModelUUIDCollision(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.controllerState.EXPECT().CheckCloudRegion(
		gomock.Any(), args.Cloud, args.CloudRegion,
	).Return(true, true, nil)
	s.controllerState.EXPECT().GetDisabledUsers(gomock.Any(), args.Users).Return(nil, nil)
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(false, true, nil)
	s.controllerState.EXPECT().SecretBackendExists(gomock.Any(), args.SecretBackend).Return(true, nil)
	s.controllerState.EXPECT().CheckImportModelCollision(
		gomock.Any(), args.ModelUUID, args.ModelName, args.ModelQualifier,
	).Return(modelmigration.ImportModelCollision{ModelExists: true}, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Check(err, tc.ErrorMatches, `.*model ".*" already exists on this controller.*`)
}

// TestPrecheckImportNameInUse asserts the prechecks reject a model whose
// name/qualifier already exists on the target.
func (s *serviceSuite) TestPrecheckImportNameInUse(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := fullPrecheckArgs(tc.Must(c, coremodel.NewUUID).String())
	s.expectPrecheckPassesUntilCollision(args)
	s.controllerState.EXPECT().CheckImportModelCollision(
		gomock.Any(), args.ModelUUID, args.ModelName, args.ModelQualifier,
	).Return(modelmigration.ImportModelCollision{ModelNameExists: true}, nil)

	err := s.service().PrecheckImport(c.Context(), args)
	c.Check(err, tc.ErrorMatches, `.*model named "prod-model" already exists.*`)
}

// expectPrecheckPassesUntilCollision sets up every precheck up to (but not
// including) the model identity collision check to succeed.
func (s *serviceSuite) expectPrecheckPassesUntilCollision(args modelmigration.ImportPrecheckArgs) {
	s.controllerState.EXPECT().CheckCloudRegion(
		gomock.Any(), args.Cloud, args.CloudRegion,
	).Return(true, true, nil)
	s.controllerState.EXPECT().GetDisabledUsers(gomock.Any(), args.Users).Return(nil, nil)
	s.controllerState.EXPECT().GetCredentialRevoked(
		gomock.Any(), args.Credential.Cloud, args.Credential.Owner, args.Credential.Name,
	).Return(false, true, nil)
	s.controllerState.EXPECT().SecretBackendExists(gomock.Any(), args.SecretBackend).Return(true, nil)
}
