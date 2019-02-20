// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package status_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
)

type StatusSuite struct{}

var _ = gc.Suite(&StatusSuite{})

func (s *StatusSuite) TestModification(c *gc.C) {
	testcases := []struct {
		name   string
		status status.Status
		valid  bool
	}{
		{
			name:   "applied",
			status: status.Applied,
			valid:  true,
		},
		{
			name:   "error",
			status: status.Error,
			valid:  true,
		},
		{
			name:   "unknown",
			status: status.Unknown,
			valid:  true,
		},
		{
			name:   "idle",
			status: status.Idle,
			valid:  true,
		},
		{
			name:   "pending",
			status: status.Pending,
			valid:  false,
		},
	}
	for k, v := range testcases {
		c.Logf("Testing modification status %d %s", k, v.name)
		c.Assert(v.status.KnownModificationStatus(), gc.Equals, v.valid)
	}
}
