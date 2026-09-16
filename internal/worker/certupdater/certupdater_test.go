// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki"
	pkitest "github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/testhelpers"
)

type certUpdaterSuite struct {
	testhelpers.IsolationSuite

	controllerNodeService *MockControllerNodeService
	controllerNetwork     *MockControllerNetworkService
	authority             *MockAuthority
	leafRequest           *MockLeafRequest
}

func TestCertUpdaterSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &certUpdaterSuite{})
	})
}

func (s *certUpdaterSuite) TestWorkerCleanKill(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.controllerNodeService.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return([]string{}, nil).AnyTimes()

	// We use the consume of the initial event as a sync point to decide
	// whether the worker has started. This channel is then used to stop
	// waiting for the worker to start.
	notifyInitialConfigConsumed := make(chan struct{})
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (watcher.Watcher[struct{}], error) {
			ch := make(chan struct{})
			go func() {
				defer close(notifyInitialConfigConsumed)

				select {
				case ch <- struct{}{}:
				case <-c.Context().Done():
					return
				}
			}()
			return watchertest.NewMockNotifyWatcher(ch), nil
		})
	w := s.newUpdater(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-notifyInitialConfigConsumed:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for worker to start")
	}
	workertest.DirtyKill(c, w)
}

func (s *certUpdaterSuite) TestInitialAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	nodeWatcher := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(nodeWatcher, nil)
	s.controllerNodeService.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return([]string{"3.4.5.6"}, nil)

	s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest)
	s.leafRequest.EXPECT().AddIPAddresses(net.ParseIP("3.4.5.6"))
	s.leafRequest.EXPECT().Commit().Return(nil, nil)

	w := s.newUpdater(c)
	defer workertest.DirtyKill(c, w)

	workertest.DirtyKill(c, w)
}

func (s *certUpdaterSuite) TestInitialAddressAsHostname(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	nodeWatcher := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(nodeWatcher, nil)
	s.controllerNodeService.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return([]string{"testhost"}, nil)

	s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest)
	s.leafRequest.EXPECT().AddDNSNames("testhost")
	s.leafRequest.EXPECT().Commit().Return(nil, nil)

	w := s.newUpdater(c)
	defer workertest.DirtyKill(c, w)

	workertest.DirtyKill(c, w)
}

func (s *certUpdaterSuite) TestInitialAddressIncludesControllerPodFQDN(c *tc.C) {
	defer s.setupMocks(c).Finish()

	addressChanges := make(chan struct{}, 1)
	addressChanges <- struct{}{}
	addressWatcher := watchertest.NewMockNotifyWatcher(addressChanges)
	networkChanges := make(chan struct{}, 1)
	networkChanges <- struct{}{}
	networkWatcher := watchertest.NewMockNotifyWatcher(networkChanges)
	fqdn := "controller-0.controller-service-endpoints.controller.svc.cluster.local"
	s.controllerNodeService.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return([]string{"3.4.5.6"}, nil)
	s.controllerNetwork.EXPECT().GetControllerPodFQDNs(gomock.Any()).Return([]string{fqdn}, nil)
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(addressWatcher, nil)
	s.controllerNetwork.EXPECT().WatchControllerRemoteEndpoints(gomock.Any()).Return(networkWatcher, nil)

	s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest)
	s.leafRequest.EXPECT().AddIPAddresses(net.ParseIP("3.4.5.6"))
	s.leafRequest.EXPECT().AddDNSNames(fqdn)
	s.leafRequest.EXPECT().Commit().Return(nil, nil)

	w := s.newUpdaterWithNetwork(c)
	defer workertest.DirtyKill(c, w)

	workertest.DirtyKill(c, w)
}

func (s *certUpdaterSuite) TestControllerPodFQDNCertificateVerification(c *tc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, tc.ErrorIsNil)
	fqdn := "controller-0.controller-service-endpoints.controller.svc.cluster.local"
	updater := CertificateUpdater{authority: authority, logger: loggertesting.WrapCheckLog(c)}

	err = updater.updateCertificate(c.Context(), []string{fqdn})

	c.Assert(err, tc.ErrorIsNil)
	leaf, err := authority.LeafForGroup(pki.ControllerIPLeafGroup)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(leaf.Certificate().VerifyHostname(fqdn), tc.ErrorIsNil)
}

