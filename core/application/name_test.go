// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type RemoteApplicationNameSuite struct {
	testhelpers.IsolationSuite
}

func TestRemoteApplicationNameSuite(t *testing.T) {
	tc.Run(t, &RemoteApplicationNameSuite{})
}

func (s *RemoteApplicationNameSuite) TestIsRemoteApplication(c *tc.C) {
	tests := []struct {
		name     string
		expected bool
	}{
		{
			name:     "remote-123e4567e89b12d3a456426655440000",
			expected: true,
		},
		{
			name:     "remote-123e4567-e89b-12d3-a456-426655440000",
			expected: false,
		},
		{
			name:     "localapp",
			expected: false,
		},
		{
			name:     "remote-xyz",
			expected: false,
		},
		{
			name:     "remote-123e4567e89b12d3a45642665544000", // 31 chars
			expected: false,
		},
		{
			name:     "remote-123e4567e89b12d3a4564266554400000", // 33 chars
			expected: false,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.name)
		result := IsRemoteApplication(test.name)
		c.Check(result, tc.Equals, test.expected)
	}
}

func (s *RemoteApplicationNameSuite) TestRemoteApplicationNameFromUUID(c *tc.C) {
	uuid := UUID("123e4567-e89b-12d3-a456-426655440000")
	expectedName := "remote-123e4567e89b12d3a456426655440000"
	generatedName := RemoteApplicationNameFromUUID(uuid)
	c.Check(generatedName, tc.Equals, expectedName)
}
