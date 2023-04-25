// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/life"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasapplicationprovisioner"
	"github.com/juju/juju/worker/caasapplicationprovisioner/mocks"
)

var _ = gc.Suite(&ApplicationWorkerSuite{})

type ApplicationWorkerSuite struct {
	coretesting.BaseSuite

	modelTag names.ModelTag
	logger   loggo.Logger
}

func (s *ApplicationWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.modelTag = names.NewModelTag("ffffffff-ffff-ffff-ffff-ffffffffffff")
	s.logger = loggo.GetLogger("test")
}

func (s *ApplicationWorkerSuite) waitDone(c *gc.C, done chan struct{}) {
	select {
	case <-done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *ApplicationWorkerSuite) TestWorker(c *gc.C) {

}

func (s *ApplicationWorkerSuite) startAppWorker(
	c *gc.C,
	clk clock.Clock,
	facade caasapplicationprovisioner.CAASProvisionerFacade,
	broker caasapplicationprovisioner.CAASBroker,
	unitFacade caasapplicationprovisioner.CAASUnitProvisionerFacade,
	ops caasapplicationprovisioner.ApplicationOps,
) worker.Worker {
	config := caasapplicationprovisioner.AppWorkerConfig{
		Name:       "test",
		Facade:     facade,
		Broker:     broker,
		ModelTag:   s.modelTag,
		Clock:      clk,
		Logger:     s.logger,
		UnitFacade: unitFacade,
		Ops:        ops,
	}
	startFunc := caasapplicationprovisioner.NewAppWorker(config)
	c.Assert(startFunc, gc.NotNil)
	appWorker, err := startFunc()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appWorker, gc.NotNil)
	return appWorker
}

func (s *ApplicationWorkerSuite) TestLifeNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)
	brokerApp := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	done := make(chan struct{})

	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(brokerApp),
		facade.EXPECT().Life("test").DoAndReturn(func(appName string) (life.Value, error) {
			close(done)
			return "", errors.NotFoundf("test charm")
		}),
	)
	appWorker := s.startAppWorker(c, nil, facade, broker, nil, ops)

	s.waitDone(c, done)
	workertest.CleanKill(c, appWorker)
}

func (s *ApplicationWorkerSuite) TestUpgradePodSpec(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockCAASBroker(ctrl)
	brokerApp := caasmocks.NewMockApplication(ctrl)
	facade := mocks.NewMockCAASProvisionerFacade(ctrl)
	ops := mocks.NewMockApplicationOps(ctrl)
	done := make(chan struct{})

	clk := testclock.NewClock(time.Time{})
	gomock.InOrder(
		broker.EXPECT().Application("test", caas.DeploymentStateful).Return(brokerApp),
		facade.EXPECT().Life("test").Return(life.Alive, nil),

		// Verify charm is v2
		ops.EXPECT().VerifyCharmUpgraded("test", gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil),

		// Operator delete loop (with a retry)
		ops.EXPECT().UpgradePodSpec("test", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil),

		// Make SetPassword return an error to exit early (we've tested what
		// we want to above).
		facade.EXPECT().SetPassword("test", gomock.Any()).DoAndReturn(func(appName, password string) error {
			close(done)
			return errors.New("exit early error")
		}),
	)

	appWorker := s.startAppWorker(c, clk, facade, broker, nil, ops)

	s.waitDone(c, done)
	workertest.DirtyKill(c, appWorker)
}
