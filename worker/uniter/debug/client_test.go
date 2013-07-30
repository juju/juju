// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug_test

import (
	"fmt"
	"regexp"

	. "launchpad.net/gocheck"

	unitdebug "launchpad.net/juju-core/worker/uniter/debug"
)

type DebugHooksClientSuite struct{}

var _ = Suite(&DebugHooksClientSuite{})

// TestClientScript tests the behaviour of DebugHooksContext.ClientScript.
func (*DebugHooksClientSuite) TestClientScript(c *C) {
	ctx := unitdebug.NewDebugHooksContext("foo/8")

	// Test the variable substitutions.
	result := ctx.ClientScript(nil)
	// No variables left behind.
	c.Assert(result, Matches, "[^{}]*")
	// tmux new-session -d -s {unit_name}
	c.Assert(result, Matches, fmt.Sprintf("(.|\n)*tmux new-session -d -s %s(.|\n)*", regexp.QuoteMeta(ctx.Unit)))
	//) 9>{exit_flock}
	c.Assert(result, Matches, fmt.Sprintf("(.|\n)*\\) 9>%s(.|\n)*", regexp.QuoteMeta(ctx.ClientExitFileLock())))
	//) 8>{entry_flock}
	c.Assert(result, Matches, fmt.Sprintf("(.|\n)*\\) 8>%s(.|\n)*", regexp.QuoteMeta(ctx.ClientFileLock())))

	// nil is the same as empty slice is the same as "*".
	// Also, if "*" is present as well as a named hook,
	// it is equivalent to "*".
	c.Assert(ctx.ClientScript(nil), Equals, ctx.ClientScript([]string{}))
	c.Assert(ctx.ClientScript([]string{"*"}), Equals, ctx.ClientScript(nil))
	c.Assert(ctx.ClientScript([]string{"*", "something"}), Equals, ctx.ClientScript([]string{"*"}))

	// ClientScript does not validate hook names, as it doesn't have
	// a full state API connection to determine valid relation hooks.
	expected := fmt.Sprintf(
		`(.|\n)*echo "something somethingelse" > %s(.|\n)*`,
		regexp.QuoteMeta(ctx.ClientFileLock()),
	)
	c.Assert(ctx.ClientScript([]string{"something somethingelse"}), Matches, expected)
}
