// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/debug"
)

type DebugHooksCommonSuite struct{}

var _ = gc.Suite(&DebugHooksCommonSuite{})

// TestHooksContext tests the behaviour of HooksContext.
func (*DebugHooksCommonSuite) TestHooksContext(c *gc.C) {
	ctx := debug.NewHooksContext("foo/8")
	c.Assert(ctx.Unit, gc.Equals, "foo/8")
	c.Assert(ctx.FlockDir, gc.Equals, "/tmp")
	ctx.FlockDir = "/var/lib/juju"
	c.Assert(ctx.ClientFileLock(), jc.SamePath, "/var/lib/juju/juju-unit-foo-8-debug-hooks")
	c.Assert(ctx.ClientExitFileLock(), jc.SamePath, "/var/lib/juju/juju-unit-foo-8-debug-hooks-exit")
}
