// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type RelationListSuite struct {
	relationSuite
}

func TestRelationListSuite(t *stdtesting.T) { tc.Run(t, &RelationListSuite{}) }

var relationListTests = []struct {
	summary            string
	relid              int
	members0, members1 []string
	remoteAppName      string
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
		out:     `invalid value "bad" for option -r: invalid relation id`,
	}, {
		summary: "no default relation, unknown arg",
		relid:   -1,
		args:    []string{"-r", "unknown:123"},
		code:    2,
		out:     `invalid value "unknown:123" for option -r: relation not found`,
	}, {
		summary: "default relation, bad arg",
		relid:   1,
		args:    []string{"-r", "bad"},
		code:    2,
		out:     `invalid value "bad" for option -r: invalid relation id`,
	}, {
		summary: "default relation, unknown arg",
		relid:   1,
		args:    []string{"-r", "unknown:123"},
		code:    2,
		out:     `invalid value "unknown:123" for option -r: relation not found`,
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
	}, {
		summary:       "remote application for relation",
		members1:      []string{}, // relation established but all units removed
		relid:         1,
		remoteAppName: "galaxy",
		args:          []string{"--app"},
		out:           "galaxy",
	},
}

func (s *RelationListSuite) TestRelationList(c *tc.C) {
	for i, t := range relationListTests {
		c.Logf("test %d: %s", i, t.summary)
		hctx, info := s.newHookContext(t.relid, "", t.remoteAppName)
		info.setRelations(0, t.members0)
		info.setRelations(1, t.members1)
		c.Logf("%#v %#v", info.rels[t.relid], t.members1)
		com, err := jujuc.NewCommand(hctx, "relation-list")
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Logf("%s", bufferString(ctx.Stderr))
		c.Assert(code, tc.Equals, t.code)
		if code == 0 {
			c.Check(bufferString(ctx.Stderr), tc.Equals, "")
			expect := t.out
			if expect != "" {
				expect += "\n"
			}
			c.Check(bufferString(ctx.Stdout), tc.Equals, expect)
		} else {
			c.Check(bufferString(ctx.Stdout), tc.Equals, "")
			expect := fmt.Sprintf(`(.|\n)*ERROR %s\n`, t.out)
			c.Check(bufferString(ctx.Stderr), tc.Matches, expect)
		}
	}
}

func (s *RelationListSuite) TestRelationListHelp(c *tc.C) {
	for relid, t := range map[int]struct {
		usage, doc string
	}{
		-1: {"", "\nDetails:\n-r must be specified when not in a relation hook\n"},
		0:  {"peer0:0", ""},
	} {
		c.Logf("test relid %d", relid)
		hctx, _ := s.newHookContext(relid, "", "")
		com, err := jujuc.NewCommand(hctx, "relation-list")
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--help"})
		c.Assert(code, tc.Equals, 0)
		c.Assert(strings.Contains(bufferString(ctx.Stdout), t.usage), tc.IsTrue)
	}
}
