package server_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/jujuc/server"
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
		c.Assert(bufferString(ctx.Stderr), Equals, fmt.Sprintf(`
usage: relation-set [options] key=value [key=value ...]
purpose: set relation settings

options:
-r (= "%s")
    relation id
`[1:], t.expect))
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
		ctxrelid: -1,
		err:      `no relation specified`,
	}, {
		ctxrelid: 1,
		err:      `expected "key=value" parameters, got nothing`,
	}, {
		ctxrelid: -1,
		args:     []string{"-r", "one"},
		err:      `invalid relation id "one"`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "one"},
		err:      `invalid relation id "one"`,
	},
	{
		ctxrelid: -1,
		args:     []string{"-r", "ignored:one"},
		err:      `invalid relation id "ignored:one"`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "ignored:one"}, err: `invalid relation id "ignored:one"`,
	}, {
		ctxrelid: -1,
		args:     []string{"-r", "2"},
		err:      `unknown relation id "2"`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "ignored:2"},
		err:      `unknown relation id "ignored:2"`,
	},
	{
		ctxrelid: -1,
		args:     []string{"-r", "ignored:0"},
		err:      `expected "key=value" parameters, got nothing`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "ignored:0"},
		err:      `expected "key=value" parameters, got nothing`,
	}, {
		ctxrelid: -1,
		args:     []string{"-r", "0"},
		err:      `expected "key=value" parameters, got nothing`,
	}, {
		ctxrelid: 1,
		args:     []string{"-r", "0"},
		err:      `expected "key=value" parameters, got nothing`,
	}, {
		ctxrelid: -1,
		args:     []string{"-r", "1"},
		err:      `expected "key=value" parameters, got nothing`,
	}, {
		ctxrelid: 0,
		args:     []string{"-r", "1"},
		err:      `expected "key=value" parameters, got nothing`,
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

// Tests start with a relation with the settings {"base": "value"}
var relationSetRunTests = []struct {
	change map[string]string
	expect map[string]interface{}
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

		// Set base settings for this test's relations.
		hctx.Relations[1].ClearCache()
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
		rset.Settings = t.change
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

		// ...until we ask for it.
		err = hctx.Relations[1].WriteSettings()
		c.Assert(err, IsNil)
		settings, err = s.relunits[1].ReadSettings("u/0")
		c.Assert(settings, DeepEquals, t.expect)

		// For paranoia's sake, check that the other relation's settings have
		// not been touched...
		settings, err = s.relunits[0].ReadSettings("u/0")
		c.Assert(err, IsNil)
		c.Assert(settings, DeepEquals, pristine)

		// ...even when flushed.
		err = hctx.Relations[0].WriteSettings()
		c.Assert(err, IsNil)
		settings, err = s.relunits[0].ReadSettings("u/0")
		c.Assert(err, IsNil)
		c.Assert(settings, DeepEquals, pristine)
	}
}
