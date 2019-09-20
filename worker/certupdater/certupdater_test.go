// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater_test

import (
	"crypto/x509"
	stdtesting "testing"
	"time"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/cert"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/apiserver/params"
	jujucert "github.com/juju/juju/cert"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/certupdater"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type CertUpdaterSuite struct {
	coretesting.BaseSuite
	stateServingInfo params.StateServingInfo
}

var _ = gc.Suite(&CertUpdaterSuite{})

func (s *CertUpdaterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(&jujucert.NewLeafKeyBits, 512)

	s.stateServingInfo = params.StateServingInfo{
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

func (s *CertUpdaterSuite) StateServingInfo() (params.StateServingInfo, bool) {
	return s.stateServingInfo, true
}

type mockConfigGetter struct{}

func (g *mockConfigGetter) ControllerConfig() (jujucontroller.Config, error) {
	return map[string]interface{}{
		jujucontroller.CACertKey: coretesting.CACert,
	}, nil
}

type mockAPIHostGetter struct{}

func (g *mockAPIHostGetter) APIHostPortsForClients() ([]network.SpaceHostPorts, error) {
	return []network.SpaceHostPorts{{
		{SpaceAddress: network.NewScopedSpaceAddress("192.168.1.1", network.ScopeCloudLocal), NetPort: 17070},
		{SpaceAddress: network.NewScopedSpaceAddress("10.1.1.1", network.ScopeMachineLocal), NetPort: 17070},
	}}, nil
}

func (s *CertUpdaterSuite) TestStartStop(c *gc.C) {
	var initialAddresses []string
	setter := func(info params.StateServingInfo) error {
		// Only care about first time called.
		if len(initialAddresses) > 0 {
			return nil
		}
		srvCert, err := cert.ParseCert(info.Cert)
		c.Assert(err, jc.ErrorIsNil)
		initialAddresses = make([]string, len(srvCert.IPAddresses))
		for i, ip := range srvCert.IPAddresses {
			initialAddresses[i] = ip.String()
		}
		return nil
	}
	changes := make(chan struct{})
	worker := certupdater.NewCertificateUpdater(certupdater.Config{
		AddressWatcher:         &mockMachine{changes},
		APIHostPortsGetter:     &mockAPIHostGetter{},
		ControllerConfigGetter: &mockConfigGetter{},
		StateServingInfoGetter: s,
		StateServingInfoSetter: setter,
	})
	workertest.CleanKill(c, worker)
	// Initial cert addresses initialised to cloud local ones.
	c.Assert(initialAddresses, jc.DeepEquals, []string{"192.168.1.1"})
}

func (s *CertUpdaterSuite) TestAddressChange(c *gc.C) {
	var srvCert *x509.Certificate
	updated := make(chan struct{})
	setter := func(info params.StateServingInfo) error {
		s.stateServingInfo = info
		var err error
		srvCert, err = cert.ParseCert(info.Cert)
		c.Assert(err, jc.ErrorIsNil)
		sanIPs := make([]string, len(srvCert.IPAddresses))
		for i, ip := range srvCert.IPAddresses {
			sanIPs[i] = ip.String()
		}
		sanIPsSet := set.NewStrings(sanIPs...)
		if sanIPsSet.Size() == 2 && sanIPsSet.Contains("0.1.2.3") && sanIPsSet.Contains("192.168.1.1") {
			close(updated)
		}
		return nil
	}
	changes := make(chan struct{})
	worker := certupdater.NewCertificateUpdater(certupdater.Config{
		AddressWatcher:         &mockMachine{changes},
		APIHostPortsGetter:     &mockAPIHostGetter{},
		ControllerConfigGetter: &mockConfigGetter{},
		StateServingInfoGetter: s,
		StateServingInfoSetter: setter,
	})
	defer workertest.CleanKill(c, worker)

	changes <- struct{}{}
	// Certificate should be updated with the address value.
	select {
	case <-updated:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for certificate to be updated")
	}

	// The server certificates must report "juju-apiserver" as a DNS
	// name for backwards-compatibility with API clients. They must
	// also report "juju-mongodb" because these certificates are also
	// used for serving MongoDB connections.
	c.Assert(srvCert.DNSNames, jc.SameContents,
		[]string{"localhost", "juju-apiserver", "juju-mongodb", "anything"})
}

type mockStateServingGetterNoCAKey struct{}

func (g *mockStateServingGetterNoCAKey) StateServingInfo() (params.StateServingInfo, bool) {
	return params.StateServingInfo{
		Cert:       coretesting.ServerCert,
		PrivateKey: coretesting.ServerKey,
		StatePort:  123,
		APIPort:    456,
	}, true

}

func (s *CertUpdaterSuite) TestAddressChangeNoCAKey(c *gc.C) {
	updated := make(chan struct{})
	setter := func(info params.StateServingInfo) error {
		close(updated)
		return nil
	}
	changes := make(chan struct{})
	worker := certupdater.NewCertificateUpdater(certupdater.Config{
		AddressWatcher:         &mockMachine{changes},
		APIHostPortsGetter:     &mockAPIHostGetter{},
		ControllerConfigGetter: &mockConfigGetter{},
		StateServingInfoGetter: &mockStateServingGetterNoCAKey{},
		StateServingInfoSetter: setter,
	})
	defer workertest.CleanKill(c, worker)

	changes <- struct{}{}
	// Certificate should not be updated with the address value.
	select {
	case <-time.After(coretesting.ShortWait):
	case <-updated:
		c.Fatalf("set state serving info unexpectedly called")
	}
}
