// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasfirewaller"
	"github.com/juju/juju/internal/worker/caasfirewaller/mocks"
)

type appWorkerSuite struct {
	testing.BaseSuite

	appName string
	appUUID coreapplication.ID

	portService        *mocks.MockPortService
	applicationService *mocks.MockApplicationService
	broker             *mocks.MockCAASBroker
	brokerApp          *caasmocks.MockApplication

	applicationChanges chan struct{}
	portsChanges       chan struct{}

	appsWatcher  watcher.NotifyWatcher
	portsWatcher watcher.NotifyWatcher
}

func TestAppWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &appWorkerSuite{})
}

func (s *appWorkerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.appName = "app1"
	s.appUUID = applicationtesting.GenApplicationUUID(c)
	s.applicationChanges = make(chan struct{})
	s.portsChanges = make(chan struct{})
}

func (s *appWorkerSuite) getController(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.appsWatcher = watchertest.NewMockNotifyWatcher(s.applicationChanges)
	s.portsWatcher = watchertest.NewMockNotifyWatcher(s.portsChanges)

	s.portService = mocks.NewMockPortService(ctrl)
	s.applicationService = mocks.NewMockApplicationService(ctrl)

	s.broker = mocks.NewMockCAASBroker(ctrl)
	s.brokerApp = caasmocks.NewMockApplication(ctrl)

	c.Cleanup(func() {
		s.appsWatcher = nil
		s.portsWatcher = nil
		s.portService = nil
		s.applicationService = nil
		s.broker = nil
		s.brokerApp = nil
	})

	return ctrl
}

func (s *appWorkerSuite) getWorker(c *tc.C) worker.Worker {
	w, err := caasfirewaller.NewApplicationWorker(
		testing.ControllerTag.Id(),
		testing.ModelTag.Id(),
		s.appUUID,
		s.portService,
		s.applicationService,
		s.broker,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *appWorkerSuite) TestWorker(c *tc.C) {
	ctrl := s.getController(c)
	defer ctrl.Finish()

	done := make(chan struct{})

	go func() {
		// 1st port change event.
		s.portsChanges <- struct{}{}
		// 2nd port change event.
		s.portsChanges <- struct{}{}
		// 3rd port change event, including another application.
		s.portsChanges <- struct{}{}

		s.applicationChanges <- struct{}{}
	}()

	gpr1 := network.GroupedPortRanges{
		"": []network.PortRange{
			network.MustParsePortRange("1000/tcp"),
		},
	}

	gpr2 := network.GroupedPortRanges{
		"": []network.PortRange{
			network.MustParsePortRange("1000/tcp"),
		},
		"monitoring-port": []network.PortRange{
			network.MustParsePortRange("2000/udp"),
		},
	}

	gomock.InOrder(
		s.applicationService.EXPECT().GetApplicationName(gomock.Any(), s.appUUID).Return(s.appName, nil),
		s.applicationService.EXPECT().WatchApplicationExposed(gomock.Any(), s.appName).Return(s.appsWatcher, nil),
		s.portService.EXPECT().WatchOpenedPortsForApplication(gomock.Any(), s.appUUID).Return(s.portsWatcher, nil),
		s.broker.EXPECT().Application(s.appName, caas.DeploymentStateful).Return(s.brokerApp),

		// initial fetch.
		s.portService.EXPECT().GetApplicationOpenedPortsByEndpoint(gomock.Any(), s.appUUID).Return(network.GroupedPortRanges{}, nil),

		// 1st triggered by port change event.
		s.portService.EXPECT().GetApplicationOpenedPortsByEndpoint(gomock.Any(), s.appUUID).Return(gpr1, nil),
		s.brokerApp.EXPECT().UpdatePorts([]caas.ServicePort{
			{
				Name:       "1000-tcp",
				Port:       1000,
				TargetPort: 1000,
				Protocol:   "tcp",
			},
		}, false).Return(nil),

		// 2nd triggered by port change event, no UpdatePorts because no diff on the portchanges.
		s.portService.EXPECT().GetApplicationOpenedPortsByEndpoint(gomock.Any(), s.appUUID).Return(gpr1, nil),

		// 3rd triggered by port change event.
		s.portService.EXPECT().GetApplicationOpenedPortsByEndpoint(gomock.Any(), s.appUUID).Return(gpr2, nil),
		s.brokerApp.EXPECT().UpdatePorts([]caas.ServicePort{
			{
				Name:       "1000-tcp",
				Port:       1000,
				TargetPort: 1000,
				Protocol:   "tcp",
			},
			{
				Name:       "2000-udp",
				Port:       2000,
				TargetPort: 2000,
				Protocol:   "udp",
			},
		}, false).Return(nil),

		s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), s.appName).DoAndReturn(func(_ context.Context, _ string) (bool, error) {
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
