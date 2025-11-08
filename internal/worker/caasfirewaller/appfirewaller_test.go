// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/caas"
	caasmocks "github.com/juju/juju/caas/mocks"
	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	domainapplicationerrors "github.com/juju/juju/domain/application/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/caasfirewaller"
	"github.com/juju/juju/internal/worker/caasfirewaller/mocks"
)

type appFirewallerSuite struct {
	portService        *mocks.MockPortService
	applicationService *mocks.MockApplicationService
	broker             *mocks.MockCAASBroker
	brokerApp          *caasmocks.MockApplication
}

func TestAppFirewallerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &appFirewallerSuite{})
}

// setupMocks is responsible for creating a testing gomock controller and
// establishing the mocks used by the suite.
func (s *appFirewallerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.portService = mocks.NewMockPortService(ctrl)
	s.applicationService = mocks.NewMockApplicationService(ctrl)
	s.broker = mocks.NewMockCAASBroker(ctrl)
	s.brokerApp = caasmocks.NewMockApplication(ctrl)

	c.Cleanup(func() {
		s.portService = nil
		s.applicationService = nil
		s.broker = nil
		s.brokerApp = nil
	})

	return ctrl
}

// makeWorker is a test suite helper to construct a new application worker off
// of the suite's mocks. [appWorkerSuite.setupMocks] must have been called by
// the test first.
func (s *appFirewallerSuite) makeWorker(
	c *tc.C, appUUID coreapplication.UUID,
) worker.Worker {
	w, err := caasfirewaller.NewAppFirewaller(
		appUUID,
		caasfirewaller.AppFirewallerConfig{
			ApplicationService: s.applicationService,
			Broker:             s.broker,
			PortService:        s.portService,
			Logger:             loggertesting.WrapCheckLog(c),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

// TestWorkerCleanShutdownOnApplicationRemoval ensures that when an application
// is removed the worker cleanly shuts down without error. This is a regression
// test after moving to Dqlite. The wrong error type was being check and the
// worker would not exit cleanly.
func (s *appFirewallerSuite) TestWorkerCleanShutdownOnApplicationRemoval(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "mysql"
	appUUID := tc.Must(c, coreapplication.NewUUID)

	// Create the ports watcher buffered with all of the events. A buffered
	// channel avoids having to use goroutines to send the events.
	portsChangeCh := make(chan struct{}, 2)
	portsWatcher := watchertest.NewMockNotifyWatcher(portsChangeCh)
	// 1st port change event.
	portsChangeCh <- struct{}{}
	// 2nd port change event but the application has been removed.
	portsChangeCh <- struct{}{}

	portChange1 := network.GroupedPortRanges{
		"": []network.PortRange{
			network.MustParsePortRange("1000/tcp"),
		},
	}

	appSvcEXP := s.applicationService.EXPECT()
	appSvcEXP.GetApplicationName(gomock.Any(), appUUID).Return(appName, nil).AnyTimes()

	portSvcExp := s.portService.EXPECT()
	portSvcExp.WatchOpenedPortsForApplication(gomock.Any(), appUUID).Return(
		portsWatcher, nil,
	)
	// Initial fetch on worker setup
	portSvcExp.GetApplicationOpenedPortsByEndpoint(
		gomock.Any(), appUUID,
	).Return(network.GroupedPortRanges{}, nil)

	// 1st change event for port change.
	portSvcExp.GetApplicationOpenedPortsByEndpoint(
		gomock.Any(), appUUID,
	).Return(portChange1, nil)

	// 2nd change event for port change, application not found as it has been
	// removed.
	portSvcExp.GetApplicationOpenedPortsByEndpoint(
		gomock.Any(), appUUID,
	).Return(nil, domainapplicationerrors.ApplicationNotFound)

	brokerExp := s.broker.EXPECT()
	brokerExp.Application(appName, caas.DeploymentStateful).Return(
		s.brokerApp).AnyTimes()

	brokerAppExp := s.brokerApp.EXPECT()
	// 1st change event port update
	brokerAppExp.UpdatePorts([]caas.ServicePort{
		{
			Name:       "1000-tcp",
			Port:       1000,
			TargetPort: 1000,
			Protocol:   "tcp",
		},
	}, false).Return(nil)

	w := s.makeWorker(c, appUUID)
	c.Check(
		w.Wait(),
		tc.ErrorIsNil,
		tc.Commentf("expected clean worker shutdown on application removal"),
	)
}

// TestWorkerPropogatesBrokerNotFoundError is a regression test for the
// [applicationWorker] to make sure that when the broker returns a
// [coreerrors.NotFound] it is propogated through the worker instead of being
// discarded.
func (s *appFirewallerSuite) TestWorkerPropogatesBrokerNotFoundError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appName := "mysql"
	appUUID := tc.Must(c, coreapplication.NewUUID)

	// Create the ports watcher buffered with all of the events. A buffered
	// channel avoids having to use goroutines to send the events.
	portsChangeCh := make(chan struct{}, 1)
	portsWatcher := watchertest.NewMockNotifyWatcher(portsChangeCh)
	// 1st port change event.
	portsChangeCh <- struct{}{}

	portChange1 := network.GroupedPortRanges{
		"": []network.PortRange{
			network.MustParsePortRange("1000/tcp"),
		},
	}

	appSvcEXP := s.applicationService.EXPECT()
	appSvcEXP.GetApplicationName(gomock.Any(), appUUID).Return(appName, nil).AnyTimes()

	portSvcExp := s.portService.EXPECT()
	portSvcExp.WatchOpenedPortsForApplication(gomock.Any(), appUUID).Return(
		portsWatcher, nil,
	)
	// Initial fetch on worker setup
	portSvcExp.GetApplicationOpenedPortsByEndpoint(
		gomock.Any(), appUUID,
	).Return(network.GroupedPortRanges{}, nil)

	// 1st change event for port change.
	portSvcExp.GetApplicationOpenedPortsByEndpoint(
		gomock.Any(), appUUID,
	).Return(portChange1, nil)

	brokerExp := s.broker.EXPECT()
	brokerExp.Application(appName, caas.DeploymentStateful).Return(
		s.brokerApp).AnyTimes()

	brokerAppExp := s.brokerApp.EXPECT()
	// 1st change event port update
	brokerAppExp.UpdatePorts([]caas.ServicePort{
		{
			Name:       "1000-tcp",
			Port:       1000,
			TargetPort: 1000,
			Protocol:   "tcp",
		},
	}, false).Return(coreerrors.NotFound) // NotFound error that cannot be ignored.

	w := s.makeWorker(c, appUUID)
	err := w.Wait()
	c.Check(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *appFirewallerSuite) TestWorker(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appName := "mysql"
	appUUID := tc.Must(c, coreapplication.NewUUID)

	// Create the ports watcher buffered with all of the events. A buffered
	// channel avoids having to use goroutines to send the events.
	portsChangeCh := make(chan struct{}, 3)
	portsWatcher := watchertest.NewMockNotifyWatcher(portsChangeCh)
	// 1st port change event.
	portsChangeCh <- struct{}{}
	// 2nd port change event.
	portsChangeCh <- struct{}{}
	// 3nd port change event.
	portsChangeCh <- struct{}{}

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

	appSvcExp := s.applicationService.EXPECT()
	appSvcExp.GetApplicationName(gomock.Any(), appUUID).Return(
		appName, nil).AnyTimes()

	portSvcExp := s.portService.EXPECT()
	portSvcExp.WatchOpenedPortsForApplication(gomock.Any(), appUUID).Return(
		portsWatcher, nil,
	)

	// initial fetch.
	portSvcExp.GetApplicationOpenedPortsByEndpoint(
		gomock.Any(), appUUID,
	).Return(network.GroupedPortRanges{}, nil)

	// 1st watcher change
	portSvcExp.GetApplicationOpenedPortsByEndpoint(
		gomock.Any(), appUUID,
	).Return(gpr1, nil)

	// 2nd watcher change, no change to ports
	portSvcExp.GetApplicationOpenedPortsByEndpoint(
		gomock.Any(), appUUID,
	).Return(gpr1, nil)

	// 3rd watcher change
	portSvcExp.GetApplicationOpenedPortsByEndpoint(
		gomock.Any(), appUUID,
	).Return(gpr2, nil)

	brokerExp := s.broker.EXPECT()
	brokerExp.Application(appName, caas.DeploymentStateful).Return(s.brokerApp)

	brokerAppExp := s.brokerApp.EXPECT()
	// 1st watcher change
	brokerAppExp.UpdatePorts([]caas.ServicePort{
		{
			Name:       "1000-tcp",
			Port:       1000,
			TargetPort: 1000,
			Protocol:   "tcp",
		},
	}, false).Return(nil)

	// Create the worker var to capture the worker in the mock closure.
	var w worker.Worker

	// 3rd watcher change
	brokerAppExp.UpdatePorts([]caas.ServicePort{
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
	}, false).DoAndReturn(func([]caas.ServicePort, bool) error {
		// Last mock expect so we can kill the worker.
		w.Kill()
		return nil
	})

	w = s.makeWorker(c, appUUID)
	c.Check(
		w.Wait(),
		tc.ErrorIsNil,
		tc.Commentf("expected clean worker shutdown on application removal"),
	)
}
