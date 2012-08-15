package server_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd/jujuc/server"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/testing"
	"os"
	"path/filepath"
	"strings"
)

type GetCommandSuite struct {
	HookContextSuite
}

var _ = Suite(&GetCommandSuite{})

var getCommandTests = []struct {
	name string
	err  string
}{
	{"close-port", ""},
	{"config-get", ""},
	{"juju-log", ""},
	{"open-port", ""},
	{"relation-get", ""},
	{"relation-set", ""},
	{"unit-get", ""},
	{"random", "unknown command: random"},
}

func (s *GetCommandSuite) TestGetCommand(c *C) {
	ctx := s.GetHookContext(c, 0, "")
	for _, t := range getCommandTests {
		com, err := ctx.NewCommand(t.name)
		if t.err == "" {
			// At this level, just check basic sanity; commands are tested in
			// more detail elsewhere.
			c.Assert(err, IsNil)
			c.Assert(com.Info().Name, Equals, t.name)
		} else {
			c.Assert(com, IsNil)
			c.Assert(err, ErrorMatches, t.err)
		}
	}
}

type RunHookSuite struct {
	HookContextSuite
}

var _ = Suite(&RunHookSuite{})

// makeCharm constructs a fake charm dir containing a single named hook with
// permissions perm and exit code code. It returns the charm directory and the
// path to which the hook script will write environment variables.
func makeCharm(c *C, hookName string, perm os.FileMode, code int) (charmDir, outPath string) {
	charmDir = c.MkDir()
	hooksDir := filepath.Join(charmDir, "hooks")
	err := os.Mkdir(hooksDir, 0755)
	c.Assert(err, IsNil)
	hook, err := os.OpenFile(filepath.Join(hooksDir, hookName), os.O_CREATE|os.O_WRONLY, perm)
	c.Assert(err, IsNil)
	defer hook.Close()
	outPath = filepath.Join(c.MkDir(), "hook.out")
	_, err = fmt.Fprintf(hook, "#!/bin/bash\nenv > %s\nexit %d", outPath, code)
	c.Assert(err, IsNil)
	return charmDir, outPath
}

func AssertEnvContains(c *C, lines []string, env map[string]string) {
	for k, v := range env {
		sought := k + "=" + v
		found := false
		for _, line := range lines {
			if line == sought {
				found = true
				continue
			}
		}
		comment := Commentf("expected to find %v among %v", sought, lines)
		c.Assert(found, Equals, true, comment)
	}
}

func AssertEnv(c *C, outPath string, charmDir string, env map[string]string) {
	out, err := ioutil.ReadFile(outPath)
	c.Assert(err, IsNil)
	lines := strings.Split(string(out), "\n")
	AssertEnvContains(c, lines, env)
	AssertEnvContains(c, lines, map[string]string{
		"PATH":                     os.Getenv("PATH"),
		"DEBIAN_FRONTEND":          "noninteractive",
		"APT_LISTCHANGES_FRONTEND": "none",
		"CHARM_DIR":                charmDir,
		"JUJU_AGENT_SOCKET":        "/path/to/socket",
	})
}

var runHookTests = []struct {
	summary string
	relid   int
	remote  string
	perms   os.FileMode
	code    int
	err     string
	env     map[string]string
}{
	{
		summary: "missing hook is not an error",
		relid:   -1,
	}, {
		summary: "report failure to execute hook",
		relid:   -1,
		perms:   0600,
		err:     `exec: .*something-happened": permission denied`,
	}, {
		summary: "report error indicated by hook's exit status",
		relid:   -1,
		perms:   0700,
		code:    99,
		err:     "exit status 99",
	}, {
		summary: "check shell environment for non-relation hook context",
		relid:   -1,
		perms:   0700,
		env: map[string]string{
			"JUJU_UNIT_NAME": "u/0",
		},
	}, {
		summary: "check shell environment for relation-broken hook context",
		relid:   1,
		perms:   0700,
		env: map[string]string{
			"JUJU_UNIT_NAME":   "u/0",
			"JUJU_RELATION":    "peer1",
			"JUJU_RELATION_ID": "peer1:1",
		},
	}, {
		summary: "check shell environment for relation hook context",
		relid:   1,
		remote:  "u/1",
		perms:   0700,
		env: map[string]string{
			"JUJU_UNIT_NAME":   "u/0",
			"JUJU_RELATION":    "peer1",
			"JUJU_RELATION_ID": "peer1:1",
			"JUJU_REMOTE_UNIT": "u/1",
		},
	},
}

