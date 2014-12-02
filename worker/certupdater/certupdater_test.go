// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater_test

import (
	stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

type mockEnviron struct {
	serverCertUpdater func(info state.StateServingInfo, oldCert string) error
}

func (e *mockEnviron) StateServingInfo() (state.StateServingInfo, error) {
	return state.StateServingInfo{
		Cert:           coretesting.ServerCert,
		PrivateKey:     coretesting.ServerKey,
		CAPrivateKey:   coretesting.CAKey,
		StatePort:      123,
		APIPort:        456,
		SharedSecret:   "secret",
		SystemIdentity: "identity",
	}, nil

}

func (e *mockEnviron) EnvironConfig() (*config.Config, error) {
	return config.New(config.NoDefaults, coretesting.FakeConfig())

}

func (e *mockEnviron) UpdateServerCertificate(info state.StateServingInfo, oldCert string) error {
	if e.serverCertUpdater != nil {
		return e.serverCertUpdater(info, oldCert)
	}
	return nil
}

func (s *CertUpdaterSuite) TestStartStop(c *gc.C) {
	setter := func(info *state.StateServingInfo) error {
		return nil
	}
	changes := make(chan struct{})
	worker := certupdater.NewCertificateUpdater(
		&mockMachine{changes}, &mockEnviron{}, setter,
	)
	worker.Kill()
	c.Assert(worker.Wait(), gc.IsNil)
}

func checkNewCertificate(c *gc.C, newCert string) bool {
	srvCert, err := cert.ParseCert(newCert)
	c.Assert(err, jc.ErrorIsNil)
	sanIPs := make([]string, len(srvCert.IPAddresses))
	for i, ip := range srvCert.IPAddresses {
		sanIPs[i] = ip.String()
	}
	if len(sanIPs) != 1 || sanIPs[0] != "0.1.2.3" {
		c.Errorf("unexpected SAN value in new certificate: %v", sanIPs)
		return false
	}
	return true
}

func (s *CertUpdaterSuite) TestAddressChange(c *gc.C) {
	updated := make(chan struct{})
	setter := func(info *state.StateServingInfo) error {
		if checkNewCertificate(c, info.Cert) {
			close(updated)
		}
		return nil
	}
	changes := make(chan struct{})
	worker := certupdater.NewCertificateUpdater(
		&mockMachine{changes}, &mockEnviron{}, setter,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	changes <- struct{}{}
	// Certificate should be updated with the address value.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for certificate to be updated")
	case <-updated:
	}
}

func (s *CertUpdaterSuite) TestCertUpdated(c *gc.C) {
	updated := make(chan struct{})
	setter := func(info *state.StateServingInfo) error {
		return nil
	}
	certUpdater := func(info state.StateServingInfo, oldCert string) error {
		if checkNewCertificate(c, info.Cert) {
			c.Assert(oldCert, gc.Equals, coretesting.ServerCert)
			// Only certificate should have been updated.
			expectedInfo := state.StateServingInfo{
				CAPrivateKey:   coretesting.CAKey,
				StatePort:      123,
				APIPort:        456,
				SharedSecret:   "secret",
				SystemIdentity: "identity",
			}
			info.Cert = ""
			info.PrivateKey = ""
			c.Assert(info, jc.DeepEquals, expectedInfo)
			close(updated)
		}
		return nil
	}
	changes := make(chan struct{})
	worker := certupdater.NewCertificateUpdater(
		&mockMachine{changes}, &mockEnviron{certUpdater}, setter,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	changes <- struct{}{}
	// Certificate should be updated with the address value.
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for certificate to be updated")
	case <-updated:
	}
}

type mockEnvironNoCAKey struct {
	mockEnviron
}

func (e *mockEnvironNoCAKey) StateServingInfo() (state.StateServingInfo, error) {
	return state.StateServingInfo{
		Cert:           coretesting.ServerCert,
		PrivateKey:     coretesting.ServerKey,
		CAPrivateKey:   coretesting.CAKey,
		StatePort:      123,
		APIPort:        456,
		SharedSecret:   "secret",
		SystemIdentity: "identity",
	}, nil

}

func (s *CertUpdaterSuite) TestAddressChangeNoCAKey(c *gc.C) {
	updated := make(chan struct{})
	setter := func(info *state.StateServingInfo) error {
		close(updated)
		return nil
	}
	changes := make(chan struct{})
	worker := certupdater.NewCertificateUpdater(
		&mockMachine{changes}, &mockEnvironNoCAKey{}, setter,
	)
	defer func() { c.Assert(worker.Wait(), gc.IsNil) }()
	defer worker.Kill()

	changes <- struct{}{}
	// Certificate should not be updated with the address value.
	select {
	case <-time.After(coretesting.ShortWait):
	case <-updated:
		c.Fatalf("set state serving ingo unexpectedly called")
	}
}
