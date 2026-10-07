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
		"": {{Address: "10.0.0.100:17070", Scope: network.ScopeCloudLocal}},
	}
	s.state.EXPECT().GetAPIAddressesForAgents(gomock.Any()).Return(addresses, nil).Times(5)

	hostPorts, err := svc.GetAPIHostPortsForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	expected := network.NewMachineHostPorts(17070, "10.0.0.100")
	expected[0].Scope = network.ScopeCloudLocal
	c.Check(hostPorts, tc.DeepEquals, []network.HostPorts{expected.HostPorts()})
	addrs, err := svc.GetAllAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrs, tc.DeepEquals, []string{"10.0.0.100:17070"})
	noProxy, err := svc.GetAllNoProxyAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(noProxy, tc.Equals, "10.0.0.100")
	_, err = svc.GetAPIHostPortsForControllerIDForAgents(c.Context(), "0")
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyAPIAddresses)
	_, err = svc.GetAPIHostPortsForControllerIDForAgents(c.Context(), "")
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyAPIAddresses)
}

func (s *serviceSuite) TestSharedClientAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state, loggertesting.WrapCheckLog(c))
	addresses := map[string]controllernode.APIAddresses{
		"": {{Address: "shared.example.com:17070", Scope: network.ScopePublic}},
	}
	s.state.EXPECT().GetAPIAddressesForClients(gomock.Any()).Return(addresses, nil).Times(2)

	hostPorts, err := svc.GetAPIHostPortsForClients(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	expected := network.NewMachineHostPorts(17070, "shared.example.com")
	expected[0].Scope = network.ScopePublic
	c.Check(hostPorts, tc.DeepEquals, []network.HostPorts{expected.HostPorts()})
	addrs, err := svc.GetAllAPIAddressesForClients(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addrs, tc.DeepEquals, []string{"shared.example.com:17070"})
}
