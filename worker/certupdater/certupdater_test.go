// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater_test

import (
	"net"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/pki"
	pkitest "github.com/juju/juju/pki/test"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/certupdater"
	"github.com/juju/juju/worker/certupdater/mocks"
)

type CertUpdaterSuite struct {
	coretesting.BaseSuite
	stateServingInfo  jujucontroller.StateServingInfo
	watchableDBGetter *mocks.MockWatchableDBGetter
	ctrlConfigService *mocks.MockControllerConfigService
}

var _ = gc.Suite(&CertUpdaterSuite{})

func (s *CertUpdaterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.watchableDBGetter = mocks.NewMockWatchableDBGetter(ctrl)
	s.ctrlConfigService = mocks.NewMockControllerConfigService(ctrl)

	s.stateServingInfo = jujucontroller.StateServingInfo{
		Cert:         coretesting.ServerCert,
		PrivateKey:   coretesting.ServerKey,
		CAPrivateKey: coretesting.CAKey,
		StatePort:    123,
		APIPort:      456,
	}
}

type mockNotifyWatcher struct {
	changes <-chan struct{}
}

func (w *mockNotifyWatcher) Changes() watcher.NotifyChannel {
	return w.changes
}

func (*mockNotifyWatcher) Stop() error {
	return nil
}

func (*mockNotifyWatcher) Kill() {}

func (*mockNotifyWatcher) Wait() error {
	return nil
}

func (*mockNotifyWatcher) Err() error {
	return nil
}

func newMockNotifyWatcher(changes <-chan struct{}) watcher.NotifyWatcher {
	return &mockNotifyWatcher{changes}
}

type mockMachine struct {
	changes chan struct{}
}

func (m *mockMachine) WatchAddresses() watcher.NotifyWatcher {
	return newMockNotifyWatcher(m.changes)
}

func (m *mockMachine) Addresses() (addresses network.SpaceAddresses) {
	return network.NewSpaceAddresses("0.1.2.3")
}

func (s *CertUpdaterSuite) StateServingInfo() (jujucontroller.StateServingInfo, bool) {
	return s.stateServingInfo, true
}

type mockAPIHostGetter struct{}

func (g *mockAPIHostGetter) APIHostPortsForClients() ([]network.SpaceHostPorts, error) {
	return []network.SpaceHostPorts{{
		{SpaceAddress: network.NewSpaceAddress("192.168.1.1", network.WithScope(network.ScopeCloudLocal)), NetPort: 17070},
		{SpaceAddress: network.NewSpaceAddress("10.1.1.1", network.WithScope(network.ScopeMachineLocal)), NetPort: 17070},
	}}, nil
}

func (s *CertUpdaterSuite) TestStartStop(c *gc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

	s.ctrlConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(jujucontroller.Config{}, nil)

	changes := make(chan struct{})
	worker, err := certupdater.NewCertificateUpdater(certupdater.Config{
		AddressWatcher:     &mockMachine{changes},
		APIHostPortsGetter: &mockAPIHostGetter{},
		Authority:          authority,
		CtrlConfigService:  s.ctrlConfigService,
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, worker)

	leaf, err := authority.LeafForGroup(pki.ControllerIPLeafGroup)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leaf.Certificate().IPAddresses, coretesting.IPsEqual,
		[]net.IP{net.ParseIP("192.168.1.1")})
}

func (s *CertUpdaterSuite) TestAddressChange(c *gc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

	s.ctrlConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(jujucontroller.Config{}, nil)

	changes := make(chan struct{})
	worker, err := certupdater.NewCertificateUpdater(certupdater.Config{
		AddressWatcher:     &mockMachine{changes},
		APIHostPortsGetter: &mockAPIHostGetter{},
		Authority:          authority,
		CtrlConfigService:  s.ctrlConfigService,
	})
	c.Assert(err, jc.ErrorIsNil)

	changes <- struct{}{}
	// Certificate should be updated with the address value.

	workertest.CleanKill(c, worker)
	leaf, err := authority.LeafForGroup(pki.ControllerIPLeafGroup)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leaf.Certificate().IPAddresses, coretesting.IPsEqual,
		[]net.IP{net.ParseIP("0.1.2.3")})
}
