// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug_test

import (
	"testing"

	. "launchpad.net/gocheck"

	unitdebug "launchpad.net/juju-core/worker/uniter/debug"
)

type DebugHooksCommonSuite struct{}

var _ = Suite(&DebugHooksCommonSuite{})

func TestPackage(t *testing.T) {
	TestingT(t)
}

// TestCommonScript tests the behaviour of DebugHooksContext.
func (*DebugHooksCommonSuite) TestDebugHooksContext(c *C) {
	ctx := unitdebug.NewDebugHooksContext("foo/8")
	c.Assert(ctx.Unit, Equals, "foo/8")
	c.Assert(ctx.FlockDir, Equals, "/tmp")
	ctx.FlockDir = "/var/lib/juju"
	c.Assert(ctx.ClientFileLock(), Equals, "/var/lib/juju/juju-unit-foo-8-debug-hooks")
	c.Assert(ctx.ClientExitFileLock(), Equals, "/var/lib/juju/juju-unit-foo-8-debug-hooks-exit")
}
