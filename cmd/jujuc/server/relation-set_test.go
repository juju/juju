package server_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/state"
)

type RelationSetSuite struct {
	HookContextSuite
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
		com, err := hctx.NewCommand("relation-set")
		c.Assert(err, IsNil)
		ctx := dummyContext(c)
		code := cmd.Main(com, ctx, []string{"--help"})
		c.Assert(code, Equals, 0)
		c.Assert(bufferString(ctx.Stdout), Equals, "")
		c.Assert(bufferString(ctx.Stderr), Equals, fmt.Sprintf(`usage: relation-set [options] <key=value> [, ...]
purpose: set relation settings

options:
-r (= "%s")
    relation id
`, t.expect))
	}
}

var relationSetInitTests = []struct {
	ctxrelid int
	args     []string
	err      string
	relid    int
	settings map[string]interface{}
}{
	{-1, nil, `no relation specified`, 0, nil},
	{1, nil, `no settings specified`, 0, nil},
	{-1, []string{"-r", "one"}, `invalid relation id "one"`, 0, nil},
	{1, []string{"-r", "one"}, `invalid relation id "one"`, 0, nil},
	{-1, []string{"-r", "ignored:one"}, `invalid relation id "ignored:one"`, 0, nil},
	{1, []string{"-r", "ignored:one"}, `invalid relation id "ignored:one"`, 0, nil},
	{-1, []string{"-r", "2"}, `unknown relation id "2"`, 0, nil},
	{1, []string{"-r", "ignored:2"}, `unknown relation id "ignored:2"`, 0, nil},
	{-1, []string{"-r", "ignored:0"}, `no settings specified`, 0, nil},
	{1, []string{"-r", "ignored:0"}, `no settings specified`, 0, nil},
	{-1, []string{"-r", "0"}, `no settings specified`, 0, nil},
	{1, []string{"-r", "0"}, `no settings specified`, 0, nil},
	{-1, []string{"-r", "1"}, `no settings specified`, 0, nil},
	{0, []string{"-r", "1"}, `no settings specified`, 0, nil},
	{1, []string{"foo='"}, `cannot parse "foo='": YAML error: .*`, 0, nil},
	{1, []string{"=haha"}, `cannot parse "=haha": no key specified`, 0, nil},
	{1, []string{"foo=bar"}, ``, 1, map[string]interface{}{"foo": "bar"}},
	{0, []string{"-r", "1", "foo=bar"}, ``, 1, map[string]interface{}{"foo": "bar"}},
	{1, []string{"foo=123", "bar=true", "baz=4.5", "qux="}, ``, 1, map[string]interface{}{
		"foo": 123, "bar": true, "baz": 4.5, "qux": nil,
	}},
}

func (s *RelationSetSuite) TestInit(c *C) {
	for i, t := range relationSetInitTests {
		c.Logf("test %d", i)
		hctx := s.GetHookContext(c, t.ctxrelid, "")
		com, err := hctx.NewCommand("relation-set")
		c.Assert(err, IsNil)
		err = com.Init(dummyFlagSet(), t.args)
		if t.err == "" {
			c.Assert(err, IsNil)
			rset := com.(*server.RelationSetCommand)
			c.Assert(rset.RelationId, Equals, t.relid)
			c.Assert(rset.Settings, DeepEquals, t.settings)
		} else {
			c.Assert(err, ErrorMatches, t.err)
		}
	}
}

var relationSetRunTests = []struct {
	settings map[string]interface{}
	expect   map[string]interface{}
}{
	{map[string]interface{}{
		"base": nil,
	}, map[string]interface{}{}},
	{map[string]interface{}{
		"foo": "bar",
	}, map[string]interface{}{
		"base": "value",
		"foo":  "bar",
	}},
	{map[string]interface{}{
		"base": "changed",
	}, map[string]interface{}{
		"base": "changed",
	}},
}

func (s *RelationSetSuite) TestRun(c *C) {
	hctx := s.GetHookContext(c, 0, "")
	for i, t := range relationSetRunTests {
		c.Logf("test %d", i)

		// Set base settings for this test's relations.
		err := hctx.Relations[1].Flush(false)
		pristine := map[string]interface{}{"pristine": "untouched"}
		setSettings(c, s.relunits[0], pristine)
		basic := map[string]interface{}{"base": "value"}
		setSettings(c, s.relunits[1], basic)
		settings, err := s.relunits[1].ReadSettings("u/0")

		// Run the command.
		c.Assert(err, IsNil)
		com, err := hctx.NewCommand("relation-set")
		c.Assert(err, IsNil)
		rset := com.(*server.RelationSetCommand)
		rset.RelationId = 1
		rset.Settings = t.settings
		ctx := dummyContext(c)
		err = com.Run(ctx)
		c.Assert(err, IsNil)

		// Check that the changes are present in the RelationContext's
		// settings...
		node, err := hctx.Relations[1].Settings()
		c.Assert(err, IsNil)
		c.Assert(node.Map(), DeepEquals, t.expect)

		// ...but are not persisted to state...
		settings, err = s.relunits[1].ReadSettings("u/0")
		c.Assert(err, IsNil)
		c.Assert(settings, DeepEquals, basic)

		// ...until the relation context is flushed.
		err = hctx.Relations[1].Flush(true)
		c.Assert(err, IsNil)
		settings, err = s.relunits[1].ReadSettings("u/0")
		c.Assert(settings, DeepEquals, t.expect)

		// For paranoia's sake, check that the other relation's settings have
		// not been touched...
		settings, err = s.relunits[0].ReadSettings("u/0")
		c.Assert(err, IsNil)
		c.Assert(settings, DeepEquals, pristine)

		// ...even when flushed.
		err = hctx.Relations[0].Flush(true)
		c.Assert(err, IsNil)
		settings, err = s.relunits[0].ReadSettings("u/0")
		c.Assert(err, IsNil)
		c.Assert(settings, DeepEquals, pristine)
	}
}

func setSettings(c *C, ru *state.RelationUnit, settings map[string]interface{}) {
	node, err := ru.Settings()
	c.Assert(err, IsNil)
	for _, k := range node.Keys() {
		node.Delete(k)
	}
	node.Update(settings)
	_, err = node.Write()
	c.Assert(err, IsNil)
}
