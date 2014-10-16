// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/names"
	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/api"
	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/jujuc"
)

var noProxies = proxy.Settings{}

type RunHookSuite struct {
	HookContextSuite
}

type MergeEnvSuite struct {
	envtesting.IsolationSuite
}

var _ = gc.Suite(&RunHookSuite{})
var _ = gc.Suite(&MergeEnvSuite{})

func (e *MergeEnvSuite) TestMergeEnviron(c *gc.C) {
	// environment does not get fully cleared on Windows
	// when using testing.IsolationSuite
	origEnv := os.Environ()
	extraExpected := []string{
		"DUMMYVAR=foo",
		"DUMMYVAR2=bar",
		"NEWVAR=ImNew",
	}
	expectEnv := append(origEnv, extraExpected...)
	os.Setenv("DUMMYVAR2", "ChangeMe")
	os.Setenv("DUMMYVAR", "foo")

	newEnv := uniter.MergeEnvironment([]string{"DUMMYVAR2=bar", "NEWVAR=ImNew"})
	c.Assert(expectEnv, jc.SameContents, newEnv)
}

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
		c.Assert(found, jc.IsTrue, comment)
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
	summary       string
	relid         int
	remote        string
	spec          hookSpec
	err           string
	env           map[string]string
	proxySettings proxy.Settings
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
		proxySettings: proxy.Settings{
			Http: "http", Https: "https", Ftp: "ftp", NoProxy: "no proxy"},
		env: map[string]string{
			"JUJU_UNIT_NAME":     "u/0",
			"JUJU_API_ADDRESSES": expectedApiAddrs,
			"JUJU_ENV_NAME":      "test-env-name",
			"http_proxy":         "http",
			"HTTP_PROXY":         "http",
			"https_proxy":        "https",
			"HTTPS_PROXY":        "https",
			"ftp_proxy":          "ftp",
			"FTP_PROXY":          "ftp",
			"no_proxy":           "no proxy",
			"NO_PROXY":           "no proxy",
		},
	}, {
		summary: "check shell environment for relation-broken hook context",
		relid:   1,
		spec:    hookSpec{perm: 0700},
		env: map[string]string{
			"JUJU_UNIT_NAME":     "u/0",
			"JUJU_API_ADDRESSES": expectedApiAddrs,
			"JUJU_ENV_NAME":      "test-env-name",
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
			"JUJU_ENV_NAME":      "test-env-name",
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
		c.Logf("\ntest %d: %s; perm %v", i, t.summary, t.spec.perm)
		ctx := s.getHookContext(c, uuid.String(), t.relid, t.remote, t.proxySettings, false)
		var charmDir, outPath string
		var hookExists bool
		if t.spec.perm == 0 {
			charmDir = c.MkDir()
		} else {
			spec := t.spec
			spec.name = "something-happened"
			c.Logf("makeCharm %#v", spec)
			charmDir, outPath = makeCharm(c, spec)
			hookExists = true
		}
		toolsDir := c.MkDir()
		t0 := time.Now()
		err := ctx.RunHook("something-happened", charmDir, toolsDir, "/path/to/socket")
		if t.err == "" && hookExists {
			c.Assert(err, gc.IsNil)
		} else if !hookExists {
			c.Assert(uniter.IsMissingHookError(err), jc.IsTrue)
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
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies, false)
	charmDir, _ := makeCharm(c, hookSpec{
		name: "something-happened",
		perm: 0700,
		code: 123,
	})

	// Mess with multiple relation settings.
	node0, err := s.relctxs[0].Settings()
	node0.Set("foo", "1")
	node1, err := s.relctxs[1].Settings()
	node1.Set("bar", "2")

	// Run the failing hook.
	err = ctx.RunHook("something-happened", charmDir, c.MkDir(), "/path/to/socket")
	c.Assert(err, gc.ErrorMatches, "exit status 123")

	// Check that the changes to the local settings nodes have been discarded.
	node0, err = s.relctxs[0].Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node0.Map(), gc.DeepEquals, params.RelationSettings{"relation-name": "db0"})
	node1, err = s.relctxs[1].Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node1.Map(), gc.DeepEquals, params.RelationSettings{"relation-name": "db1"})

	// Check that the changes have been written to state.
	settings0, err := s.relunits[0].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings0, gc.DeepEquals, map[string]interface{}{"relation-name": "db0"})
	settings1, err := s.relunits[1].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings1, gc.DeepEquals, map[string]interface{}{"relation-name": "db1"})

	// Create a charm with a working hook, and mess with settings again.
	charmDir, _ = makeCharm(c, hookSpec{
		name: "something-happened",
		perm: 0700,
	})
	node0.Set("baz", "3")
	node1.Set("qux", "4")

	// Run the hook.
	err = ctx.RunHook("something-happened", charmDir, c.MkDir(), "/path/to/socket")
	c.Assert(err, gc.IsNil)

	// Check that the changes to the local settings nodes are still there.
	node0, err = s.relctxs[0].Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node0.Map(), gc.DeepEquals, params.RelationSettings{
		"relation-name": "db0",
		"baz":           "3",
	})
	node1, err = s.relctxs[1].Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node1.Map(), gc.DeepEquals, params.RelationSettings{
		"relation-name": "db1",
		"qux":           "4",
	})

	// Check that the changes have been written to state.
	settings0, err = s.relunits[0].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings0, gc.DeepEquals, map[string]interface{}{
		"relation-name": "db0",
		"baz":           "3",
	})
	settings1, err = s.relunits[1].ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings1, gc.DeepEquals, map[string]interface{}{
		"relation-name": "db1",
		"qux":           "4",
	})
}

