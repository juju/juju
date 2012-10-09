package jujuc_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/uniter/jujuc"
	"os"
	"path/filepath"
	"strings"
	"time"
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

type hookSpec struct {
	// name is the name of the hook.
	name string
	// perm is the file permissions of the hook.
	perm os.FileMode
	// code is the exit status of the hook.
	code int
	// stdout holds a string to print to stdout
	stdout string
	// stderr holds a string to print to stderr
	stderr string
	// background holds a string to print in the background after 0.2s.
	background string
}

// makeCharm constructs a fake charm dir containing a single named hook
// with permissions perm and exit code code.  If output is non-empty,
// the charm will write it to stdout and stderr, with each one prefixed
// by name of the stream.  It returns the charm directory and the path
// to which the hook script will write environment variables.
func makeCharm(c *C, spec hookSpec) (charmDir, outPath string) {
	charmDir = c.MkDir()
	hooksDir := filepath.Join(charmDir, "hooks")
	err := os.Mkdir(hooksDir, 0755)
	c.Assert(err, IsNil)
	c.Logf("openfile perm %v", spec.perm)
	hook, err := os.OpenFile(filepath.Join(hooksDir, spec.name), os.O_CREATE|os.O_WRONLY, spec.perm)
	c.Assert(err, IsNil)
	defer hook.Close()
	printf := func(f string, a ...interface{}) {
		if _, err := fmt.Fprintf(hook, f+"\n", a...); err != nil {
			panic(err)
		}
	}
	outPath = filepath.Join(c.MkDir(), "hook.out")
	printf("#!/bin/bash")
	printf("env > " + outPath)
	if spec.stdout != "" {
		printf("echo %s", spec.stdout)
	}
	if spec.stderr != "" {
		printf("echo %s >&2", spec.stderr)
	}
	if spec.background != "" {
		// Print something fairly quickly, then sleep for
		// quite a long time - if the hook execution is
		// blocking because of the background process,
		// the hook execution will take much longer than
		// expected.
		printf("(sleep 0.2; echo %s; sleep 10) &", spec.background)
	}
	printf("exit %d", spec.code)
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
		"DEBIAN_FRONTEND":          "noninteractive",
		"APT_LISTCHANGES_FRONTEND": "none",
		"CHARM_DIR":                charmDir,
		"JUJU_AGENT_SOCKET":        "/path/to/socket",
	})
}

// LineBufferSize matches the constant used when creating
// the bufio line reader.
const lineBufferSize = 4096

var runHookTests = []struct {
	summary string
	relid   int
	remote  string
	spec    hookSpec
	err     string
	env     map[string]string
}{
	{
		summary: "missing hook is not an error",
		relid:   -1,
	}, {
		summary: "report failure to execute hook",
		relid:   -1,
		spec:    hookSpec{perm: 0600},
		err:     `exec: .*something-happened": permission denied`,
	}, {
		summary: "report error indicated by hook's exit status",
		relid:   -1,
		spec: hookSpec{
			perm: 0700,
			code: 99,
		},
		err: "exit status 99",
	}, {
		summary: "output logging",
		relid:   -1,
		spec: hookSpec{
			perm:   0700,
			stdout: "stdout",
			stderr: "stderr",
		},
	}, {
		summary: "output logging with background process",
		relid:   -1,
		spec: hookSpec{
			perm:       0700,
			stdout:     "stdout",
			background: "not printed",
		},
	}, {
		summary: "long line split",
		relid:   -1,
		spec: hookSpec{
			perm:   0700,
			stdout: strings.Repeat("a", lineBufferSize+10),
		},
	}, {
		summary: "check shell environment for non-relation hook context",
		relid:   -1,
		spec:    hookSpec{perm: 0700},
		env: map[string]string{
			"JUJU_UNIT_NAME": "u/0",
		},
	}, {
		summary: "check shell environment for relation-broken hook context",
		relid:   1,
		spec:    hookSpec{perm: 0700},
		env: map[string]string{
			"JUJU_UNIT_NAME":   "u/0",
			"JUJU_RELATION":    "peer1",
			"JUJU_RELATION_ID": "peer1:1",
		},
	}, {
		summary: "check shell environment for relation hook context",
		relid:   1,
		remote:  "u/1",
		spec:    hookSpec{perm: 0700},
		env: map[string]string{
			"JUJU_UNIT_NAME":   "u/0",
			"JUJU_RELATION":    "peer1",
			"JUJU_RELATION_ID": "peer1:1",
			"JUJU_REMOTE_UNIT": "u/1",
		},
	},
}

