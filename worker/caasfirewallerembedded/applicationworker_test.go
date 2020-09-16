// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallerembedded_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	charmscommon "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasfirewallerembedded"
	"github.com/juju/juju/worker/caasfirewallerembedded/mocks"
)

type appWorkerSuite struct {
	testing.BaseSuite

	appName string

	firewallerAPI *mocks.MockCAASFirewallerAPI
	lifeGetter    *mocks.MockLifeGetter
	logger        *mocks.MockLogger
	broker        *mocks.MockCAASBroker
	brokerApp     *caasmocks.MockApplication

	applicationChanges chan struct{}
	portsChanges       chan []string

	appsWatcher  watcher.NotifyWatcher
	portsWatcher watcher.StringsWatcher
}

var _ = gc.Suite(&appWorkerSuite{})

func (s *appWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.appName = "app1"
	s.applicationChanges = make(chan struct{})
	s.portsChanges = make(chan []string)
}

func (s *appWorkerSuite) getController(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.appsWatcher = watchertest.NewMockNotifyWatcher(s.applicationChanges)
	s.portsWatcher = watchertest.NewMockStringsWatcher(s.portsChanges)

	s.firewallerAPI = mocks.NewMockCAASFirewallerAPI(ctrl)

	s.lifeGetter = mocks.NewMockLifeGetter(ctrl)
	s.logger = mocks.NewMockLogger(ctrl)
	s.broker = mocks.NewMockCAASBroker(ctrl)
	s.brokerApp = caasmocks.NewMockApplication(ctrl)

	return ctrl
}

func (s *appWorkerSuite) getWorker(c *gc.C) worker.Worker {
	w, err := caasfirewallerembedded.NewApplicationWorker(
		testing.ControllerTag.Id(),
		testing.ModelTag.Id(),
		s.appName,
		s.firewallerAPI,
		s.broker,
		s.lifeGetter,
		s.logger,
	)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *appWorkerSuite) TestWorker(c *gc.C) {
	ctrl := s.getController(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	appCharmURL := &charm.URL{
		Schema:   "cs",
		Name:     "test",
		Revision: -1,
	}
	appCharmInfo := &charmscommon.CharmInfo{
		Meta: &charm.Meta{
			Name: "test",
			Deployment: &charm.Deployment{
				DeploymentMode: charm.ModeEmbedded,
				DeploymentType: charm.DeploymentStateful,
			},
		},
	}

	go func() {
		s.portsChanges <- []string{"port changes"}

		s.applicationChanges <- struct{}{}
	}()

	gomock.InOrder(
		s.firewallerAPI.EXPECT().WatchApplication(s.appName).Return(s.appsWatcher, nil),
		s.firewallerAPI.EXPECT().WatchOpenedPorts().Return(s.portsWatcher, nil),
		s.firewallerAPI.EXPECT().ApplicationCharmURL(s.appName).Return(appCharmURL, nil),
		s.firewallerAPI.EXPECT().CharmInfo("cs:test").Return(appCharmInfo, nil),

		s.broker.EXPECT().Application(s.appName, caas.DeploymentStateful).Return(s.brokerApp),

		s.firewallerAPI.EXPECT().IsExposed(s.appName).DoAndReturn(func(_ string) (bool, error) {
			close(done)
			return false, nil
		}),
	)

	w := s.getWorker(c)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
	workertest.CleanKill(c, w)
}
