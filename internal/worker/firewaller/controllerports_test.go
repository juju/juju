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
	coretesting "github.com/juju/juju/internal/testing"
)

type controllerPortsSuite struct{}

func TestControllerPortsSuite(t *testing.T) {
	tc.Run(t, &controllerPortsSuite{})
}

func (s *controllerPortsSuite) TestControllerFirewallPorts(c *tc.C) {
	s.checkControllerPorts(c, map[string]any{
		controller.APIPort:            17777,
		controller.SSHServerPort:      17722,
		controller.AutocertDNSNameKey: "example.com",
	}, []network.PortRange{
		{FromPort: 17777, ToPort: 17777, Protocol: "tcp"},
		{FromPort: 17722, ToPort: 17722, Protocol: "tcp"},
		{FromPort: 80, ToPort: 80, Protocol: "tcp"},
	})
}

func (s *controllerPortsSuite) TestControllerFirewallPortsNoAutocert(c *tc.C) {
	// Without autocert, only the API and default SSH server ports are needed.
	s.checkControllerPorts(c, map[string]any{
		controller.APIPort: 17070,
	}, []network.PortRange{
		{FromPort: 17070, ToPort: 17070, Protocol: "tcp"},
		{FromPort: 17022, ToPort: 17022, Protocol: "tcp"},
	})
}

func (s *controllerPortsSuite) checkControllerPorts(c *tc.C, attrs map[string]any, expected []network.PortRange) {
	cfg, err := controller.NewConfig(coretesting.ControllerTag.Id(), coretesting.CACert, attrs)
	c.Assert(err, tc.ErrorIsNil)
	adapter := &firewallerAPIAdapter{
		ctrlConfigSvc: controllerConfigDomainServiceFunc(func(context.Context) (controller.Config, error) {
			return cfg, nil
		}),
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
	}
	ports, err := adapter.ControllerFirewallPorts(c.Context())
	c.Assert(err, tc.ErrorIs, configErr)
	c.Check(ports, tc.IsNil)
}

type controllerConfigDomainServiceFunc func(context.Context) (controller.Config, error)

func (f controllerConfigDomainServiceFunc) ControllerConfig(ctx context.Context) (controller.Config, error) {
	return f(ctx)
}
