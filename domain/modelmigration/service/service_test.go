// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	controllerState     *MockControllerState
	modelState          *MockModelState
	watcherFactory      *MockWatcherFactory
	instanceProvider    *MockInstanceProvider
	resourceProvider    *MockResourceProvider
	credentialValidator *MockCredentialValidator
	modelUUID           string
	controllerUUID      string
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

// TestAdoptResources is testing the happy path of adopting a models cloud
// resources.
func (s *serviceSuite) TestAdoptResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sourceControllerVersion, err := semversion.Parse("4.1.1")
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetControllerUUID(gomock.Any()).Return(s.controllerUUID, nil)
	s.resourceProvider.EXPECT().AdoptResources(
		gomock.Any(),
		s.controllerUUID,
		sourceControllerVersion,
	).Return(nil)

	err = NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
	).AdoptResources(c.Context(), sourceControllerVersion)
	c.Check(err, tc.ErrorIsNil)
}

// TestAdoptResourcesProviderNotSupported is asserting that if the provider does
// not support the Resources interface we don't attempt to migrate any cloud
// resources and no error is produced.
func (s *serviceSuite) TestAdoptResourcesProviderNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resourceGetter := func(_ context.Context) (ResourceProvider, error) {
		return nil, coreerrors.NotSupported
	}

	sourceControllerVersion, err := semversion.Parse("4.1.1")
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetControllerUUID(gomock.Any()).Return(s.controllerUUID, nil).AnyTimes()

	err = NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		resourceGetter,
		s.credentialValidator,
	).AdoptResources(c.Context(), sourceControllerVersion)
	c.Check(err, tc.ErrorIsNil)
}

// TestAdoptResourcesProviderNotImplemented is asserting that if the resource
// provider returns a not implemented error while trying to adopt a models
// resources no error is produced from the service and no resources are adopted.
func (s *serviceSuite) TestAdoptResourcesProviderNotImplemented(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sourceControllerVersion, err := semversion.Parse("4.1.1")
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetControllerUUID(gomock.Any()).Return(s.controllerUUID, nil)
	s.resourceProvider.EXPECT().AdoptResources(
		gomock.Any(),
		s.controllerUUID,
		sourceControllerVersion,
	).Return(coreerrors.NotImplemented)

	err = NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
	).AdoptResources(c.Context(), sourceControllerVersion)
	c.Check(err, tc.ErrorIsNil)
}

// TestMachinesFromProviderNotInModel checks that [Service.CheckMachines]
// reports a discrepancy, with an empty machine name, for a provider instance
// that is not tracked by any model machine.
func (s *serviceSuite) TestMachinesFromProviderNotInModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetModelCloudCredential(gomock.Any(), s.modelUUID).Return(nil, nil)
	s.instanceProvider.EXPECT().AllInstances(gomock.Any()).
		Return([]instances.Instance{
			&instanceStub{
				id: "instance0",
			},
			&instanceStub{
				id: "instance1",
			},
		},
			nil)
	s.modelState.EXPECT().GetMachineInstanceIDs(gomock.Any()).
		Return(map[string]string{"instance0": "0"}, nil)

	discrepancies, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
	).CheckMachines(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(discrepancies, tc.DeepEquals, []modelmigration.MigrationMachineDiscrepancy{{
		MachineName:     "",
		CloudInstanceId: instance.Id("instance1"),
	}})
}

// TestMachineInstanceIDsNotInProvider checks that [Service.CheckMachines]
// reports a discrepancy, naming the offending machine, for a model machine
// whose cloud instance the provider does not report.
func (s *serviceSuite) TestMachineInstanceIDsNotInProvider(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetModelCloudCredential(gomock.Any(), s.modelUUID).Return(nil, nil)
	s.instanceProvider.EXPECT().AllInstances(gomock.Any()).
		Return([]instances.Instance{
			&instanceStub{
				id: "instance0",
			},
		},
			nil)
	s.modelState.EXPECT().GetMachineInstanceIDs(gomock.Any()).
		Return(map[string]string{"instance0": "0", "instance1": "1"}, nil)

	discrepancies, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
	).CheckMachines(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(discrepancies, tc.DeepEquals, []modelmigration.MigrationMachineDiscrepancy{{
		MachineName:     "1",
		CloudInstanceId: instance.Id("instance1"),
	}})
}