func (s *RunHookSuite) TestRunHookMetricSending(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies, true)
	charmDir, _ := makeCharm(c, hookSpec{
		name: "collect-metrics",
		perm: 0700,
	})

	now := time.Now()
	ctx.AddMetrics("key", "50", now)

	// Run the hook.
	err = ctx.RunHook("collect-metrics", charmDir, c.MkDir(), "/path/to/socket")
	c.Assert(err, gc.IsNil)

	metricBatches, err := s.State.MetricBatches()
	c.Assert(err, gc.IsNil)
	c.Assert(metricBatches, gc.HasLen, 1)
	metrics := metricBatches[0].Metrics()
	c.Assert(metrics, gc.HasLen, 1)
	c.Assert(metrics[0].Key, gc.Equals, "key")
	c.Assert(metrics[0].Value, gc.Equals, "50")
}

func (s *RunHookSuite) TestRunHookMetricSendingDisabled(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies, false)
	charmDir, _ := makeCharm(c, hookSpec{
		name: "some-hook",
		perm: 0700,
	})

	now := time.Now()
	err = ctx.AddMetrics("key", "50", now)
	c.Assert(err, gc.ErrorMatches, "metrics disabled")

	// Run the hook.
	err = ctx.RunHook("some-hook", charmDir, c.MkDir(), "/path/to/socket")
	c.Assert(err, gc.IsNil)

	metricBatches, err := s.State.MetricBatches()
	c.Assert(err, gc.IsNil)
	c.Assert(metricBatches, gc.HasLen, 0)
}

func (s *RunHookSuite) TestRunHookOpensAndClosesPendingPorts(c *gc.C) {
	// Initially, no port ranges are open on the unit or its machine.
	unitRanges, err := s.unit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(unitRanges, gc.HasLen, 0)
	machinePorts, err := s.machine.AllPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(machinePorts, gc.HasLen, 0)

	// Add another unit on the same machine.
	otherUnit, err := s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = otherUnit.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)

	// Open some ports on both units.
	err = s.unit.OpenPorts("tcp", 100, 200)
	c.Assert(err, gc.IsNil)
	err = otherUnit.OpenPorts("udp", 200, 300)
	c.Assert(err, gc.IsNil)

	unitRanges, err = s.unit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(unitRanges, jc.DeepEquals, []network.PortRange{
		{100, 200, "tcp"},
	})

	// Get the context.
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	ctx := s.getHookContext(c, uuid.String(), -1, "", noProxies, false)
	charmDir, _ := makeCharm(c, hookSpec{
		name: "some-hook",
		perm: 0700,
	})

	// Try opening some ports via the context.
	err = ctx.OpenPorts("tcp", 100, 200)
	c.Assert(err, gc.IsNil) // duplicates are ignored
	err = ctx.OpenPorts("udp", 200, 300)
	c.Assert(err, gc.ErrorMatches, `cannot open 200-300/udp \(unit "u/0"\): conflicts with existing 200-300/udp \(unit "u/1"\)`)
	err = ctx.OpenPorts("udp", 100, 200)
	c.Assert(err, gc.ErrorMatches, `cannot open 100-200/udp \(unit "u/0"\): conflicts with existing 200-300/udp \(unit "u/1"\)`)
	err = ctx.OpenPorts("udp", 10, 20)
	c.Assert(err, gc.IsNil)
	err = ctx.OpenPorts("tcp", 50, 100)
	c.Assert(err, gc.ErrorMatches, `cannot open 50-100/tcp \(unit "u/0"\): conflicts with existing 100-200/tcp \(unit "u/0"\)`)
	err = ctx.OpenPorts("tcp", 50, 80)
	c.Assert(err, gc.IsNil)
	err = ctx.OpenPorts("tcp", 40, 90)
	c.Assert(err, gc.ErrorMatches, `cannot open 40-90/tcp \(unit "u/0"\): conflicts with 50-80/tcp requested earlier`)

	// Now try closing some ports as well.
	err = ctx.ClosePorts("udp", 8080, 8088)
	c.Assert(err, gc.IsNil) // not existing -> ignored
	err = ctx.ClosePorts("tcp", 100, 200)
	c.Assert(err, gc.IsNil)
	err = ctx.ClosePorts("tcp", 100, 200)
	c.Assert(err, gc.IsNil) // duplicates are ignored
	err = ctx.ClosePorts("udp", 200, 300)
	c.Assert(err, gc.ErrorMatches, `cannot close 200-300/udp \(opened by "u/1"\) from "u/0"`)
	err = ctx.ClosePorts("tcp", 50, 80)
	c.Assert(err, gc.IsNil) // still pending -> no longer pending

	// Ensure the ports are not actually changed on the unit yet.
	unitRanges, err = s.unit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(unitRanges, jc.DeepEquals, []network.PortRange{
		{100, 200, "tcp"},
	})

	// Simulate a hook ran and was committed successfully.
	err = ctx.RunHook("some-hook", charmDir, c.MkDir(), "/path/to/socket")
	c.Assert(err, gc.IsNil)

	// Verify the unit ranges are now open.
	expectUnitRanges := []network.PortRange{
		{FromPort: 10, ToPort: 20, Protocol: "udp"},
	}
	unitRanges, err = s.unit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(unitRanges, jc.DeepEquals, expectUnitRanges)

	// Test idempotency by running the hook again.
	err = ctx.RunHook("some-hook", charmDir, c.MkDir(), "/path/to/socket")
	c.Assert(err, gc.IsNil)

	unitRanges, err = s.unit.OpenedPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(unitRanges, jc.DeepEquals, expectUnitRanges)
}