type logRecorder struct {
	c      *C
	prefix string
	lines  []string
}

func (l *logRecorder) Output(calldepth int, s string) error {
	if strings.HasPrefix(s, l.prefix) {
		l.lines = append(l.lines, s[len(l.prefix):])
	}
	l.c.Logf("%s", s)
	return nil
}

func (s *RunHookSuite) TestRunHook(c *C) {
	oldLogger := log.Target
	defer func() {
		log.Target = oldLogger
	}()
	logger := &logRecorder{c: c, prefix: "JUJU HOOK "}
	log.Target = logger
	for i, t := range runHookTests {
		c.Logf("test %d: %s; perm %v", i, t.summary, t.spec.perm)
		ctx := s.GetHookContext(c, t.relid, t.remote)
		var charmDir, outPath string
		if t.spec.perm == 0 {
			charmDir = c.MkDir()
		} else {
			spec := t.spec
			spec.name = "something-happened"
			c.Logf("makeCharm %#v", spec)
			charmDir, outPath = makeCharm(c, spec)
		}
		toolsDir := c.MkDir()
		logger.lines = nil
		t0 := time.Now()
		err := ctx.RunHook("something-happened", charmDir, toolsDir, "/path/to/socket")
		if t.err == "" {
			c.Assert(err, IsNil)
		} else {
			c.Assert(err, ErrorMatches, t.err)
		}
		if t.env != nil {
			env := map[string]string{"PATH": toolsDir + ":" + os.Getenv("PATH")}
			for k, v := range t.env {
				env[k] = v
			}
			AssertEnv(c, outPath, charmDir, env)
		}
		var expectLog []string
		if t.spec.stdout != "" {
			expectLog = append(expectLog, splitLine(t.spec.stdout)...)
		}
		if t.spec.stderr != "" {
			expectLog = append(expectLog, splitLine(t.spec.stderr)...)
		}
		if t.spec.background != "" && time.Now().Sub(t0) > 5*time.Second {
			c.Errorf("background process holding up hook execution")
		}
		c.Assert(logger.lines, DeepEquals, expectLog)
	}
}

// split the line into buffer-sized lengths.
func splitLine(s string) []string {
	var ss []string
	for len(s) > lineBufferSize {
		ss = append(ss, s[0:lineBufferSize])
		s = s[lineBufferSize:]
	}
	if len(s) > 0 {
		ss = append(ss, s)
	}
	return ss
}

func (s *RunHookSuite) TestRunHookRelationFlushing(c *C) {
	// Create a charm with a breaking hook.
	ctx := s.GetHookContext(c, -1, "")
	charmDir, _ := makeCharm(c, hookSpec{
		name: "something-happened",
		perm: 0700,
		code: 123,
	})

	// Mess with multiple relation settings.
	node0, err := s.relctxs[0].Settings()
	node0.Set("foo", 1)
	node1, err := s.relctxs[1].Settings()
	node1.Set("bar", 2)

	// Run the failing hook.
	err = ctx.RunHook("something-happened", charmDir, c.MkDir(), "/path/to/socket")
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
	charmDir, _ = makeCharm(c, hookSpec{
		name: "something-happened",
		perm: 0700,
	})
	node0.Set("baz", 3)
	node1.Set("qux", 4)

	// Run the hook.
	err = ctx.RunHook("something-happened", charmDir, c.MkDir(), "/path/to/socket")
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
	testing.JujuConnSuite
	svc *state.Service
	rel *state.Relation
	ru  *state.RelationUnit
}

var _ = Suite(&RelationContextSuite{})

func (s *RelationContextSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
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
	err = s.ru.EnterScope()
	c.Assert(err, IsNil)
}

