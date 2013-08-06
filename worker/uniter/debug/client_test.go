// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug_test

import (
	"fmt"
	"regexp"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/worker/uniter/debug"
)

type DebugHooksClientSuite struct{}

var _ = Suite(&DebugHooksClientSuite{})

func (*DebugHooksClientSuite) TestClientScript(c *C) {
	ctx := debug.NewHooksContext("foo/8")

	// Test the variable substitutions.
	result := debug.ClientScript(ctx, nil)
	// No variables left behind.
	c.Assert(result, Matches, "[^{}]*")
	// tmux new-session -d -s {unit_name}
	c.Assert(result, Matches, fmt.Sprintf("(.|\n)*tmux new-session -s %s(.|\n)*", regexp.QuoteMeta(ctx.Unit)))
	//) 9>{exit_flock}
	c.Assert(result, Matches, fmt.Sprintf("(.|\n)*\\) 9>%s(.|\n)*", regexp.QuoteMeta(ctx.ClientExitFileLock())))
	//) 8>{entry_flock}
	c.Assert(result, Matches, fmt.Sprintf("(.|\n)*\\) 8>%s(.|\n)*", regexp.QuoteMeta(ctx.ClientFileLock())))

	// nil is the same as empty slice is the same as "*".
	// Also, if "*" is present as well as a named hook,
	// it is equivalent to "*".
	c.Assert(debug.ClientScript(ctx, nil), Equals, debug.ClientScript(ctx, []string{}))
	c.Assert(debug.ClientScript(ctx, []string{"*"}), Equals, debug.ClientScript(ctx, nil))
	c.Assert(debug.ClientScript(ctx, []string{"*", "something"}), Equals, debug.ClientScript(ctx, []string{"*"}))

	// debug.ClientScript does not validate hook names, as it doesn't have
	// a full state API connection to determine valid relation hooks.
	expected := fmt.Sprintf(
		`(.|\n)*echo "aG9va3M6Ci0gc29tZXRoaW5nIHNvbWV0aGluZ2Vsc2UK" | base64 -d > %s(.|\n)*`,
		regexp.QuoteMeta(ctx.ClientFileLock()),
	)
	c.Assert(debug.ClientScript(ctx, []string{"something somethingelse"}), Matches, expected)
}
