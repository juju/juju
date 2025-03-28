// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/jujuctesting"
)

type RelationGetSuite struct {
	relationSuite
}

var _ = gc.Suite(&RelationGetSuite{})

func (s *RelationGetSuite) newHookContext(relid int, remote string, app string) (jujuc.Context, *relationInfo) {
	hctx, info := s.relationSuite.newHookContext(relid, remote, app)
	info.rels[0].Units["u/0"]["private-address"] = "foo: bar\n"
	info.rels[1].SetRelated("m/0", jujuctesting.Settings{"pew": "pew\npew\n"})
	info.rels[1].SetRelated("u/1", jujuctesting.Settings{"value": "12345"})
	return hctx, info
}

var relationGetTests = []struct {
	summary     string
	relid       int
	unit        string
	args        []string
	code        int
	out         string
	key         string
	application bool
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
		out:     `invalid value "burble:123" for option -r: relation not found`,
	}, {
		summary: "default relation, no unit chosen",
		relid:   1,
		code:    2,
		out:     `no unit or application specified`,
	}, {
		summary: "explicit relation, no unit chosen",
		relid:   -1,
		code:    2,
		args:    []string{"-r", "burble:1"},
		out:     `no unit or application specified`,
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
		hctx, _ := s.newHookContext(t.relid, t.unit, "")
		com, err := jujuc.NewHookCommand(hctx, "relation-get")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.args)
		c.Check(code, gc.Equals, t.code)
		if code == 0 {
			c.Check(bufferString(ctx.Stderr), gc.Equals, "")
			expect := t.out
			if len(expect) > 0 {
				expect += "\n"
			}
			c.Check(bufferString(ctx.Stdout), gc.Equals, expect)
		} else {
			c.Check(bufferString(ctx.Stdout), gc.Equals, "")
			expect := fmt.Sprintf(`(.|\n)*ERROR %s\n`, t.out)
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
			hctx, _ := s.newHookContext(t.relid, t.unit, "")
			com, err := jujuc.NewHookCommand(hctx, "relation-get")
			c.Assert(err, jc.ErrorIsNil)
			ctx := cmdtesting.Context(c)
			args := append(t.args, "--format", format)
			code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, args)
			c.Check(code, gc.Equals, 0)
			c.Check(bufferString(ctx.Stderr), gc.Equals, "")
			stdout := bufferString(ctx.Stdout)
			c.Check(stdout, checker, t.out)
		}
	}
	testFormat("yaml", jc.YAMLEquals)
	testFormat("json", jc.JSONEquals)
}

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
		hctx, _ := s.newHookContext(t.relid, t.unit, "")
		com, err := jujuc.NewHookCommand(hctx, "relation-get")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--help"})
		c.Assert(code, gc.Equals, 0)
		if t.unit != "" {
			unitHelp := fmt.Sprintf("Current default unit id is %q.\n", t.unit)
			c.Assert(strings.Contains(bufferString(ctx.Stdout), unitHelp), jc.IsTrue)
		} else {
			c.Assert(strings.Contains(bufferString(ctx.Stdout), "Current default unit id"), jc.IsFalse)
		}
	}
}

func (s *RelationGetSuite) TestOutputPath(c *gc.C) {
	hctx, _ := s.newHookContext(1, "m/0", "")
	com, err := jujuc.NewHookCommand(hctx, "relation-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--output", "some-file", "pew"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	content, err := os.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "pew\npew\n\n")
}

type relationGetInitTest struct {
	summary     string
	ctxrelid    int
	ctxunit     string
	ctxapp      string
	args        []string
	err         string
	relid       int
	key         string
	unit        string
	application bool
}

func (t relationGetInitTest) log(c *gc.C, i int) {
	var summary string
	if t.summary != "" {
		summary = " - " + t.summary
	}
	c.Logf("test %d%s", i, summary)
}

func (t relationGetInitTest) init(c *gc.C, s *RelationGetSuite) (cmd.Command, []string) {
	args := make([]string, len(t.args))
	copy(args, t.args)

	hctx, _ := s.newHookContext(t.ctxrelid, t.ctxunit, t.ctxapp)
	com, err := jujuc.NewHookCommand(hctx, "relation-get")
	c.Assert(err, jc.ErrorIsNil)

	return com, args
}

