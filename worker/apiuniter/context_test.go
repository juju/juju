// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiuniter_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/worker/apiuniter"
	"launchpad.net/juju-core/worker/apiuniter/jujuc"
)

type RunHookSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&RunHookSuite{})

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
func makeCharm(c *gc.C, spec hookSpec) (charmDir, outPath string) {
	charmDir = c.MkDir()
	hooksDir := filepath.Join(charmDir, "hooks")
	err := os.Mkdir(hooksDir, 0755)
	c.Assert(err, gc.IsNil)
	c.Logf("openfile perm %v", spec.perm)
	hook, err := os.OpenFile(filepath.Join(hooksDir, spec.name), os.O_CREATE|os.O_WRONLY, spec.perm)
	c.Assert(err, gc.IsNil)
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

func AssertEnvContains(c *gc.C, lines []string, env map[string]string) {
	for k, v := range env {
		sought := k + "=" + v
		found := false
		for _, line := range lines {
			if line == sought {
				found = true
				continue
			}
		}
		comment := gc.Commentf("expected to find %v among %v", sought, lines)
		c.Assert(found, gc.Equals, true, comment)
	}
}

func AssertEnv(c *gc.C, outPath string, charmDir string, env map[string]string, uuid string) {
	out, err := ioutil.ReadFile(outPath)
	c.Assert(err, gc.IsNil)
	lines := strings.Split(string(out), "\n")
	AssertEnvContains(c, lines, env)
	AssertEnvContains(c, lines, map[string]string{
		"DEBIAN_FRONTEND":          "noninteractive",
		"APT_LISTCHANGES_FRONTEND": "none",
		"CHARM_DIR":                charmDir,
		"JUJU_AGENT_SOCKET":        "/path/to/socket",
		"JUJU_ENV_UUID":            uuid,
	})
}

// LineBufferSize matches the constant used when creating
// the bufio line reader.
const lineBufferSize = 4096

var apiAddrs = []string{"a1:123", "a2:123"}
var expectedApiAddrs = strings.Join(apiAddrs, " ")

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
			"JUJU_UNIT_NAME":     "u/0",
			"JUJU_API_ADDRESSES": expectedApiAddrs,
		},
	}, {
		summary: "check shell environment for relation-broken hook context",
		relid:   1,
		spec:    hookSpec{perm: 0700},
		env: map[string]string{
			"JUJU_UNIT_NAME":     "u/0",
			"JUJU_API_ADDRESSES": expectedApiAddrs,
			"JUJU_RELATION":      "db",
			"JUJU_RELATION_ID":   "db:1",
			"JUJU_REMOTE_UNIT":   "",
		},
	}, {
		summary: "check shell environment for relation hook context",
		relid:   1,
		remote:  "r/1",
		spec:    hookSpec{perm: 0700},
		env: map[string]string{
			"JUJU_UNIT_NAME":     "u/0",
			"JUJU_API_ADDRESSES": expectedApiAddrs,
			"JUJU_RELATION":      "db",
			"JUJU_RELATION_ID":   "db:1",
			"JUJU_REMOTE_UNIT":   "r/1",
		},
	},
}

