// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"net"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	coreerrors "github.com/juju/juju/core/errors"
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

	s.expectWatchers()
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForCertificates(gomock.Any()).Return([]string{}, nil)
	s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest)
	committed := make(chan struct{})
	s.leafRequest.EXPECT().Commit().DoAndReturn(func() (pki.Leaf, error) {
		close(committed)
		return nil, nil
	})
	w := s.newUpdater(c)
	defer workertest.DirtyKill(c, w)

	<-committed
	workertest.CleanKill(c, w)
}

func (s *certUpdaterSuite) TestSetUpWatchesCertificateAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	addressChanges := make(chan struct{})
	addressWatcher := watchertest.NewMockNotifyWatcher(addressChanges)
	s.controllerNodeService.EXPECT().WatchControllerAddressesForCertificates(gomock.Any()).Return(addressWatcher, nil)
	updater := CertificateUpdater{controllerNodeService: s.controllerNodeService}
	w, err := updater.SetUp(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.Equals, addressWatcher)
	workertest.CleanKill(c, w)
}

func (s *certUpdaterSuite) TestInitialAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	watcherChannel := make(chan struct{}, 2)
	s.expectWatcherWithChanges(watcherChannel)
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForCertificates(gomock.Any()).Return([]string{"3.4.5.6", "2001:db8::1"}, nil)

	s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest)
	s.leafRequest.EXPECT().AddIPAddresses(net.ParseIP("3.4.5.6"))
	s.leafRequest.EXPECT().AddIPAddresses(net.ParseIP("2001:db8::1"))
	committed := make(chan struct{})
	s.leafRequest.EXPECT().Commit().DoAndReturn(func() (pki.Leaf, error) {
		close(committed)
		return nil, nil
	})

	w := s.newUpdater(c)
	defer workertest.DirtyKill(c, w)

	<-committed
	workertest.CleanKill(c, w)
}

func (s *certUpdaterSuite) TestInitialAddressAsHostname(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	watcherChannel := make(chan struct{}, 2)
	s.expectWatcherWithChanges(watcherChannel)
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForCertificates(gomock.Any()).Return([]string{"testhost"}, nil)

	s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest)
	s.leafRequest.EXPECT().AddDNSNames("testhost")
	committed := make(chan struct{})
	s.leafRequest.EXPECT().Commit().DoAndReturn(func() (pki.Leaf, error) {
		close(committed)
		return nil, nil
	})

	w := s.newUpdater(c)
	defer workertest.DirtyKill(c, w)

	<-committed
	workertest.CleanKill(c, w)
}

func (s *certUpdaterSuite) TestAddressChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	watcherChannel := make(chan struct{}, 2)
	s.expectWatcherWithChanges(watcherChannel)

	// initial addresses
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForCertificates(gomock.Any()).Return([]string{"3.4.5.6"}, nil)
	s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest)
	s.leafRequest.EXPECT().AddIPAddresses(net.ParseIP("3.4.5.6"))
	s.leafRequest.EXPECT().Commit().Return(nil, nil)

	// new address
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForCertificates(gomock.Any()).Return([]string{"0.1.2.3"}, nil)
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

	// The initial event is sent by the merged watcher; this is the later change.
	watcherChannel <- struct{}{}

	// Assert: Wait for the worker to process the event.
	<-sync

	workertest.CleanKill(c, w)
}

func (s *certUpdaterSuite) TestEmptyAddressSelectionReplacesCertificate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	watcherChannel := make(chan struct{}, 2)
	s.expectWatcherWithChanges(watcherChannel)
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForCertificates(gomock.Any()).Return([]string{"3.4.5.6"}, nil)
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForCertificates(gomock.Any()).Return([]string{}, nil)

	s.authority.EXPECT().LeafRequestForGroup(pki.ControllerIPLeafGroup).Return(s.leafRequest).Times(2)
	s.leafRequest.EXPECT().AddIPAddresses(net.ParseIP("3.4.5.6"))
	firstCommit := make(chan struct{})
	s.leafRequest.EXPECT().Commit().DoAndReturn(func() (pki.Leaf, error) {
		close(firstCommit)
		return nil, nil
	})
	secondCommit := make(chan struct{})
	s.leafRequest.EXPECT().Commit().DoAndReturn(func() (pki.Leaf, error) {
		close(secondCommit)
		return nil, nil
	})

	w := s.newUpdater(c)
	defer workertest.DirtyKill(c, w)

	<-firstCommit
	watcherChannel <- struct{}{}
	<-secondCommit
	workertest.CleanKill(c, w)
}

func (s *certUpdaterSuite) TestNewCertificateUpdaterValidatesConfig(c *tc.C) {
	_, err := NewCertificateUpdater(Config{})
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
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

func (s *certUpdaterSuite) expectWatchers() {
	changes := make(chan struct{}, 1)
	changes <- struct{}{}
	s.controllerNodeService.EXPECT().WatchControllerAddressesForCertificates(gomock.Any()).Return(watchertest.NewMockNotifyWatcher(changes), nil)
}

func (s *certUpdaterSuite) expectWatcherWithChanges(changes chan struct{}) {
	changes <- struct{}{}
	s.controllerNodeService.EXPECT().WatchControllerAddressesForCertificates(gomock.Any()).Return(watchertest.NewMockNotifyWatcher(changes), nil)
}