func (s *RelationContextSuite) TestChangeMembers(c *C) {
	ctx := jujuc.NewRelationContext(s.ru, nil)
	c.Assert(ctx.Units(), HasLen, 0)

	// Check the units and settings after a simple update.
	ctx.UpdateMembers(jujuc.SettingsMap{
		"u/2": {"baz": 2},
		"u/4": {"qux": 4},
	})
	c.Assert(ctx.Units(), DeepEquals, []string{"u/2", "u/4"})
	assertSettings := func(unit string, expect map[string]interface{}) {
		actual, err := ctx.ReadSettings(unit)
		c.Assert(err, IsNil)
		c.Assert(actual, DeepEquals, expect)
	}
	assertSettings("u/2", map[string]interface{}{"baz": 2})
	assertSettings("u/4", map[string]interface{}{"qux": 4})

	// Send a second update; check that members are only added, not removed.
	ctx.UpdateMembers(jujuc.SettingsMap{
		"u/1": {"foo": 1},
		"u/2": nil,
		"u/3": {"bar": 3},
	})
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1", "u/2", "u/3", "u/4"})

	// Check that all settings remain cached, including u/2's (which lacked
	// new settings data in the second update).
	assertSettings("u/1", map[string]interface{}{"foo": 1})
	assertSettings("u/2", map[string]interface{}{"baz": 2})
	assertSettings("u/3", map[string]interface{}{"bar": 3})
	assertSettings("u/4", map[string]interface{}{"qux": 4})

	// Delete a member, and check that it is no longer a member...
	ctx.DeleteMember("u/2")
	c.Assert(ctx.Units(), DeepEquals, []string{"u/1", "u/3", "u/4"})

	// ...and that its settings are no longer cached.
	_, err := ctx.ReadSettings("u/2")
	c.Assert(err, ErrorMatches, `cannot read settings for unit "u/2" in relation "u:ring": not found`)
}

func (s *RelationContextSuite) TestMemberCaching(c *C) {
	unit, err := s.svc.AddUnit()
	c.Assert(err, IsNil)
	ru, err := s.rel.Unit(unit)
	c.Assert(err, IsNil)
	err = unit.SetPrivateAddress("u-1.example.com")
	c.Assert(err, IsNil)
	err = ru.EnterScope()
	c.Assert(err, IsNil)
	settings, err := ru.Settings()
	c.Assert(err, IsNil)
	settings.Set("ping", "pong")
	_, err = settings.Write()
	c.Assert(err, IsNil)
	ctx := jujuc.NewRelationContext(s.ru, map[string]int64{"u/1": 0})

	// Check that uncached settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	expect := settings.Map()
	c.Assert(m, DeepEquals, expect)

	// Check that changes to state do not affect the cached settings.
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(m, DeepEquals, expect)

	// Check that ClearCache spares the members cache.
	ctx.ClearCache()
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(m, DeepEquals, expect)

	// Check that updating the context overwrites the cached settings, and
	// that the contents of state are ignored.
	ctx.UpdateMembers(jujuc.SettingsMap{"u/1": {"entirely": "different"}})
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(m, DeepEquals, map[string]interface{}{"entirely": "different"})
}

func (s *RelationContextSuite) TestNonMemberCaching(c *C) {
	unit, err := s.svc.AddUnit()
	c.Assert(err, IsNil)
	ru, err := s.rel.Unit(unit)
	c.Assert(err, IsNil)
	err = unit.SetPrivateAddress("u-1.example.com")
	c.Assert(err, IsNil)
	err = ru.EnterScope()
	c.Assert(err, IsNil)
	settings, err := ru.Settings()
	c.Assert(err, IsNil)
	settings.Set("ping", "pong")
	_, err = settings.Write()
	c.Assert(err, IsNil)
	ctx := jujuc.NewRelationContext(s.ru, nil)

	// Check that settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	expect := settings.Map()
	c.Assert(m, DeepEquals, expect)

	// Check that changes to state do not affect the obtained settings...
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(m, DeepEquals, expect)

	// ...until the caches are cleared.
	ctx.ClearCache()
	c.Assert(err, IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, IsNil)
	c.Assert(m["ping"], Equals, "pow")
}

func (s *RelationContextSuite) TestSettings(c *C) {
	ctx := jujuc.NewRelationContext(s.ru, nil)

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
