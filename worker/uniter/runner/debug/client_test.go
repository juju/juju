// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug_test

import (
	"encoding/base64"
	"fmt"
	"regexp"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/worker/uniter/runner/debug"
)

type DebugHooksClientSuite struct{}

var _ = gc.Suite(&DebugHooksClientSuite{})

func (*DebugHooksClientSuite) TestClientScript(c *gc.C) {
	ctx := debug.NewHooksContext("foo/8")

	// Test the variable substitutions.
	result := debug.ClientScript(ctx, nil, "")
	// No variables left behind.
	c.Assert(result, gc.Not(gc.Matches), "(.|\n)*{unit_name}(.|\n)*")
	c.Assert(result, gc.Not(gc.Matches), "(.|\n)*{tmux_conf}(.|\n)*")
	c.Assert(result, gc.Not(gc.Matches), "(.|\n)*{entry_flock}(.|\n)*")
	c.Assert(result, gc.Not(gc.Matches), "(.|\n)*{exit_flock}(.|\n)*")
	// tmux new-session -d -s {unit_name}
	c.Assert(result, gc.Matches, fmt.Sprintf("(.|\n)*tmux attach-session -t %s(.|\n)*", regexp.QuoteMeta(ctx.Unit)))
	//) 9>{exit_flock}
	c.Assert(result, gc.Matches, fmt.Sprintf("(.|\n)*\\) 9>%s(.|\n)*", regexp.QuoteMeta(ctx.ClientExitFileLock())))
	//) 8>{entry_flock}
	c.Assert(result, gc.Matches, fmt.Sprintf("(.|\n)*\\) 8>%s(.|\n)*", regexp.QuoteMeta(ctx.ClientFileLock())))

	// nil is the same as empty slice is the same as "*".
	// Also, if "*" is present as well as a named hook,
	// it is equivalent to "*".
	c.Check(debug.ClientScript(ctx, nil, ""),
		gc.Equals, debug.ClientScript(ctx, []string{}, ""))
	c.Check(debug.ClientScript(ctx, []string{"*"}, ""),
		gc.Equals, debug.ClientScript(ctx, nil, ""))
	c.Check(debug.ClientScript(ctx, []string{"*", "something"}, ""),
		gc.Equals, debug.ClientScript(ctx, []string{"*"}, ""))

	// debug.ClientScript does not validate hook names, as it doesn't have
	// a full state API connection to determine valid relation hooks.
	// Note: jam 2020-04-01, This is a very easy to get wrong test.
	//  Without escaping the '|' it was actually just asserting that 'base64 -d' existed in the
	//  file.
	c.Check(debug.Base64HookArgs([]string{"something somethingelse"}, ""),
		gc.Equals, "aG9va3M6Ci0gc29tZXRoaW5nIHNvbWV0aGluZ2Vsc2UK")
	expected := fmt.Sprintf(
		`(.|\n)*echo "aG9va3M6Ci0gc29tZXRoaW5nIHNvbWV0aGluZ2Vsc2UK" \| base64 -d > %s(.|\n)*`,
		regexp.QuoteMeta(ctx.ClientFileLock()),
	)
	c.Assert(debug.ClientScript(ctx, []string{"something somethingelse"}, ""), gc.Matches, expected)
	expected = fmt.Sprintf(
		`(.|\n)*echo "%s" \| base64 -d > %s(.|\n)*`,
		debug.Base64HookArgs(nil, "breakpoint-string"),
		regexp.QuoteMeta(ctx.ClientFileLock()),
	)
	c.Assert(debug.ClientScript(ctx, []string{}, "breakpoint-string"),
		gc.Matches, expected)
}

func (*DebugHooksClientSuite) TestBase64HookArgsNoValues(c *gc.C) {
	// Tests of how we encode parameters for how debug-hooks will operate
	testEncodeRoundTrips(c, nil, "", map[string]interface{}{})
}

func (*DebugHooksClientSuite) TestBase64HookArgsHookList(c *gc.C) {
	// Tests of how we encode parameters for how debug-hooks will operate
	testEncodeRoundTrips(c, []string{"install", "start"}, "", map[string]interface{}{
		"hooks": []interface{}{"install", "start"},
	})
}

func (*DebugHooksClientSuite) TestBase64HookArgsDebugAt(c *gc.C) {
	// Tests of how we encode parameters for how debug-hooks will operate
	testEncodeRoundTrips(c, nil, "all,broken", map[string]interface{}{
		"debug-at": "all,broken",
	})
}

func (*DebugHooksClientSuite) TestBase64HookArgsBoth(c *gc.C) {
	// Tests of how we encode parameters for how debug-hooks will operate
	testEncodeRoundTrips(c, []string{"db-relation-changed", "stop"}, "brokepoint",
		map[string]interface{}{
			"hooks":    []interface{}{"db-relation-changed", "stop"},
			"debug-at": "brokepoint",
		})
}

func testEncodeRoundTrips(c *gc.C, match []string, debugAt string, decoded map[string]interface{}) {
	base64Args := debug.Base64HookArgs(match, debugAt)
	args := decodeArgs(c, base64Args)
	c.Check(args, gc.DeepEquals, decoded)
}

func decodeArgs(c *gc.C, base64Args string) map[string]interface{} {
	c.Assert(base64Args, gc.Not(gc.Equals), "")
	yamlArgs, err := base64.StdEncoding.DecodeString(base64Args)
	c.Assert(err, jc.ErrorIsNil)
	var decoded map[string]interface{}
	c.Assert(goyaml.Unmarshal(yamlArgs, &decoded), jc.ErrorIsNil)
	return decoded
}