type ContextRelationSuite struct {
	testing.JujuConnSuite
	svc *state.Service
	rel *state.Relation
	ru  *state.RelationUnit

	st         *api.State
	uniter     *apiuniter.State
	apiRelUnit *apiuniter.RelationUnit
}

var _ = gc.Suite(&ContextRelationSuite{})

func (s *ContextRelationSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)

	ch := s.AddTestingCharm(c, "riak")
	s.svc = s.AddTestingService(c, "u", ch)
	rels, err := s.svc.Relations()
	c.Assert(err, gc.IsNil)
	c.Assert(rels, gc.HasLen, 1)
	s.rel = rels[0]
	unit, err := s.svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.AssignToMachine(machine)
	s.ru, err = s.rel.Unit(unit)
	c.Assert(err, gc.IsNil)
	err = s.ru.EnterScope(nil)
	c.Assert(err, gc.IsNil)

	password, err = utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAs(c, unit.Tag(), password)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, gc.IsNil)
	c.Assert(s.uniter, gc.NotNil)

	apiRel, err := s.uniter.Relation(s.rel.Tag().(names.RelationTag))
	c.Assert(err, gc.IsNil)
	apiUnit, err := s.uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, gc.IsNil)
	s.apiRelUnit, err = apiRel.Unit(apiUnit)
	c.Assert(err, gc.IsNil)
}

func (s *ContextRelationSuite) TestChangeMembers(c *gc.C) {
	ctx := uniter.NewContextRelation(s.apiRelUnit, nil)
	c.Assert(ctx.UnitNames(), gc.HasLen, 0)

	// Check the units and settings after a simple update.
	ctx.UpdateMembers(uniter.SettingsMap{
		"u/2": {"baz": "2"},
		"u/4": {"qux": "4"},
	})
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/2", "u/4"})
	assertSettings := func(unit string, expect params.RelationSettings) {
		actual, err := ctx.ReadSettings(unit)
		c.Assert(err, gc.IsNil)
		c.Assert(actual, gc.DeepEquals, expect)
	}
	assertSettings("u/2", params.RelationSettings{"baz": "2"})
	assertSettings("u/4", params.RelationSettings{"qux": "4"})

	// Send a second update; check that members are only added, not removed.
	ctx.UpdateMembers(uniter.SettingsMap{
		"u/1": {"foo": "1"},
		"u/2": {"abc": "2"},
		"u/3": {"bar": "3"},
	})
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1", "u/2", "u/3", "u/4"})

	// Check that all settings remain cached.
	assertSettings("u/1", params.RelationSettings{"foo": "1"})
	assertSettings("u/2", params.RelationSettings{"abc": "2"})
	assertSettings("u/3", params.RelationSettings{"bar": "3"})
	assertSettings("u/4", params.RelationSettings{"qux": "4"})

	// Delete a member, and check that it is no longer a member...
	ctx.DeleteMember("u/2")
	c.Assert(ctx.UnitNames(), gc.DeepEquals, []string{"u/1", "u/3", "u/4"})

	// ...and that its settings are no longer cached.
	_, err := ctx.ReadSettings("u/2")
	c.Assert(err, gc.ErrorMatches, "cannot read settings for unit \"u/2\" in relation \"u:ring\": settings not found")
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
	ctx := uniter.NewContextRelation(s.apiRelUnit, map[string]int64{"u/1": 0})

	// Check that uncached settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	expectMap := settings.Map()
	expectSettings := convertMap(expectMap)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// Check that changes to state do not affect the cached settings.
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, gc.IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// Check that ClearCache spares the members cache.
	ctx.ClearCache()
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// Check that updating the context overwrites the cached settings, and
	// that the contents of state are ignored.
	ctx.UpdateMembers(uniter.SettingsMap{"u/1": {"entirely": "different"}})
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, params.RelationSettings{"entirely": "different"})
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
	ctx := uniter.NewContextRelation(s.apiRelUnit, nil)

	// Check that settings are read from state.
	m, err := ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	expectMap := settings.Map()
	expectSettings := convertMap(expectMap)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// Check that changes to state do not affect the obtained settings...
	settings.Set("ping", "pow")
	_, err = settings.Write()
	c.Assert(err, gc.IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.DeepEquals, expectSettings)

	// ...until the caches are cleared.
	ctx.ClearCache()
	c.Assert(err, gc.IsNil)
	m, err = ctx.ReadSettings("u/1")
	c.Assert(err, gc.IsNil)
	c.Assert(m["ping"], gc.Equals, "pow")
}

