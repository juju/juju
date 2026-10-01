// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/controllernode"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/internal/uuid"
)

func (s *stateSuite) TestSetAPIAddressesProjectionMetadataAndIdentity(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO controller_api_address (controller_id, address, scope)
VALUES ('0', 'legacy.example.com:17070', 'public');
INSERT INTO controller_peer_address (uuid, controller_id, address, scope)
VALUES ('peer', '0', 'peer.example.com:17070', 'local-cloud')`)
	c.Assert(err, tc.ErrorIsNil)

	addrs := controllernode.APIAddresses{{
		Address: "10.0.0.1:17070", IsAgent: true, Scope: network.ScopeCloudLocal,
	}}
	s.addControllerAddressProjections(c, "0", addrs)
	originalUUID := addrs[0].UUID
	_, err = s.DB().ExecContext(c.Context(), "DELETE FROM change_log")
	c.Assert(err, tc.ErrorIsNil)

	// Services supply fresh UUIDs on each reconciliation. Existing rows keep
	// their identities, and unchanged projections do not emit notifications.
	addrs[0].UUID = tc.Must0(c, uuid.NewUUID).String()
	s.addControllerAddressProjections(c, "0", addrs)
	var count int
	err = s.DB().QueryRowContext(c.Context(), "SELECT COUNT(*) FROM change_log").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	addrs[0].Scope = network.ScopePublic
	addrs[0].Priority = 2
	s.addControllerAddressProjections(c, "0", addrs)
	s.checkControllerAddressProjections(c, "0", addrs)
	for _, table := range []string{"controller_client_address", "controller_agent_address"} {
		var storedUUID string
		err := s.DB().QueryRowContext(c.Context(), "SELECT uuid FROM "+table).Scan(&storedUUID)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(storedUUID, tc.Equals, originalUUID)
		err = s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM change_log AS log
JOIN change_log_namespace AS namespace ON namespace.id = log.namespace_id
WHERE namespace.namespace = ? AND log.edit_type_id = 2`, table).Scan(&count)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(count, tc.Equals, 1)
	}

	// This stage writes neither the legacy projection nor the peer projection.
	for table, expected := range map[string]string{
		"controller_api_address":  "legacy.example.com:17070",
		"controller_peer_address": "peer.example.com:17070",
	} {
		var address string
		err := s.DB().QueryRowContext(c.Context(), "SELECT address FROM "+table).Scan(&address)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(address, tc.Equals, expected)
	}
}

func (s *stateSuite) TestSetAPIAddressesProjectionRollback(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	_, err := s.DB().ExecContext(c.Context(), `
CREATE TRIGGER fail_agent_address BEFORE INSERT ON controller_agent_address
BEGIN
    SELECT RAISE(ABORT, 'agent write failed');
END`)
	c.Assert(err, tc.ErrorIsNil)

	err = s.setAPIAddresses(c, map[string]controllernode.APIAddresses{
		"0": {{Address: "10.0.0.1:17070", IsAgent: true}},
	})
	c.Assert(err, tc.ErrorMatches, ".*agent write failed.*")
	for _, table := range []string{"controller_client_address", "controller_agent_address"} {
		var count int
		err := s.DB().QueryRowContext(c.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(count, tc.Equals, 0)
	}
}

func (s *stateSuite) TestSetAPIAddressesRejectsDyingAndDeadControllers(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	addrs := controllernode.APIAddresses{{Address: "10.0.0.1:17070", IsAgent: true}}
	s.addControllerAddressProjections(c, "0", addrs)
	for _, life := range []int{1, 2} {
		_, err := s.DB().ExecContext(c.Context(), "UPDATE controller_node SET life_id = ? WHERE controller_id = '0'", life)
		c.Assert(err, tc.ErrorIsNil)
		err = s.setAPIAddresses(c, map[string]controllernode.APIAddresses{
			"0": {{Address: "10.0.0.2:17070", IsAgent: true}},
		})
		c.Assert(err, tc.ErrorIs, controllernodeerrors.NotFound)
		s.checkControllerAddressProjections(c, "0", addrs)
	}
}

func (s *stateSuite) TestSetAPIAddressesDuplicateAudienceSelection(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	addrs := controllernode.APIAddresses{
		{Address: "10.0.0.1:17070", IsAgent: true},
		{Address: "10.0.0.1:17070", IsAgent: false, Scope: network.ScopePublic},
	}
	s.addControllerAddressProjections(c, "0", addrs)
	s.checkControllerAddressProjections(c, "0", addrs[1:])
}

func (s *stateSuite) TestSetAPIAddressesEmptyInput(c *tc.C) {
	c.Assert(s.state.AddDqliteNodeID(c.Context(), "0"), tc.ErrorIsNil)
	addrs := controllernode.APIAddresses{{Address: "10.0.0.1:17070", IsAgent: true}}
	s.addControllerAddressProjections(c, "0", addrs)
	c.Assert(s.state.SetAPIAddresses(c.Context(), nil), tc.ErrorIsNil)
	s.checkControllerAddressProjections(c, "0", addrs)
}