// TestCheckMachinesCredentialError checks that [Service.CheckMachines] fails
// before any provider instance reconciliation when the model credential
// cannot be read.
func (s *serviceSuite) TestCheckMachinesCredentialError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetModelCloudCredential(gomock.Any(), s.modelUUID).
		Return(nil, errors.Errorf("boom"))

	_, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
	).CheckMachines(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*validating model credential: getting model cloud credential: boom")
}

// TestCheckMachinesRevokedCredential checks that [Service.CheckMachines]
// rejects a model whose credential is revoked, without invoking the validator.
func (s *serviceSuite) TestCheckMachinesRevokedCredential(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetModelCloudCredential(gomock.Any(), s.modelUUID).
		Return(&modelmigration.ModelCloudCredential{
			Cloud:   "aws",
			Owner:   "fred",
			Name:    "default",
			Revoked: true,
		}, nil)

	_, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
	).CheckMachines(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*credential.*revoked.*")
}

// TestCheckMachinesCredentialValidationError checks that [Service.CheckMachines]
// fails when the credential validator rejects the imported credential.
func (s *serviceSuite) TestCheckMachinesCredentialValidationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	credential := modelmigration.ModelCloudCredential{
		Cloud:      "aws",
		Owner:      "fred",
		Name:       "default",
		AuthType:   "access-key",
		Attributes: map[string]string{"access-key": "foo"},
	}
	s.controllerState.EXPECT().GetModelCloudCredential(gomock.Any(), s.modelUUID).
		Return(&credential, nil)
	s.credentialValidator.EXPECT().Validate(gomock.Any(), s.modelUUID, credential).
		Return(errors.Errorf("invalid credential"))

	_, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
	).CheckMachines(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*invalid credential.*")
}

// expectImportValidationPasses sets up the read-only ValidateImportedModel
// state reads to report a model with no external secrets, so validation passes.
func (s *serviceSuite) expectImportValidationPasses() {
	mExp := s.modelState.EXPECT()
	mExp.GetSecretBackendUUIDsInUse(gomock.Any()).Return(nil, nil)
	mExp.GetExternalSecretRevisionBackends(gomock.Any()).Return(nil, nil)
	mExp.GetRelationValidationData(gomock.Any()).Return(nil, nil)
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
	cExp.GetControllerTargetVersion(gomock.Any()).Return("4.0.1", nil).AnyTimes()

	err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
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
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

// expectAgentBinaryCheckAllPresent sets up MissingAgentBinaryArchitectures to
// report a model with no running agents, so nothing is missing and the
// agent-version bump proceeds.
func (s *serviceSuite) expectAgentBinaryCheckAllPresent() {
	mExp := s.modelState.EXPECT()
	mExp.GetModelType(gomock.Any()).Return("iaas", nil)
	mExp.GetRunningAgentArchitectures(gomock.Any()).Return(nil, nil)
}

func (s *serviceSuite) TestActivateImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentVersion := semversion.MustParse("4.0.0").String()
	desiredVersion := semversion.MustParse("4.0.1").String()

	s.expectImportValidationPasses()
	s.expectAgentBinaryCheckAllPresent()

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	// These are expected to be called in order. The agent version must be
	// updated before the model importing status is deleted. And we want the
	// controller state to have the model importing status deleted last.
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
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorMatches, ".*missing secret backend references.*rev-1.*")
}

func (s *serviceSuite) TestActivateImportSameVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentVersion := semversion.MustParse("4.0.0").String()
	desiredVersion := semversion.MustParse("4.0.0").String()

	s.expectImportValidationPasses()

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	// These are expected to be called in order. The agent version must be
	// updated before the model importing status is deleted. And we want the
	// controller state to have the model importing status deleted last.
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
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestActivateImportControllerFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectImportValidationPasses()

	cExp := s.controllerState.EXPECT()

	cExp.GetControllerTargetVersion(gomock.Any()).Return("", errors.Errorf("front fell off"))

	err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorMatches, ".*front fell off")
}

func (s *serviceSuite) TestActivateImportModelFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	desiredVersion := semversion.MustParse("4.0.1").String()

	s.expectImportValidationPasses()

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	cExp.GetControllerTargetVersion(gomock.Any()).Return(desiredVersion, nil)
	mExp.GetModelTargetAgentVersion(gomock.Any()).Return("", errors.Errorf("front fell off"))

	err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorMatches, ".*front fell off")
}