func (s *certUpdaterSuite) TestControllerPodFQDNChangeRenewsCertificate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	addressChanges := make(chan struct{}, 1)
	addressChanges <- struct{}{}
	addressWatcher := watchertest.NewMockNotifyWatcher(addressChanges)
	networkChanges := make(chan struct{}, 1)
	networkChanges <- struct{}{}
	networkWatcher := watchertest.NewMockNotifyWatcher(networkChanges)
	firstFQDN := "controller-0.controller-service-endpoints.controller.svc.cluster.local"
	secondFQDN := "controller-0.controller-service-endpoints.other.svc.cluster.local"
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(addressWatcher, nil)
	s.controllerNetwork.EXPECT().WatchControllerRemoteEndpoints(gomock.Any()).Return(networkWatcher, nil)

	renewed := make(chan struct{})
	gomock.InOrder(
		s.controllerNodeService.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return([]string{"3.4.5.6"}, nil),
		s.controllerNetwork.EXPECT().GetControllerPodFQDNs(gomock.Any()).Return([]string{firstFQDN}, nil),
		s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest),
		s.leafRequest.EXPECT().AddIPAddresses(net.ParseIP("3.4.5.6")),
		s.leafRequest.EXPECT().AddDNSNames(firstFQDN),
		s.leafRequest.EXPECT().Commit().Return(nil, nil),
		s.controllerNodeService.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return([]string{"3.4.5.6"}, nil),
		s.controllerNetwork.EXPECT().GetControllerPodFQDNs(gomock.Any()).Return([]string{secondFQDN}, nil),
		s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest),
		s.leafRequest.EXPECT().AddIPAddresses(net.ParseIP("3.4.5.6")),
		s.leafRequest.EXPECT().AddDNSNames(secondFQDN),
		s.leafRequest.EXPECT().Commit().DoAndReturn(func() (pki.Leaf, error) {
			close(renewed)
			return nil, nil
		}),
		s.controllerNodeService.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return([]string{"3.4.5.6"}, nil),
		s.controllerNetwork.EXPECT().GetControllerPodFQDNs(gomock.Any()).Return([]string{secondFQDN}, nil),
	)

	w := s.newUpdaterWithNetwork(c)
	defer workertest.DirtyKill(c, w)

	select {
	case networkChanges <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to send controller FQDN change")
	}
	select {
	case <-renewed:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for controller certificate renewal")
	}

	workertest.DirtyKill(c, w)
}

func (s *certUpdaterSuite) TestAddressChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	watcherChannel := make(chan struct{})
	nodeWatcher := watchertest.NewMockNotifyWatcher(watcherChannel)
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(nodeWatcher, nil)

	// initial addresses
	s.controllerNodeService.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return([]string{"3.4.5.6"}, nil)
	s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest)
	s.leafRequest.EXPECT().AddIPAddresses(net.ParseIP("3.4.5.6"))
	s.leafRequest.EXPECT().Commit().Return(nil, nil)

	// new address
	s.controllerNodeService.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return([]string{"0.1.2.3"}, nil)
	s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest)
	s.leafRequest.EXPECT().AddIPAddresses(net.ParseIP("0.1.2.3"))

	// Synchronization point to ensure the worker processes the event.
	sync := make(chan struct{})
	s.leafRequest.EXPECT().Commit().DoAndReturn(func() (pki.Leaf, error) {
		close(sync)
		return nil, nil
	})

	w := s.newUpdater(c)
	defer workertest.DirtyKill(c, w)

	// Act
	watcherChannel <- struct{}{}

	// Assert: Wait for the worker to process the event.
	select {
	case <-sync:
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("timed out waiting for leaf request commit")
	}

	workertest.DirtyKill(c, w)
}

func (s *certUpdaterSuite) newUpdater(c *tc.C) worker.Worker {
	s.controllerNetwork.EXPECT().GetControllerPodFQDNs(gomock.Any()).Return(nil, nil).AnyTimes()
	s.controllerNetwork.EXPECT().WatchControllerRemoteEndpoints(gomock.Any()).DoAndReturn(
		func(context.Context) (watcher.NotifyWatcher, error) {
			changes := make(chan struct{}, 1)
			changes <- struct{}{}
			return watchertest.NewMockNotifyWatcher(changes), nil
		}).AnyTimes()
	w, err := NewCertificateUpdater(Config{
		Authority:             s.authority,
		ControllerNodeService: s.controllerNodeService,
		ControllerNetwork:     s.controllerNetwork,
		Logger:                loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *certUpdaterSuite) newUpdaterWithNetwork(c *tc.C) worker.Worker {
	w, err := NewCertificateUpdater(Config{
		Authority:             s.authority,
		ControllerNodeService: s.controllerNodeService,
		ControllerNetwork:     s.controllerNetwork,
		Logger:                loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *certUpdaterSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authority = NewMockAuthority(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.controllerNetwork = NewMockControllerNetworkService(ctrl)
	s.leafRequest = NewMockLeafRequest(ctrl)

	c.Cleanup(func() {
		s.authority = nil
		s.controllerNodeService = nil
		s.controllerNetwork = nil
		s.leafRequest = nil
	})

	return ctrl
}
