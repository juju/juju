package server_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/jujuc/server"
)

type RelationListSuite struct {
	HookContextSuite
}

var _ = Suite(&RelationListSuite{})

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
		out:     "no relation specified",
	}, {
		summary: "no default relation, bad arg",
		relid:   -1,
		args:    []string{"bad"},
		code:    2,
		out:     `invalid relation id`,
	}, {
		summary: "no default relation, unknown arg",
		relid:   -1,
		args:    []string{"unknown:123"},
		code:    2,
		out:     "unknown relation id",
	}, {
		summary: "default relation, bad arg",
		relid:   1,
		args:    []string{"bad"},
		code:    2,
		out:     `invalid relation id`,
	}, {
		summary: "default relation, unknown arg",
		relid:   1,
		args:    []string{"unknown:123"},
		code:    2,
		out:     "unknown relation id",
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
		args:     []string{"ignored:0"},
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
		out:     "null",
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

func (s *RelationListSuite) TestRelationList(c *C) {
	for i, t := range relationListTests {
		c.Logf("test %d: %s", i, t.summary)
		hctx := s.GetHookContext(c, t.relid, "")
		setMembers(hctx.Relations[0], t.members0)
		setMembers(hctx.Relations[1], t.members1)
		com, err := hctx.NewCommand("relation-list")
		c.Assert(err, IsNil)
		ctx := dummyContext(c)
		code := cmd.Main(com, ctx, t.args)
		c.Logf(bufferString(ctx.Stderr))
		c.Assert(code, Equals, t.code)
		if code == 0 {
			c.Assert(bufferString(ctx.Stderr), Equals, "")
			expect := t.out
			if expect != "" {
				expect = expect + "\n"
			}
			c.Assert(bufferString(ctx.Stdout), Equals, expect)
		} else {
			c.Assert(bufferString(ctx.Stdout), Equals, "")
			expect := fmt.Sprintf(`(.|\n)*error: %s\n`, t.out)
			c.Assert(bufferString(ctx.Stderr), Matches, expect)
		}
	}
}

func (s *RelationListSuite) TestRelationListHelp(c *C) {
	template := `
usage: relation-list [options] %s
purpose: list relation units

options:
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file
`[1:]

	for relid, usage := range map[int]string{-1: "<id>", 0: "[<id (= peer0:0)>]"} {
		c.Logf("test relid %d", relid)
		hctx := s.GetHookContext(c, relid, "")
		com, err := hctx.NewCommand("relation-list")
		c.Assert(err, IsNil)
		ctx := dummyContext(c)
		code := cmd.Main(com, ctx, []string{"--help"})
		c.Assert(code, Equals, 0)
		c.Assert(bufferString(ctx.Stdout), Equals, "")
		expect := fmt.Sprintf(template, usage)
		c.Assert(bufferString(ctx.Stderr), Equals, expect)

	}
}

func setMembers(rctx *server.RelationContext, members []string) {
	m := server.SettingsMap{}
	for _, name := range members {
		m[name] = nil
	}
	rctx.SetMembers(m)
}
