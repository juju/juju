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
)

type RelationSetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&RelationSetSuite{})

var helpTests = []struct {
	relid  int
	expect string
}{{-1, ""}, {0, "peer0:0"}}

func (s *RelationSetSuite) TestHelp(c *gc.C) {
	for i, t := range helpTests {
		c.Logf("test %d", i)
		hctx := s.GetHookContext(c, t.relid, "")
		com, err := jujuc.NewCommand(hctx, cmdString("relation-set"))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, []string{"--help"})
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, fmt.Sprintf(`
usage: relation-set [options] key=value [key=value ...]
purpose: set relation settings

options:
--file (= "")
    file containing key-value pairs
--format (= "")
    deprecated format flag
-r, --relation  (= %s)
    specify a relation by id

"relation-set" writes the local unit's settings for some relation.
If no relation is specified then the current relation is used. The
setting values are not inspected and are stored as strings. Setting
an empty string causes the setting to be removed. Duplicate settings
are not allowed.

The --file option should be used when one or more key-value pairs are
too long to fit within the command length limit of the shell or
operating system. The file should contain key-value pairs in the same
format as on the commandline. They may also span multiple lines. Blank
lines and lines starting with # are ignored. Settings in the file will
be overridden by any duplicate key-value arguments.
`[1:], t.expect))
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	}
}

type relationSetInitTest struct {
	summary  string
	ctxrelid int
	args     []string
	content  string
	err      string
	relid    int
	settings map[string]string
}

func (t relationSetInitTest) log(c *gc.C, i int) {
	var summary string
	if t.summary != "" {
		summary = " - " + t.summary
	}
	c.Logf("test %d%s", i, summary)
}

func (t relationSetInitTest) filename() (string, int) {
	for i, arg := range t.args {
		next := i + 1
		if arg == "--file" && next < len(t.args) {
			return t.args[next], next
		}
	}
	return "", -1
}

func (t relationSetInitTest) init(c *gc.C, s *RelationSetSuite) (cmd.Command, []string) {
	args := make([]string, len(t.args))
	copy(args, t.args)
	if filename, i := t.filename(); filename != "" {
		filename = filepath.Join(c.MkDir(), filename)
		args[i] = filename
		err := ioutil.WriteFile(filename, []byte(t.content), 0644)
		c.Assert(err, jc.ErrorIsNil)
	}

	hctx := s.GetHookContext(c, t.ctxrelid, "")
	com, err := jujuc.NewCommand(hctx, cmdString("relation-set"))
	c.Assert(err, jc.ErrorIsNil)

	return com, args
}

