// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/jujuc"
)

type RelationListSuite struct {
	ContextSuite
}

var _ = gc.Suite(&RelationListSuite{})

var relationListTests = []struct {
	summary            string
	relid              int
	members0, members1 []string
	args               []string
	code               int
	out                string
}{
	{
		summary: "no default relation, no arg",
		relid:   -1,
		code:    2,
		out:     "no relation id specified",
	}, {
		summary: "no default relation, bad arg",
		relid:   -1,
		args:    []string{"-r", "bad"},
		code:    2,
		out:     `invalid value "bad" for flag -r: invalid relation id`,
	}, {
		summary: "no default relation, unknown arg",
		relid:   -1,
		args:    []string{"-r", "unknown:123"},
		code:    2,
		out:     `invalid value "unknown:123" for flag -r: unknown relation id`,
	}, {
		summary: "default relation, bad arg",
		relid:   1,
		args:    []string{"-r", "bad"},
		code:    2,
		out:     `invalid value "bad" for flag -r: invalid relation id`,
	}, {
		summary: "default relation, unknown arg",
		relid:   1,
		args:    []string{"-r", "unknown:123"},
		code:    2,
		out:     `invalid value "unknown:123" for flag -r: unknown relation id`,
	}, {
		summary: "default relation, no members",
		relid:   1,
	}, {
		summary:  "default relation, members",
		members1: []string{"foo", "bar", "baz"},
		relid:    1,
		out:      "bar\nbaz\nfoo",
	}, {
		summary:  "alternative relation, members",
		members0: []string{"pew", "pow", "paw"},
		relid:    1,
		args:     []string{"-r", "ignored:0"},
		out:      "paw\npew\npow",
	}, {
		summary: "explicit smart formatting 1",
		relid:   1,
		args:    []string{"--format", "smart"},
	}, {
		summary:  "explicit smart formatting 2",
		members1: []string{"foo", "bar", "baz"},
		relid:    1,
		args:     []string{"--format", "smart"},
		out:      "bar\nbaz\nfoo",
	}, {
		summary: "json formatting 1",
		relid:   1,
		args:    []string{"--format", "json"},
		out:     "[]",
	}, {
		summary:  "json formatting 2",
		members1: []string{"foo", "bar", "baz"},
		relid:    1,
		args:     []string{"--format", "json"},
		out:      `["bar","baz","foo"]`,
	}, {
		summary: "yaml formatting 1",
		relid:   1,
		args:    []string{"--format", "yaml"},
		out:     "[]",
	}, {
		summary:  "yaml formatting 2",
		members1: []string{"foo", "bar", "baz"},
		relid:    1,
		args:     []string{"--format", "yaml"},
		out:      "- bar\n- baz\n- foo",
	},
}

func (s *RelationListSuite) TestRelationList(c *gc.C) {
	for i, t := range relationListTests {
		c.Logf("test %d: %s", i, t.summary)
		hctx := s.GetHookContext(c, t.relid, "")
		setMembers(hctx.rels[0], t.members0)
		setMembers(hctx.rels[1], t.members1)
		com, err := jujuc.NewCommand(hctx, "relation-list")
		c.Assert(err, gc.IsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Logf(bufferString(ctx.Stderr))
		c.Assert(code, gc.Equals, t.code)
		if code == 0 {
			c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
			expect := t.out
			if expect != "" {
				expect = expect + "\n"
			}
			c.Assert(bufferString(ctx.Stdout), gc.Equals, expect)
		} else {
			c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
			expect := fmt.Sprintf(`(.|\n)*error: %s\n`, t.out)
			c.Assert(bufferString(ctx.Stderr), gc.Matches, expect)
		}
	}
}

func (s *RelationListSuite) TestRelationListHelp(c *gc.C) {
	template := `
usage: relation-list [options]
purpose: list relation units

options:
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file
-r  (= %s)
    specify a relation by id
%s`[1:]

	for relid, t := range map[int]struct {
		usage, doc string
	}{
		-1: {"", "\n-r must be specified when not in a relation hook\n"},
		0:  {"peer0:0", ""},
	} {
		c.Logf("test relid %d", relid)
		hctx := s.GetHookContext(c, relid, "")
		com, err := jujuc.NewCommand(hctx, "relation-list")
		c.Assert(err, gc.IsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, []string{"--help"})
		c.Assert(code, gc.Equals, 0)
		expect := fmt.Sprintf(template, t.usage, t.doc)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, expect)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	}
}

func setMembers(rctx *ContextRelation, members []string) {
	rctx.units = map[string]Settings{}
	for _, name := range members {
		rctx.units[name] = nil
	}
}
