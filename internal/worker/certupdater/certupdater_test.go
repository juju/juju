// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"context"
	"net"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/testhelpers"
)

type certUpdaterSuite struct {
	testhelpers.IsolationSuite

	controllerNodeService *MockControllerNodeService
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
	s.controllerNodeService.EXPECT().WatchControllerClientAddresses(gomock.Any()).DoAndReturn(
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

	<-notifyInitialConfigConsumed
	workertest.CleanKill(c, w)
}

func (s *certUpdaterSuite) TestSetUpWaitsForInitialEvent(c *tc.C) {
	defer s.setupMocks(c).Finish()
	nodeWatcher := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	defer workertest.CleanKill(c, nodeWatcher)
	s.controllerNodeService.EXPECT().WatchControllerClientAddresses(gomock.Any()).Return(nodeWatcher, nil)
	updater := CertificateUpdater{controllerNodeService: s.controllerNodeService}
	w, err := updater.SetUp(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.Equals, nodeWatcher)
}

func (s *certUpdaterSuite) TestInitialAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	watcherChannel := make(chan struct{})
	nodeWatcher := watchertest.NewMockNotifyWatcher(watcherChannel)
	s.controllerNodeService.EXPECT().WatchControllerClientAddresses(gomock.Any()).Return(nodeWatcher, nil)
	s.controllerNodeService.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return([]string{"3.4.5.6"}, nil)

	s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest)
	s.leafRequest.EXPECT().AddIPAddresses(net.ParseIP("3.4.5.6"))
	committed := make(chan struct{})
	s.leafRequest.EXPECT().Commit().DoAndReturn(func() (pki.Leaf, error) {
		close(committed)
		return nil, nil
	})

	w := s.newUpdater(c)
	defer workertest.DirtyKill(c, w)

	watcherChannel <- struct{}{}
	<-committed
	workertest.CleanKill(c, w)
}

func (s *certUpdaterSuite) TestInitialAddressAsHostname(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	watcherChannel := make(chan struct{})
	nodeWatcher := watchertest.NewMockNotifyWatcher(watcherChannel)
	s.controllerNodeService.EXPECT().WatchControllerClientAddresses(gomock.Any()).Return(nodeWatcher, nil)
	s.controllerNodeService.EXPECT().GetAllCloudLocalAPIAddresses(gomock.Any()).Return([]string{"testhost"}, nil)

	s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest)
	s.leafRequest.EXPECT().AddDNSNames("testhost")
	committed := make(chan struct{})
	s.leafRequest.EXPECT().Commit().DoAndReturn(func() (pki.Leaf, error) {
		close(committed)
		return nil, nil
	})

	w := s.newUpdater(c)
	defer workertest.DirtyKill(c, w)

	watcherChannel <- struct{}{}
	<-committed
	workertest.CleanKill(c, w)
}

func (s *certUpdaterSuite) TestAddressChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	watcherChannel := make(chan struct{})
	nodeWatcher := watchertest.NewMockNotifyWatcher(watcherChannel)
	s.controllerNodeService.EXPECT().WatchControllerClientAddresses(gomock.Any()).Return(nodeWatcher, nil)

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

	// Initial event followed by a change. Both reads follow a notification.
	watcherChannel <- struct{}{}
	watcherChannel <- struct{}{}

	// Assert: Wait for the worker to process the event.
	<-sync

	workertest.CleanKill(c, w)
}

func (s *certUpdaterSuite) newUpdater(c *tc.C) worker.Worker {
	w, err := NewCertificateUpdater(Config{
		Authority:             s.authority,
		ControllerNodeService: s.controllerNodeService,
		Logger:                loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *certUpdaterSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authority = NewMockAuthority(ctrl)
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.leafRequest = NewMockLeafRequest(ctrl)

	c.Cleanup(func() {
		s.authority = nil
		s.controllerNodeService = nil
		s.leafRequest = nil
	})

	return ctrl
}