func (s *RunHookSuite) TestRunHook(c *C) {
	for i, t := range runHookTests {
		c.Logf("test %d: %s", i, t.summary)
		ctx := s.GetHookContext(c, t.relid, t.remote)
		var charmDir, outPath string
		if t.perms == 0 {
			charmDir = c.MkDir()
		} else {
			charmDir, outPath = makeCharm(c, "something-happened", t.perms, t.code)
		}
		err := ctx.RunHook("something-happened", charmDir, "/path/to/socket")
		if t.err == "" {
			c.Assert(err, IsNil)
		} else {
			c.Assert(err, ErrorMatches, t.err)
		}
		if t.env != nil {
			AssertEnv(c, outPath, charmDir, t.env)
		}
	}
}

func (s *RunHookSuite) TestRunHookRelationFlushing(c *C) {
	// Create a charm with a breaking hook.
	ctx := s.GetHookContext(c, -1, "")
	charmDir, _ := makeCharm(c, "something-happened", 0700, 123)

	// Mess with multiple relation settings.
	node0, err := s.relctxs[0].Settings()
	node0.Set("foo", 1)
	node1, err := s.relctxs[1].Settings()
	node1.Set("bar", 2)

	// Run the failing hook.
	err = ctx.RunHook("something-happened", charmDir, "/path/to/socket")
	c.Assert(err, ErrorMatches, "exit status 123")

	// Check that the changes to the local settings nodes have been discarded.
	node0, err = s.relctxs[0].Settings()
	c.Assert(err, IsNil)
	c.Assert(node0.Map(), DeepEquals, map[string]interface{}{
		"private-address": "u-0.example.com",
	})
	node1, err = s.relctxs[1].Settings()
	c.Assert(err, IsNil)
	c.Assert(node1.Map(), DeepEquals, map[string]interface{}{
		"private-address": "u-0.example.com",
	})

	// Check that the changes have been written to state.
	settings0, err := s.relunits[0].ReadSettings("u/0")
	c.Assert(err, IsNil)
	c.Assert(settings0, DeepEquals, node0.Map())
	settings1, err := s.relunits[1].ReadSettings("u/0")
	c.Assert(err, IsNil)
	c.Assert(settings1, DeepEquals, node1.Map())

	// Create a charm with a working hook, and mess with settings again.
	charmDir, _ = makeCharm(c, "something-happened", 0700, 0)
	node0.Set("baz", 3)
	node1.Set("qux", 4)

	// Run the hook.
	err = ctx.RunHook("something-happened", charmDir, "/path/to/socket")
	c.Assert(err, IsNil)

	// Check that the changes to the local settings nodes are still there.
	node0, err = s.relctxs[0].Settings()
	c.Assert(err, IsNil)
	c.Assert(node0.Map(), DeepEquals, map[string]interface{}{
		"private-address": "u-0.example.com",
		"baz":             3,
	})
	node1, err = s.relctxs[1].Settings()
	c.Assert(err, IsNil)
	c.Assert(node1.Map(), DeepEquals, map[string]interface{}{
		"private-address": "u-0.example.com",
		"qux":             4,
	})

	// Check that the changes have been written to state.
	settings0, err = s.relunits[0].ReadSettings("u/0")
	c.Assert(err, IsNil)
	c.Assert(settings0, DeepEquals, node0.Map())
	settings1, err = s.relunits[1].ReadSettings("u/0")
	c.Assert(err, IsNil)
	c.Assert(settings1, DeepEquals, node1.Map())
}

type RelationContextSuite struct {
	testing.StateSuite
	svc *state.Service
	rel *state.Relation
	ru  *state.RelationUnit
}

var _ = Suite(&RelationContextSuite{})

func (s *RelationContextSuite) SetUpTest(c *C) {
	s.StateSuite.SetUpTest(c)
	ch := s.AddTestingCharm(c, "dummy")
	var err error
	s.svc, err = s.State.AddService("u", ch)
	c.Assert(err, IsNil)
	s.rel, err = s.State.AddRelation(
		state.RelationEndpoint{"u", "ifce", "ring", state.RolePeer, charm.ScopeGlobal},
	)
	c.Assert(err, IsNil)
	unit, err := s.svc.AddUnit()
	c.Assert(err, IsNil)
	unit.SetPrivateAddress("u-0.example.com")
	c.Assert(err, IsNil)
	s.ru, err = s.rel.Unit(unit)
	c.Assert(err, IsNil)
	// Quickest way to get needed ZK paths in place:
	p, err := s.ru.Join()
	c.Assert(err, IsNil)
	err = p.Kill()
	c.Assert(err, IsNil)
}

