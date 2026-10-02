// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/controllernode"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/internal/uuid"
)

func (s *stateSuite) TestGetAPIAddressesForAgents(c *tc.C) {
	s.checkAddressReader(c, "controller_agent_address", s.state.GetAPIAddressesForAgents)
}

func (s *stateSuite) TestGetAPIAddressesForClients(c *tc.C) {
	s.checkAddressReader(c, "controller_client_address", s.state.GetAPIAddressesForClients)
}

func (s *stateSuite) checkAddressReader(c *tc.C, table string, read func(context.Context) (map[string]controllernode.APIAddresses, error)) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "1"), tc.ErrorIsNil)
	// No other projection, including the old table, supplies these reads.
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO controller_api_address (controller_id, address, scope, is_agent)
VALUES ('0', 'legacy.example.com:17070', 'public', true)`)
	c.Assert(err, tc.ErrorIsNil)
	for _, other := range []string{"controller_agent_address", "controller_client_address", "controller_peer_address"} {
		if other != table {
			s.insertProjectedAddress(c, other, "0", "other.example.com:17070", network.ScopePublic, 0)
		}
	}
	_, err = read(c.Context())
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyAPIAddresses)

	// Insert in reverse priority order. Equal priorities have a stable address
	// tie-breaker, and a shared endpoint does not acquire a controller ID.
	last := s.insertProjectedAddress(c, table, "0", "10.0.0.1:17070", network.ScopeCloudLocal, 20)
	second := s.insertProjectedAddress(c, table, "0", "10.0.0.3:17070", network.ScopeCloudLocal, 10)
	first := s.insertProjectedAddress(c, table, "0", "10.0.0.2:17070", network.ScopeCloudLocal, 10)
	shared := s.insertProjectedAddress(c, table, "", "shared.example.com:17070", network.ScopePublic, 0)
	node1 := s.insertProjectedAddress(c, table, "1", "10.0.1.1:17070", network.ScopeCloudLocal, 0)
	s.insertProjectedAddress(c, table, "1", "", network.ScopeCloudLocal, 0)

	addresses, err := read(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.DeepEquals, map[string]controllernode.APIAddresses{
		"": {shared}, "0": {first, second, last}, "1": {node1},
	})
}

func (s *stateSuite) TestGetAPIAddressesAfterReplacement(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	old := controllernode.APIAddressProjections{"0": {
		Clients: controllernode.APIAddresses{{Address: "10.0.0.1:17070"}},
		Agents:  controllernode.APIAddresses{{Address: "10.0.0.1:17070"}},
	}}
	c.Assert(s.setAPIAddresses(c, old), tc.ErrorIsNil)
	agentsWant := controllernode.APIAddresses{{Address: "10.0.0.2:17070", Scope: network.ScopeCloudLocal}}
	clientsWant := controllernode.APIAddresses{
		agentsWant[0],
		{Address: "public.example.com:17070", Scope: network.ScopePublic},
	}
	c.Assert(s.setAPIAddresses(c, controllernode.APIAddressProjections{"0": {
		Clients: clientsWant,
		Agents:  agentsWant,
	}}), tc.ErrorIsNil)
	agents, err := s.state.GetAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(agents, tc.DeepEquals, map[string]controllernode.APIAddresses{"0": agentsWant})
	clients, err := s.state.GetAPIAddressesForClients(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(clients, tc.DeepEquals, map[string]controllernode.APIAddresses{"0": clientsWant})

	c.Assert(s.setAPIAddresses(c, controllernode.APIAddressProjections{"0": {}}), tc.ErrorIsNil)
	_, err = s.state.GetAPIAddressesForAgents(c.Context())
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyAPIAddresses)
	_, err = s.state.GetAPIAddressesForClients(c.Context())
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyAPIAddresses)
}

func (s *stateSuite) TestGetAllCloudLocalAPIAddresses(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	s.insertProjectedAddress(c, "controller_agent_address", "0", "10.0.0.9:17070", network.ScopeCloudLocal, 0)
	_, err := s.state.GetAllCloudLocalAPIAddresses(c.Context())
	c.Assert(err, tc.ErrorIs, controllernodeerrors.EmptyAPIAddresses)
	s.insertProjectedAddress(c, "controller_client_address", "0", "10.0.0.1:17070", network.ScopeCloudLocal, 0)
	s.insertProjectedAddress(c, "controller_client_address", "", "10.0.0.2:17070", network.ScopeCloudLocal, 0)
	s.insertProjectedAddress(c, "controller_client_address", "0", "10.0.0.3:17070", network.ScopePublic, 0)
	s.insertProjectedAddress(c, "controller_client_address", "0", "127.0.0.1:17070", network.ScopeMachineLocal, 0)
	addresses, err := s.state.GetAllCloudLocalAPIAddresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.SameContents, []string{"10.0.0.1:17070", "10.0.0.2:17070"})
}

func (s *stateSuite) insertProjectedAddress(c *tc.C, table, controllerID, address string, scope network.Scope, priority int) controllernode.APIAddress {
	addr := controllernode.APIAddress{
		UUID: tc.Must0(c, uuid.NewUUID).String(), Address: address, Scope: scope, Priority: priority,
	}
	_, err := s.DB().ExecContext(c.Context(),
		"INSERT INTO "+table+" (uuid, controller_id, address, scope, priority) VALUES (?, NULLIF(?, ''), ?, ?, ?)",
		addr.UUID, controllerID, addr.Address, addr.Scope, addr.Priority)
	c.Assert(err, tc.ErrorIsNil)
	return addr
}
