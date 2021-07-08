// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallersidecar_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/errors"
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
	"github.com/juju/juju/worker/caasfirewallersidecar"
	"github.com/juju/juju/worker/caasfirewallersidecar/mocks"
)

type appWorkerSuite struct {
	testing.BaseSuite

	appName string

	firewallerAPI *mocks.MockCAASFirewallerAPI
	lifeGetter    *mocks.MockLifeGetter
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
	s.broker = mocks.NewMockCAASBroker(ctrl)
	s.brokerApp = caasmocks.NewMockApplication(ctrl)

	return ctrl
}

func (s *appWorkerSuite) getWorker(c *gc.C) worker.Worker {
	w, err := caasfirewallersidecar.NewApplicationWorker(
		testing.ControllerTag.Id(),
		testing.ModelTag.Id(),
		s.appName,
		s.firewallerAPI,
		s.broker,
		s.lifeGetter,
		noopLogger{},
	)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *appWorkerSuite) TestWorker(c *gc.C) {
	ctrl := s.getController(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	charmInfo := &charmscommon.CharmInfo{ // v2 charm
		Meta: &charm.Meta{
			Name: "test",

			Containers: map[string]charm.Container{
				"test": {
					Resource: "test-oci",
				},
			},
			Resources: map[string]charmresource.Meta{
				"test-oci": {
					Type: charmresource.TypeContainerImage,
				},
			},
		},
		Manifest: &charm.Manifest{
			Bases: []charm.Base{{
				Name: "ubuntu",
				Channel: charm.Channel{
					Track: "20.04",
					Risk:  "stable",
				},
			}},
		},
	}

	go func() {
		s.portsChanges <- []string{"port changes"}
		s.applicationChanges <- struct{}{}
	}()

	gomock.InOrder(
		s.firewallerAPI.EXPECT().WatchApplication(s.appName).Return(s.appsWatcher, nil),
		s.firewallerAPI.EXPECT().WatchOpenedPorts().Return(s.portsWatcher, nil),
		s.broker.EXPECT().Application(s.appName, caas.DeploymentStateful).Return(s.brokerApp),
		s.firewallerAPI.EXPECT().ApplicationCharmInfo(s.appName).Return(charmInfo, nil),
		s.firewallerAPI.EXPECT().IsExposed(s.appName).DoAndReturn(func(_ string) (bool, error) {
			close(done)
			return false, nil
		}),
	)

	w := s.getWorker(c)
	s.waitDone(c, done)
	workertest.CleanKill(c, w)
}

func (s *appWorkerSuite) waitDone(c *gc.C, done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *appWorkerSuite) TestV1CharmStopsWorker(c *gc.C) {
	ctrl := s.getController(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	go func() {
		s.applicationChanges <- struct{}{}
	}()

	gomock.InOrder(
		s.firewallerAPI.EXPECT().WatchApplication(s.appName).Return(s.appsWatcher, nil),
		s.firewallerAPI.EXPECT().WatchOpenedPorts().Return(s.portsWatcher, nil),
		s.broker.EXPECT().Application(s.appName, caas.DeploymentStateful).Return(s.brokerApp),
		s.firewallerAPI.EXPECT().ApplicationCharmInfo(s.appName).DoAndReturn(func(_ string) (*charmscommon.CharmInfo, error) {
			close(done)
			charmInfo := &charmscommon.CharmInfo{ // v1 charm
				Meta:     &charm.Meta{},
				Manifest: &charm.Manifest{},
			}
			return charmInfo, nil
		}),
	)

	w := s.getWorker(c)
	s.waitDone(c, done)
	workertest.CleanKill(c, w)
}

func (s *appWorkerSuite) TestNotFoundCharmStopsWorker(c *gc.C) {
	ctrl := s.getController(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	go func() {
		s.applicationChanges <- struct{}{}
	}()

	gomock.InOrder(
		s.firewallerAPI.EXPECT().WatchApplication(s.appName).Return(s.appsWatcher, nil),
		s.firewallerAPI.EXPECT().WatchOpenedPorts().Return(s.portsWatcher, nil),
		s.broker.EXPECT().Application(s.appName, caas.DeploymentStateful).Return(s.brokerApp),
		s.firewallerAPI.EXPECT().ApplicationCharmInfo(s.appName).DoAndReturn(func(appName string) (*charmscommon.CharmInfo, error) {
			close(done)
			return nil, errors.NotFoundf("charm %q", appName)
		}),
	)

	w := s.getWorker(c)
	s.waitDone(c, done)
	workertest.CleanKill(c, w)
}
