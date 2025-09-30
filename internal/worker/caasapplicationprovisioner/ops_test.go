// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"github.com/juju/charm/v12"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	charmscommon "github.com/juju/juju/api/common/charms"
	api "github.com/juju/juju/api/controller/caasapplicationprovisioner"
	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner/mocks"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&OpsSuite{})

type OpsSuite struct {
	coretesting.BaseSuite

	modelTag names.ModelTag
	logger   loggo.Logger
}

func (s *OpsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggo.GetLogger("test")
}

func (s *OpsSuite) TestVerifyCharmUpgraded(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmInfoV1 := &charmscommon.CharmInfo{
		Meta: &charm.Meta{Name: "test"},
	}
	charmInfoV2 := &charmscommon.CharmInfo{
		Meta:     &charm.Meta{Name: "test"},
		Manifest: &charm.Manifest{Bases: []charm.Base{{}}},
	}

	appStateChan := make(chan struct{}, 1)
	appStateWatcher := watchertest.NewMockNotifyWatcher(appStateChan)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	done := make(chan struct{})
	defer close(done)
	tomb := &mockTomb{done}

	gomock.InOrder(
		// Wait till charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfoV1, nil),
		facade.EXPECT().Life("test").DoAndReturn(func(appName string) (life.Value, error) {
			appStateChan <- struct{}{}
			return life.Alive, nil
		}),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfoV2, nil),
	)

	shouldExit, err := caasapplicationprovisioner.AppOps.VerifyCharmUpgraded("test", facade, tomb, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(shouldExit, jc.IsFalse)
}

