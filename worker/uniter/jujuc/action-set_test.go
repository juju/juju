// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/jujuc"
)

type ActionSetSuite struct {
	ContextSuite
}

type actionTestContext struct {
	jujuc.Context
	commands [][]string
}

func (a *actionTestContext) UpdateActionResults(keys []string, value string) {
	if a.commands == nil {
		a.commands = make([][]string, 0)
	}

	a.commands = append(a.commands, append(keys, value))
}

var _ = gc.Suite(&ActionSetSuite{})

func (s *ActionSetSuite) TestActionSet(c *gc.C) {
	var actionSetTests = []struct {
		summary  string
		command  []string
		expected [][]string
		errMsg   string
		code     int
	}{{
		summary: "bare value(s) are an Init error",
		command: []string{"result"},
		errMsg:  "error: argument \"result\" must be of the form key...=value\n",
		code:    2,
	}, {
		summary: "a response of one key to one value",
		command: []string{"outfile=foo.bz2"},
		expected: [][]string{
			[]string{"outfile", "foo.bz2"},
		},
	}, {
		summary: "two keys, two values",
		command: []string{"outfile=foo.bz2", "size=10G"},
		expected: [][]string{
			[]string{"outfile", "foo.bz2"},
			[]string{"size", "10G"},
		},
	}, {
		summary: "multiple = are ok",
		command: []string{"outfile=foo=bz2"},
		expected: [][]string{
			[]string{"outfile", "foo=bz2"},
		},
	}, {
		summary: "several interleaved values",
		command: []string{"outfile.name=foo.bz2",
			"outfile.kind.util=bzip2",
			"outfile.kind.ratio=high"},
		expected: [][]string{
			[]string{"outfile", "name", "foo.bz2"},
			[]string{"outfile", "kind", "util", "bzip2"},
			[]string{"outfile", "kind", "ratio", "high"},
		},
	}, {
		summary: "conflicting simple values",
		command: []string{"util=bzip2", "util=5"},
		expected: [][]string{
			[]string{"util", "bzip2"},
			[]string{"util", "5"},
		},
	}, {
		summary: "conflicted map spec: {map1:{key:val}} vs {map1:val2}",
		command: []string{"map1.key=val", "map1=val"},
		expected: [][]string{
			[]string{"map1", "key", "val"},
			[]string{"map1", "val"},
		},
	}}

	for i, t := range actionSetTests {
		c.Logf("test %d: %s", i, t.summary)
		hctx := &actionTestContext{}
		com, err := jujuc.NewCommand(hctx, "action-set")
		c.Assert(err, gc.IsNil)
		ctx := testing.Context(c)
		c.Logf("  command list: %#v", t.command)
		code := cmd.Main(com, ctx, t.command)
		c.Check(code, gc.Equals, t.code)
		c.Check(bufferString(ctx.Stderr), gc.Equals, t.errMsg)
		c.Check(hctx.commands, jc.DeepEquals, t.expected)
	}
}

func (s *ActionSetSuite) TestHelp(c *gc.C) {
	hctx := &actionTestContext{}
	com, err := jujuc.NewCommand(hctx, "action-set")
	c.Assert(err, gc.IsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `usage: action-set <key>=<value> [<key>=<value> ...]
purpose: set action results

action-set adds the given values to the results map of the Action.  This map
is returned to the user after the completion of the Action.

Example usage:
 action-set outfile.size=10G
 action-set foo.bar=2
 action-set foo.baz.val=3
 action-set foo.bar.zab=4
 action-set foo.baz=1

 will yield:

 outfile:
   size: "10G"
 foo:
   bar:
     zab: "4"
   baz: "1"
`)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}
