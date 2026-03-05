// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type secretSuite struct {
	testhelpers.IsolationSuite
}

func TestSecretSuite(t *testing.T) {
	tc.Run(t, &secretSuite{})
}

func (s *secretSuite) TestIsRemoteSecretGrant(c *tc.C) {
	tests := []struct {
		name     string
		tag      names.Tag
		expected bool
	}{
		{
			name:     "application tag with remote- prefix",
			tag:      names.NewApplicationTag("remote-app"),
			expected: true,
		},
		{
			name:     "application tag without remote- prefix",
			tag:      names.NewApplicationTag("app"),
			expected: false,
		},
		{
			name:     "non-application tag with remote- prefix",
			tag:      names.NewUnitTag("remote-unit/0"),
			expected: false,
		},
		{
			name:     "application tag with remote- prefix but not at the start",
			tag:      names.NewApplicationTag("not-remote-app"),
			expected: false,
		},
		{
			name:     "empty application tag",
			tag:      names.NewApplicationTag(""),
			expected: false,
		},
	}

	for _, test := range tests {
		c.Logf("Test case: %s", test.name)
		result := IsRemoteSecretGrant(test.tag)
		c.Assert(result, tc.Equals, test.expected)
	}
}