// TestWatchForMigration asserts that WatchForMigration asks the watcher
// factory for a notify watcher filtering on the controller-side export
// namespace scoped to this service's model UUID.
func (s *serviceSuite) TestWatchForMigration(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var (
		namespace       string
		changeMask      changestream.ChangeType
		matchesUUID     bool
		matchesOtherID  bool
		predicateCalled bool
	)

	otherUUID := tc.Must(c, uuid.NewUUID).String()
	ch := make(chan struct{}, 1)
	s.controllerState.EXPECT().NamespaceForWatchExport().Return("model_migration_export")
	s.watcherFactory.EXPECT().NewNotifyWatcher(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fo eventsource.FilterOption, _ ...eventsource.FilterOption) (watcher.Watcher[struct{}], error) {
			namespace = fo.Namespace()
			changeMask = fo.ChangeMask()
			// The predicate captured here confirms we're scoping to the
			// service's model UUID.
			if pred := fo.ChangePredicate(); pred != nil {
				predicateCalled = true
				matchesUUID = pred(s.modelUUID)
				matchesOtherID = pred(otherUUID)
			}
			return watchertest.NewMockNotifyWatcher(ch), nil
		},
	)

	svc := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
	)
	w, err := svc.WatchForMigration(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Check(namespace, tc.Equals, "model_migration_export")
	c.Check(changeMask, tc.Equals, changestream.All)
	c.Check(predicateCalled, tc.IsTrue)
	c.Check(matchesUUID, tc.IsTrue)
	c.Check(matchesOtherID, tc.IsFalse)
}

// TestWatchForMigrationError asserts that if the watcher factory returns an
// error, WatchForMigration propagates it to the caller.
func (s *serviceSuite) TestWatchForMigrationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().NamespaceForWatchExport().Return("model_migration_export")
	s.watcherFactory.EXPECT().NewNotifyWatcher(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, errors.Errorf("boom"),
	)

	svc := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.credentialValidator,
	)
	_, err := svc.WatchForMigration(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

// TestWatchMigrationPhase asserts the phase watcher filters the controller-side
// phase namespace by this model's UUID.
func (s *serviceSuite) TestWatchMigrationPhase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var (
		namespace   string
		matchesUUID bool
	)
	ch := make(chan struct{}, 1)
	s.controllerState.EXPECT().NamespaceForWatchPhase().Return("model_migration_export_phase")
	s.watcherFactory.EXPECT().NewNotifyWatcher(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, fo eventsource.FilterOption, _ ...eventsource.FilterOption) (watcher.Watcher[struct{}], error) {
			namespace = fo.Namespace()
			if pred := fo.ChangePredicate(); pred != nil {
				matchesUUID = pred(s.modelUUID)
			}
			return watchertest.NewMockNotifyWatcher(ch), nil
		},
	)

	w, err := s.service(c).WatchMigrationPhase(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Check(namespace, tc.Equals, "model_migration_export_phase")
	c.Check(matchesUUID, tc.IsTrue)
}

// TestWatchMinionReports asserts the minion watcher resolves the active
// migration and filters the minion-sync namespace by its UUID.
func (s *serviceSuite) TestWatchMinionReports(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var (
		namespace      string
		matchesMigID   bool
		matchesModelID bool
	)
	migUUID := tc.Must(c, uuid.NewUUID).String()
	ch := make(chan struct{}, 1)
	gomock.InOrder(
		s.controllerState.EXPECT().GetActiveExportUUID(gomock.Any(), s.modelUUID).Return(migUUID, nil),
		s.controllerState.EXPECT().NamespaceForWatchMinionSync().Return("model_migration_export_minion_sync"),
		s.watcherFactory.EXPECT().NewNotifyWatcher(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, _ string, fo eventsource.FilterOption, _ ...eventsource.FilterOption) (watcher.Watcher[struct{}], error) {
				namespace = fo.Namespace()
				if pred := fo.ChangePredicate(); pred != nil {
					matchesMigID = pred(migUUID)
					matchesModelID = pred(s.modelUUID)
				}
				return watchertest.NewMockNotifyWatcher(ch), nil
			},
		),
	)

	w, err := s.service(c).WatchMinionReports(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Check(namespace, tc.Equals, "model_migration_export_minion_sync")
	c.Check(matchesMigID, tc.IsTrue)
	c.Check(matchesModelID, tc.IsFalse)
}

