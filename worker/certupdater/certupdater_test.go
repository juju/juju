// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater_test

import (
	"context"
	"net"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/pki"
	pkitest "github.com/juju/juju/pki/test"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/certupdater"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type CertUpdaterSuite struct {
	jujutesting.BaseSuite
	stateServingInfo controller.StateServingInfo
}

var _ = gc.Suite(&CertUpdaterSuite{})

func (s *CertUpdaterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stateServingInfo = controller.StateServingInfo{
		Cert:         jujutesting.ServerCert,
		PrivateKey:   jujutesting.ServerKey,
		CAPrivateKey: jujutesting.CAKey,
		StatePort:    123,
		APIPort:      456,
	}
}

func (s *CertUpdaterSuite) TestStartStop(c *gc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

	changes := make(chan struct{})
	worker, err := certupdater.NewCertificateUpdater(certupdater.Config{
		AddressWatcher:         &mockMachine{changes: changes},
		APIHostPortsGetter:     &mockAPIHostGetter{},
		Authority:              authority,
		ControllerConfigGetter: &mockControllerConfigGetter{},
		Logger:                 jujutesting.NewCheckLogger(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, worker)

	leaf, err := authority.LeafForGroup(pki.ControllerIPLeafGroup)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leaf.Certificate().IPAddresses, jujutesting.IPsEqual,
		[]net.IP{net.ParseIP("192.168.1.1")})
}

func (s *CertUpdaterSuite) TestAddressChange(c *gc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

	changes := make(chan struct{})
	worker, err := certupdater.NewCertificateUpdater(certupdater.Config{
		AddressWatcher:         &mockMachine{changes: changes},
		APIHostPortsGetter:     &mockAPIHostGetter{},
		Authority:              authority,
		ControllerConfigGetter: &mockControllerConfigGetter{},
		Logger:                 jujutesting.NewCheckLogger(c),
	})
	c.Assert(err, jc.ErrorIsNil)

	changes <- struct{}{}
	// Certificate should be updated with the address value.

	workertest.CleanKill(c, worker)
	leaf, err := authority.LeafForGroup(pki.ControllerIPLeafGroup)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leaf.Certificate().IPAddresses, jujutesting.IPsEqual,
		[]net.IP{net.ParseIP("0.1.2.3")})
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

func (s *CertUpdaterSuite) StateServingInfo() (controller.StateServingInfo, bool) {
	return s.stateServingInfo, true
}

type mockAPIHostGetter struct{}

func (g *mockAPIHostGetter) APIHostPortsForClients(controller.Config) ([]network.SpaceHostPorts, error) {
	return []network.SpaceHostPorts{{
		{SpaceAddress: network.NewSpaceAddress("192.168.1.1", network.WithScope(network.ScopeCloudLocal)), NetPort: 17070},
		{SpaceAddress: network.NewSpaceAddress("10.1.1.1", network.WithScope(network.ScopeMachineLocal)), NetPort: 17070},
	}}, nil
}

type mockControllerConfigGetter struct{}

func (*mockControllerConfigGetter) ControllerConfig(context.Context) (controller.Config, error) {
	return jujutesting.FakeControllerConfig(), nil
}
