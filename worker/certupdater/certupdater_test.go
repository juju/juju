// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater_test

import (
	"crypto/x509"
	stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/certupdater"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type CertUpdaterSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&CertUpdaterSuite{})

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

func (m *mockMachine) Addresses() (addresses []network.Address) {
	return []network.Address{{
		Value: "0.1.2.3",
	}}
}

type mockStateServingGetter struct{}

func (g *mockStateServingGetter) StateServingInfo() (params.StateServingInfo, bool) {
	return params.StateServingInfo{
		Cert:         coretesting.ServerCert,
		PrivateKey:   coretesting.ServerKey,
		CAPrivateKey: coretesting.CAKey,
		StatePort:    123,
		APIPort:      456,
	}, true
}

type mockConfigGetter struct{}

func (g *mockConfigGetter) EnvironConfig() (*config.Config, error) {
	return config.New(config.NoDefaults, coretesting.FakeConfig())

}

func (s *CertUpdaterSuite) TestStartStop(c *gc.C) {
	setter := func(info params.StateServingInfo) error {
		return nil
	}
	changes := make(chan struct{})
	certChangedChan := make(chan params.StateServingInfo)
	worker := certupdater.NewCertificateUpdater(
		&mockMachine{changes}, &mockStateServingGetter{}, &mockConfigGetter{}, setter, certChangedChan,
	)
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func (s *CertUpdaterSuite) TestAddressChange(c *gc.C) {
	var srvCert *x509.Certificate
	updated := make(chan struct{})
	setter := func(info params.StateServingInfo) error {
		var err error
		srvCert, err = cert.ParseCert(info.Cert)
		c.Assert(err, jc.ErrorIsNil)
		sanIPs := make([]string, len(srvCert.IPAddresses))
		for i, ip := range srvCert.IPAddresses {
			sanIPs[i] = ip.String()
		}
		if len(sanIPs) == 1 && sanIPs[0] == "0.1.2.3" {
			close(updated)
		}
		return nil
	}
	changes := make(chan struct{})
	certChangedChan := make(chan params.StateServingInfo)
	worker := certupdater.NewCertificateUpdater(
		&mockMachine{changes}, &mockStateServingGetter{}, &mockConfigGetter{}, setter, certChangedChan,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	changes <- struct{}{}
	// Certificate should be updated with the address value.
	select {
	case <-updated:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for certificate to be updated")
	}

	// The server certificates must report "juju-apiserver" as a DNS
	// name for backwards-compatibility with API clients. They must
	// also report "juju-mongodb" because these certicates are also
	// used for serving MongoDB connections.
	c.Assert(srvCert.DNSNames, gc.DeepEquals,
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
	certChangedChan := make(chan params.StateServingInfo)
	worker := certupdater.NewCertificateUpdater(
		&mockMachine{changes}, &mockStateServingGetterNoCAKey{}, &mockConfigGetter{}, setter, certChangedChan,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	changes <- struct{}{}
	// Certificate should not be updated with the address value.
	select {
	case <-time.After(coretesting.ShortWait):
	case <-updated:
		c.Fatalf("set state serving info unexpectedly called")
	}
}
