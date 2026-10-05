// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/controllernode"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/internal/uuid"
)

func (s *stateSuite) TestSetAPIAddressesIndependentAudiences(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	// Full reconciliation removes obsolete rows written by the previous shared
	// endpoint representation.
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO controller_client_address (uuid, controller_id, address, scope)
VALUES ('old-client', NULL, 'client.example.com:17070', 'public');
INSERT INTO controller_agent_address (uuid, controller_id, address, scope)
VALUES ('old-agent', NULL, 'agent.example.com:17070', 'public')`)
	c.Assert(err, tc.ErrorIsNil)
	projections := controllernode.APIAddressProjections{
		"0": {
			Clients: controllernode.APIAddresses{{Address: "client.example.com:17070", Scope: network.ScopePublic}},
			Agents:  controllernode.APIAddresses{{Address: "agent.example.com:17070", Scope: network.ScopeCloudLocal}},
			Peers:   controllernode.APIAddresses{{Address: "10.0.0.3:17070", Scope: network.ScopeCloudLocal}},
		},
	}
	c.Assert(s.setAPIAddresses(c, projections), tc.ErrorIsNil)
	s.checkAddressProjections(c, projections)
}

func (s *stateSuite) TestSetAPIAddressesEmptySnapshotClearsAllProjections(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	projection := controllernode.APIAddressProjection{
		Clients: controllernode.APIAddresses{{Address: "10.0.0.1:17070"}},
		Agents:  controllernode.APIAddresses{{Address: "10.0.0.2:17070"}},
		Peers:   controllernode.APIAddresses{{Address: "10.0.0.3:17070"}},
	}
	c.Assert(s.setAPIAddresses(c, controllernode.APIAddressProjections{"0": projection}), tc.ErrorIsNil)
	c.Assert(s.setAPIAddresses(c, controllernode.APIAddressProjections{"0": {}}), tc.ErrorIsNil)
	s.checkAddressProjections(c, controllernode.APIAddressProjections{"0": {}})
}

func (s *stateSuite) TestSetAPIAddressesCleansOmittedInactiveController(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "1"), tc.ErrorIsNil)
	projections := controllernode.APIAddressProjections{
		"0": {Clients: controllernode.APIAddresses{{Address: "10.0.0.1:17070"}}},
		"1": {Peers: controllernode.APIAddresses{{Address: "10.0.0.2:17070"}}},
	}
	c.Assert(s.setAPIAddresses(c, projections), tc.ErrorIsNil)
	_, err := s.DB().ExecContext(c.Context(), "UPDATE controller_node SET life_id = 2 WHERE controller_id = '1'")
	c.Assert(err, tc.ErrorIsNil)

	want := controllernode.APIAddressProjections{
		"0": {Clients: projections["0"].Clients},
	}
	c.Assert(s.setAPIAddresses(c, want), tc.ErrorIsNil)
	s.checkAddressProjections(c, want)
}

func (s *stateSuite) TestSetAPIAddressesRequiresExactActiveMembership(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "1"), tc.ErrorIsNil)

	err := s.setAPIAddresses(c, controllernode.APIAddressProjections{"0": {}})
	c.Assert(err, tc.ErrorIs, controllernodeerrors.StaleControllerMembership)
	err = s.setAPIAddresses(c, controllernode.APIAddressProjections{"0": {}, "1": {}, "2": {}})
	c.Assert(err, tc.ErrorIs, controllernodeerrors.StaleControllerMembership)
	s.checkAddressProjections(c, nil)
}

func (s *stateSuite) TestSetAPIAddressesSharedEndpointsDoNotAffectMembership(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	projections := controllernode.APIAddressProjections{
		"": {
			Clients: controllernode.APIAddresses{{Address: "client.example.com:17070"}},
			Agents:  controllernode.APIAddresses{{Address: "agent.example.com:17070"}},
		},
		"0": {
			Peers: controllernode.APIAddresses{{Address: "controller-0.example.com:17070"}},
		},
	}
	c.Assert(s.setAPIAddresses(c, projections), tc.ErrorIsNil)
	s.checkAddressProjections(c, projections)
}

func (s *stateSuite) TestSetAPIAddressesSharedEndpointsWithoutControllers(c *tc.C) {
	projections := controllernode.APIAddressProjections{
		"": {
			Clients: controllernode.APIAddresses{{Address: "client.example.com:17070"}},
			Agents:  controllernode.APIAddresses{{Address: "agent.example.com:17070"}},
		},
	}
	c.Assert(s.setAPIAddresses(c, projections), tc.ErrorIsNil)
	s.checkAddressProjections(c, projections)

	for _, table := range []string{"controller_client_address", "controller_agent_address"} {
		var controllerID sql.NullString
		err := s.DB().QueryRowContext(
			c.Context(), "SELECT controller_id FROM "+table,
		).Scan(&controllerID)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(controllerID.Valid, tc.IsFalse, tc.Commentf("table %s", table))
	}
}

func (s *stateSuite) TestSetAPIAddressesMetadataUpdatePreservesUUID(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	original := controllernode.APIAddressProjections{"0": {
		Clients: controllernode.APIAddresses{{Address: "10.0.0.1:17070", Scope: network.ScopeCloudLocal}},
		Agents:  controllernode.APIAddresses{{Address: "10.0.0.2:17070", Scope: network.ScopeCloudLocal}},
		Peers:   controllernode.APIAddresses{{Address: "10.0.0.3:17070", Scope: network.ScopeCloudLocal}},
	}}
	c.Assert(s.setAPIAddresses(c, original), tc.ErrorIsNil)
	stored := s.addressUUIDs(c)

	updated := controllernode.APIAddressProjections{"0": {
		Clients: controllernode.APIAddresses{{Address: "10.0.0.1:17070", Scope: network.ScopePublic, Priority: 3}},
		Agents:  controllernode.APIAddresses{{Address: "10.0.0.2:17070", Scope: network.ScopePublic, Priority: 2}},
		Peers:   controllernode.APIAddresses{{Address: "10.0.0.3:17070", Scope: network.ScopePublic, Priority: 1}},
	}}
	c.Assert(s.setAPIAddresses(c, updated), tc.ErrorIsNil)
	c.Check(s.addressUUIDs(c), tc.DeepEquals, stored)
	s.checkAddressProjections(c, updated)
}

func (s *stateSuite) TestSetAPIAddressesNoOpMakesNoChanges(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	projections := controllernode.APIAddressProjections{"0": {
		Clients: controllernode.APIAddresses{{Address: "10.0.0.1:17070"}},
		Agents:  controllernode.APIAddresses{{Address: "10.0.0.2:17070"}},
		Peers:   controllernode.APIAddresses{{Address: "10.0.0.3:17070"}},
	}}
	c.Assert(s.setAPIAddresses(c, projections), tc.ErrorIsNil)
	_, err := s.DB().ExecContext(c.Context(), "DELETE FROM change_log")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.setAPIAddresses(c, projections), tc.ErrorIsNil)

	var count int
	err = s.DB().QueryRowContext(c.Context(), "SELECT COUNT(*) FROM change_log").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *stateSuite) TestSetAPIAddressesRollsBackWhenPeerWriteFails(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	_, err := s.DB().ExecContext(c.Context(), `
CREATE TRIGGER fail_peer_address BEFORE INSERT ON controller_peer_address
BEGIN
    SELECT RAISE(ABORT, 'peer write failed');
END`)
	c.Assert(err, tc.ErrorIsNil)

	err = s.setAPIAddresses(c, controllernode.APIAddressProjections{"0": {
		Clients: controllernode.APIAddresses{{Address: "10.0.0.1:17070"}},
		Agents:  controllernode.APIAddresses{{Address: "10.0.0.2:17070"}},
		Peers:   controllernode.APIAddresses{{Address: "10.0.0.3:17070"}},
	}})
	c.Assert(err, tc.ErrorMatches, ".*peer write failed.*")
	s.checkAddressProjections(c, nil)
}

func (s *stateSuite) setAPIAddresses(c *tc.C, projections controllernode.APIAddressProjections) error {
	for controllerID, projection := range projections {
		for _, addresses := range []*controllernode.APIAddresses{
			&projection.Clients, &projection.Agents, &projection.Peers,
		} {
			for i := range *addresses {
				if (*addresses)[i].UUID == "" {
					(*addresses)[i].UUID = tc.Must0(c, uuid.NewUUID).String()
				}
			}
		}
		projections[controllerID] = projection
	}
	return s.state.SetAPIAddresses(c.Context(), projections)
}

func (s *stateSuite) checkAddressProjections(c *tc.C, projections controllernode.APIAddressProjections) {
	tables := map[string]func(controllernode.APIAddressProjection) controllernode.APIAddresses{
		"controller_client_address": func(projection controllernode.APIAddressProjection) controllernode.APIAddresses {
			return projection.Clients
		},
		"controller_agent_address": func(projection controllernode.APIAddressProjection) controllernode.APIAddresses {
			return projection.Agents
		},
		"controller_peer_address": func(projection controllernode.APIAddressProjection) controllernode.APIAddresses {
			return projection.Peers
		},
	}
	for table, selectAddresses := range tables {
		func() {
			var want []publishedControllerAddress
			for controllerID, projection := range projections {
				identity := sql.NullString{String: controllerID, Valid: controllerID != ""}
				for _, address := range selectAddresses(projection) {
					want = append(want, publishedControllerAddress{
						ControllerID: identity,
						Address:      address.Address,
						Scope:        string(address.Scope),
						Priority:     address.Priority,
					})
				}
			}

			rows, err := s.DB().QueryContext(c.Context(), "SELECT uuid, controller_id, address, scope, priority FROM "+table)
			c.Assert(err, tc.ErrorIsNil)
			defer func() {
				c.Check(rows.Close(), tc.ErrorIsNil)
			}()
			var got []publishedControllerAddress
			for rows.Next() {
				var row publishedControllerAddress
				c.Assert(rows.Scan(&row.UUID, &row.ControllerID, &row.Address, &row.Scope, &row.Priority), tc.ErrorIsNil)
				c.Check(uuid.IsValidUUIDString(row.UUID), tc.IsTrue)
				row.UUID = ""
				got = append(got, row)
			}
			c.Assert(rows.Err(), tc.ErrorIsNil)
			c.Check(got, tc.SameContents, want, tc.Commentf("table %s", table))
		}()
	}
}

func (s *stateSuite) addressUUIDs(c *tc.C) map[string]string {
	result := make(map[string]string)
	for _, table := range []string{"controller_client_address", "controller_agent_address", "controller_peer_address"} {
		var id string
		err := s.DB().QueryRowContext(c.Context(), "SELECT uuid FROM "+table).Scan(&id)
		c.Assert(err, tc.ErrorIsNil)
		result[table] = id
	}
	return result
}
