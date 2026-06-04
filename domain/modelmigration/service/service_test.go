// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
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
	controllerState  *MockControllerState
	modelState       *MockModelState
	watcherFactory   *MockWatcherFactory
	instanceProvider *MockInstanceProvider
	resourceProvider *MockResourceProvider
	modelUUID        string
	controllerUUID   string
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
	).AdoptResources(c.Context(), sourceControllerVersion)
	c.Check(err, tc.ErrorIsNil)
}

// TestMachinesFromProviderDiscrepancy is testing the return value from
// [Service.CheckMachines] and that it reports discrepancies from the cloud.
func (s *serviceSuite) TestMachinesFromProviderNotInModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

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
	s.modelState.EXPECT().GetAllInstanceIDs(gomock.Any()).
		Return(set.NewStrings("instance0"), nil)

	_, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
	).CheckMachines(c.Context())
	c.Check(err, tc.ErrorMatches, "provider instance IDs.*instance1.*")
}

// TestMachineInstanceIDsNotInProvider is testing the return value from
// [Service.CheckMachines] and that it reports discrepancies from the model
// on the DB.
func (s *serviceSuite) TestMachineInstanceIDsNotInProvider(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.instanceProvider.EXPECT().AllInstances(gomock.Any()).
		Return([]instances.Instance{
			&instanceStub{
				id: "instance0",
			},
		},
			nil)
	s.modelState.EXPECT().GetAllInstanceIDs(gomock.Any()).
		Return(set.NewStrings("instance0", "instance1"), nil)

	_, err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
	).CheckMachines(c.Context())
	c.Check(err, tc.ErrorMatches, "instance IDs.*instance1.*")
}

func (s *serviceSuite) TestActivateImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentVersion := semversion.MustParse("4.0.0").String()
	desiredVersion := semversion.MustParse("4.0.1").String()

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
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestActivateImportSameVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentVersion := semversion.MustParse("4.0.0").String()
	desiredVersion := semversion.MustParse("4.0.0").String()

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
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestActivateImportControllerFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cExp := s.controllerState.EXPECT()

	cExp.GetControllerTargetVersion(gomock.Any()).Return("", errors.Errorf("front fell off"))

	err := NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorMatches, ".*front fell off")
}

func (s *serviceSuite) TestActivateImportModelFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	desiredVersion := semversion.MustParse("4.0.1").String()

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
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorMatches, ".*front fell off")
}

// TestWatchForMigration asserts that WatchForMigration asks the watcher
// factory for a notify watcher filtering on the model_migrating namespace
// scoped to this service's model UUID.
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
	s.modelState.EXPECT().GetNamespaceModelMigrating().Return("model_migrating")
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
	)
	w, err := svc.WatchForMigration(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Check(namespace, tc.Equals, "model_migrating")
	c.Check(changeMask, tc.Equals, changestream.All)
	c.Check(predicateCalled, tc.IsTrue)
	c.Check(matchesUUID, tc.IsTrue)
	c.Check(matchesOtherID, tc.IsFalse)
}

// TestWatchForMigrationError asserts that if the watcher factory returns an
// error, WatchForMigration propagates it to the caller.
func (s *serviceSuite) TestWatchForMigrationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetNamespaceModelMigrating().Return("model_migrating")
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
	)
	_, err := svc.WatchForMigration(c.Context())
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

// service constructs a Service backed by the suite mocks.
func (s *serviceSuite) service() *Service {
	return NewService(
		s.controllerState,
		s.modelState,
		s.modelUUID,
		s.watcherFactory,
		func(context.Context) (InstanceProvider, error) { return s.instanceProvider, nil },
		func(context.Context) (ResourceProvider, error) { return s.resourceProvider, nil },
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

	migUUID, err := s.service().InitiateMigration(c.Context(), s.validTargetInfo())
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
	_, err := s.service().InitiateMigration(c.Context(), migration.TargetInfo{})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestMigration asserts an active migration is returned.
func (s *serviceSuite) TestMigration(c *tc.C) {
	defer s.setupMocks(c).Finish()

	migUUID := tc.Must(c, uuid.NewUUID).String()
	stateMig := modelmigrationinternal.Migration{UUID: migUUID, Phase: migration.IMPORT}
	s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(stateMig, nil)

	mig, err := s.service().Migration(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mig.UUID, tc.Equals, migUUID)
	c.Check(mig.Phase, tc.Equals, migration.IMPORT)
}

// TestMigrationNone asserts a model with no active migration reports phase NONE.
func (s *serviceSuite) TestMigrationNone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(
		modelmigrationinternal.Migration{}, modelmigrationerrors.ErrMigrationNotFound)

	mig, err := s.service().Migration(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(mig.Phase, tc.Equals, migration.NONE)
}

// TestModelMigrationMode asserts the mode is passed through from state.
func (s *serviceSuite) TestModelMigrationMode(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerState.EXPECT().GetMigrationMode(gomock.Any(), s.modelUUID).Return(
		modelmigration.MigrationModeExporting, nil)

	mode, err := s.service().ModelMigrationMode(c.Context())
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

	err := s.service().SetMigrationPhase(c.Context(), migration.IMPORT)
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

	err := s.service().SetMigrationStatusMessage(c.Context(), "hello")
	c.Assert(err, tc.ErrorIsNil)
}

// TestReportFromMachine asserts a machine report is keyed by its machine tag.
func (s *serviceSuite) TestReportFromMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	migUUID := tc.Must(c, uuid.NewUUID).String()
	gomock.InOrder(
		s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(
			modelmigrationinternal.Migration{UUID: migUUID}, nil),
		s.controllerState.EXPECT().InsertMinionReport(
			gomock.Any(), migUUID, migration.IMPORT, "machine-0", true).Return(nil),
	)

	err := s.service().ReportFromMachine(c.Context(), machine.Name("0"), migration.IMPORT, true)
	c.Assert(err, tc.ErrorIsNil)
}

// TestReportFromUnit asserts a unit report is keyed by its unit tag.
func (s *serviceSuite) TestReportFromUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	migUUID := tc.Must(c, uuid.NewUUID).String()
	gomock.InOrder(
		s.controllerState.EXPECT().GetActiveExport(gomock.Any(), s.modelUUID).Return(
			modelmigrationinternal.Migration{UUID: migUUID}, nil),
		s.controllerState.EXPECT().InsertMinionReport(
			gomock.Any(), migUUID, migration.IMPORT, "unit-foo-0", false).Return(nil),
	)

	err := s.service().ReportFromUnit(c.Context(), unit.Name("foo/0"), migration.IMPORT, false)
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
			set.NewStrings(
				"machine-0",
				"unit-foo-0",
				"machine-1",
				"unit-bar-0",
				"application-legacy",
			), nil),
	)

	reports, err := s.service().MinionReports(c.Context())
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
			set.NewStrings("machine-0"), nil),
	)

	reports, err := s.service().MinionReports(c.Context())
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

	s.instanceProvider = NewMockInstanceProvider(ctrl)
	s.resourceProvider = NewMockResourceProvider(ctrl)

	c.Cleanup(func() {
		s.controllerState = nil
		s.modelState = nil
		s.watcherFactory = nil
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
