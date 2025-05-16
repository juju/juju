// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/worker/uniter/runner/debug"
)

type DebugHooksCommonSuite struct{}

func TestDebugHooksCommonSuite(t *stdtesting.T) { tc.Run(t, &DebugHooksCommonSuite{}) }

// TestHooksContext tests the behaviour of HooksContext.
func (*DebugHooksCommonSuite) TestHooksContext(c *tc.C) {
	ctx := debug.NewHooksContext("foo/8")
	c.Assert(ctx.Unit, tc.Equals, "foo/8")
	c.Assert(ctx.FlockDir, tc.Equals, "/tmp")
	ctx.FlockDir = "/var/lib/juju"
	c.Assert(ctx.ClientFileLock(), tc.SamePath, "/var/lib/juju/juju-unit-foo-8-debug-hooks")
	c.Assert(ctx.ClientExitFileLock(), tc.SamePath, "/var/lib/juju/juju-unit-foo-8-debug-hooks-exit")
}