// service constructs a Service backed by the suite mocks.
func (s *serviceSuite) service(c *tc.C) *Service {
	return NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		func(context.Context) (InstanceProvider, error) { return s.instanceProvider, nil },
		func(context.Context) (ResourceProvider, error) { return s.resourceProvider, nil },
		s.credentialValidator,
	)
}

// validTargetInfo returns a TargetInfo that passes validation.
func (s *serviceSuite) validTargetInfo() migration.TargetInfo {
	return migration.TargetInfo{
		ControllerUUID: s.controllerUUID,
		Addrs:          []string{"10.0.0.1:17070"},
		CACert:         "ca-cert",
		User:           "admin",
		Password:       "secret",
	}
}

// TestInitiateMigration asserts a new migration is recorded and its generated
// UUID returned.
func (s *serviceSuite) TestInitiateMigration(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var captured modelmigrationinternal.MigrationSpec
	s.controllerState.EXPECT().InsertExport(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, spec modelmigrationinternal.MigrationSpec) error {
			captured = spec
			return nil
		},
	)

	migUUID, err := s.service(c).InitiateMigration(c.Context(), s.validTargetInfo())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(migUUID, tc.Not(tc.Equals), "")
	c.Check(captured.MigrationUUID, tc.Equals, migUUID)
	c.Check(captured.ModelUUID, tc.Equals, s.modelUUID)
	c.Check(captured.TargetControllerUUID, tc.Equals, s.controllerUUID)
	c.Check(captured.TargetUser, tc.Equals, "admin")
	c.Check(captured.TargetMacaroons, tc.Equals, "")
	c.Check(captured.TargetAddrs, tc.HasLen, 1)
	c.Check(captured.TargetAddrs[0].UUID, tc.Not(tc.Equals), "")
	c.Check(captured.TargetAddrs[0].Address, tc.Equals, "10.0.0.1:17070")
}

// TestInitiateMigrationInvalidTarget asserts an invalid target is rejected
// before any state write.
func (s *serviceSuite) TestInitiateMigrationInvalidTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// No InsertExport call expected.
	_, err := s.service(c).InitiateMigration(c.Context(), migration.TargetInfo{})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestMigration asserts an active migration is returned.
func (s *serviceSuite) TestMigration(c *tc.C) {
	defer s.setupMocks(c).Finish()

	migUUID := tc.Must(c, uuid.NewUUID).String()
	stateMig := modelmigrationinternal.Migration{UUID: migUUID, Phase: migration.IMPORT}
	s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(stateMig, nil)

	mig, err := s.service(c).Migration(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mig.UUID, tc.Equals, migUUID)
	c.Check(mig.Phase, tc.Equals, migration.IMPORT)
}

// TestMigrationNone asserts a model with no active migration reports phase NONE.
func (s *serviceSuite) TestMigrationNone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(
		modelmigrationinternal.Migration{}, modelmigrationerrors.ErrMigrationNotFound)

	mig, err := s.service(c).Migration(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mig.Phase, tc.Equals, migration.NONE)
}

// TestGetControllerModelInfo asserts the service reads the model's offer UUIDs
// and third-party remote-offerer pairs from the model DB and passes them to
// the controller-state read, returning the aggregated controller model info.
func (s *serviceSuite) TestGetControllerModelInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	offerUUIDs := []string{"offer-1", "offer-2"}
	offererModels := []modelmigrationinternal.OffererModel{
		{ControllerUUID: "ctrl-1", ModelUUID: "consumed-1"},
	}
	expected := modelmigration.ControllerModelInfo{
		ModelInfo: modelmigration.ModelIdentityInfo{UUID: s.modelUUID, Name: "prod"},
	}

	s.modelState.EXPECT().GetOfferUUIDs(gomock.Any()).Return(offerUUIDs, nil)
	s.modelState.EXPECT().GetThirdPartyOffererModels(gomock.Any()).Return(offererModels, nil)
	s.controllerState.EXPECT().
		GetControllerModelInfo(gomock.Any(), s.modelUUID, offerUUIDs, offererModels).
		Return(expected, nil)

	info, err := s.service(c).GetControllerModelInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info, tc.DeepEquals, expected)
}

