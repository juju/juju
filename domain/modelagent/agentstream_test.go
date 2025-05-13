// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package modelagent

import (
	"github.com/juju/tc"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type agentStreamSuite struct {
	schematesting.ModelSuite
}

var _ = tc.Suite(&agentStreamSuite{})

// TestAgentStreamDBValues tests that the values in the agent_stream table
// against the established enums in this package to make sure there is no skew
// between the database and the source code.
func (s *agentStreamSuite) TestAgentStreamDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, name FROM agent_stream")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[AgentStream]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[AgentStream(id)] = name
	}
	c.Assert(dbValues, tc.DeepEquals, map[AgentStream]string{
		AgentStreamReleased: "released",
		AgentStreamDevel:    "devel",
		AgentStreamTesting:  "testing",
		AgentStreamProposed: "proposed",
	})
}

// TestAgentStreamFromCoreAgentStream tests that the conversion from
// [coreagentbinary.AgentStream] to [AgentStream] works as expected. This test
// won't pick up if there exists discrepencies in the number of enums that exist
// across the packages.
func (s *agentStreamSuite) TestAgentStreamFromCoreAgentStream(c *tc.C) {
	tests := []struct {
		in       coreagentbinary.AgentStream
		expected AgentStream
	}{
		{
			in:       coreagentbinary.AgentStreamReleased,
			expected: AgentStreamReleased,
		},
		{
			in:       coreagentbinary.AgentStreamTesting,
			expected: AgentStreamTesting,
		},
		{
			in:       coreagentbinary.AgentStreamProposed,
			expected: AgentStreamProposed,
		},
		{
			in:       coreagentbinary.AgentStreamDevel,
			expected: AgentStreamDevel,
		},
	}

	for _, test := range tests {
		rval, err := AgentStreamFromCoreAgentStream(test.in)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(rval, tc.Equals, test.expected)
	}
}
