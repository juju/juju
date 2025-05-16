// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package uuid

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type uuidSuite struct {
	testhelpers.IsolationSuite
}

func TestUuidSuite(t *stdtesting.T) { tc.Run(t, &uuidSuite{}) }
func (*uuidSuite) TestUUID(c *tc.C) {
	uuid, err := NewUUID()
	c.Assert(err, tc.IsNil)
	uuidCopy := uuid.Copy()
	uuidRaw := uuid.Raw()
	uuidStr := uuid.String()
	c.Assert(uuidRaw, tc.HasLen, 16)
	c.Assert(uuidStr, tc.Satisfies, IsValidUUIDString)
	uuid[0] = 0x00
	uuidCopy[0] = 0xFF
	c.Assert(uuid, tc.Not(tc.DeepEquals), uuidCopy)
	uuidRaw[0] = 0xFF
	c.Assert(uuid, tc.Not(tc.DeepEquals), uuidRaw)
	nextUUID, err := NewUUID()
	c.Assert(err, tc.IsNil)
	c.Assert(uuid, tc.Not(tc.DeepEquals), nextUUID)
}

func (*uuidSuite) TestIsValidUUIDFailsWhenNotValid(c *tc.C) {
	tests := []struct {
		input    string
		expected bool
	}{
		{
			input:    UUID{}.String(),
			expected: true,
		},
		{
			input:    "",
			expected: false,
		},
		{
			input:    "blah",
			expected: false,
		},
		{
			input:    "blah-9f484882-2f18-4fd2-967d-db9663db7bea",
			expected: false,
		},
		{
			input:    "9f484882-2f18-4fd2-967d-db9663db7bea-blah",
			expected: false,
		},
		{
			input:    "9f484882-2f18-4fd2-967d-db9663db7bea",
			expected: true,
		},
	}
	for i, t := range tests {
		c.Logf("Running test %d", i)
		c.Check(IsValidUUIDString(t.input), tc.Equals, t.expected)
	}
}

func (*uuidSuite) TestUUIDFromString(c *tc.C) {
	_, err := UUIDFromString("blah")
	c.Assert(err, tc.ErrorMatches, `invalid UUID: "blah"`)
	validUUID := "9f484882-2f18-4fd2-967d-db9663db7bea"
	uuid, err := UUIDFromString(validUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(uuid.String(), tc.Equals, validUUID)
}