// TestSourceControllerInfoArrangesRawStateAddresses asserts the service
// arranges raw controller API address rows into the client-facing order.
func (s *serviceSuite) TestSourceControllerInfoArrangesRawStateAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := modelmigrationinternal.SourceControllerInfo{
		ControllerUUID:  s.controllerUUID,
		ControllerAlias: "source",
		CACert:          "ca-cert",
		Addrs: []modelmigrationinternal.SourceControllerAddress{{
			ControllerID: "2",
			Address:      "10.0.0.2:17070",
			Scope:        string(network.ScopeCloudLocal),
			IsAgent:      true,
		}, {
			ControllerID: "1",
			Address:      "10.0.0.1:17070",
			Scope:        string(network.ScopeCloudLocal),
			IsAgent:      true,
		}, {
			ControllerID: "1",
			Address:      "192.0.2.1:17070",
			Scope:        string(network.ScopePublic),
			IsAgent:      true,
		}},
	}
	s.controllerState.EXPECT().GetSourceControllerInfo(gomock.Any()).Return(stateInfo, nil)

	info, err := s.service(c).SourceControllerInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.ControllerTag, tc.DeepEquals, names.NewControllerTag(s.controllerUUID))
	c.Check(info.ControllerAlias, tc.Equals, "source")
	c.Check(info.Addrs, tc.DeepEquals, []string{
		"192.0.2.1:17070",
		"10.0.0.1:17070",
		"10.0.0.2:17070",
	})
	c.Check(info.CACert, tc.Equals, "ca-cert")
}

// TestSourceControllerInfoSingleAddress asserts a single raw address is
// surfaced unchanged.
func (s *serviceSuite) TestSourceControllerInfoSingleAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := modelmigrationinternal.SourceControllerInfo{
		ControllerUUID:  s.controllerUUID,
		ControllerAlias: "source",
		CACert:          "ca-cert",
		Addrs: []modelmigrationinternal.SourceControllerAddress{{
			ControllerID: "1",
			Address:      "10.0.0.1:17070",
			Scope:        string(network.ScopeCloudLocal),
			IsAgent:      true,
		}},
	}
	s.controllerState.EXPECT().GetSourceControllerInfo(gomock.Any()).Return(stateInfo, nil)

	info, err := s.service(c).SourceControllerInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Addrs, tc.DeepEquals, []string{"10.0.0.1:17070"})
}

// TestSourceControllerInfoNoAddresses asserts that a controller with no
// recorded API addresses cannot act as a migration source: the target would
// have nothing to dial back to advance the migration.
func (s *serviceSuite) TestSourceControllerInfoNoAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := modelmigrationinternal.SourceControllerInfo{
		ControllerUUID:  s.controllerUUID,
		ControllerAlias: "source",
		CACert:          "ca-cert",
	}
	s.controllerState.EXPECT().GetSourceControllerInfo(gomock.Any()).Return(stateInfo, nil)

	_, err := s.service(c).SourceControllerInfo(c.Context())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrSourceControllerNoAPIAddresses)
}

// TestSourceControllerInfoOnlyUnusableAddresses asserts that raw addresses that
// do not survive scope prioritization (e.g. machine-local only) are treated the
// same as having no addresses: the guard sits on the arranged client-facing
// list, not on the raw state rows.
func (s *serviceSuite) TestSourceControllerInfoOnlyUnusableAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	stateInfo := modelmigrationinternal.SourceControllerInfo{
		ControllerUUID:  s.controllerUUID,
		ControllerAlias: "source",
		CACert:          "ca-cert",
		Addrs: []modelmigrationinternal.SourceControllerAddress{{
			ControllerID: "1",
			Address:      "127.0.0.1:17070",
			Scope:        string(network.ScopeMachineLocal),
			IsAgent:      true,
		}},
	}
	s.controllerState.EXPECT().GetSourceControllerInfo(gomock.Any()).Return(stateInfo, nil)

	_, err := s.service(c).SourceControllerInfo(c.Context())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrSourceControllerNoAPIAddresses)
}

