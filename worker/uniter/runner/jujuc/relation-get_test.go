// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	jujuctesting "github.com/juju/juju/worker/uniter/runner/jujuc/testing"
)

type RelationGetSuite struct {
	relationSuite
}

var _ = gc.Suite(&RelationGetSuite{})

func (s *RelationGetSuite) newHookContext(relid int, remote string) (jujuc.Context, *relationInfo) {
	hctx, info := s.relationSuite.newHookContext(relid, remote)
	info.rels[0].Units["u/0"]["private-address"] = "foo: bar\n"
	info.rels[1].SetRelated("m/0", jujuctesting.Settings{"pew": "pew\npew\n"})
	info.rels[1].SetRelated("u/1", jujuctesting.Settings{"value": "12345"})
	return hctx, info
}

var relationGetTests = []struct {
	summary  string
	relid    int
	unit     string
	args     []string
	code     int
	out      string
	checkctx func(*gc.C, *cmd.Context)
}{
	{
		summary: "no default relation",
		relid:   -1,
		code:    2,
		out:     `no relation id specified`,
	}, {
		summary: "explicit relation, not known",
		relid:   -1,
		code:    2,
		args:    []string{"-r", "burble:123"},
		out:     `invalid value "burble:123" for flag -r: unknown relation id`,
	}, {
		summary: "default relation, no unit chosen",
		relid:   1,
		code:    2,
		out:     `no unit id specified`,
	}, {
		summary: "explicit relation, no unit chosen",
		relid:   -1,
		code:    2,
		args:    []string{"-r", "burble:1"},
		out:     `no unit id specified`,
	}, {
		summary: "missing key",
		relid:   1,
		unit:    "m/0",
		args:    []string{"ker-plunk"},
	}, {
		summary: "missing unit",
		relid:   1,
		unit:    "bad/0",
		code:    1,
		out:     `unknown unit bad/0`,
	}, {
		summary: "all keys with explicit non-member",
		relid:   1,
		args:    []string{"-", "u/1"},
		out:     `value: "12345"`,
	}, {
		summary: "specific key with implicit member",
		relid:   1,
		unit:    "m/0",
		args:    []string{"pew"},
		out:     "pew\npew\n",
	}, {
		summary: "specific key with explicit member",
		relid:   1,
		args:    []string{"pew", "m/0"},
		out:     "pew\npew\n",
	}, {
		summary: "specific key with explicit non-member",
		relid:   1,
		args:    []string{"value", "u/1"},
		out:     "12345",
	}, {
		summary: "specific key with explicit local",
		relid:   0,
		args:    []string{"private-address", "u/0"},
		out:     "foo: bar\n",
	}, {
		summary: "all keys with implicit member",
		relid:   1,
		unit:    "m/0",
		out:     "pew: |\n  pew\n  pew",
	}, {
		summary: "all keys with explicit member",
		relid:   1,
		args:    []string{"-", "m/0"},
		out:     "pew: |\n  pew\n  pew",
	}, {
		summary: "all keys with explicit local",
		relid:   0,
		args:    []string{"-", "u/0"},
		out:     "private-address: |\n  foo: bar",
	}, {
		summary: "explicit smart formatting 1",
		relid:   1,
		unit:    "m/0",
		args:    []string{"--format", "smart"},
		out:     "pew: |\n  pew\n  pew",
	}, {
		summary: "explicit smart formatting 2",
		relid:   1,
		unit:    "m/0",
		args:    []string{"pew", "--format", "smart"},
		out:     "pew\npew\n",
	}, {
		summary: "explicit smart formatting 3",
		relid:   1,
		args:    []string{"value", "u/1", "--format", "smart"},
		out:     "12345",
	}, {
		summary: "explicit smart formatting 4",
		relid:   1,
		args:    []string{"missing", "u/1", "--format", "smart"},
		out:     "",
	},
}