func (s *RunHookSuite) TestRunHook(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	for i, t := range runHookTests {
		c.Logf("test %d: %s; perm %v", i, t.summary, t.spec.perm)
		ctx := s.GetHookContext(c, uuid.String(), t.relid, t.remote)
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
		t0 := time.Now()
		err := ctx.RunHook("something-happened", charmDir, toolsDir, "/path/to/socket")
		if t.err == "" {
			c.Assert(err, gc.IsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
		if t.env != nil {
			env := map[string]string{"PATH": toolsDir + ":" + os.Getenv("PATH")}
			for k, v := range t.env {
				env[k] = v
			}
			AssertEnv(c, outPath, charmDir, env, uuid.String())
		}
		if t.spec.background != "" && time.Now().Sub(t0) > 5*time.Second {
			c.Errorf("background process holding up hook execution")
		}
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

func (s *RunHookSuite) TestRunHookRelationFlushing(c *gc.C) {
	// Create a charm with a breaking hook.
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	ctx := s.GetHookContext(c, uuid.String(), -1, "")
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
	c.Assert(err, gc.ErrorMatches, "exit status 123")

	// Check that the changes to the local settings nodes have been discarded.
	node0, err = s.relctxs[0].Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node0.Map(), gc.DeepEquals, map[string]interface{}{"relation-name": "db0"})
	node1, err = s.relctxs[1].Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node1.Map(), gc.DeepEquals, map[string]interface{}{"relation-name": "db1"})

	// Check that the changes have been written to state.
	settings0, err := s.relunits[0].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings0, gc.DeepEquals, node0.Map())
	settings1, err := s.relunits[1].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings1, gc.DeepEquals, node1.Map())

	// Create a charm with a working hook, and mess with settings again.
	charmDir, _ = makeCharm(c, hookSpec{
		name: "something-happened",
		perm: 0700,
	})
	node0.Set("baz", 3)
	node1.Set("qux", 4)

	// Run the hook.
	err = ctx.RunHook("something-happened", charmDir, c.MkDir(), "/path/to/socket")
	c.Assert(err, gc.IsNil)

	// Check that the changes to the local settings nodes are still there.
	node0, err = s.relctxs[0].Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node0.Map(), gc.DeepEquals, map[string]interface{}{
		"relation-name": "db0",
		"baz":           3,
	})
	node1, err = s.relctxs[1].Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node1.Map(), gc.DeepEquals, map[string]interface{}{
		"relation-name": "db1",
		"qux":           4,
	})

	// Check that the changes have been written to state.
	settings0, err = s.relunits[0].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings0, gc.DeepEquals, node0.Map())
	settings1, err = s.relunits[1].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings1, gc.DeepEquals, node1.Map())
}

type ContextRelationSuite struct {
	testing.JujuConnSuite
	svc *state.Service
	rel *state.Relation
	ru  *state.RelationUnit
}

var _ = gc.Suite(&ContextRelationSuite{})

func (s *ContextRelationSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	ch := s.AddTestingCharm(c, "riak")
	var err error
	s.svc, err = s.State.AddService("u", ch)
	c.Assert(err, gc.IsNil)
	rels, err := s.svc.Relations()
	c.Assert(err, gc.IsNil)
	c.Assert(rels, gc.HasLen, 1)
	s.rel = rels[0]
	unit, err := s.svc.AddUnit()
	c.Assert(err, gc.IsNil)
	s.ru, err = s.rel.Unit(unit)
	c.Assert(err, gc.IsNil)
	err = s.ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)
}

func (s *ContextRelationSuite) TestChangeMembers(c *gc.C) {
	ctx := apiuniter.NewContextRelation(s.ru, nil)
	c.Assert(ctx.UnitNames(), gc.HasLen, 0)

	// Check the units and settings after a simple update.
	ctx.UpdateMembers(apiuniter.SettingsMap{
		"u/2": {"baz": 2},
		"u/4": {"qux": 4},
	})
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/2", "u/4"})
	assertSettings := func(unit string, expect map[string]interface{}) {
		actual, err := ctx.ReadSettings(unit)
		c.Assert(err, gc.IsNil)
		c.Assert(actual, gc.DeepEquals, expect)
	}
	assertSettings("u/2", map[string]interface{}{"baz": 2})
	assertSettings("u/4", map[string]interface{}{"qux": 4})

	// Send a second update; check that members are only added, not removed.
	ctx.UpdateMembers(apiuniter.SettingsMap{
		"u/1": {"foo": 1},
		"u/2": {"abc": 2},
		"u/3": {"bar": 3},
	})
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1", "u/2", "u/3", "u/4"})

	// Check that all settings remain cached.
	assertSettings("u/1", map[string]interface{}{"foo": 1})
	assertSettings("u/2", map[string]interface{}{"abc": 2})
	assertSettings("u/3", map[string]interface{}{"bar": 3})
	assertSettings("u/4", map[string]interface{}{"qux": 4})

	// Delete a member, and check that it is no longer a member...
	ctx.DeleteMember("u/2")
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1", "u/3", "u/4"})

	// ...and that its settings are no longer cached.
	_, err := ctx.ReadSettings("u/2")
	c.Assert(err, gc.ErrorMatches, `cannot read settings for unit "u/2" in relation "u:ring": settings not found`)
}