// TestSourceControllerInfoError asserts a controller-state read failure is
// surfaced.
func (s *serviceSuite) TestSourceControllerInfoError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetSourceControllerInfo(gomock.Any()).
		Return(modelmigrationinternal.SourceControllerInfo{}, errors.New("boom"))

	_, err := s.service(c).SourceControllerInfo(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

// TestGetControllerModelInfoOffererModelsError asserts offerer-pair read
// failures are surfaced and the controller-state read is not attempted.
func (s *serviceSuite) TestGetControllerModelInfoOffererModelsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetOfferUUIDs(gomock.Any()).Return([]string{"offer-1"}, nil)
	s.modelState.EXPECT().GetThirdPartyOffererModels(gomock.Any()).Return(nil, errors.New("boom"))

	_, err := s.service(c).GetControllerModelInfo(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*reading model offerer models.*boom")
}

// TestGetControllerModelInfoOfferUUIDsError asserts a model-DB read failure is
// surfaced and the controller-state read is not attempted.
func (s *serviceSuite) TestGetControllerModelInfoOfferUUIDsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetOfferUUIDs(gomock.Any()).Return(nil, errors.New("boom"))

	_, err := s.service(c).GetControllerModelInfo(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*reading model offer UUIDs.*boom")
}

// TestModelMigrationMode asserts the mode is passed through from state.
func (s *serviceSuite) TestModelMigrationMode(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetMigrationMode(gomock.Any(), s.modelUUID).Return(
		modelmigration.MigrationModeExporting, nil)

	mode, err := s.service(c).ModelMigrationMode(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mode, tc.Equals, modelmigration.MigrationModeExporting)
}

// TestSetMigrationPhase asserts the active migration is resolved and the phase
// set against it.
func (s *serviceSuite) TestSetMigrationPhase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	migUUID := tc.Must(c, uuid.NewUUID).String()
	gomock.InOrder(
		s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(
			modelmigrationinternal.Migration{UUID: migUUID}, nil),
		s.controllerState.EXPECT().SetPhase(gomock.Any(), migUUID, migration.IMPORT).Return(nil),
	)

	err := s.service(c).SetMigrationPhase(c.Context(), migration.IMPORT)
	c.Assert(err, tc.ErrorIsNil)
}

