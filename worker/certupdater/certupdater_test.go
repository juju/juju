// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater_test

import (
	"net"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	pkitest "github.com/juju/juju/pki/test"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/certupdater"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type CertUpdaterSuite struct {
	coretesting.BaseSuite
	stateServingInfo jujucontroller.StateServingInfo
}

var _ = gc.Suite(&CertUpdaterSuite{})

func (s *CertUpdaterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

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

func (w *mockNotifyWatcher) Changes() <-chan struct{} {
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

func newMockNotifyWatcher(changes <-chan struct{}) state.NotifyWatcher {
	return &mockNotifyWatcher{changes}
}

type mockMachine struct {
	changes chan struct{}
}

func (m *mockMachine) WatchAddresses() state.NotifyWatcher {
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
		{SpaceAddress: network.NewScopedSpaceAddress("192.168.1.1", network.ScopeCloudLocal), NetPort: 17070},
		{SpaceAddress: network.NewScopedSpaceAddress("10.1.1.1", network.ScopeMachineLocal), NetPort: 17070},
	}}, nil
}

func (s *CertUpdaterSuite) TestStartStop(c *gc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

	changes := make(chan struct{})
	worker := certupdater.NewCertificateUpdater(certupdater.Config{
		AddressWatcher:     &mockMachine{changes},
		APIHostPortsGetter: &mockAPIHostGetter{},
		Authority:          authority,
	})
	workertest.CleanKill(c, worker)

	leaf, err := authority.LeafForGroup(certupdater.ControllerIPLeafGroup)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leaf.Certificate().IPAddresses, coretesting.IPsEqual,
		[]net.IP{net.ParseIP("192.168.1.1")})
}

func (s *CertUpdaterSuite) TestAddressChange(c *gc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

	changes := make(chan struct{})
	worker := certupdater.NewCertificateUpdater(certupdater.Config{
		AddressWatcher:     &mockMachine{changes},
		APIHostPortsGetter: &mockAPIHostGetter{},
		Authority:          authority,
	})

	changes <- struct{}{}
	// Certificate should be updated with the address value.

	workertest.CleanKill(c, worker)
	leaf, err := authority.LeafForGroup(certupdater.ControllerIPLeafGroup)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leaf.Certificate().IPAddresses, coretesting.IPsEqual,
		[]net.IP{net.ParseIP("0.1.2.3")})
}

type mockStateServingGetterNoCAKey struct{}

func (g *mockStateServingGetterNoCAKey) StateServingInfo() (jujucontroller.StateServingInfo, bool) {
	return jujucontroller.StateServingInfo{
		Cert:       coretesting.ServerCert,
		PrivateKey: coretesting.ServerKey,
		StatePort:  123,
		APIPort:    456,
	}, true
}