func (s *RelationContextSuite) TestSetMembers(c *C) {
	ctx := server.NewRelationContext(s.ru, nil)
	c.Assert(ctx.Units(), HasLen, 0)

	// Check the units and settings after a simple update.
	ctx.SetMembers(server.SettingsMap{
		"u/2": {"baz": 2},
	})
	c.Assert(ctx.Units(), DeepEquals, []string{"u/2"})
	settings, err := ctx.ReadSettings("u/2")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, map[string]interface{}{"baz": 2})

	// Check that a second update entirely overwrites the first.
	ctx.SetMembers(server.SettingsMap{
		"u/1": {"foo": 1},
		"u/3": {"bar": 3},
	})
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1", "u/3"})

	// Check that the second settings were cached.
	settings, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, map[string]interface{}{"foo": 1})
	settings, err = ctx.ReadSettings("u/3")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, map[string]interface{}{"bar": 3})

	// Check that the first settings are not still cached.
	_, err = ctx.ReadSettings("u/2")
	c.Assert(err, ErrorMatches, `cannot read settings for unit "u/2" in relation "u:ring": unit settings do not exist`)
}

func (s *RelationContextSuite) TestMemberCaching(c *C) {
	unit, err := s.svc.AddUnit()
	c.Assert(err, IsNil)
	ru, err := s.rel.Unit(unit)
	c.Assert(err, IsNil)
	node, err := ru.Settings()
	c.Assert(err, IsNil)
	node.Set("ping", "pong")
	_, err = node.Write()
	c.Assert(err, IsNil)
	ctx := server.NewRelationContext(s.ru, map[string]int{"u/1": 0})

	// Check that uncached settings are read from state.
	settings, err := ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	expect := node.Map()
	c.Assert(settings, DeepEquals, expect)

	// Check that changes to state do not affect the cached settings.
	node.Set("ping", "pow")
	_, err = node.Write()
	c.Assert(err, IsNil)
	settings, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, expect)

	// Check that ClearCache spares the members cache.
	ctx.ClearCache()
	settings, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, expect)

	// Check that updating the context overwrites the cached settings, and
	// that the contents of state are ignored.
	ctx.SetMembers(server.SettingsMap{"u/1": {"entirely": "different"}})
	settings, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, map[string]interface{}{"entirely": "different"})
}

func (s *RelationContextSuite) TestNonMemberCaching(c *C) {
	unit, err := s.svc.AddUnit()
	c.Assert(err, IsNil)
	ru, err := s.rel.Unit(unit)
	c.Assert(err, IsNil)
	node, err := ru.Settings()
	c.Assert(err, IsNil)
	node.Set("ping", "pong")
	_, err = node.Write()
	c.Assert(err, IsNil)
	ctx := server.NewRelationContext(s.ru, nil)

	// Check that settings are read from state.
	settings, err := ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	expect := node.Map()
	c.Assert(settings, DeepEquals, expect)

	// Check that changes to state do not affect the obtained settings...
	node.Set("ping", "pow")
	_, err = node.Write()
	c.Assert(err, IsNil)
	settings, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, expect)

	// ...until the caches are cleared.
	ctx.ClearCache()
	c.Assert(err, IsNil)
	settings, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, map[string]interface{}{"ping": "pow"})
}

func (s *RelationContextSuite) TestSettings(c *C) {
	ctx := server.NewRelationContext(s.ru, nil)

	// Change Settings, then clear cache without writing.
	node, err := ctx.Settings()
	c.Assert(err, IsNil)
	expect := node.Map()
	node.Set("change", "exciting")
	ctx.ClearCache()

	// Check that the change is not cached...
	node, err = ctx.Settings()
	c.Assert(err, IsNil)
	c.Assert(node.Map(), DeepEquals, expect)

	// ...and not written to state.
	settings, err := s.ru.ReadSettings("u/0")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, expect)

	// Change again, write settings, and clear caches.
	node.Set("change", "exciting")
	err = ctx.WriteSettings()
	c.Assert(err, IsNil)
	ctx.ClearCache()

	// Check that the change is reflected in Settings...
	expect["change"] = "exciting"
	node, err = ctx.Settings()
	c.Assert(err, IsNil)
	c.Assert(node.Map(), DeepEquals, expect)

	// ...and was written to state.
	settings, err = s.ru.ReadSettings("u/0")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, expect)
}