// TestMarkModelAsGone asserts the full REAP algorithm runs: capture offers,
// stage redirect, and run the purge transaction (which stages the model
// database deletion).
func (s *serviceSuite) TestMarkModelAsGone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	migUUID := tc.Must(c, uuid.NewUUID).String()
	target := modelmigrationinternal.TargetInfo{
		ControllerUUID:  "target-controller-uuid",
		ControllerAlias: "target-alias",
		Addrs:           []string{"10.0.0.1:17070"},
		CACert:          "ca-cert",
	}
	mig := modelmigrationinternal.Migration{
		UUID:   migUUID,
		Phase:  migration.REAP,
		Target: target,
	}

	gomock.InOrder(
		s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(mig, nil),
		s.modelState.EXPECT().GetOfferUUIDs(gomock.Any()).Return([]string{"offer-1"}, nil),
		s.controllerState.EXPECT().EnsureExportOffers(gomock.Any(), migUUID, []string{"offer-1"}).Return(nil),
		s.controllerState.EXPECT().GetModelUsersForRedirect(gomock.Any(), s.modelUUID).Return(nil, nil),
		s.controllerState.EXPECT().StageModelRedirect(gomock.Any(), migUUID, s.modelUUID, gomock.Any(), gomock.Any()).Return(nil),
		s.controllerState.EXPECT().CompleteModelRedirectAndPurge(gomock.Any(), migUUID, s.modelUUID).Return(nil),
	)

	err := s.service(c).MarkModelAsGone(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

// TestMarkModelAsGoneNoProviderDestroy asserts that REAP never reaches the
// provider destruction path. The instance provider and resource provider must
// never be called during source REAP — it is a migration-specific purge, not
// normal model removal.
func (s *serviceSuite) TestMarkModelAsGoneNoProviderDestroy(c *tc.C) {
	defer s.setupMocks(c).Finish()

	migUUID := tc.Must(c, uuid.NewUUID).String()
	mig := modelmigrationinternal.Migration{
		UUID:  migUUID,
		Phase: migration.REAP,
		Target: modelmigrationinternal.TargetInfo{
			ControllerUUID:  "target-controller-uuid",
			ControllerAlias: "target-alias",
			Addrs:           []string{"10.0.0.1:17070"},
			CACert:          "ca-cert",
		},
	}

	gomock.InOrder(
		s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(mig, nil),
		s.modelState.EXPECT().GetOfferUUIDs(gomock.Any()).Return(nil, nil),
		s.controllerState.EXPECT().EnsureExportOffers(gomock.Any(), migUUID, gomock.Any()).Return(nil),
		s.controllerState.EXPECT().GetModelUsersForRedirect(gomock.Any(), s.modelUUID).Return(nil, nil),
		s.controllerState.EXPECT().StageModelRedirect(gomock.Any(), migUUID, s.modelUUID, gomock.Any(), gomock.Any()).Return(nil),
		s.controllerState.EXPECT().CompleteModelRedirectAndPurge(gomock.Any(), migUUID, s.modelUUID).Return(nil),
	)

	err := s.service(c).MarkModelAsGone(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

// TestMarkModelAsGoneRetryAfterPurgeFailure asserts that a failure of the
// purge transaction (the commit point) leaves REAP retryable: every step
// before it is idempotent, so the whole sequence can simply run again.
func (s *serviceSuite) TestMarkModelAsGoneRetryAfterPurgeFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	migUUID := tc.Must(c, uuid.NewUUID).String()
	mig := modelmigrationinternal.Migration{
		UUID:  migUUID,
		Phase: migration.REAP,
		Target: modelmigrationinternal.TargetInfo{
			ControllerUUID: "target-controller-uuid",
			Addrs:          []string{"10.0.0.1:17070"},
			CACert:         "ca-cert",
		},
	}

	s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(mig, nil).Times(2)
	s.modelState.EXPECT().GetOfferUUIDs(gomock.Any()).Return([]string{"offer-1"}, nil).Times(2)
	s.controllerState.EXPECT().EnsureExportOffers(gomock.Any(), migUUID, gomock.Any()).Return(nil).Times(2)
	s.controllerState.EXPECT().GetModelUsersForRedirect(gomock.Any(), s.modelUUID).Return(nil, nil).Times(2)
	s.controllerState.EXPECT().StageModelRedirect(gomock.Any(), migUUID, s.modelUUID, gomock.Any(), gomock.Any()).Return(nil).Times(2)
	// First purge attempt fails; nothing destructive is committed for it.
	s.controllerState.EXPECT().CompleteModelRedirectAndPurge(gomock.Any(), migUUID, s.modelUUID).Return(errors.New("dqlite hiccup")).Times(1)
	s.controllerState.EXPECT().CompleteModelRedirectAndPurge(gomock.Any(), migUUID, s.modelUUID).Return(nil).Times(1)

	// First call fails at the commit point: nothing destructive happened.
	err := s.service(c).MarkModelAsGone(c.Context())
	c.Check(err, tc.ErrorMatches, "purging source model .*: dqlite hiccup")

	// Retry succeeds end to end.
	err = s.service(c).MarkModelAsGone(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

// TestMarkModelAsGoneNoActiveMigration asserts that when no active export
// exists (already DONE from a previous run), MarkModelAsGone returns nil
// idempotently.
func (s *serviceSuite) TestMarkModelAsGoneNoActiveMigration(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(
		modelmigrationinternal.Migration{}, modelmigrationerrors.ErrMigrationNotFound)

	err := s.service(c).MarkModelAsGone(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetMigrationStatusMessage asserts the message is recorded against the
// active migration.
func (s *serviceSuite) TestSetMigrationStatusMessage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	migUUID := tc.Must(c, uuid.NewUUID).String()
	gomock.InOrder(
		s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(
			modelmigrationinternal.Migration{UUID: migUUID}, nil),
		s.controllerState.EXPECT().SetStatusMessage(gomock.Any(), migUUID, "hello").Return(nil),
	)

	err := s.service(c).SetMigrationStatusMessage(c.Context(), "hello")
	c.Assert(err, tc.ErrorIsNil)
}

// TestReportMinion asserts a minion report is recorded with the entity key
// supplied by the facade.
func (s *serviceSuite) TestReportMinion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	migUUID := tc.Must(c, uuid.NewUUID).String()
	gomock.InOrder(
		s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(
			modelmigrationinternal.Migration{UUID: migUUID}, nil),
		s.controllerState.EXPECT().InsertMinionReport(
			gomock.Any(), migUUID, migration.IMPORT, "machine-0", true).Return(nil),
	)

	err := s.service(c).ReportMinion(c.Context(), "machine-0", migration.IMPORT, true)
	c.Assert(err, tc.ErrorIsNil)
}

// TestMinionReports asserts the aggregated reports are mapped into a
// core/migration.MinionReports, splitting failed and unknown entities by kind.
func (s *serviceSuite) TestMinionReports(c *tc.C) {
	defer s.setupMocks(c).Finish()

	migUUID := tc.Must(c, uuid.NewUUID).String()
	gomock.InOrder(
		s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(
			modelmigrationinternal.Migration{UUID: migUUID, Phase: migration.QUIESCE}, nil),
		s.controllerState.EXPECT().AggregateMinionReports(gomock.Any(), migUUID, migration.QUIESCE).Return(
			modelmigrationinternal.MinionReports{
				Phase:     migration.QUIESCE,
				Succeeded: []string{"machine-0", "unit-foo-0"},
				Failed:    []string{"machine-1", "unit-bar-0"},
			}, nil),
		s.modelState.EXPECT().GetMigrationAgents(gomock.Any()).Return(
			modelmigrationinternal.MigrationAgents{
				Machines:     []string{"0", "1"},
				Units:        []string{"foo/0", "bar/0"},
				Applications: []string{"legacy"},
			}, nil),
	)

	reports, err := s.service(c).MinionReports(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reports.MigrationId, tc.Equals, migUUID)
	c.Check(reports.Phase, tc.Equals, migration.QUIESCE)
	c.Check(reports.TotalCount, tc.Equals, 5)
	c.Check(reports.SuccessCount, tc.Equals, 2)
	c.Check(reports.UnknownCount, tc.Equals, 1)
	c.Check(reports.FailedMachines, tc.SameContents, []string{"1"})
	c.Check(reports.FailedUnits, tc.SameContents, []string{"bar/0"})
	c.Check(reports.SomeUnknownApplications, tc.SameContents, []string{"legacy"})
}

func (s *serviceSuite) TestMinionReportsDoesNotValidateReportedAgentInventory(c *tc.C) {
	defer s.setupMocks(c).Finish()

	migUUID := tc.Must(c, uuid.NewUUID).String()
	gomock.InOrder(
		s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(
			modelmigrationinternal.Migration{UUID: migUUID, Phase: migration.QUIESCE}, nil),
		s.controllerState.EXPECT().AggregateMinionReports(gomock.Any(), migUUID, migration.QUIESCE).Return(
			modelmigrationinternal.MinionReports{
				Phase:     migration.QUIESCE,
				Succeeded: []string{"machine-0", "machine-42"},
			}, nil),
		s.modelState.EXPECT().GetMigrationAgents(gomock.Any()).Return(
			modelmigrationinternal.MigrationAgents{
				Machines: []string{"0"},
			}, nil),
	)

	reports, err := s.service(c).MinionReports(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reports.TotalCount, tc.Equals, 1)
	c.Check(reports.SuccessCount, tc.Equals, 2)
	c.Check(reports.UnknownCount, tc.Equals, 0)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelUUID = tc.Must(c, uuid.NewUUID).String()
	s.controllerUUID = tc.Must(c, uuid.NewUUID).String()
	s.controllerState = NewMockControllerState(ctrl)
	s.modelState = NewMockModelState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)
	s.credentialValidator = NewMockCredentialValidator(ctrl)

	s.instanceProvider = NewMockInstanceProvider(ctrl)
	s.resourceProvider = NewMockResourceProvider(ctrl)

	c.Cleanup(func() {
		s.controllerState = nil
		s.modelState = nil
		s.watcherFactory = nil
		s.credentialValidator = nil
		s.instanceProvider = nil
		s.resourceProvider = nil
		s.modelUUID = ""
		s.controllerUUID = ""
	})

	return ctrl
}

func (s *serviceSuite) instanceProviderGetter(_ *tc.C) providertracker.ProviderGetter[InstanceProvider] {
	return func(_ context.Context) (InstanceProvider, error) {
		return s.instanceProvider, nil
	}
}

func (s *serviceSuite) resourceProviderGetter(_ *tc.C) providertracker.ProviderGetter[ResourceProvider] {
	return func(_ context.Context) (ResourceProvider, error) {
		return s.resourceProvider, nil
	}
}

type instanceStub struct {
	instances.Instance
	id string
}

func (i *instanceStub) Id() instance.Id {
	return instance.Id(i.id)
}

func (i *instanceStub) Status(context.Context) instance.Status {
	return instance.Status{
		Status:  status.Maintenance,
		Message: "some message",
	}
}

func (i *instanceStub) Addresses(context.Context) (network.ProviderAddresses, error) {
	return network.ProviderAddresses{}, nil
}