func (t relationSetInitTest) check(c *gc.C, com cmd.Command, err error) {
	if t.err == "" {
		if !c.Check(err, jc.ErrorIsNil) {
			return
		}

		rset := com.(*jujuc.RelationSetCommand)
		c.Check(rset.RelationId, gc.Equals, t.relid)

		settings := t.settings
		if settings == nil {
			settings = map[string]string{}
		}
		c.Check(rset.Settings, jc.DeepEquals, settings)
	} else {
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

var relationSetInitTests = []relationSetInitTest{
	{
	// compatibility: 0 args is valid.
	}, {
		ctxrelid: -1,
		err:      `no relation id specified`,
	}, {
		ctxrelid: -1,
		args:     []string{"-r", "one"},
		err:      `invalid value "one" for flag -r: invalid relation id`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "one"},
		err:      `invalid value "one" for flag -r: invalid relation id`,
	}, {
		ctxrelid: -1,
		args:     []string{"-r", "ignored:one"},
		err:      `invalid value "ignored:one" for flag -r: invalid relation id`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "ignored:one"},
		err:      `invalid value "ignored:one" for flag -r: invalid relation id`,
	}, {
		ctxrelid: -1,
		args:     []string{"-r", "2"},
		err:      `invalid value "2" for flag -r: unknown relation id`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "ignored:2"},
		err:      `invalid value "ignored:2" for flag -r: unknown relation id`,
	}, {
		ctxrelid: -1,
		err:      `no relation id specified`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "ignored:0"},
		relid:    0,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "0"},
		relid:    0,
	}, {
		ctxrelid: -1,
		args:     []string{"-r", "1"},
		relid:    1,
	}, {
		ctxrelid: 0,
		args:     []string{"-r", "1"},
		relid:    1,
	}, {
		ctxrelid: 1,
		args:     []string{"haha"},
		err:      `expected "key=value", got "haha"`,
	}, {
		ctxrelid: 1,
		args:     []string{"=haha"},
		err:      `expected "key=value", got "=haha"`,
	}, {
		ctxrelid: 1,
		args:     []string{"foo="},
		relid:    1,
		settings: map[string]string{"foo": ""},
	}, {
		ctxrelid: 1,
		args:     []string{"foo='"},
		relid:    1,
		settings: map[string]string{"foo": "'"},
	}, {
		ctxrelid: 1,
		args:     []string{"foo=bar"},
		relid:    1,
		settings: map[string]string{"foo": "bar"},
	}, {
		ctxrelid: 1,
		args:     []string{"foo=bar=baz=qux"},
		relid:    1,
		settings: map[string]string{"foo": "bar=baz=qux"},
	}, {
		ctxrelid: 1,
		args:     []string{"foo=foo: bar"},
		relid:    1,
		settings: map[string]string{"foo": "foo: bar"},
	}, {
		ctxrelid: 0,
		args:     []string{"-r", "1", "foo=bar"},
		relid:    1,
		settings: map[string]string{"foo": "bar"},
	}, {
		ctxrelid: 1,
		args:     []string{"foo=123", "bar=true", "baz=4.5", "qux="},
		relid:    1,
		settings: map[string]string{"foo": "123", "bar": "true", "baz": "4.5", "qux": ""},
	}, {
		summary:  "file with a valid setting",
		args:     []string{"--file", "spam"},
		content:  "foo=bar",
		settings: map[string]string{"foo": "bar"},
	}, {
		summary:  "an empty file",
		args:     []string{"--file", "spam"},
		content:  "",
		settings: map[string]string{},
	}, {
		summary: "an invalid file",
		args:    []string{"--file", "spam"},
		content: "haha",
		err:     `expected "key=value", got "haha"`,
	}, {
		summary: "an invalid file",
		args:    []string{"--file", "spam"},
		content: "=haha",
		err:     `expected "key=value", got "=haha"`,
	}, {
		summary:  "a messy file",
		args:     []string{"--file", "spam"},
		content:  "  \n # a comment \n\n  \nfoo=bar  \nham=eggs\n\n good=bad\n",
		settings: map[string]string{"foo": "bar", "ham": "eggs", "good": "bad"},
	}, {
		summary:  "file + settings",
		args:     []string{"--file", "spam", "foo=bar"},
		content:  "ham=eggs",
		settings: map[string]string{"ham": "eggs", "foo": "bar"},
	}, {
		summary:  "file overridden by settings",
		args:     []string{"--file", "spam", "foo=bar"},
		content:  "foo=baz",
		settings: map[string]string{"foo": "bar"},
	},
}

func (s *RelationSetSuite) TestInit(c *gc.C) {
	for i, t := range relationSetInitTests {
		t.log(c, i)
		com, args := t.init(c, s)

		err := testing.InitCommand(com, args)

		t.check(c, com, err)
	}
}

// Tests start with a relation with the settings {"base": "value"}
var relationSetRunTests = []struct {
	change map[string]string
	expect Settings
}{
	{
		map[string]string{"base": ""},
		Settings{},
	}, {
		map[string]string{"foo": "bar"},
		Settings{"base": "value", "foo": "bar"},
	}, {
		map[string]string{"base": "changed"},
		Settings{"base": "changed"},
	},
}

func (s *RelationSetSuite) TestRun(c *gc.C) {
	hctx := s.GetHookContext(c, 0, "")
	for i, t := range relationSetRunTests {
		c.Logf("test %d", i)

		pristine := Settings{"pristine": "untouched"}
		hctx.rels[0].units["u/0"] = pristine
		basic := Settings{"base": "value"}
		hctx.rels[1].units["u/0"] = basic

		// Run the command.
		com, err := jujuc.NewCommand(hctx, cmdString("relation-set"))
		c.Assert(err, jc.ErrorIsNil)
		rset := com.(*jujuc.RelationSetCommand)
		rset.RelationId = 1
		rset.Settings = t.change
		ctx := testing.Context(c)
		err = com.Run(ctx)
		c.Assert(err, jc.ErrorIsNil)

		// Check changes.
		c.Assert(hctx.rels[0].units["u/0"], gc.DeepEquals, pristine)
		c.Assert(hctx.rels[1].units["u/0"], gc.DeepEquals, t.expect)
	}
}

func (s *RelationSetSuite) TestRunDeprecationWarning(c *gc.C) {
	hctx := s.GetHookContext(c, 0, "")
	com, _ := jujuc.NewCommand(hctx, cmdString("relation-set"))

	// The rel= is needed to make this a valid command.
	ctx, err := testing.RunCommand(c, com, "--format", "foo", "rel=")

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, "")
	c.Assert(testing.Stderr(ctx), gc.Equals, "--format flag deprecated for command \"relation-set\"")
}
