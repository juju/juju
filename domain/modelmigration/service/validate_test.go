// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// TestMissingAgentBinaryArchitecturesCAAS checks the CAAS short-circuit: CAAS
// agents run from OCI images, not the agent binary store, so no architecture is
// ever reported missing and neither object store is consulted.
func (s *serviceSuite) TestMissingAgentBinaryArchitecturesCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetModelType(gomock.Any()).Return("caas", nil)

	missing, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).MissingAgentBinaryArchitectures(c.Context(), "4.0.1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(missing, tc.HasLen, 0)
}

// TestActivateImportRejectsUnitNotInRelation checks that activation refuses
// an imported model whose relation endpoint application has a unit without
// a relation_unit row.
func (s *serviceSuite) TestActivateImportRejectsUnitNotInRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	mExp.GetSecretBackendUUIDsInUse(gomock.Any()).Return(nil, nil)
	mExp.GetExternalSecretRevisionBackends(gomock.Any()).Return(nil, nil)
	mExp.GetRelationValidationData(gomock.Any()).Return([]modelmigrationinternal.RelationValidationData{{
		UUID: "relation-uuid",
		ID:   7,
		Key:  "wordpress:db mysql:db",
		Endpoints: []modelmigrationinternal.RelationValidationEndpoint{
			{ApplicationName: "mysql"},
			{ApplicationName: "wordpress"},
		},
	}}, nil)
	mExp.GetApplicationUnitNames(gomock.Any()).Return(map[string][]string{
		"wordpress": {"wordpress/0", "wordpress/1"},
		"mysql":     {"mysql/0"},
	}, nil)
	mExp.GetRelationUnitsByApplication(gomock.Any()).Return(map[string]map[string][]string{
		"relation-uuid": {
			"wordpress": {"wordpress/0"},
			"mysql":     {"mysql/0"},
		},
	}, nil)
	mExp.GetSubordinateUnitPrincipals(gomock.Any()).Return(nil, nil)
	cExp.GetControllerTargetVersion(gomock.Any()).Return("4.0.1", nil).AnyTimes()

	err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).ActivateImport(c.Context())
	c.Assert(err, tc.ErrorMatches, `.*unit wordpress/1 hasn't joined relation "wordpress:db mysql:db" yet.*`)
}

// TestActivateImportRelationValidationPasses checks activation proceeds when
// all units in relation endpoint applications have relation_unit rows.
func (s *serviceSuite) TestActivateImportRelationValidationPasses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentVersion := semversion.MustParse("4.0.0").String()
	desiredVersion := semversion.MustParse("4.0.1").String()

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	mExp.GetSecretBackendUUIDsInUse(gomock.Any()).Return(nil, nil)
	mExp.GetExternalSecretRevisionBackends(gomock.Any()).Return(nil, nil)
	mExp.GetRelationValidationData(gomock.Any()).Return([]modelmigrationinternal.RelationValidationData{{
		UUID: "relation-uuid",
		ID:   7,
		Key:  "wordpress:db mysql:db",
		Endpoints: []modelmigrationinternal.RelationValidationEndpoint{
			{ApplicationName: "mysql"},
			{ApplicationName: "wordpress"},
		},
	}}, nil)
	mExp.GetApplicationUnitNames(gomock.Any()).Return(map[string][]string{
		"wordpress": {"wordpress/0"},
		"mysql":     {"mysql/0"},
	}, nil)
	mExp.GetRelationUnitsByApplication(gomock.Any()).Return(map[string]map[string][]string{
		"relation-uuid": {
			"wordpress": {"wordpress/0"},
			"mysql":     {"mysql/0"},
		},
	}, nil)
	mExp.GetSubordinateUnitPrincipals(gomock.Any()).Return(nil, nil)

	mExp.GetModelType(gomock.Any()).Return("iaas", nil)
	mExp.GetRunningAgentArchitectures(gomock.Any()).Return(nil, nil)

	gomock.InOrder(
		cExp.GetControllerTargetVersion(gomock.Any()).Return(desiredVersion, nil),
		mExp.GetModelTargetAgentVersion(gomock.Any()).Return(currentVersion, nil),
		mExp.SetModelTargetAgentVersion(gomock.Any(), currentVersion, desiredVersion).Return(nil),
		mExp.DeleteModelImportingStatus(gomock.Any()).Return(nil),
		cExp.DeleteModelImportingStatus(gomock.Any(), s.modelUUID).Return(nil),
	)

	err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

