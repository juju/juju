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

type GetCommandSuite struct{}

var _ = Suite(&GetCommandSuite{})

var getCommandTests = []struct {
	name string
	err  string
}{
	{"close-port", ""},
	{"config-get", ""},
	{"juju-log", ""},
	{"open-port", ""},
	{"unit-get", ""},
	{"random", "unknown command: random"},
}

func (s *GetCommandSuite) TestGetCommand(c *C) {
	ctx := &server.ClientContext{
		Id:            "ctxid",
		State:         &state.State{},
		LocalUnitName: "minecraft/0",
	}
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
	outPath string
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

func AssertEnv(c *C, outPath string, env map[string]string) {
	out, err := ioutil.ReadFile(outPath)
	c.Assert(err, IsNil)
	lines := strings.Split(string(out), "\n")
	AssertEnvContains(c, lines, env)
	AssertEnvContains(c, lines, map[string]string{
		"PATH":                     os.Getenv("PATH"),
		"DEBIAN_FRONTEND":          "noninteractive",
		"APT_LISTCHANGES_FRONTEND": "none",
	})
}

func (s *RunHookSuite) TestNoHook(c *C) {
	ctx := &server.ClientContext{}
	err := ctx.RunHook("tree-fell-in-forest", c.MkDir(), "")
	c.Assert(err, IsNil)
}

func (s *RunHookSuite) TestNonExecutableHook(c *C) {
	ctx := &server.ClientContext{}
	charmDir, _ := makeCharm(c, "something-happened", 0600, 0)
	err := ctx.RunHook("something-happened", charmDir, "")
	c.Assert(err, ErrorMatches, `exec: ".*/something-happened": permission denied`)
}

func (s *RunHookSuite) TestBadHook(c *C) {
	ctx := &server.ClientContext{Id: "ctx-id"}
	charmDir, outPath := makeCharm(c, "occurrence-occurred", 0700, 99)
	socketPath := "/path/to/socket"
	err := ctx.RunHook("occurrence-occurred", charmDir, socketPath)
	c.Assert(err, ErrorMatches, "exit status 99")
	AssertEnv(c, outPath, map[string]string{
		"CHARM_DIR":         charmDir,
		"JUJU_AGENT_SOCKET": socketPath,
		"JUJU_CONTEXT_ID":   "ctx-id",
	})
}

func (s *RunHookSuite) TestGoodHookWithVars(c *C) {
	ctx := &server.ClientContext{
		Id:             "some-id",
		LocalUnitName:  "local/99",
		RemoteUnitName: "remote/123",
		RelationName:   "rel",
	}
	charmDir, outPath := makeCharm(c, "something-happened", 0700, 0)
	socketPath := "/path/to/socket"
	err := ctx.RunHook("something-happened", charmDir, socketPath)
	c.Assert(err, IsNil)
	AssertEnv(c, outPath, map[string]string{
		"CHARM_DIR":         charmDir,
		"JUJU_AGENT_SOCKET": socketPath,
		"JUJU_CONTEXT_ID":   "some-id",
		"JUJU_UNIT_NAME":    "local/99",
		"JUJU_REMOTE_UNIT":  "remote/123",
		"JUJU_RELATION":     "rel",
	})
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

func (s *RelationContextSuite) TestUpdateMembers(c *C) {
	ctx := server.NewRelationContext(s.State, s.ru, nil)
	c.Assert(ctx.Units(), HasLen, 0)

	// Check the units and settings after a simple update.
	ctx.Update(map[string]map[string]interface{}{
		"u/2": {"baz": 2},
	})
	c.Assert(ctx.Units(), DeepEquals, []string{"u/2"})
	settings, err := ctx.ReadSettings("u/2")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, map[string]interface{}{"baz": 2})

	// Check that a second update entirely overwrites the first.
	ctx.Update(map[string]map[string]interface{}{
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
	ctx := server.NewRelationContext(s.State, s.ru, map[string]int{"u/1": 0})

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

	// Check that flushing the context does not affect the cached settings.
	err = ctx.Flush(true)
	c.Assert(err, IsNil)
	settings, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, expect)

	// Check that updating the context overwrites the cached settings, and
	// that the contents of state are ignored.
	ctx.Update(map[string]map[string]interface{}{"u/1": {"entirely": "different"}})
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
	ctx := server.NewRelationContext(s.State, s.ru, nil)

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

	// ...until the context is flushed.
	err = ctx.Flush(true)
	c.Assert(err, IsNil)
	settings, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, map[string]interface{}{"ping": "pow"})
}

func (s *RelationContextSuite) TestSettings(c *C) {
	ctx := server.NewRelationContext(s.State, s.ru, nil)

	// Change Settings, then flush without writing.
	node, err := ctx.Settings()
	c.Assert(err, IsNil)
	expect := node.Map()
	node.Set("change", "exciting")
	ctx.Flush(false)

	// Check that the change is not cached...
	node, err = ctx.Settings()
	c.Assert(err, IsNil)
	c.Assert(node.Map(), DeepEquals, expect)

	// ...and not written to state.
	settings, err := s.ru.ReadSettings("u/0")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, expect)

	// Change again, and flush with a write.
	node.Set("change", "exciting")
	ctx.Flush(true)

	// Check that the change is reflected in Settings...
	expect["change"] = "exciting"
	node, err = ctx.Settings()
	c.Assert(err, IsNil)
	c.Assert(node.Map(), DeepEquals, expect)

	// ...and written to state.
	settings, err = s.ru.ReadSettings("u/0")
	c.Assert(err, IsNil)
	c.Assert(settings, DeepEquals, expect)
}
