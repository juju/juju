// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type streamSuite struct {
	schematesting.ModelSuite
}

func TestAgentStreamSuite(t *testing.T) {
	tc.Run(t, &streamSuite{})
}

// TestAgentStreamDBValues tests that the values in the agent_stream table
// against the established enums in this package to make sure there is no skew
// between the database and the source code.
func (s *streamSuite) TestAgentStreamDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, name FROM agent_stream")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[Stream]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[Stream(id)] = name

		c.Run(fmt.Sprintf("test agent stream %d/%s IsValid()", id, name), func(t *testing.T) {
			tc.Assert(t, Stream(id).IsValid(), tc.IsTrue)
		})
	}
	c.Assert(dbValues, tc.DeepEquals, map[Stream]string{
		AgentStreamReleased: AgentStreamReleased.String(),
		AgentStreamDevel:    AgentStreamDevel.String(),
		AgentStreamTesting:  AgentStreamTesting.String(),
		AgentStreamProposed: AgentStreamProposed.String(),
	})
}

// TestAgentStreamFromCoreAgentStream tests that the conversion from
// [coreagentbinary.AgentStream] to [Stream] works as expected. This test
// won't pick up if there exists discrepencies in the number of enums that exist
// across the packages.
func (s *streamSuite) TestAgentStreamFromCoreAgentStream(c *tc.C) {
	tests := []struct {
		in       coreagentbinary.AgentStream
		expected Stream
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
		rval, err := StreamFromCoreAgentBinaryStream(test.in)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(rval, tc.Equals, test.expected)
	}
}