func (s *ContextRelationSuite) TestSettings(c *gc.C) {
	ctx := uniter.NewContextRelation(s.apiRelUnit, nil)

	// Change Settings, then clear cache without writing.
	node, err := ctx.Settings()
	c.Assert(err, gc.IsNil)
	expectSettings := node.Map()
	expectMap := convertSettings(expectSettings)
	node.Set("change", "exciting")
	ctx.ClearCache()

	// Check that the change is not cached...
	node, err = ctx.Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node.Map(), gc.DeepEquals, expectSettings)

	// ...and not written to state.
	settings, err := s.ru.ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, expectMap)

	// Change again, write settings, and clear caches.
	node.Set("change", "exciting")
	err = ctx.WriteSettings()
	c.Assert(err, gc.IsNil)
	ctx.ClearCache()

	// Check that the change is reflected in Settings...
	expectSettings["change"] = "exciting"
	expectMap["change"] = expectSettings["change"]
	node, err = ctx.Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(node.Map(), gc.DeepEquals, expectSettings)

	// ...and was written to state.
	settings, err = s.ru.ReadSettings("u/0")
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, expectMap)
}

type InterfaceSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&InterfaceSuite{})

func (s *InterfaceSuite) GetContext(c *gc.C, relId int,
	remoteName string) jujuc.Context {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	return s.HookContextSuite.getHookContext(c, uuid.String(), relId, remoteName, noProxies, false)
}

func (s *InterfaceSuite) TestUtils(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
	r, found := ctx.HookRelation()
	c.Assert(found, jc.IsFalse)
	c.Assert(r, gc.IsNil)
	name, found := ctx.RemoteUnitName()
	c.Assert(found, jc.IsFalse)
	c.Assert(name, gc.Equals, "")
	c.Assert(ctx.RelationIds(), gc.HasLen, 2)
	r, found = ctx.Relation(0)
	c.Assert(found, jc.IsTrue)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:0")
	r, found = ctx.Relation(123)
	c.Assert(found, jc.IsFalse)
	c.Assert(r, gc.IsNil)

	ctx = s.GetContext(c, 1, "")
	r, found = ctx.HookRelation()
	c.Assert(found, jc.IsTrue)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:1")

	ctx = s.GetContext(c, 1, "u/123")
	name, found = ctx.RemoteUnitName()
	c.Assert(found, jc.IsTrue)
	c.Assert(name, gc.Equals, "u/123")
}

func (s *InterfaceSuite) TestUnitCaching(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	pr, ok := ctx.PrivateAddress()
	c.Assert(ok, jc.IsTrue)
	c.Assert(pr, gc.Equals, "u-0.testing.invalid")
	pa, ok := ctx.PublicAddress()
	c.Assert(ok, jc.IsTrue)
	// Initially the public address is the same as the private address since
	// the "most public" address is chosen.
	c.Assert(pr, gc.Equals, pa)

	// Change remote state.
	err := s.machine.SetAddresses(
		network.NewAddress("blah.testing.invalid", network.ScopePublic))
	c.Assert(err, gc.IsNil)

	// Local view is unchanged.
	pr, ok = ctx.PrivateAddress()
	c.Assert(ok, jc.IsTrue)
	c.Assert(pr, gc.Equals, "u-0.testing.invalid")
	pa, ok = ctx.PublicAddress()
	c.Assert(ok, jc.IsTrue)
	c.Assert(pr, gc.Equals, pa)
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

func (s *InterfaceSuite) TestValidatePortRange(c *gc.C) {
	tests := []struct {
		about     string
		proto     string
		ports     []int
		portRange network.PortRange
		expectErr string
	}{{
		about:     "invalid range - 0-0/tcp",
		proto:     "tcp",
		ports:     []int{0, 0},
		expectErr: "invalid port range 0-0/tcp",
	}, {
		about:     "invalid range - 0-1/tcp",
		proto:     "tcp",
		ports:     []int{0, 1},
		expectErr: "invalid port range 0-1/tcp",
	}, {
		about:     "invalid range - -1-1/tcp",
		proto:     "tcp",
		ports:     []int{-1, 1},
		expectErr: "invalid port range -1-1/tcp",
	}, {
		about:     "invalid range - 1-99999/tcp",
		proto:     "tcp",
		ports:     []int{1, 99999},
		expectErr: "invalid port range 1-99999/tcp",
	}, {
		about:     "invalid range - 88888-99999/tcp",
		proto:     "tcp",
		ports:     []int{88888, 99999},
		expectErr: "invalid port range 88888-99999/tcp",
	}, {
		about:     "invalid protocol - 1-65535/foo",
		proto:     "foo",
		ports:     []int{1, 65535},
		expectErr: `invalid protocol "foo", expected "tcp" or "udp"`,
	}, {
		about: "valid range - 100-200/udp",
		proto: "UDP",
		ports: []int{100, 200},
		portRange: network.PortRange{
			FromPort: 100,
			ToPort:   200,
			Protocol: "udp",
		},
	}, {
		about: "valid single port - 100/tcp",
		proto: "TCP",
		ports: []int{100, 100},
		portRange: network.PortRange{
			FromPort: 100,
			ToPort:   100,
			Protocol: "tcp",
		},
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)
		portRange, err := uniter.ValidatePortRange(
			test.proto,
			test.ports[0],
			test.ports[1],
		)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
			c.Check(portRange, jc.DeepEquals, network.PortRange{})
		} else {
			c.Check(err, gc.IsNil)
			c.Check(portRange, jc.DeepEquals, test.portRange)
		}
	}
}

