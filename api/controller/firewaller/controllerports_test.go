// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
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
	var calls int
	caller := apitesting.BestVersionCaller{
		BestVersion: 7,
		APICallerFunc: func(objType string, version int, id, request string, arg, result any) error {
			c.Check(objType, tc.Equals, "Firewaller")
			c.Check(version, tc.Equals, 7)
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "ControllerConfig")
			c.Check(arg, tc.IsNil)
			c.Assert(result, tc.FitsTypeOf, &params.ControllerConfigResult{})
			*result.(*params.ControllerConfigResult) = params.ControllerConfigResult{
				Config: params.ControllerConfig(cfg),
			}
			calls++
			return nil
		},
	}
	client, err := firewaller.NewClient(caller)
	c.Assert(err, tc.ErrorIsNil)
	ports, err := client.ControllerFirewallPorts(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ports, tc.DeepEquals, expected)
	c.Check(calls, tc.Equals, 1)
}

func (s *controllerPortsSuite) TestControllerFirewallPortsConfigError(c *tc.C) {
	// Configuration failures propagate instead of producing partial rules.
	configErr := errors.New("controller config unavailable")
	var calls int
	caller := apitesting.BestVersionCaller{
		BestVersion: 7,
		APICallerFunc: func(objType string, version int, id, request string, arg, result any) error {
			c.Check(request, tc.Equals, "ControllerConfig")
			calls++
			return configErr
		},
	}
	client, err := firewaller.NewClient(caller)
	c.Assert(err, tc.ErrorIsNil)
	ports, err := client.ControllerFirewallPorts(c.Context())
	c.Assert(err, tc.ErrorIs, configErr)
	c.Check(ports, tc.IsNil)
	c.Check(calls, tc.Equals, 1)
}
