// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package status_test

import (
	jc "github.com/juju/testing/checkers"
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

func (s *StatusSuite) TestValidModelStatus(c *gc.C) {
	for _, v := range []status.Status{
		status.Available,
		status.Busy,
		status.Destroying,
		status.Error,
		status.Suspended,
	} {
		c.Assert(status.ValidModelStatus(v), jc.IsTrue, gc.Commentf("status %q is not valid for a model", v))
	}
}

func (s *StatusSuite) TestInvalidModelStatus(c *gc.C) {
	for _, v := range []status.Status{
		status.Active,
		status.Allocating,
		status.Applied,
		status.Attached,
		status.Attaching,
		status.Blocked,
		status.Broken,
		status.Detached,
		status.Detaching,
		status.Down,
		status.Empty,
		status.Executing,
		status.Failed,
		status.Idle,
		status.Joined,
		status.Joining,
		status.Lost,
		status.Maintenance,
		status.Pending,
		status.Provisioning,
		status.ProvisioningError,
		status.Rebooting,
		status.Running,
		status.Suspending,
		status.Started,
		status.Stopped,
		status.Terminated,
		status.Unknown,
		status.Waiting,
	} {
		c.Assert(status.ValidModelStatus(v), jc.IsFalse, gc.Commentf("status %q is valid for a model", v))
	}
}
