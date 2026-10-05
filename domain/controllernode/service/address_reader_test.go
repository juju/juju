// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/controllernode"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

func (s *serviceSuite) TestSharedAgentAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))
	addresses := map[string]controllernode.APIAddresses{
		"":  {{Address: "10.0.0.100:17070", Scope: network.ScopeCloudLocal}},
		"0": {{Address: "10.0.0.1:17070", Scope: network.ScopeCloudLocal}},
	}
	s.state.EXPECT().GetAPIAddressesForAgents(gomock.Any()).Return(addresses, nil).Times(6)

	hostPorts, err := svc.GetAPIHostPortsForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	shared := network.NewMachineHostPorts(17070, "10.0.0.100")
	shared[0].Scope = network.ScopeCloudLocal
	named := network.NewMachineHostPorts(17070, "10.0.0.1")
	named[0].Scope = network.ScopeCloudLocal
	c.Check(hostPorts, tc.DeepEquals, []network.HostPorts{shared.HostPorts(), named.HostPorts()})
	addrs, err := svc.GetAllAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrs, tc.DeepEquals, []string{"10.0.0.100:17070", "10.0.0.1:17070"})
	noProxy, err := svc.GetAllNoProxyAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(noProxy, tc.Equals, "10.0.0.1,10.0.0.100")
	byController, err := svc.GetAPIAddressesByControllerIDForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(byController, tc.DeepEquals, map[string][]string{"0": {"10.0.0.1:17070"}})
	controllerHostPorts, err := svc.GetAPIHostPortsForControllerIDForAgents(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerHostPorts, tc.DeepEquals, named.HostPorts())
	_, err = svc.GetAPIHostPortsForControllerIDForAgents(c.Context(), "")
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyAPIAddresses)
}

func (s *serviceSuite) TestSharedClientAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))
	addresses := map[string]controllernode.APIAddresses{
		"":  {{Address: "shared.example.com:17070", Scope: network.ScopePublic}},
		"0": {{Address: "controller.example.com:17070", Scope: network.ScopePublic}},
	}
	s.state.EXPECT().GetAPIAddressesForClients(gomock.Any()).Return(addresses, nil).Times(3)

	hostPorts, err := svc.GetAPIHostPortsForClients(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	shared := network.NewMachineHostPorts(17070, "shared.example.com")
	shared[0].Scope = network.ScopePublic
	named := network.NewMachineHostPorts(17070, "controller.example.com")
	named[0].Scope = network.ScopePublic
	c.Check(hostPorts, tc.DeepEquals, []network.HostPorts{shared.HostPorts(), named.HostPorts()})
	addrs, err := svc.GetAllAPIAddressesForClients(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrs, tc.DeepEquals, []string{"shared.example.com:17070", "controller.example.com:17070"})
	byController, err := svc.GetAPIAddressesByControllerIDForClients(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(byController, tc.DeepEquals, map[string][]string{"0": {"controller.example.com:17070"}})
}