func makeMachinePorts(
	unitName, proto string, fromPort, toPort int,
) map[network.PortRange]params.RelationUnit {
	result := make(map[network.PortRange]params.RelationUnit)
	portRange := network.PortRange{
		FromPort: fromPort,
		ToPort:   toPort,
		Protocol: proto,
	}
	unitTag := ""
	if unitName != "invalid" {
		unitTag = names.NewUnitTag(unitName).String()
	} else {
		unitTag = unitName
	}
	result[portRange] = params.RelationUnit{
		Unit: unitTag,
	}
	return result
}

func makePendingPorts(
	proto string, fromPort, toPort int, shouldOpen bool,
) map[uniter.PortRange]uniter.PortRangeInfo {
	result := make(map[uniter.PortRange]uniter.PortRangeInfo)
	portRange := network.PortRange{
		FromPort: fromPort,
		ToPort:   toPort,
		Protocol: proto,
	}
	key := uniter.PortRange{
		Ports:      portRange,
		RelationId: -1,
	}
	result[key] = uniter.PortRangeInfo{
		ShouldOpen: shouldOpen,
	}
	return result
}

type portsTest struct {
	about         string
	proto         string
	ports         []int
	machinePorts  map[network.PortRange]params.RelationUnit
	pendingPorts  map[uniter.PortRange]uniter.PortRangeInfo
	expectErr     string
	expectPending map[uniter.PortRange]uniter.PortRangeInfo
}

func (p portsTest) withDefaults(proto string, fromPort, toPort int) portsTest {
	if p.proto == "" {
		p.proto = proto
	}
	if len(p.ports) != 2 {
		p.ports = []int{fromPort, toPort}
	}
	if p.pendingPorts == nil {
		p.pendingPorts = make(map[uniter.PortRange]uniter.PortRangeInfo)
	}
	return p
}

func (s *InterfaceSuite) TestTryOpenPorts(c *gc.C) {
	tests := []portsTest{{
		about:     "invalid port range",
		ports:     []int{0, 0},
		expectErr: "invalid port range 0-0/tcp",
	}, {
		about:     "invalid protocol - 10-20/foo",
		proto:     "foo",
		expectErr: `invalid protocol "foo", expected "tcp" or "udp"`,
	}, {
		about:         "open a new range (no machine ports yet)",
		expectPending: makePendingPorts("tcp", 10, 20, true),
	}, {
		about:         "open an existing range (ignored)",
		machinePorts:  makeMachinePorts("u/0", "tcp", 10, 20),
		expectPending: map[uniter.PortRange]uniter.PortRangeInfo{},
	}, {
		about:         "open a range pending to be closed already",
		pendingPorts:  makePendingPorts("tcp", 10, 20, false),
		expectPending: makePendingPorts("tcp", 10, 20, true),
	}, {
		about:         "open a range pending to be opened already (ignored)",
		pendingPorts:  makePendingPorts("tcp", 10, 20, true),
		expectPending: makePendingPorts("tcp", 10, 20, true),
	}, {
		about:        "try opening a range when machine ports has invalid unit tag",
		machinePorts: makeMachinePorts("invalid", "tcp", 80, 90),
		expectErr:    `machine ports 80-90/tcp contain invalid unit tag: "invalid" is not a valid tag`,
	}, {
		about:        "try opening a range conflicting with another unit",
		machinePorts: makeMachinePorts("u/1", "tcp", 10, 20),
		expectErr:    `cannot open 10-20/tcp \(unit "u/0"\): conflicts with existing 10-20/tcp \(unit "u/1"\)`,
	}, {
		about:         "open a range conflicting with the same unit (ignored)",
		machinePorts:  makeMachinePorts("u/0", "tcp", 10, 20),
		expectPending: map[uniter.PortRange]uniter.PortRangeInfo{},
	}, {
		about:        "try opening a range conflicting with another pending range",
		pendingPorts: makePendingPorts("tcp", 5, 25, true),
		expectErr:    `cannot open 10-20/tcp \(unit "u/0"\): conflicts with 5-25/tcp requested earlier`,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)

		test = test.withDefaults("tcp", 10, 20)
		err := uniter.TryOpenPorts(
			test.proto,
			test.ports[0],
			test.ports[1],
			names.NewUnitTag("u/0"),
			test.machinePorts,
			test.pendingPorts,
		)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, gc.IsNil)
			c.Check(test.pendingPorts, jc.DeepEquals, test.expectPending)
		}
	}
}

