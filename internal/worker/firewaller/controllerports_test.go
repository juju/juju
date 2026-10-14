// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
)

type controllerPortsSuite struct{}

func TestControllerPortsSuite(t *testing.T) {
	tc.Run(t, &controllerPortsSuite{})
}

func (s *controllerPortsSuite) TestControllerFirewallPorts(c *tc.C) {
	// The ssh server port comes from the ssh domain, not the stale value in
	// controller config.
	s.checkControllerPorts(c, map[string]any{
		controller.APIPort:            17777,
		controller.SSHServerPort:      17722,
		controller.AutocertDNSNameKey: "example.com",
	}, 17099, []network.PortRange{
		{FromPort: 17777, ToPort: 17777, Protocol: "tcp"},
		{FromPort: 17099, ToPort: 17099, Protocol: "tcp"},
		{FromPort: 80, ToPort: 80, Protocol: "tcp"},
	})
}

func (s *controllerPortsSuite) TestControllerFirewallPortsNoAutocert(c *tc.C) {
	// Without autocert, only the API and SSH server ports are needed.
	s.checkControllerPorts(c, map[string]any{
		controller.APIPort: 17070,
	}, 17022, []network.PortRange{
		{FromPort: 17070, ToPort: 17070, Protocol: "tcp"},
		{FromPort: 17022, ToPort: 17022, Protocol: "tcp"},
	})
}

func (s *controllerPortsSuite) checkControllerPorts(c *tc.C, attrs map[string]any, sshServerPort int, expected []network.PortRange) {
	cfg, err := controller.NewConfig(coretesting.ControllerTag.Id(), coretesting.CACert, attrs)
	c.Assert(err, tc.ErrorIsNil)
	adapter := &firewallerAPIAdapter{
		ctrlConfigSvc: controllerConfigDomainServiceFunc(func(context.Context) (controller.Config, error) {
			return cfg, nil
		}),
		ctrlSSHSvc: fakeControllerSSHDomainService{port: sshServerPort},
	}
	ports, err := adapter.ControllerFirewallPorts(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ports, tc.DeepEquals, expected)
}

func (s *controllerPortsSuite) TestControllerFirewallPortsConfigError(c *tc.C) {
	// Configuration failures propagate instead of producing partial rules.
	configErr := errors.New("controller config unavailable")
	adapter := &firewallerAPIAdapter{
		ctrlConfigSvc: controllerConfigDomainServiceFunc(func(context.Context) (controller.Config, error) {
			return nil, configErr
		}),
		ctrlSSHSvc: fakeControllerSSHDomainService{port: 17022},
	}
	ports, err := adapter.ControllerFirewallPorts(c.Context())
	c.Assert(err, tc.ErrorIs, configErr)
	c.Check(ports, tc.IsNil)
}

func (s *controllerPortsSuite) TestControllerFirewallPortsSSHServerPortError(c *tc.C) {
	// A failure reading the ssh server port propagates too.
	portErr := errors.New("ssh server port unavailable")
	adapter := &firewallerAPIAdapter{
		ctrlConfigSvc: controllerConfigDomainServiceFunc(func(context.Context) (controller.Config, error) {
			return controller.Config{controller.APIPort: 17070}, nil
		}),
		ctrlSSHSvc: fakeControllerSSHDomainService{err: portErr},
	}
	ports, err := adapter.ControllerFirewallPorts(c.Context())
	c.Assert(err, tc.ErrorIs, portErr)
	c.Check(ports, tc.IsNil)
}

func (s *controllerPortsSuite) TestWatchControllerFirewallPorts(c *tc.C) {
	// The controller firewall ports watcher is the ssh server port watcher.
	w := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	adapter := &firewallerAPIAdapter{
		ctrlSSHSvc: fakeControllerSSHDomainService{watcher: w},
	}
	got, err := adapter.WatchControllerFirewallPorts(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.Equals, watcher.NotifyWatcher(w))
}

type controllerConfigDomainServiceFunc func(context.Context) (controller.Config, error)

func (f controllerConfigDomainServiceFunc) ControllerConfig(ctx context.Context) (controller.Config, error) {
	return f(ctx)
}

type fakeControllerSSHDomainService struct {
	port    int
	err     error
	watcher watcher.NotifyWatcher
}

func (f fakeControllerSSHDomainService) GetSSHServerPort(context.Context) (int, error) {
	return f.port, f.err
}

func (f fakeControllerSSHDomainService) WatchSSHServerPort(context.Context) (watcher.NotifyWatcher, error) {
	return f.watcher, nil
}

var _ ControllerSSHDomainService = fakeControllerSSHDomainService{}