func (s *ContextRelationSuite) TestMemberCaching(c *gc.C) {
	unit, err := s.svc.AddUnit()
	c.Assert(err, gc.IsNil)
	ru, err := s.rel.Unit(unit)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(map[string]interface{}{"blib": "blob"})
	c.Assert(err, gc.IsNil)
	settings, err := ru.Settings()
	c.Assert(err, gc.IsNil)
	settings.Set("ping", "pong")
	_, err = settings.Write()
	c.Assert(err, gc.IsNil)
	ctx := apiuniter.NewContextRelation(s.ru, map[string]int64{"u/1": 0})

	// Check that uncached settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	expect := settings.Map()
	c.Assert(m, gc.DeepEquals, expect)

	// Check that changes to state do not affect the cached settings.
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, gc.IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, expect)

	// Check that ClearCache spares the members cache.
	ctx.ClearCache()
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, expect)

	// Check that updating the context overwrites the cached settings, and
	// that the contents of state are ignored.
	ctx.UpdateMembers(apiuniter.SettingsMap{"u/1": {"entirely": "different"}})
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, map[string]interface{}{"entirely": "different"})
}

func (s *ContextRelationSuite) TestNonMemberCaching(c *gc.C) {
	unit, err := s.svc.AddUnit()
	c.Assert(err, gc.IsNil)
	ru, err := s.rel.Unit(unit)
	c.Assert(err, gc.IsNil)
	err = ru.EnterScope(map[string]interface{}{"blib": "blob"})
	c.Assert(err, gc.IsNil)
	settings, err := ru.Settings()
	c.Assert(err, gc.IsNil)
	settings.Set("ping", "pong")
	_, err = settings.Write()
	c.Assert(err, gc.IsNil)
	ctx := apiuniter.NewContextRelation(s.ru, nil)

	// Check that settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	expect := settings.Map()
	c.Assert(m, gc.DeepEquals, expect)

	// Check that changes to state do not affect the obtained settings...
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, gc.IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, expect)

	// ...until the caches are cleared.
	ctx.ClearCache()
	c.Assert(err, gc.IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m["ping"], gc.Equals, "pow")
}

func (s *ContextRelationSuite) TestSettings(c *gc.C) {
	ctx := apiuniter.NewContextRelation(s.ru, nil)

	// Change Settings, then clear cache without writing.
	node, err := ctx.Settings()
	c.Assert(err, gc.IsNil)
	expect := node.Map()
	node.Set("change", "exciting")
	ctx.ClearCache()

	// Check that the change is not cached...
	node, err = ctx.Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node.Map(), gc.DeepEquals, expect)

	// ...and not written to state.
	settings, err := s.ru.ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, expect)

	// Change again, write settings, and clear caches.
	node.Set("change", "exciting")
	err = ctx.WriteSettings()
	c.Assert(err, gc.IsNil)
	ctx.ClearCache()

	// Check that the change is reflected in Settings...
	expect["change"] = "exciting"
	node, err = ctx.Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node.Map(), gc.DeepEquals, expect)

	// ...and was written to state.
	settings, err = s.ru.ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, expect)
}

type InterfaceSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&InterfaceSuite{})

func (s *InterfaceSuite) GetContext(c *gc.C, relId int,
	remoteName string) jujuc.Context {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	return s.HookContextSuite.GetHookContext(c, uuid.String(), relId, remoteName)
}

