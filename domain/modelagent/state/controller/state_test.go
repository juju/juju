// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

// controllerStateSuite is a collection of tests to assert the contracts of
// offered by [ControllerState].
type controllerStateSuite struct {
	schematesting.ControllerSuite
}

// TestControllerStateSuite runs the tests in [controllerStateSuite].
func TestControllerStateSuite(t *testing.T) {
	tc.Run(t, &controllerStateSuite{})
}

// seedControllersAndAgents inserts values to the controller_node and
// controller_node_agent_version that will be used to query the versions.
func (s *controllerStateSuite) seedControllersAndAgents(c *tc.C) {
	res, err := s.DB().Exec(`
INSERT INTO controller_node(controller_id, dqlite_node_id)
VALUES ('1', '1'), 
       ('2', '2'), 
       ('3', '3'),
       ('4', '4')`)

	c.Assert(err, tc.ErrorIsNil)

	rows, err := res.RowsAffected()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rows, tc.Equals, int64(4))

	// The architecture_id for amd64 is 0.
	res, err = s.DB().Exec(`
INSERT INTO controller_node_agent_version (controller_id, version, architecture_id)
VALUES (1, '4.0.1', 0), (2, '4.0.2', 0), (3, '4.0.3', 0), (4, '4.0.3', 0)`)
	c.Assert(err, tc.ErrorIsNil)

	rows, err = res.RowsAffected()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rows, tc.Equals, int64(4))
}

// TestGetControllerAgentVersions tests that we can get the
// agent versions running on all controllers. It also tests that the GROUP BY
// clause works as intended.
func (s *controllerStateSuite) TestGetControllerAgentVersions(c *tc.C) {
	s.seedControllersAndAgents(c)
	st := NewState(s.TxnRunnerFactory())

	versions, err := st.GetControllerAgentVersions(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	version1, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)
	version2, err := semversion.Parse("4.0.2")
	c.Assert(err, tc.ErrorIsNil)
	version3, err := semversion.Parse("4.0.3")
	c.Assert(err, tc.ErrorIsNil)

	expected := []semversion.Number{
		version1,
		version2,
		version3,
	}
	c.Assert(versions, tc.SameContents, expected)
}

// TestGetControllerAgentVersionsNoneFound tests a sad case
// that controller agents are not found.
func (s *controllerStateSuite) TestGetControllerAgentVersionsNoneFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	versions, err := st.GetControllerAgentVersions(c.Context())

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(versions, tc.HasLen, 0)
}
