// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	charmscommon "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasapplicationprovisioner"
	"github.com/juju/juju/worker/caasapplicationprovisioner/mocks"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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

	shouldExit, err := caasapplicationprovisioner.VerifyCharmUpgraded("test", facade, tomb, s.logger)
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

	shouldExit, err := caasapplicationprovisioner.VerifyCharmUpgraded("test", facade, tomb, s.logger)
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

	shouldExit, err := caasapplicationprovisioner.VerifyCharmUpgraded("test", facade, tomb, s.logger)
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

	shouldExit, err := caasapplicationprovisioner.VerifyCharmUpgraded("test", facade, tomb, s.logger)
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

	err := caasapplicationprovisioner.UpgradePodSpec("test", broker, clk, tomb, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}
