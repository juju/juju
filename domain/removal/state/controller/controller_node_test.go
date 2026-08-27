// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type controllerNodeSuite struct {
	schematesting.ControllerSuite
}

func TestControllerNodeSuite(t *testing.T) {
	tc.Run(t, &controllerNodeSuite{})
}

func (s *controllerNodeSuite) TestDeleteControllerNode(c *tc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id) VALUES ('99')")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec("INSERT INTO controller_api_address (controller_id, address, scope) VALUES ('99', '10.0.0.1:17070', 'local-cloud')")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec("INSERT INTO controller_node_agent_version (controller_id, version, architecture_id) VALUES ('99', '4.1.0', 0)")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec("INSERT INTO controller_node_password (controller_id) VALUES ('99')")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec("INSERT INTO controller_node_nonce (controller_id, nonce) VALUES ('99', 'abc123')")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec("INSERT INTO upgrade_info (uuid, previous_version, target_version, state_type_id) VALUES ('upgrade-uuid', '4.0.0', '4.1.0', 0)")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec("INSERT INTO upgrade_info_controller_node (uuid, controller_node_id, upgrade_info_uuid) VALUES ('upgrade-node-uuid', '99', 'upgrade-uuid')")
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteControllerNode(c.Context(), "99")
	c.Assert(err, tc.ErrorIsNil)

	for _, q := range []struct {
		query string
		args  []any
	}{
		{"SELECT COUNT(*) FROM controller_api_address WHERE controller_id = ?", []any{"99"}},
		{"SELECT COUNT(*) FROM controller_node_agent_version WHERE controller_id = ?", []any{"99"}},
		{"SELECT COUNT(*) FROM controller_node_password WHERE controller_id = ?", []any{"99"}},
		{"SELECT COUNT(*) FROM controller_node_nonce WHERE controller_id = ?", []any{"99"}},
		{"SELECT COUNT(*) FROM upgrade_info_controller_node WHERE controller_node_id = ?", []any{"99"}},
		{"SELECT COUNT(*) FROM controller_node WHERE controller_id = ?", []any{"99"}},
	} {
		var count int
		err := db.QueryRow(q.query, q.args...).Scan(&count)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(count, tc.Equals, 0)
	}
}

func (s *controllerNodeSuite) TestDeleteControllerNodeIdempotent(c *tc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id) VALUES ('98')")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec("INSERT INTO controller_api_address (controller_id, address, scope) VALUES ('98', '10.0.0.1:17070', 'local-cloud')")
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteControllerNode(c.Context(), "98")
	c.Assert(err, tc.ErrorIsNil)
	err = st.DeleteControllerNode(c.Context(), "98")
	c.Assert(err, tc.ErrorIsNil)

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM controller_node WHERE controller_id = '98'").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *controllerNodeSuite) TestDeleteControllerNodePreservesOtherNodes(c *tc.C) {
	db := s.DB()

	for _, cID := range []string{"96", "97"} {
		_, err := db.Exec("INSERT INTO controller_node (controller_id) VALUES (?)", cID)
		c.Assert(err, tc.ErrorIsNil)
		_, err = db.Exec("INSERT INTO controller_api_address (controller_id, address, scope) VALUES (?, '10.0.0.' || ? || ':17070', 'local-cloud')", cID, cID)
		c.Assert(err, tc.ErrorIsNil)
	}

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.DeleteControllerNode(c.Context(), "96")
	c.Assert(err, tc.ErrorIsNil)

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM controller_node WHERE controller_id = '96'").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	err = db.QueryRow("SELECT COUNT(*) FROM controller_node WHERE controller_id = '97'").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	err = db.QueryRow("SELECT COUNT(*) FROM controller_api_address WHERE controller_id = '96'").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	err = db.QueryRow("SELECT COUNT(*) FROM controller_api_address WHERE controller_id = '97'").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}
