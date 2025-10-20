// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
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
       ('3', '3')`)

	c.Assert(err, tc.ErrorIsNil)

	rows, err := res.RowsAffected()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rows, tc.Equals, int64(3))

	res, err = s.DB().Exec(`
INSERT INTO controller_node_agent_version (controller_id, version, architecture_id)
SELECT v.controller_id, v.version, a.id
FROM (
  SELECT '1' AS controller_id, '4.0.1' AS version
  UNION ALL
  SELECT '2', '4.0.2'
  UNION ALL
  SELECT '3', '4.0.3'
) AS v
JOIN architecture a ON a.name = 'amd64'`)
	c.Assert(err, tc.ErrorIsNil)

	rows, err = res.RowsAffected()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rows, tc.Equals, int64(3))
}

// TestGetControllerAgentVersionsByArchitecture tests that we can get the
// versions fot the given architectures.
func (s *controllerStateSuite) TestGetControllerAgentVersionsByArchitecture(c *tc.C) {
	s.seedControllersAndAgents(c)
	st := NewState(s.TxnRunnerFactory())

	versions, err := st.GetControllerAgentVersionsByArchitecture(
		c.Context(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	)
	c.Assert(err, tc.ErrorIsNil)

	version1, err := semversion.Parse("4.0.1")
	c.Assert(err, tc.ErrorIsNil)
	version2, err := semversion.Parse("4.0.2")
	c.Assert(err, tc.ErrorIsNil)
	version3, err := semversion.Parse("4.0.3")
	c.Assert(err, tc.ErrorIsNil)

	expected := map[agentbinary.Architecture][]semversion.Number{
		agentbinary.AMD64: {
			version1,
			version2,
			version3,
		},
	}
	c.Assert(versions, tc.DeepEquals, expected)
}

// TestGetControllerAgentVersionsByArchitectureNoneFound tests a sad case
// that no agents are found for the given architecture.
func (s *controllerStateSuite) TestGetControllerAgentVersionsByArchitectureNoneFound(c *tc.C) {
	s.seedControllersAndAgents(c)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetControllerAgentVersionsByArchitecture(
		c.Context(),
		[]agentbinary.Architecture{agentbinary.ARM64},
	)
	c.Assert(err, tc.ErrorMatches, "no controller agents found")
}