func (s *InterfaceSuite) TestTryClosePorts(c *gc.C) {
	tests := []portsTest{{
		about:     "invalid port range",
		ports:     []int{0, 0},
		expectErr: "invalid port range 0-0/tcp",
	}, {
		about:     "invalid protocol - 10-20/foo",
		proto:     "foo",
		expectErr: `invalid protocol "foo", expected "tcp" or "udp"`,
	}, {
		about:         "close a new range (no machine ports yet; ignored)",
		expectPending: map[uniter.PortRange]uniter.PortRangeInfo{},
	}, {
		about:         "close an existing range",
		machinePorts:  makeMachinePorts("u/0", "tcp", 10, 20),
		expectPending: makePendingPorts("tcp", 10, 20, false),
	}, {
		about:         "close a range pending to be opened already (removed from pending)",
		pendingPorts:  makePendingPorts("tcp", 10, 20, true),
		expectPending: map[uniter.PortRange]uniter.PortRangeInfo{},
	}, {
		about:         "close a range pending to be closed already (ignored)",
		pendingPorts:  makePendingPorts("tcp", 10, 20, false),
		expectPending: makePendingPorts("tcp", 10, 20, false),
	}, {
		about:        "try closing an existing range when machine ports has invalid unit tag",
		machinePorts: makeMachinePorts("invalid", "tcp", 10, 20),
		expectErr:    `machine ports 10-20/tcp contain invalid unit tag: "invalid" is not a valid tag`,
	}, {
		about:        "try closing a range of another unit",
		machinePorts: makeMachinePorts("u/1", "tcp", 10, 20),
		expectErr:    `cannot close 10-20/tcp \(opened by "u/1"\) from "u/0"`,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)

		test = test.withDefaults("tcp", 10, 20)
		err := uniter.TryClosePorts(
			test.proto,
			test.ports[0],
			test.ports[1],
			names.NewUnitTag("u/0"),
			test.machinePorts,
			test.pendingPorts,
		)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, gc.IsNil)
			c.Check(test.pendingPorts, jc.DeepEquals, test.expectPending)
		}
	}
}

type HookContextSuite struct {
	testing.JujuConnSuite
	service  *state.Service
	unit     *state.Unit
	machine  *state.Machine
	relch    *state.Charm
	relunits map[int]*state.RelationUnit
	relctxs  map[int]*uniter.ContextRelation

	st      *api.State
	uniter  *apiuniter.State
	apiUnit *apiuniter.Unit
}

func (s *HookContextSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	var err error
	sch := s.AddTestingCharm(c, "wordpress")
	s.service = s.AddTestingService(c, "u", sch)
	s.unit = s.AddUnit(c, s.service)

	password, err := utils.RandomPassword()
	err = s.unit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAs(c, s.unit.Tag(), password)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, gc.IsNil)
	c.Assert(s.uniter, gc.NotNil)

	// Note: The unit must always have a charm URL set, because this
	// happens as part of the installation process (that happens
	// before the initial install hook).
	err = s.unit.SetCharmURL(sch.URL())
	c.Assert(err, gc.IsNil)
	s.relch = s.AddTestingCharm(c, "mysql")
	s.relunits = map[int]*state.RelationUnit{}
	s.relctxs = map[int]*uniter.ContextRelation{}
	s.AddContextRelation(c, "db0")
	s.AddContextRelation(c, "db1")
}

func (s *HookContextSuite) AddUnit(c *gc.C, svc *state.Service) *state.Unit {
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = unit.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)
	name := strings.Replace(unit.Name(), "/", "-", 1)
	privateAddr := network.NewAddress(name+".testing.invalid", network.ScopeCloudLocal)
	err = s.machine.SetAddresses(privateAddr)
	c.Assert(err, gc.IsNil)
	return unit
}

func (s *HookContextSuite) AddContextRelation(c *gc.C, name string) {
	s.AddTestingService(c, name, s.relch)
	eps, err := s.State.InferEndpoints("u", name)
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, gc.IsNil)
	s.relunits[rel.Id()] = ru
	err = ru.EnterScope(map[string]interface{}{"relation-name": name})
	c.Assert(err, gc.IsNil)
	s.apiUnit, err = s.uniter.Unit(s.unit.Tag().(names.UnitTag))
	c.Assert(err, gc.IsNil)
	// TODO(dfc) uniter.Relation should take a names.RelationTag
	apiRel, err := s.uniter.Relation(rel.Tag().(names.RelationTag))
	c.Assert(err, gc.IsNil)
	apiRelUnit, err := apiRel.Unit(s.apiUnit)
	c.Assert(err, gc.IsNil)
	s.relctxs[rel.Id()] = uniter.NewContextRelation(apiRelUnit, nil)
}

