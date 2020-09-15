// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallerembedded_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	// "github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	charmscommon "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	// "github.com/juju/juju/core/life"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasfirewallerembedded"
	"github.com/juju/juju/worker/caasfirewallerembedded/mocks"
)

type appWorkerSuite struct {
	testing.BaseSuite

	appName string
	w       worker.Worker

	firewallerAPI *mocks.MockCAASFirewallerAPI
	lifeGetter    *mocks.MockLifeGetter
	logger        *mocks.MockLogger
	broker        *mocks.MockCAASBroker
	brokerApp     *caasmocks.MockApplication

	applicationChanges chan struct{}
	portsChanges       chan []string

	appsWatcher  *mocks.MockNotifyWatcher
	portsWatcher *mocks.MockStringsWatcher
}

var _ = gc.Suite(&appWorkerSuite{})

func (s *appWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.appName = "app1"
	s.applicationChanges = make(chan struct{})
	s.portsChanges = make(chan []string)
}

func (s *appWorkerSuite) TearDownTest(c *gc.C) {
	s.appName = ""
	s.w = nil

	s.applicationChanges = nil
	s.portsChanges = nil

	s.appsWatcher = nil
	s.portsWatcher = nil

	s.firewallerAPI = nil
	s.lifeGetter = nil
	s.logger = nil
	s.broker = nil
	s.brokerApp = nil

	s.BaseSuite.TearDownTest(c)
}

func (s *appWorkerSuite) initWorker(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.appsWatcher = mocks.NewMockNotifyWatcher(ctrl)
	s.appsWatcher.EXPECT().Changes().AnyTimes().Return(s.applicationChanges)

	s.portsWatcher = mocks.NewMockStringsWatcher(ctrl)
	s.portsWatcher.EXPECT().Changes().AnyTimes().Return(s.portsChanges)

	s.firewallerAPI = mocks.NewMockCAASFirewallerAPI(ctrl)
	s.firewallerAPI.EXPECT().WatchApplication(s.appName).Return(s.appsWatcher, nil)
	s.firewallerAPI.EXPECT().WatchOpenedPorts().AnyTimes().Return(s.portsWatcher, nil)

	s.lifeGetter = mocks.NewMockLifeGetter(ctrl)
	s.logger = mocks.NewMockLogger(ctrl)
	s.broker = mocks.NewMockCAASBroker(ctrl)
	s.brokerApp = caasmocks.NewMockApplication(ctrl)
	s.broker.EXPECT().Application(s.appName, caas.DeploymentStateful).AnyTimes().Return(s.brokerApp)

	var err error
	s.w, err = caasfirewallerembedded.NewApplicationWorker(
		testing.ControllerTag.Id(),
		testing.ModelTag.Id(),
		s.appName,
		s.firewallerAPI,
		s.broker,
		s.lifeGetter,
		s.logger,
	)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl
}

func (s *appWorkerSuite) TestWorker(c *gc.C) {
	c.Assert(s.w, gc.IsNil)

	ctrl := s.initWorker(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	// Added app watcher to catacomb.
	s.appsWatcher.EXPECT().Wait().Return(nil)

	// Added ports watcher to catacomb.
	s.portsWatcher.EXPECT().Wait().Return(nil)

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
		s.firewallerAPI.EXPECT().ApplicationCharmURL(s.appName).Return(appCharmURL, nil),
		s.firewallerAPI.EXPECT().CharmInfo("cs:test").Return(appCharmInfo, nil),

		s.firewallerAPI.EXPECT().IsExposed(s.appName).DoAndReturn(func(_ string) (bool, error) {
			close(done)
			return false, nil
		}),
	)

	// s.appsWatcher.EXPECT().Kill()
	// // s.appsWatcher.EXPECT().Wait().Return(nil)

	// s.portsWatcher.EXPECT().Kill()
	// // s.portsWatcher.EXPECT().Wait().Return(nil)

	c.Assert(s.w, gc.NotNil)

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
	workertest.CleanKill(c, s.w)
}