// TestActivateImportSkipsBumpWhenBinariesMissing checks 3.6 parity: when the
// target lacks agent binaries for a running architecture at the desired
// version, activation does not bump the model agent version and does not fail.
func (s *serviceSuite) TestActivateImportSkipsBumpWhenBinariesMissing(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentVersion := semversion.MustParse("4.0.0").String()
	desiredVersion := semversion.MustParse("4.0.1").String()

	s.expectImportValidationPasses()

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	// A running arm64 agent exists, but the target has no arm64 binary for the
	// desired version in either store: the bump is skipped, activation proceeds,
	// and SetModelTargetAgentVersion is never called.
	mExp.GetModelType(gomock.Any()).Return("iaas", nil)
	mExp.GetRunningAgentArchitectures(gomock.Any()).Return([]string{"arm64"}, nil)
	cExp.GetAgentBinaryArchitecturesForVersion(gomock.Any(), desiredVersion).Return([]string{"amd64"}, nil)
	mExp.GetAgentBinaryArchitecturesForVersion(gomock.Any(), desiredVersion).Return(nil, nil)

	gomock.InOrder(
		cExp.GetControllerTargetVersion(gomock.Any()).Return(desiredVersion, nil),
		mExp.GetModelTargetAgentVersion(gomock.Any()).Return(currentVersion, nil),
		mExp.DeleteModelImportingStatus(gomock.Any()).Return(nil),
		cExp.DeleteModelImportingStatus(gomock.Any(), s.modelUUID).Return(nil),
	)

	err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

// TestActivateImportRejectsUnknownSecretBackend checks that activation refuses
// an imported model whose external secrets reference a backend that does not
// exist on this controller (an un-rewritten source backend UUID), before any
// activation write.
func (s *serviceSuite) TestActivateImportRejectsUnknownSecretBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	mExp.GetSecretBackendUUIDsInUse(gomock.Any()).Return([]string{"source-backend-uuid"}, nil)
	cExp.GetKnownSecretBackends(gomock.Any(), []string{"source-backend-uuid"}).Return(nil, nil)

	err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorMatches, ".*secret backend.*source-backend-uuid.*do not exist.*")
}

// TestActivateImportRejectsMissingBackendReference checks that activation
// refuses an imported model whose external secret revision has no matching
// controller secret_backend_reference row (re-attach did not happen).
func (s *serviceSuite) TestActivateImportRejectsMissingBackendReference(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	mExp.GetSecretBackendUUIDsInUse(gomock.Any()).Return([]string{"backend-1"}, nil)
	cExp.GetKnownSecretBackends(gomock.Any(), []string{"backend-1"}).Return([]string{"backend-1"}, nil)
	mExp.GetExternalSecretRevisionBackends(gomock.Any()).Return(map[string]string{"rev-1": "backend-1"}, nil)
	cExp.GetSecretBackendReferencesForModel(gomock.Any(), s.modelUUID).Return(map[string]string{}, nil)

	err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorMatches, ".*missing secret backend references.*rev-1.*")
}

// TestActivateImportRejectsMismatchedBackendReference checks that activation
// refuses an imported model whose controller secret_backend_reference row
// points at a different backend than the model's own secret value ref.
func (s *serviceSuite) TestActivateImportRejectsMismatchedBackendReference(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	mExp.GetSecretBackendUUIDsInUse(gomock.Any()).Return([]string{"backend-1"}, nil)
	cExp.GetKnownSecretBackends(gomock.Any(), []string{"backend-1"}).Return([]string{"backend-1"}, nil)
	mExp.GetExternalSecretRevisionBackends(gomock.Any()).Return(map[string]string{"rev-1": "backend-1"}, nil)
	cExp.GetSecretBackendReferencesForModel(gomock.Any(), s.modelUUID).
		Return(map[string]string{"rev-1": "backend-2"}, nil)

	err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorMatches, ".*do not match the secret value refs.*rev-1.*")
}