func (s *HookContextSuite) getHookContext(c *gc.C, uuid string, relid int,
	remote string, proxies proxy.Settings, addMetrics bool) *uniter.HookContext {
	if relid != -1 {
		_, found := s.relctxs[relid]
		c.Assert(found, jc.IsTrue)
	}
	facade, err := s.st.Uniter()
	c.Assert(err, gc.IsNil)
	context, err := uniter.NewHookContext(s.apiUnit, facade, "TestCtx", uuid,
		"test-env-name", relid, remote, s.relctxs, apiAddrs, names.NewUserTag("owner"),
		proxies, addMetrics, nil, s.machine.Tag().(names.MachineTag))
	c.Assert(err, gc.IsNil)
	return context
}

// TestNonActionCallsToActionMethodsFail does exactly what its name says:
// it simply makes sure that Action-related calls to HookContexts with a nil
// actionData member error out correctly.
func (s *HookContextSuite) TestNonActionCallsToActionMethodsFail(c *gc.C) {
	ctx := uniter.HookContext{}
	_, err := ctx.ActionParams()
	c.Check(err, gc.ErrorMatches, "not running an action")
	err = ctx.SetActionFailed()
	c.Check(err, gc.ErrorMatches, "not running an action")
	err = ctx.SetActionMessage("foo")
	c.Check(err, gc.ErrorMatches, "not running an action")
	err = ctx.RunAction("asdf", "fdsa", "qwerty", "uiop")
	c.Check(err, gc.ErrorMatches, "not running an action")
	err = ctx.UpdateActionResults([]string{"1", "2", "3"}, "value")
	c.Check(err, gc.ErrorMatches, "not running an action")
}

// TestUpdateActionResults demonstrates that UpdateActionResults functions
// as expected.
func (s *HookContextSuite) TestUpdateActionResults(c *gc.C) {
	tests := []struct {
		initial  map[string]interface{}
		keys     []string
		value    string
		expected map[string]interface{}
	}{{
		initial: map[string]interface{}{},
		keys:    []string{"foo"},
		value:   "bar",
		expected: map[string]interface{}{
			"foo": "bar",
		},
	}, {
		initial: map[string]interface{}{
			"foo": "bar",
		},
		keys:  []string{"foo", "bar"},
		value: "baz",
		expected: map[string]interface{}{
			"foo": map[string]interface{}{
				"bar": "baz",
			},
		},
	}, {
		initial: map[string]interface{}{
			"foo": map[string]interface{}{
				"bar": "baz",
			},
		},
		keys:  []string{"foo"},
		value: "bar",
		expected: map[string]interface{}{
			"foo": "bar",
		},
	}}

	for i, t := range tests {
		c.Logf("UpdateActionResults test %d: %#v: %#v", i, t.keys, t.value)
		hctx := uniter.GetStubActionContext(t.initial)
		err := hctx.UpdateActionResults(t.keys, t.value)
		c.Assert(err, gc.IsNil)
		c.Check(hctx.ActionResultsMap(), jc.DeepEquals, t.expected)
	}
}

// TestSetActionFailed ensures SetActionFailed works properly.
func (s *HookContextSuite) TestSetActionFailed(c *gc.C) {
	hctx := uniter.GetStubActionContext(nil)
	err := hctx.SetActionFailed()
	c.Assert(err, gc.IsNil)
	c.Check(hctx.ActionFailed(), jc.IsTrue)
}

// TestSetActionMessage ensures SetActionMessage works properly.
func (s *HookContextSuite) TestSetActionMessage(c *gc.C) {
	hctx := uniter.GetStubActionContext(nil)
	err := hctx.SetActionMessage("because reasons")
	c.Assert(err, gc.IsNil)
	c.Check(hctx.ActionMessage(), gc.Equals, "because reasons")
}

func convertSettings(settings params.RelationSettings) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range settings {
		result[k] = v
	}
	return result
}

func convertMap(settingsMap map[string]interface{}) params.RelationSettings {
	result := make(params.RelationSettings)
	for k, v := range settingsMap {
		result[k] = v.(string)
	}
	return result
}

type RunCommandSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&RunCommandSuite{})

func (s *RunCommandSuite) getHookContext(c *gc.C, addMetrics bool) *uniter.HookContext {
	uuid, err := utils.NewUUID()
	c.Assert(err, gc.IsNil)
	return s.HookContextSuite.getHookContext(c, uuid.String(), -1, "", noProxies, addMetrics)
}