func (s *RelationGetSuite) TestRelationGet(c *gc.C) {
	for i, t := range relationGetTests {
		c.Logf("test %d: %s", i, t.summary)
		hctx, _ := s.newHookContext(t.relid, t.unit)
		com, err := jujuc.NewCommand(hctx, cmdString("relation-get"))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Check(code, gc.Equals, t.code)
		if code == 0 {
			c.Check(bufferString(ctx.Stderr), gc.Equals, "")
			expect := t.out
			if expect != "" {
				expect = expect + "\n"
			}
			c.Check(bufferString(ctx.Stdout), gc.Equals, expect)
		} else {
			c.Check(bufferString(ctx.Stdout), gc.Equals, "")
			expect := fmt.Sprintf(`(.|\n)*error: %s\n`, t.out)
			c.Check(bufferString(ctx.Stderr), gc.Matches, expect)
		}
	}
}

var relationGetFormatTests = []struct {
	summary string
	relid   int
	unit    string
	args    []string
	out     interface{}
}{
	{
		summary: "formatting 1",
		relid:   1,
		unit:    "m/0",
		out:     map[string]interface{}{"pew": "pew\npew\n"},
	}, {
		summary: "formatting 2",
		relid:   1,
		unit:    "m/0",
		args:    []string{"pew"},
		out:     "pew\npew\n",
	}, {
		summary: "formatting 3",
		relid:   1,
		args:    []string{"value", "u/1"},
		out:     "12345",
	}, {
		summary: "formatting 4",
		relid:   1,
		args:    []string{"missing", "u/1"},
		out:     nil,
	},
}

func (s *RelationGetSuite) TestRelationGetFormat(c *gc.C) {
	testFormat := func(format string, checker gc.Checker) {
		for i, t := range relationGetFormatTests {
			c.Logf("test %d: %s %s", i, format, t.summary)
			hctx, _ := s.newHookContext(t.relid, t.unit)
			com, err := jujuc.NewCommand(hctx, cmdString("relation-get"))
			c.Assert(err, jc.ErrorIsNil)
			ctx := testing.Context(c)
			args := append(t.args, "--format", format)
			code := cmd.Main(com, ctx, args)
			c.Check(code, gc.Equals, 0)
			c.Check(bufferString(ctx.Stderr), gc.Equals, "")
			stdout := bufferString(ctx.Stdout)
			c.Check(stdout, checker, t.out)
		}
	}
	testFormat("yaml", jc.YAMLEquals)
	testFormat("json", jc.JSONEquals)
}

var helpTemplate = `
usage: %s
purpose: get relation settings

options:
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file
-r, --relation  (= %s)
    specify a relation by id

relation-get prints the value of a unit's relation setting, specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.
%s`[1:]

var relationGetHelpTests = []struct {
	summary string
	relid   int
	unit    string
	usage   string
	rel     string
}{
	{
		summary: "no default relation",
		relid:   -1,
		usage:   "relation-get [options] <key> <unit id>",
	}, {
		summary: "no default unit",
		relid:   1,
		usage:   "relation-get [options] <key> <unit id>",
		rel:     "peer1:1",
	}, {
		summary: "default unit",
		relid:   1,
		unit:    "any/1",
		usage:   `relation-get [options] [<key> [<unit id>]]`,
		rel:     "peer1:1",
	},
}

func (s *RelationGetSuite) TestHelp(c *gc.C) {
	for i, t := range relationGetHelpTests {
		c.Logf("test %d", i)
		hctx, _ := s.newHookContext(t.relid, t.unit)
		com, err := jujuc.NewCommand(hctx, cmdString("relation-get"))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, []string{"--help"})
		c.Assert(code, gc.Equals, 0)
		unitHelp := ""
		if t.unit != "" {
			unitHelp = fmt.Sprintf("Current default unit id is %q.\n", t.unit)
		}
		expect := fmt.Sprintf(helpTemplate, t.usage, t.rel, unitHelp)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, expect)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	}
}

func (s *RelationGetSuite) TestOutputPath(c *gc.C) {
	hctx, _ := s.newHookContext(1, "m/0")
	com, err := jujuc.NewCommand(hctx, cmdString("relation-get"))
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--output", "some-file", "pew"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "pew\npew\n\n")
}