func (t relationGetInitTest) check(c *gc.C, com cmd.Command, err error) {
	if t.err == "" {
		if !c.Check(err, jc.ErrorIsNil) {
			return
		}

		rset := com.(*jujuc.RelationGetCommand)
		c.Check(rset.RelationId, gc.Equals, t.relid)
		c.Check(rset.Key, gc.Equals, t.key)
		c.Check(rset.UnitOrAppName, gc.Equals, t.unit)
		c.Check(rset.Application, gc.Equals, t.application)
	} else {
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

var relationGetInitTests = []relationGetInitTest{
	{
		summary:  "no relation id",
		ctxrelid: -1,
		err:      `no relation id specified`,
	}, {
		summary:  "invalid relation id",
		ctxrelid: -1,
		args:     []string{"-r", "one"},
		err:      `invalid value "one" for option -r: invalid relation id`,
	}, {
		summary:  "invalid relation id with builtin context relation id",
		ctxrelid: 1,
		args:     []string{"-r", "one"},
		err:      `invalid value "one" for option -r: invalid relation id`,
	}, {
		summary:  "relation not found",
		ctxrelid: -1,
		args:     []string{"-r", "2"},
		err:      `invalid value "2" for option -r: relation not found`,
	}, {
		summary:  "-r overrides context relation id",
		ctxrelid: 1,
		ctxunit:  "u/0",
		unit:     "u/0",
		args:     []string{"-r", "ignored:0"},
		relid:    0,
	}, {
		summary:  "key=value for relation-get (maybe should be invalid?)",
		ctxrelid: 1,
		relid:    1,
		ctxunit:  "u/0",
		unit:     "u/0",
		args:     []string{"key=value"},
		key:      "key=value",
	}, {
		summary:  "key supplied",
		ctxrelid: 1,
		relid:    1,
		ctxunit:  "u/0",
		unit:     "u/0",
		args:     []string{"key"},
		key:      "key",
	}, {
		summary: "magic key supplied",
		ctxunit: "u/0",
		unit:    "u/0",
		args:    []string{"-"},
		key:     "",
	}, {
		summary: "override ctxunit with explicit unit",
		ctxunit: "u/0",
		args:    []string{"key", "u/1"},
		key:     "key",
		unit:    "u/1",
	}, {
		summary: "magic key with unit",
		ctxunit: "u/0",
		args:    []string{"-", "u/1"},
		key:     "",
		unit:    "u/1",
	}, {
		summary:     "supply --app will use context app",
		ctxunit:     "u/0",
		ctxapp:      "u",
		args:        []string{"--app"},
		application: true,
		unit:        "u",
	}, {
		summary:     "supply --app and app name",
		ctxunit:     "u/0",
		args:        []string{"--app", "-", "mysql"},
		application: true,
		unit:        "mysql",
	}, {
		summary:     "--app plus unit/0 name passes in app name",
		ctxunit:     "u/0",
		ctxapp:      "u",
		args:        []string{"--app", "-", "mysql/0"},
		application: true,
		unit:        "mysql",
	}, {
		summary:     "--app without context app and no args is an error",
		ctxapp:      "",
		args:        []string{"--app"},
		application: false,
		err:         `no unit or application specified`,
	}, {
		summary:     "default with no context unit but a context app is --app",
		ctxunit:     "",
		ctxapp:      "u",
		unit:        "u",
		application: true,
	}, {
		summary:     "app name in context but overridden by args",
		ctxunit:     "",
		ctxapp:      "u",
		unit:        "mysql/0",
		args:        []string{"-", "mysql/0"},
		application: false,
	}, {
		summary: "extra arguments",
		ctxunit: "u/0",
		ctxapp:  "u",
		args:    []string{"-", "--app", "mysql", "args"},
		err:     `unrecognized args: \["args"\]`,
	}, {
		summary: "app name in context but overridden by app args",
		ctxunit: "",
		ctxapp:  "u",
		unit:    "mysql",
		args:    []string{"-", "--app", "mysql"},
		// This doesn't get auto set if we didn't pull it from the context
		application: true,
	}, {
		summary: "application name with no --app",
		ctxunit: "u/0",
		ctxapp:  "u",
		args:    []string{"-", "mysql"},
		err:     `expected unit name, got application name "mysql"`,
	}, {
		summary: "invalid unit name",
		ctxunit: "u/0",
		ctxapp:  "u",
		args:    []string{"-", "unit//0"},
		err:     `invalid unit name "unit//0"`,
	},
}

func (s *RelationGetSuite) TestInit(c *gc.C) {
	for i, t := range relationGetInitTests {
		t.log(c, i)
		com, args := t.init(c, s)

		err := cmdtesting.InitCommand(com, args)
		t.check(c, com, err)
	}
}
