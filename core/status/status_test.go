// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/status"
)

type StatusSuite struct{}

func TestStatusSuite(t *stdtesting.T) { tc.Run(t, &StatusSuite{}) }
func (s *StatusSuite) TestModification(c *tc.C) {
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
		c.Assert(v.status.KnownModificationStatus(), tc.Equals, v.valid)
	}
}

func (s *StatusSuite) TestValidModelStatus(c *tc.C) {
	for _, v := range []status.Status{
		status.Available,
		status.Busy,
		status.Destroying,
		status.Error,
		status.Suspended,
	} {
		c.Assert(status.ValidModelStatus(v), tc.IsTrue, tc.Commentf("status %q is not valid for a model", v))
	}
}

func (s *StatusSuite) TestInvalidModelStatus(c *tc.C) {
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
		c.Assert(status.ValidModelStatus(v), tc.IsFalse, tc.Commentf("status %q is valid for a model", v))
	}
}

// TestKnownInstanceStatus asserts that the KnownInstanceStatus method checks
// for the correct statuses for instances.
func (s *StatusSuite) TestKnownInstanceStatus(c *tc.C) {
	for _, t := range []struct {
		status status.Status
		known  bool
	}{
		{status.Active, false},
		{status.Attached, false},
		{status.Attaching, false},
		{status.Available, false},
		{status.Blocked, false},
		{status.Broken, false},
		{status.Busy, false},
		{status.Destroying, false},
		{status.Detached, false},
		{status.Detaching, false},
		{status.Down, false},
		{status.Empty, false},
		{status.Executing, false},
		{status.Failed, false},
		{status.Idle, false},
		{status.Joined, false},
		{status.Joining, false},
		{status.Lost, false},
		{status.Maintenance, false},
		{status.Rebooting, false},
		{status.Suspended, false},
		{status.Suspending, false},
		{status.Started, false},
		{status.Stopped, false},
		{status.Terminated, false},
		{status.Waiting, false},
		{status.Unset, false},

		{status.Pending, true},
		{status.ProvisioningError, true},
		{status.Allocating, true},
		{status.Provisioning, true},
		{status.Running, true},
		{status.Error, true},
		{status.Unknown, true},
	} {
		c.Check(t.status.KnownInstanceStatus(), tc.Equals, t.known, tc.Commentf("checking status %q", t.status))
	}
}

// TestKnownMachineStatus asserts that the KnownMachineStatus method checks for the correct statuses for machines.
func (s *StatusSuite) TestKnownMachineStatus(c *tc.C) {
	for _, t := range []struct {
		status status.Status
		known  bool
	}{
		{status.Active, false},
		{status.Applied, false},
		{status.Attached, false},
		{status.Attaching, false},
		{status.Available, false},
		{status.Blocked, false},
		{status.Broken, false},
		{status.Busy, false},
		{status.Destroying, false},
		{status.Detached, false},
		{status.Detaching, false},
		{status.Empty, false},
		{status.Executing, false},
		{status.Failed, false},
		{status.Idle, false},
		{status.Joined, false},
		{status.Joining, false},
		{status.Lost, false},
		{status.Maintenance, false},
		{status.ProvisioningError, false},
		{status.Rebooting, false},
		{status.Suspended, false},
		{status.Suspending, false},
		{status.Terminated, false},
		{status.Waiting, false},
		{status.Unset, false},
		{status.Unknown, false},

		{status.Error, true},
		{status.Started, true},
		{status.Pending, true},
		{status.Stopped, true},
		{status.Down, true},
	} {
		c.Check(t.status.KnownMachineStatus(), tc.Equals, t.known, tc.Commentf("checking status %q", t.status))
	}
}