func (s *RunCommandSuite) TestRunCommandsHasEnvironSet(c *gc.C) {
	context := s.getHookContext(c, false)
	charmDir := c.MkDir()
	result, err := context.RunCommands("env | sort", charmDir, "/path/to/tools", "/path/to/socket")
	c.Assert(err, gc.IsNil)

	executionEnvironment := map[string]string{}
	for _, value := range strings.Split(string(result.Stdout), "\n") {
		bits := strings.SplitN(value, "=", 2)
		if len(bits) == 2 {
			executionEnvironment[bits[0]] = bits[1]
		}
	}
	expected := map[string]string{
		"APT_LISTCHANGES_FRONTEND": "none",
		"DEBIAN_FRONTEND":          "noninteractive",
		"CHARM_DIR":                charmDir,
		"JUJU_CONTEXT_ID":          "TestCtx",
		"JUJU_AGENT_SOCKET":        "/path/to/socket",
		"JUJU_UNIT_NAME":           "u/0",
		"JUJU_ENV_NAME":            "test-env-name",
	}
	for key, value := range expected {
		c.Check(executionEnvironment[key], gc.Equals, value)
	}
}

func (s *RunCommandSuite) TestRunCommandsHasEnvironSetWithMeterStatus(c *gc.C) {
	context := s.getHookContext(c, false)
	defer context.PatchMeterStatus("GREEN", "Operating normally.")()

	charmDir := c.MkDir()
	result, err := context.RunCommands("env | sort", charmDir, "/path/to/tools", "/path/to/socket")
	c.Assert(err, gc.IsNil)

	executionEnvironment := map[string]string{}
	for _, value := range strings.Split(string(result.Stdout), "\n") {
		bits := strings.SplitN(value, "=", 2)
		if len(bits) == 2 {
			executionEnvironment[bits[0]] = bits[1]
		}
	}
	expected := map[string]string{
		"JUJU_METER_STATUS": "GREEN",
		"JUJU_METER_INFO":   "Operating normally.",
	}
	for key, value := range expected {
		c.Check(executionEnvironment[key], gc.Equals, value)
	}
}

func (s *RunCommandSuite) TestRunCommandsStdOutAndErrAndRC(c *gc.C) {
	context := s.getHookContext(c, false)
	charmDir := c.MkDir()
	commands := `
echo this is standard out
echo this is standard err >&2
exit 42
`
	result, err := context.RunCommands(commands, charmDir, "/path/to/tools", "/path/to/socket")
	c.Assert(err, gc.IsNil)

	c.Assert(result.Code, gc.Equals, 42)
	c.Assert(string(result.Stdout), gc.Equals, "this is standard out\n")
	c.Assert(string(result.Stderr), gc.Equals, "this is standard err\n")
}

type WindowsHookSuite struct{}

var _ = gc.Suite(&WindowsHookSuite{})

func (s *WindowsHookSuite) TestHookCommandPowerShellScript(c *gc.C) {
	restorer := envtesting.PatchValue(&version.Current.OS, version.Windows)

	hookname := "powerShellScript.ps1"
	expected := []string{
		"powershell.exe",
		"-NonInteractive",
		"-ExecutionPolicy",
		"RemoteSigned",
		"-File",
		hookname,
	}

	c.Assert(uniter.HookCommand(hookname), gc.DeepEquals, expected)
	restorer()
}

func (s *WindowsHookSuite) TestHookCommandNotPowerShellScripts(c *gc.C) {
	restorer := envtesting.PatchValue(&version.Current.OS, version.Windows)

	cmdhook := "somehook.cmd"
	c.Assert(uniter.HookCommand(cmdhook), gc.DeepEquals, []string{cmdhook})

	bathook := "somehook.bat"
	c.Assert(uniter.HookCommand(bathook), gc.DeepEquals, []string{bathook})

	restorer()
}

func (s *WindowsHookSuite) TestSearchHookUbuntu(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping hook with no extension on Windows")
	}
	charmDir, _ := makeCharm(c, hookSpec{
		name: "something-happened",
		perm: 0755,
	})

	expected, err := uniter.LookPath(filepath.Join(charmDir, "hooks", "something-happened"))
	c.Assert(err, gc.IsNil)
	obtained, err := uniter.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.Equals, expected)
}

func (s *WindowsHookSuite) TestSearchHookWindows(c *gc.C) {
	charmDir, _ := makeCharm(c, hookSpec{
		name: "something-happened.ps1",
		perm: 0755,
	})

	restorer := envtesting.PatchValue(&version.Current.OS, version.Windows)

	defer restorer()
	obtained, err := uniter.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.Equals, filepath.Join(charmDir, "hooks", "something-happened.ps1"))
}

func (s *WindowsHookSuite) TestSearchHookWindowsError(c *gc.C) {
	charmDir, _ := makeCharm(c, hookSpec{
		name: "something-happened.linux",
		perm: 0755,
	})

	restorer := envtesting.PatchValue(&version.Current.OS, version.Windows)
	defer restorer()
	obtained, err := uniter.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
	c.Assert(err, gc.ErrorMatches, "hooks/something-happened does not exist")
	c.Assert(obtained, gc.Equals, "")
}