func (s *InterfaceSuite) TestUtils(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
	r, found := ctx.HookRelation()
	c.Assert(found, gc.Equals, false)
	c.Assert(r, gc.IsNil)
	name, found := ctx.RemoteUnitName()
	c.Assert(found, gc.Equals, false)
	c.Assert(name, gc.Equals, "")
	c.Assert(ctx.RelationIds(), gc.HasLen, 2)
	r, found = ctx.Relation(0)
	c.Assert(found, gc.Equals, true)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:0")
	r, found = ctx.Relation(123)
	c.Assert(found, gc.Equals, false)
	c.Assert(r, gc.IsNil)

	ctx = s.GetContext(c, 1, "")
	r, found = ctx.HookRelation()
	c.Assert(found, gc.Equals, true)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:1")

	ctx = s.GetContext(c, 1, "u/123")
	name, found = ctx.RemoteUnitName()
	c.Assert(found, gc.Equals, true)
	c.Assert(name, gc.Equals, "u/123")
}

func (s *InterfaceSuite) TestUnitCaching(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	pr, ok := ctx.PrivateAddress()
	c.Assert(ok, gc.Equals, true)
	c.Assert(pr, gc.Equals, "u-0.testing.invalid")
	_, ok = ctx.PublicAddress()
	c.Assert(ok, gc.Equals, false)

	// Change remote state.
	u, err := s.State.Unit("u/0")
	c.Assert(err, gc.IsNil)
	err = u.SetPrivateAddress("")
	c.Assert(err, gc.IsNil)
	err = u.SetPublicAddress("blah.testing.invalid")
	c.Assert(err, gc.IsNil)

	// Local view is unchanged.
	pr, ok = ctx.PrivateAddress()
	c.Assert(ok, gc.Equals, true)
	c.Assert(pr, gc.Equals, "u-0.testing.invalid")
	_, ok = ctx.PublicAddress()
	c.Assert(ok, gc.Equals, false)
}

func (s *InterfaceSuite) TestConfigCaching(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	settings, err := ctx.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})

	// Change remote config.
	err = s.service.UpdateConfigSettings(charm.Settings{
		"blog-title": "Something Else",
	})
	c.Assert(err, gc.IsNil)

	// Local view is not changed.
	settings, err = ctx.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})
}

type HookContextSuite struct {
	testing.JujuConnSuite
	service  *state.Service
	unit     *state.Unit
	relch    *state.Charm
	relunits map[int]*state.RelationUnit
	relctxs  map[int]*apiuniter.ContextRelation
}

func (s *HookContextSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	sch := s.AddTestingCharm(c, "wordpress")
	s.service, err = s.State.AddService("u", sch)
	c.Assert(err, gc.IsNil)
	s.unit = s.AddUnit(c, s.service)
	// Note: The unit must always have a charm URL set, because this
	// happens as part of the installation process (that happens
	// before the initial install hook).
	err = s.unit.SetCharmURL(sch.URL())
	c.Assert(err, gc.IsNil)
	s.relch = s.AddTestingCharm(c, "mysql")
	s.relunits = map[int]*state.RelationUnit{}
	s.relctxs = map[int]*apiuniter.ContextRelation{}
	s.AddContextRelation(c, "db0")
	s.AddContextRelation(c, "db1")
}

func (s *HookContextSuite) AddUnit(c *gc.C, svc *state.Service) *state.Unit {
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	name := strings.Replace(unit.Name(), "/", "-", 1)
	err = unit.SetPrivateAddress(name + ".testing.invalid")
	c.Assert(err, gc.IsNil)
	return unit
}

func (s *HookContextSuite) AddContextRelation(c *gc.C, name string) {
	_, err := s.State.AddService(name, s.relch)
	c.Assert(err, gc.IsNil)
	eps, err := s.State.InferEndpoints([]string{"u", name})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, gc.IsNil)
	s.relunits[rel.Id()] = ru
	err = ru.EnterScope(map[string]interface{}{"relation-name": name})
	c.Assert(err, gc.IsNil)
	s.relctxs[rel.Id()] = apiuniter.NewContextRelation(ru, nil)
}

func (s *HookContextSuite) GetHookContext(c *gc.C, uuid string, relid int,
	remote string) *apiuniter.HookContext {
	if relid != -1 {
		_, found := s.relctxs[relid]
		c.Assert(found, gc.Equals, true)
	}
	return apiuniter.NewHookContext(s.unit, "TestCtx", uuid, relid, remote,
		s.relctxs, apiAddrs)
}
