// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/jujuctesting"
)

var _ = gc.Suite(&ActionSetSuite{})

type ActionSetSuite struct {
	jujuctesting.ContextSuite
}

type actionSettingContext struct {
	jujuc.Context
	commands [][]string
}

func (a *actionSettingContext) UpdateActionResults(keys []string, value interface{}) error {
	if a.commands == nil {
		a.commands = make([][]string, 0)
	}

	a.commands = append(a.commands, append(keys, fmt.Sprintf("%v", value)))
	return nil
}

type nonActionSettingContext struct {
	jujuc.Context
}

func (a *nonActionSettingContext) UpdateActionResults(keys []string, value interface{}) error {
	return fmt.Errorf("not running an action")
}

func (s *ActionSetSuite) TestActionSetOnNonActionContextFails(c *gc.C) {
	hctx := &nonActionSettingContext{}
	com, err := jujuc.NewCommand(hctx, "action-set")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"oops=nope"})
	c.Check(code, gc.Equals, 1)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "")
	expect := fmt.Sprintf(`(\n)*ERROR %s\n`, "not running an action")
	c.Check(bufferString(ctx.Stderr), gc.Matches, expect)
}

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
		errMsg:  "ERROR argument \"result\" must be of the form key...=value\n",
		code:    2,
	}, {
		summary: "invalid keys are an error",
		command: []string{"result-Value=5"},
		errMsg:  "ERROR key \"result-Value\" must start and end with lowercase alphanumeric, and contain only lowercase alphanumeric, hyphens and periods\n",
		code:    2,
	}, {
		summary: "reserved key is an error",
		command: []string{"stdout=foo"},
		errMsg:  "ERROR cannot set reserved action key \"stdout\"\n",
		code:    2,
	}, {
		summary: "empty values are not an error",
		command: []string{"result="},
		expected: [][]string{
			{"result", ""},
		},
	}, {
		summary: "a response of one key to one value",
		command: []string{"outfile=foo.bz2"},
		expected: [][]string{
			{"outfile", "foo.bz2"},
		},
	}, {
		summary: "two keys, two values",
		command: []string{"outfile=foo.bz2", "size=10G"},
		expected: [][]string{
			{"outfile", "foo.bz2"},
			{"size", "10G"},
		},
	}, {
		summary: "multiple = are ok",
		command: []string{"outfile=foo=bz2"},
		expected: [][]string{
			{"outfile", "foo=bz2"},
		},
	}, {
		summary: "several interleaved values",
		command: []string{"outfile.name=foo.bz2",
			"outfile.kind.util=bzip2",
			"outfile.kind.ratio=high"},
		expected: [][]string{
			{"outfile", "name", "foo.bz2"},
			{"outfile", "kind", "util", "bzip2"},
			{"outfile", "kind", "ratio", "high"},
		},
	}, {
		summary: "conflicting simple values",
		command: []string{"util=bzip2", "util=5"},
		expected: [][]string{
			{"util", "bzip2"},
			{"util", "5"},
		},
	}, {
		summary: "conflicted map spec: {map1:{key:val}} vs {map1:val2}",
		command: []string{"map1.key=val", "map1=val"},
		expected: [][]string{
			{"map1", "key", "val"},
			{"map1", "val"},
		},
	}}

	for i, t := range actionSetTests {
		c.Logf("test %d: %s", i, t.summary)
		hctx := &actionSettingContext{}
		com, err := jujuc.NewCommand(hctx, "action-set")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		c.Logf("  command list: %#v", t.command)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.command)
		c.Check(code, gc.Equals, t.code)
		c.Check(bufferString(ctx.Stderr), gc.Equals, t.errMsg)
		c.Check(hctx.commands, jc.DeepEquals, t.expected)
	}
}