// TestActivateImportSubordinateOnOtherPrincipalPasses checks 3.6 parity for
// container-scoped relations (state.RelationUnit.Valid): a subordinate deployed
// against two principals only enters the scope of the relation belonging to its
// own principal, so its other units must not be reported as missing.
func (s *serviceSuite) TestActivateImportSubordinateOnOtherPrincipalPasses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentVersion := semversion.MustParse("4.0.0").String()

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	mExp.GetSecretBackendUUIDsInUse(gomock.Any()).Return(nil, nil)
	mExp.GetExternalSecretRevisionBackends(gomock.Any()).Return(nil, nil)
	// nrpe is subordinate to both ubuntu and mysql, via one container-scoped
	// relation each.
	mExp.GetRelationValidationData(gomock.Any()).Return([]modelmigrationinternal.RelationValidationData{{
		UUID: "rel-ubuntu",
		ID:   1,
		Key:  "nrpe:general-info ubuntu:juju-info",
		Endpoints: []modelmigrationinternal.RelationValidationEndpoint{
			{ApplicationName: "nrpe", ContainerScoped: true, Subordinate: true},
			{ApplicationName: "ubuntu", ContainerScoped: true},
		},
	}, {
		UUID: "rel-mysql",
		ID:   2,
		Key:  "nrpe:general-info mysql:juju-info",
		Endpoints: []modelmigrationinternal.RelationValidationEndpoint{
			{ApplicationName: "mysql", ContainerScoped: true},
			{ApplicationName: "nrpe", ContainerScoped: true, Subordinate: true},
		},
	}}, nil)
	mExp.GetApplicationUnitNames(gomock.Any()).Return(map[string][]string{
		"ubuntu": {"ubuntu/0"},
		"mysql":  {"mysql/0"},
		"nrpe":   {"nrpe/0", "nrpe/1"},
	}, nil)
	mExp.GetRelationUnitsByApplication(gomock.Any()).Return(map[string]map[string][]string{
		"rel-ubuntu": {
			"ubuntu": {"ubuntu/0"},
			"nrpe":   {"nrpe/0"},
		},
		"rel-mysql": {
			"mysql": {"mysql/0"},
			"nrpe":  {"nrpe/1"},
		},
	}, nil)
	mExp.GetSubordinateUnitPrincipals(gomock.Any()).Return(map[string]string{
		"nrpe/0": "ubuntu",
		"nrpe/1": "mysql",
	}, nil)

	gomock.InOrder(
		cExp.GetControllerTargetVersion(gomock.Any()).Return(currentVersion, nil),
		mExp.GetModelTargetAgentVersion(gomock.Any()).Return(currentVersion, nil),
		mExp.DeleteModelImportingStatus(gomock.Any()).Return(nil),
		cExp.DeleteModelImportingStatus(gomock.Any(), s.modelUUID).Return(nil),
	)

	err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

// TestActivateImportRejectsSubordinateNotInOwnRelation checks that the
// container-scope skip is narrow: a subordinate unit missing from the relation
// of its own principal is still a structural inconsistency.
func (s *serviceSuite) TestActivateImportRejectsSubordinateNotInOwnRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	mExp.GetSecretBackendUUIDsInUse(gomock.Any()).Return(nil, nil)
	mExp.GetExternalSecretRevisionBackends(gomock.Any()).Return(nil, nil)
	mExp.GetRelationValidationData(gomock.Any()).Return([]modelmigrationinternal.RelationValidationData{{
		UUID: "rel-ubuntu",
		ID:   1,
		Key:  "nrpe:general-info ubuntu:juju-info",
		Endpoints: []modelmigrationinternal.RelationValidationEndpoint{
			{ApplicationName: "nrpe", ContainerScoped: true, Subordinate: true},
			{ApplicationName: "ubuntu", ContainerScoped: true},
		},
	}}, nil)
	mExp.GetApplicationUnitNames(gomock.Any()).Return(map[string][]string{
		"ubuntu": {"ubuntu/0"},
		"nrpe":   {"nrpe/0"},
	}, nil)
	mExp.GetRelationUnitsByApplication(gomock.Any()).Return(map[string]map[string][]string{
		"rel-ubuntu": {"ubuntu": {"ubuntu/0"}},
	}, nil)
	mExp.GetSubordinateUnitPrincipals(gomock.Any()).Return(map[string]string{
		"nrpe/0": "ubuntu",
	}, nil)
	cExp.GetControllerTargetVersion(gomock.Any()).Return("4.0.1", nil).AnyTimes()

	err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
		loggertesting.WrapCheckLog(c),
	).ActivateImport(c.Context())
	c.Assert(err, tc.ErrorMatches, `.*unit nrpe/0 hasn't joined relation "nrpe:general-info ubuntu:juju-info" yet.*`)
}