func (s *OpsSuite) TestVerifyCharmUpgradeLifeDead(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmInfoV1 := &charmscommon.CharmInfo{
		Meta: &charm.Meta{Name: "test"},
	}

	appStateChan := make(chan struct{}, 1)
	appStateWatcher := watchertest.NewMockNotifyWatcher(appStateChan)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	done := make(chan struct{})
	tomb := &mockTomb{done}

	gomock.InOrder(
		// Wait till charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfoV1, nil),
		facade.EXPECT().Life("test").DoAndReturn(func(appName string) (life.Value, error) {
			close(done)
			return life.Dead, nil
		}),
	)

	shouldExit, err := caasapplicationprovisioner.AppOps.VerifyCharmUpgraded("test", facade, tomb, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(shouldExit, jc.IsTrue)
}

func (s *OpsSuite) TestVerifyCharmUpgradeLifeNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmInfoV1 := &charmscommon.CharmInfo{
		Meta: &charm.Meta{Name: "test"},
	}

	appStateChan := make(chan struct{}, 1)
	appStateWatcher := watchertest.NewMockNotifyWatcher(appStateChan)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	done := make(chan struct{})
	tomb := &mockTomb{done}

	gomock.InOrder(
		// Wait till charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").Return(charmInfoV1, nil),
		facade.EXPECT().Life("test").DoAndReturn(func(appName string) (life.Value, error) {
			close(done)
			return "", errors.NotFoundf("test charm")
		}),
	)

	shouldExit, err := caasapplicationprovisioner.AppOps.VerifyCharmUpgraded("test", facade, tomb, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(shouldExit, jc.IsTrue)
}

func (s *OpsSuite) TestVerifyCharmUpgradeInfoNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appStateChan := make(chan struct{}, 1)
	appStateWatcher := watchertest.NewMockNotifyWatcher(appStateChan)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	done := make(chan struct{})
	tomb := &mockTomb{done}

	gomock.InOrder(
		// Wait till charm is v2
		facade.EXPECT().WatchApplication("test").Return(appStateWatcher, nil),
		facade.EXPECT().ApplicationCharmInfo("test").DoAndReturn(func(appName string) (*charmscommon.CharmInfo, error) {
			close(done)
			return nil, errors.NotFoundf("test charm")
		}),
	)

	shouldExit, err := caasapplicationprovisioner.AppOps.VerifyCharmUpgraded("test", facade, tomb, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(shouldExit, jc.IsTrue)
}

func (s *OpsSuite) TestUpgradePodSpec(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)

	done := make(chan struct{})
	defer close(done)
	tomb := &mockTomb{done}

	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)

	gomock.InOrder(
		broker.EXPECT().OperatorExists("test").Return(caas.DeploymentState{Exists: true}, nil),
		broker.EXPECT().DeleteService("test").Return(nil),
		broker.EXPECT().Units("test", caas.ModeWorkload).Return([]caas.Unit{}, nil),
		broker.EXPECT().DeleteOperator("test").Return(nil),
		broker.EXPECT().OperatorExists("test").Return(caas.DeploymentState{Exists: false}, nil),
	)

	err := caasapplicationprovisioner.AppOps.UpgradePodSpec("test", broker, clk, tomb, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureTrust(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)
	app := caasmocks.NewMockApplication(ctrl)

	gomock.InOrder(
		unitFacade.EXPECT().ApplicationTrust("test").Return(true, nil),
		app.EXPECT().Trust(true).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureTrust("test", app, unitFacade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpsSuite) TestUpdateState(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	broker := mocks.NewMockCAASBroker(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)

	appTag := names.NewApplicationTag("test").String()
	service := &caas.Service{
		Id: "provider-id",
		Status: status.StatusInfo{
			Status:  status.Active,
			Message: "nice message",
			Data: map[string]interface{}{
				"nice": "data",
			},
		},
		Addresses: network.ProviderAddresses{{
			MachineAddress: network.NewMachineAddress("1.2.3.4"),
			SpaceName:      "space-name",
		}},
	}
	updateServiceArg := params.UpdateApplicationServiceArg{
		ApplicationTag: appTag,
		ProviderId:     "provider-id",
		Addresses: []params.Address{{
			Value:     "1.2.3.4",
			SpaceName: "space-name",
			Type:      "ipv4",
			Scope:     "public",
		}},
	}
	units := []caas.Unit{{
		Id:       "a",
		Address:  "1.2.3.5",
		Ports:    []string{"80", "443"},
		Stateful: true,
		Status: status.StatusInfo{
			Status:  status.Active,
			Message: "different",
		},
		FilesystemInfo: []caas.FilesystemInfo{{
			StorageName:  "s",
			FilesystemId: "fsid",
			Volume: caas.VolumeInfo{
				VolumeId: "vid",
			},
		}},
	}, {
		Id:       "b",
		Address:  "1.2.3.6",
		Ports:    []string{"80", "443"},
		Stateful: true,
		Status: status.StatusInfo{
			Status:  status.Allocating,
			Message: "same",
		},
	}}
	updateUnitsArg := params.UpdateApplicationUnits{
		ApplicationTag: appTag,
		Status: params.EntityStatus{
			Status: status.Active,
			Info:   "nice message",
			Data: map[string]interface{}{
				"nice": "data",
			},
		},
		Scale: nil,
		Units: []params.ApplicationUnitParams{{
			ProviderId: "a",
			Address:    "1.2.3.5",
			Ports:      []string{"80", "443"},
			Stateful:   true,
			Status:     "active",
			Info:       "different",
			FilesystemInfo: []params.KubernetesFilesystemInfo{{
				StorageName:  "s",
				FilesystemId: "fsid",
				Volume: params.KubernetesVolumeInfo{
					VolumeId: "vid",
				},
			}},
		}, {
			ProviderId: "b",
			Address:    "1.2.3.6",
			Ports:      []string{"80", "443"},
			Stateful:   true,
			Status:     "unknown",
		}},
	}
	appUnitInfo := &params.UpdateApplicationUnitsInfo{
		Units: []params.ApplicationUnitInfo{{
			UnitTag:    "unit-test-0",
			ProviderId: "a",
		}, {
			UnitTag:    "unit-test-1",
			ProviderId: "b",
		}},
	}
	gomock.InOrder(
		app.EXPECT().Service().Return(service, nil),
		unitFacade.EXPECT().UpdateApplicationService(updateServiceArg).Return(nil),
		app.EXPECT().Units().Return(units, nil),
		facade.EXPECT().UpdateUnits(updateUnitsArg).Return(appUnitInfo, nil),
		broker.EXPECT().AnnotateUnit("test", caas.ModeSidecar, "a", names.NewUnitTag("test/0")).Return(nil),
		broker.EXPECT().AnnotateUnit("test", caas.ModeSidecar, "b", names.NewUnitTag("test/1")).Return(nil),
	)

	lastReportedStatus := map[string]status.StatusInfo{
		"b": {
			Status:  status.Allocating,
			Message: "same",
		},
	}
	currentReportedStatus, err := caasapplicationprovisioner.AppOps.UpdateState("test", app, lastReportedStatus, broker, facade, unitFacade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currentReportedStatus, jc.DeepEquals, map[string]status.StatusInfo{
		"a": {Status: "active", Message: "different"},
		"b": {Status: "allocating", Message: "same"},
	})
}

func (s *OpsSuite) TestRefreshApplicationStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	appLife := life.Alive
	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	appState := caas.ApplicationState{
		DesiredReplicas: 2,
	}
	units := []params.CAASUnit{{
		UnitStatus: &params.UnitStatus{AgentStatus: params.DetailedStatus{Status: "active"}},
	}, {
		UnitStatus: &params.UnitStatus{AgentStatus: params.DetailedStatus{Status: "allocating"}},
	}}
	gomock.InOrder(
		app.EXPECT().State().Return(appState, nil),
		facade.EXPECT().Units("test").Return(units, nil),
		facade.EXPECT().SetOperatorStatus("test", status.Waiting, "waiting for units to settle down", nil).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.RefreshApplicationStatus("test", app, appLife, facade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpsSuite) TestWaitForTerminated(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)

	gomock.InOrder(
		app.EXPECT().Exists().Return(caas.DeploymentState{
			Exists: true,
		}, nil),
	)
	err := caasapplicationprovisioner.AppOps.WaitForTerminated("test", app, clk)
	c.Assert(err, gc.ErrorMatches, `application "test" should be terminating but is now running`)

	gomock.InOrder(
		app.EXPECT().Exists().Return(caas.DeploymentState{
			Exists:      true,
			Terminating: true,
		}, nil),
		app.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
	)
	err = caasapplicationprovisioner.AppOps.WaitForTerminated("test", app, clk)
	c.Assert(err, jc.ErrorIsNil)
}

// TestReconcileDeadUnitScaleNotScaling tests that nothing happens when not scaling
func (s *OpsSuite) TestReconcileDeadUnitScaleNotScaling(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	units := []params.CAASUnit{{Tag: names.NewUnitTag("test/0")}}
	ps := params.CAASApplicationProvisioningState{
		Scaling:     false, // Not scaling
		ScaleTarget: 1,
	}

	gomock.InOrder(
		facade.EXPECT().Units("test").Return(units, nil),
		facade.EXPECT().ProvisioningState("test").Return(&ps, nil),
		// Should return early - no further calls
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale("test", app, facade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

// TestReconcileDeadUnitScaleNilProvisioningState tests that nothing happens with nil provisioning state
func (s *OpsSuite) TestReconcileDeadUnitScaleNilProvisioningState(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	units := []params.CAASUnit{{Tag: names.NewUnitTag("test/0")}}

	gomock.InOrder(
		facade.EXPECT().Units("test").Return(units, nil),
		facade.EXPECT().ProvisioningState("test").Return(nil, nil),
		// Should return early - no further calls
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale("test", app, facade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

// TestReconcileDeadUnitScaleScaleUp tests scale up scenario - app.Scale should NOT be called
func (s *OpsSuite) TestReconcileDeadUnitScaleScaleUp(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	// Scale UP: 2 current units -> 5 target units
	units := []params.CAASUnit{
		{Tag: names.NewUnitTag("test/0")},
		{Tag: names.NewUnitTag("test/1")},
	}
	ps := params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 5, // Scale up to 5 units
	}

	gomock.InOrder(
		facade.EXPECT().Units("test").Return(units, nil),
		facade.EXPECT().ProvisioningState("test").Return(&ps, nil),
		facade.EXPECT().Life("test/0").Return(life.Alive, nil), // Below target
		facade.EXPECT().Life("test/1").Return(life.Dead, nil),  // Below target, but dead
		// No app.Scale should be called - all units are below target
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale("test", app, facade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

// TestReconcileDeadUnitScaleScaleDownNotAllDead tests scale down when not all excess units are dead - app.Scale should NOT be called
func (s *OpsSuite) TestReconcileDeadUnitScaleScaleDownNotAllDead(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	// Scale DOWN: 4 current units -> 2 target units, but not all excess units are dead
	units := []params.CAASUnit{
		{Tag: names.NewUnitTag("test/0")},
		{Tag: names.NewUnitTag("test/1")},
		{Tag: names.NewUnitTag("test/2")}, // >= target
		{Tag: names.NewUnitTag("test/3")}, // >= target
	}
	ps := params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 2, // Scale down to 2 units
	}

	gomock.InOrder(
		facade.EXPECT().Units("test").Return(units, nil),
		facade.EXPECT().ProvisioningState("test").Return(&ps, nil),
		facade.EXPECT().Life("test/0").Return(life.Alive, nil), // Below target
		facade.EXPECT().Life("test/1").Return(life.Dead, nil),  // Below target, but dead
		facade.EXPECT().Life("test/2").Return(life.Dead, nil),  // >= target and dead
		facade.EXPECT().Life("test/3").Return(life.Alive, nil), // >= target but alive - prevents scaling
		// No app.Scale should be called - not all excess units (2,3) are dead
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale("test", app, facade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

// TestReconcileDeadUnitScaleScaleDownAllExcessDead tests scale down when all excess units are dead - app.Scale SHOULD be called
func (s *OpsSuite) TestReconcileDeadUnitScaleScaleDownAllExcessDead(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	// Scale DOWN: 4 current units -> 2 target units, all excess units are dead
	units := []params.CAASUnit{
		{Tag: names.NewUnitTag("test/0")},
		{Tag: names.NewUnitTag("test/1")},
		{Tag: names.NewUnitTag("test/2")}, // >= target
		{Tag: names.NewUnitTag("test/3")}, // >= target
	}
	ps := params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 2, // Scale down to 2 units
	}
	appState := caas.ApplicationState{
		Replicas: []string{"a", "b"}, // Already at target scale
	}
	newPs := params.CAASApplicationProvisioningState{
		Scaling:     false,
		ScaleTarget: 0,
	}

	gomock.InOrder(
		facade.EXPECT().Units("test").Return(units, nil),
		facade.EXPECT().ProvisioningState("test").Return(&ps, nil),
		facade.EXPECT().Life("test/0").Return(life.Alive, nil), // Below target
		facade.EXPECT().Life("test/1").Return(life.Dead, nil),  // Below target, but dead
		facade.EXPECT().Life("test/2").Return(life.Dead, nil),  // >= target and dead
		facade.EXPECT().Life("test/3").Return(life.Dead, nil),  // >= target and dead
		// All excess units (2,3) are dead - scaling should proceed
		facade.EXPECT().FilesystemProvisioningInfo("test").Return(api.FilesystemProvisioningInfo{}, nil),
		app.EXPECT().EnsurePVCs(gomock.Any(), gomock.Any()).Return(nil),
		app.EXPECT().Scale(2).Return(nil),
		app.EXPECT().State().Return(appState, nil),
		facade.EXPECT().RemoveUnit("test/2").Return(nil),
		facade.EXPECT().RemoveUnit("test/3").Return(nil),
		facade.EXPECT().SetProvisioningState("test", newPs).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale("test", app, facade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

// TestReconcileDeadUnitScaleScaleDownNoExcessUnits tests when no units need to be removed
func (s *OpsSuite) TestReconcileDeadUnitScaleScaleDownNoExcessUnits(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	// Already at target scale
	units := []params.CAASUnit{
		{Tag: names.NewUnitTag("test/0")},
		{Tag: names.NewUnitTag("test/1")},
	}
	ps := params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 2, // Already at target
	}

	gomock.InOrder(
		facade.EXPECT().Units("test").Return(units, nil),
		facade.EXPECT().ProvisioningState("test").Return(&ps, nil),
		facade.EXPECT().Life("test/0").Return(life.Alive, nil), // Below target
		facade.EXPECT().Life("test/1").Return(life.Dead, nil),  // Below target, but dead
		// No units >= target, so no scaling needed
	)

	err := caasapplicationprovisioner.AppOps.ReconcileDeadUnitScale("test", app, facade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAlive(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)

	ps := params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 1,
	}
	units := []params.CAASUnit{{
		Tag: names.NewUnitTag("test/0"),
	}, {
		Tag: names.NewUnitTag("test/1"),
	}}
	unitsToDestroy := []string{"test/1"}
	gomock.InOrder(
		unitFacade.EXPECT().ApplicationScale("test").Return(1, nil),
		facade.EXPECT().ProvisioningState("test").Return(nil, nil),
		facade.EXPECT().SetProvisioningState("test", ps).Return(nil),
		facade.EXPECT().Units("test").Return(units, nil),
		app.EXPECT().UnitsToRemove(gomock.Any(), 1).Return(unitsToDestroy, nil),
		facade.EXPECT().DestroyUnits(unitsToDestroy).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale("test", app, life.Alive, facade, unitFacade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleAliveRetry(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)

	ps := params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 1,
	}
	units := []params.CAASUnit{{
		Tag: names.NewUnitTag("test/0"),
	}, {
		Tag: names.NewUnitTag("test/1"),
	}}
	unitsToDestroy := []string{"test/1"}
	gomock.InOrder(
		unitFacade.EXPECT().ApplicationScale("test").Return(10, nil),
		facade.EXPECT().ProvisioningState("test").Return(&ps, nil),
		facade.EXPECT().Units("test").Return(units, nil),
		app.EXPECT().UnitsToRemove(gomock.Any(), 1).Return(unitsToDestroy, nil),
		facade.EXPECT().DestroyUnits(unitsToDestroy).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale("test", app, life.Alive, facade, unitFacade, s.logger)
	c.Assert(err, gc.ErrorMatches, `try again`)
}

func (s *OpsSuite) TestEnsureScaleDyingDead(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)

	ps := params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 0,
	}
	units := []params.CAASUnit{{
		Tag: names.NewUnitTag("test/0"),
	}, {
		Tag: names.NewUnitTag("test/1"),
	}}
	unitsToDestroy := []string{"test/0", "test/1"}
	gomock.InOrder(
		facade.EXPECT().ProvisioningState("test").Return(nil, nil),
		facade.EXPECT().SetProvisioningState("test", ps).Return(nil),
		facade.EXPECT().Units("test").Return(units, nil),
		app.EXPECT().UnitsToRemove(gomock.Any(), 0).Return(unitsToDestroy, nil),
		facade.EXPECT().DestroyUnits(unitsToDestroy).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale("test", app, life.Dead, facade, unitFacade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleWithAttachStorage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)

	// Test scenario where we need to scale up and have attached storage
	ps := params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 2,
	}

	// Current units (less than scale target)
	units := []params.CAASUnit{{
		Tag: names.NewUnitTag("test/0"),
	}}

	// FilesystemProvisioningInfo with filesystem attachments
	provisioningInfo := api.FilesystemProvisioningInfo{
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "data",
			Size:        100,
			Provider:    storage.ProviderType("kubernetes"),
		}},
	}

	gomock.InOrder(
		unitFacade.EXPECT().ApplicationScale("test").Return(2, nil),
		facade.EXPECT().ProvisioningState("test").Return(nil, nil),
		facade.EXPECT().SetProvisioningState("test", ps).Return(nil),
		facade.EXPECT().Units("test").Return(units, nil),
		facade.EXPECT().FilesystemProvisioningInfo("test").Return(provisioningInfo, nil),
		app.EXPECT().EnsurePVCs(gomock.Any(), gomock.Any()).Return(nil),
		app.EXPECT().Scale(2).Return(nil),
		facade.EXPECT().SetProvisioningState("test", params.CAASApplicationProvisioningState{}).Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale("test", app, life.Alive, facade, unitFacade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpsSuite) TestEnsureScaleWithAttachStorageEnsurePVCsFails(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)

	ps := params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 2,
	}

	units := []params.CAASUnit{{
		Tag: names.NewUnitTag("test/0"),
	}}

	provisioningInfo := api.FilesystemProvisioningInfo{
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "data",
			Size:        100,
			Provider:    storage.ProviderType("kubernetes"),
		}},
	}

	gomock.InOrder(
		unitFacade.EXPECT().ApplicationScale("test").Return(2, nil),
		facade.EXPECT().ProvisioningState("test").Return(nil, nil),
		facade.EXPECT().SetProvisioningState("test", ps).Return(nil),
		facade.EXPECT().Units("test").Return(units, nil),
		facade.EXPECT().FilesystemProvisioningInfo("test").Return(provisioningInfo, nil),
		app.EXPECT().EnsurePVCs(gomock.Any(), gomock.Any()).Return(errors.New("PVC creation failed")),
	)

	err := caasapplicationprovisioner.AppOps.EnsureScale("test", app, life.Alive, facade, unitFacade, s.logger)
	c.Assert(err, gc.ErrorMatches, "PVC creation failed")
}

func (s *OpsSuite) TestAppAlive(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)
	password := "123456789"
	lastApplied := caas.ApplicationConfig{}

	pi := api.ProvisioningInfo{
		CharmURL: charm.MustParseURL("ch:my-app"),
		ImageDetails: resources.DockerImageDetails{
			RegistryPath: "test-repo/jujud-operator:2.9.99",
			ImageRepoDetails: docker.ImageRepoDetails{
				Repository:    "test-repo",
				ServerAddress: "registry.com",
			},
		},
		Base: corebase.Base{
			OS: "ubuntu",
			Channel: corebase.Channel{
				Track: "22.04",
				Risk:  corebase.Stable,
			},
		},
		Version:              version.MustParse("2.9.99"),
		CharmModifiedVersion: 123,
		APIAddresses:         []string{"1.2.3.1", "1.2.3.2", "1.2.3.3"},
		CACert:               "CACERT",
		Tags: map[string]string{
			"tag": "tag-value",
		},
		Trust:       true,
		Scale:       10,
		Constraints: constraints.MustParse("mem=1G"),
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "data",
			Size:        100,
		}},
		Devices: []devices.KubernetesDeviceParams{},
	}
	charmInfo := charmscommon.CharmInfo{
		Meta: &charm.Meta{
			Containers: map[string]charm.Container{
				"mysql": {
					Resource: "mysql-image",
					Mounts: []charm.Mount{{
						Storage:  "data",
						Location: "/data",
					}},
				},
				"rootless": {
					Resource: "rootless-image",
					Uid:      intPtr(5000),
					Gid:      intPtr(5001),
				},
			},
		},
	}
	ds := caas.DeploymentState{
		Exists:      true,
		Terminating: true,
	}
	oci := map[string]resources.DockerImageDetails{
		"mysql-image": {
			RegistryPath: "mysql/ubuntu:latest-22.04",
		},
		"rootless-image": {
			RegistryPath: "rootless:foo-bar",
		},
	}
	ensureParams := caas.ApplicationConfig{
		AgentVersion:         version.Number{Major: 2, Minor: 9, Patch: 99},
		AgentImagePath:       "test-repo/jujud-operator:2.9.99",
		CharmBaseImagePath:   "test-repo/charm-base:ubuntu-22.04",
		CharmModifiedVersion: 123,
		Containers: map[string]caas.ContainerConfig{
			"mysql": {
				Name: "mysql",
				Image: resources.DockerImageDetails{
					RegistryPath: "mysql/ubuntu:latest-22.04",
				},
				Mounts: []caas.MountConfig{{
					StorageName: "data",
					Path:        "/data",
				}},
			},
			"rootless": {
				Name: "rootless",
				Image: resources.DockerImageDetails{
					RegistryPath: "rootless:foo-bar",
				},
				Uid: intPtr(5000),
				Gid: intPtr(5001),
			},
		},
		IntroductionSecret:   "123456789",
		ControllerAddresses:  "1.2.3.1,1.2.3.2,1.2.3.3",
		ControllerCertBundle: "CACERT",
		ResourceTags: map[string]string{
			"tag": "tag-value",
		},
		Constraints: constraints.MustParse("mem=1G"),
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "data",
			Size:        100,
		}},
		Devices:             []devices.KubernetesDeviceParams{},
		Trust:               true,
		InitialScale:        0,
		CharmUser:           caas.RunAsDefault,
		ProvisionedAppScale: 10,
	}
	gomock.InOrder(
		facade.EXPECT().ProvisioningInfo("test").Return(pi, nil),
		facade.EXPECT().CharmInfo("ch:my-app").Return(&charmInfo, nil),
		app.EXPECT().Exists().Return(ds, nil),
		app.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
		facade.EXPECT().ApplicationOCIResources("test").Return(oci, nil),
		app.EXPECT().Ensure(gomock.Any()).DoAndReturn(func(config caas.ApplicationConfig) error {
			c.Check(config, gc.DeepEquals, ensureParams)
			return nil
		}),
	)

	err := caasapplicationprovisioner.AppOps.AppAlive("test", app, password, &lastApplied, facade, clk, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpsSuite) TestAppDying(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)

	gomock.InOrder(
		facade.EXPECT().ProvisioningState("test").Return(nil, nil),
		facade.EXPECT().SetProvisioningState("test", params.CAASApplicationProvisioningState{Scaling: true, ScaleTarget: 0}).Return(nil),
		facade.EXPECT().Units("test").Return(nil, nil),
		facade.EXPECT().FilesystemProvisioningInfo("test").Return(api.FilesystemProvisioningInfo{}, nil),
		app.EXPECT().EnsurePVCs(gomock.Any(), gomock.Any()).Return(nil),
		app.EXPECT().Scale(0).Return(nil),
		facade.EXPECT().SetProvisioningState("test", params.CAASApplicationProvisioningState{Scaling: false, ScaleTarget: 0}).Return(nil),
		facade.EXPECT().Units("test").Return(nil, nil),
		facade.EXPECT().ProvisioningState("test").Return(nil, nil),
	)

	err := caasapplicationprovisioner.AppOps.AppDying("test", app, life.Dying, facade, unitFacade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpsSuite) TestAppDead(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	unitFacade := mocks.NewMockCAASUnitProvisionerFacade(ctrl)
	broker := mocks.NewMockCAASBroker(ctrl)

	clk := testclock.NewDilatedWallClock(coretesting.ShortWait)

	appTag := names.NewApplicationTag("test").String()
	updateUnitsArgs := params.UpdateApplicationUnits{
		ApplicationTag: appTag,
	}
	gomock.InOrder(
		app.EXPECT().Delete().Return(nil),
		app.EXPECT().Exists().Return(caas.DeploymentState{}, nil),
		app.EXPECT().Service().Return(nil, errors.NotFound),
		app.EXPECT().Units().Return(nil, nil),
		facade.EXPECT().UpdateUnits(updateUnitsArgs).Return(nil, nil),
		facade.EXPECT().ClearApplicationResources("test").Return(nil),
	)

	err := caasapplicationprovisioner.AppOps.AppDead("test", app, broker, facade, unitFacade, clk, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpsSuite) TestReconcileApplicationStorage(c *gc.C) {
	provisioningInfo := api.FilesystemProvisioningInfo{
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "data",
			Size:        100,
			Provider:    storage.ProviderType("kubernetes"),
		}},
	}

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	facade.EXPECT().FilesystemProvisioningInfo("test").Return(provisioningInfo, nil)
	app.EXPECT().ReconcileStorage(provisioningInfo.Filesystems).Return(nil)

	err := caasapplicationprovisioner.AppOps.ReconcileApplicationStorage("test", app, facade, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *OpsSuite) TestReconcileApplicationStorageError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	app := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)

	facade.EXPECT().FilesystemProvisioningInfo("test").Return(api.FilesystemProvisioningInfo{}, errors.New("something went wrong"))

	err := caasapplicationprovisioner.AppOps.ReconcileApplicationStorage("test", app, facade, s.logger)
	c.Assert(err, gc.NotNil)
}

func intPtr(i int) *int {
	return &i
}
