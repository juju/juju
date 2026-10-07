// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"fmt"

	"github.com/juju/tc"
)

func (s *controllerSchemaSuite) TestControllerDiscoveryAddressesAllowSharedEndpoints(c *tc.C) {
	s.applyDDL(c, ControllerDDL())

	for _, table := range []string{
		"controller_agent_address",
		"controller_client_address",
	} {
		query := fmt.Sprintf(`
INSERT INTO %s (uuid, address, scope)
VALUES (?, 'api.example.com:17070', 'public')`, table)
		// Shared endpoints do not require a controller node. The same endpoint
		// may occur in both audience projections, but only once in each.
		s.assertExecSQL(c, query, "shared")
		s.assertExecSQLError(c, query,
			"UNIQUE constraint failed: "+table+".address", "duplicate")

		var priority int
		err := s.DB().QueryRowContext(c.Context(),
			"SELECT priority FROM "+table+" WHERE uuid = 'shared'",
		).Scan(&priority)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(priority, tc.Equals, 0)
	}
}

func (s *controllerSchemaSuite) TestControllerAddressControllerIdentity(c *tc.C) {
	s.applyDDL(c, ControllerDDL())
	s.assertExecSQL(c, "INSERT INTO controller_node (controller_id) VALUES ('0'), ('1')")

	for _, table := range []string{
		"controller_agent_address",
		"controller_client_address",
		"controller_peer_address",
	} {
		query := fmt.Sprintf(`
INSERT INTO %s (uuid, controller_id, address, scope, priority)
VALUES (?, ?, '[2001:db8::1]:17070', 'local-cloud', 1)`, table)
		s.assertExecSQL(c, query, "first", "0")
		s.assertExecSQL(c, query, "second", "1")
		s.assertExecSQLError(c, query,
			"UNIQUE constraint failed: "+table+".controller_id, "+table+".address",
			"duplicate", "0")
		s.assertExecSQLError(c, query,
			"FOREIGN KEY constraint failed", "missing", "2")
	}

	s.assertExecSQLError(c, `
INSERT INTO controller_peer_address (uuid, address, scope)
VALUES ('shared', 'api.example.com:17070', 'public')`,
		"NOT NULL constraint failed: controller_peer_address.controller_id")
}

func (s *controllerSchemaSuite) TestControllerAddressProjectionChanges(c *tc.C) {
	s.applyDDL(c, ControllerDDL())
	s.assertExecSQL(c, "INSERT INTO controller_node (controller_id) VALUES ('0')")

	for _, table := range []string{
		"controller_agent_address",
		"controller_client_address",
		"controller_peer_address",
	} {
		changed := "address"
		if table == "controller_peer_address" {
			changed = "0"
		}
		s.assertExecSQL(c, fmt.Sprintf(`
INSERT INTO %s (uuid, controller_id, address, scope, priority)
VALUES ('address', '0', '10.0.0.1:17070', 'local-cloud', 0)`, table))
		s.assertChangeEvent(c, table, changed)

		// Metadata-only changes must notify projection readers too.
		for _, update := range []string{
			"address = 'api.example.com:17070'",
			"scope = 'public'",
			"priority = 1",
		} {
			s.assertExecSQL(c, "UPDATE "+table+" SET "+update)
			s.assertChangeEvent(c, table, changed)
		}

		if table != "controller_peer_address" {
			// Notifications also cover acquiring or losing node specificity.
			s.assertExecSQL(c, "UPDATE "+table+" SET controller_id = NULL")
			s.assertChangeEvent(c, table, changed)
			s.assertExecSQL(c, "UPDATE "+table+" SET controller_id = '0'")
			s.assertChangeEvent(c, table, changed)
		}

		s.assertExecSQL(c, "UPDATE "+table+" SET priority = priority")
		var count int
		err := s.DB().QueryRowContext(c.Context(), `
SELECT COUNT(*) FROM change_log AS log
JOIN change_log_namespace AS namespace ON namespace.id = log.namespace_id
WHERE namespace.namespace = ?`, table).Scan(&count)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(count, tc.Equals, 0)

		s.assertExecSQL(c, "DELETE FROM "+table)
		s.assertChangeEvent(c, table, changed)
	}
}
