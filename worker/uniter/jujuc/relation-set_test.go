// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/jujuc"
)

type RelationSetSuite struct {
	ContextSuite
}

var _ = Suite(&RelationSetSuite{})

var helpTests = []struct {
	relid  int
	expect string
}{{-1, ""}, {0, "peer0:0"}}

func (s *RelationSetSuite) TestHelp(c *C) {
	for i, t := range helpTests {
		c.Logf("test %d", i)
		hctx := s.GetHookContext(c, t.relid, "")
		com, err := jujuc.NewCommand(hctx, "relation-set")
		c.Assert(err, IsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, []string{"--help"})
		c.Assert(code, Equals, 0)
		c.Assert(bufferString(ctx.Stdout), Equals, fmt.Sprintf(`
usage: relation-set [options] key=value [key=value ...]
purpose: set relation settings

options:
--format (= "")
    deprecated format flag
-r  (= %s)
    specify a relation by id
`[1:], t.expect))
		c.Assert(bufferString(ctx.Stderr), Equals, "")
	}
}

var relationSetInitTests = []struct {
	ctxrelid int
	args     []string
	err      string
	relid    int
	settings map[string]string
}{
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
	},
}

func (s *RelationSetSuite) TestInit(c *C) {
	for i, t := range relationSetInitTests {
		c.Logf("test %d", i)
		hctx := s.GetHookContext(c, t.ctxrelid, "")
		com, err := jujuc.NewCommand(hctx, "relation-set")
		c.Assert(err, IsNil)
		err = testing.InitCommand(com, t.args)
		if t.err == "" {
			c.Assert(err, IsNil)
			rset := com.(*jujuc.RelationSetCommand)
			c.Assert(rset.RelationId, Equals, t.relid)
			settings := t.settings
			if settings == nil {
				settings = map[string]string{}
			}
			c.Assert(rset.Settings, DeepEquals, settings)
		} else {
			c.Assert(err, ErrorMatches, t.err)
		}
	}
}

// Tests start with a relation with the settings {"base": "value"}
var relationSetRunTests = []struct {
	change map[string]string
	expect Settings
}{
	{
		map[string]string{"base": ""},
		map[string]interface{}{},
	}, {
		map[string]string{"foo": "bar"},
		map[string]interface{}{"base": "value", "foo": "bar"},
	}, {
		map[string]string{"base": "changed"},
		map[string]interface{}{"base": "changed"},
	},
}

func (s *RelationSetSuite) TestRun(c *C) {
	hctx := s.GetHookContext(c, 0, "")
	for i, t := range relationSetRunTests {
		c.Logf("test %d", i)

		pristine := Settings{"pristine": "untouched"}
		hctx.rels[0].units["u/0"] = pristine
		basic := Settings{"base": "value"}
		hctx.rels[1].units["u/0"] = basic

		// Run the command.
		com, err := jujuc.NewCommand(hctx, "relation-set")
		c.Assert(err, IsNil)
		rset := com.(*jujuc.RelationSetCommand)
		rset.RelationId = 1
		rset.Settings = t.change
		ctx := testing.Context(c)
		err = com.Run(ctx)
		c.Assert(err, IsNil)

		// Check changes.
		c.Assert(hctx.rels[0].units["u/0"], DeepEquals, pristine)
		c.Assert(hctx.rels[1].units["u/0"], DeepEquals, t.expect)
	}
}

func (s *RelationSetSuite) TestRunDeprecationWarning(c *C) {
	hctx := s.GetHookContext(c, 0, "")
	com, _ := jujuc.NewCommand(hctx, "relation-set")
	// The rel= is needed to make this a valid command.
	ctx, err := testing.RunCommand(c, com, []string{"--format", "foo", "rel="})

	c.Assert(err, IsNil)
	c.Assert(testing.Stdout(ctx), Equals, "")
	c.Assert(testing.Stderr(ctx), Equals, "--format flag deprecated for command \"relation-set\"")
}
