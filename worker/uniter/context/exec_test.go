// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter/context"
)

type MergeEnvSuite struct {
	envtesting.IsolationSuite
}

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

	newEnv := context.MergeEnvironment([]string{"DUMMYVAR2=bar", "NEWVAR=ImNew"})
	c.Assert(expectEnv, jc.SameContents, newEnv)
}

type RunCommandSuite struct {
	HookContextSuite
}

var _ = gc.Suite(&RunCommandSuite{})

func (s *RunCommandSuite) getHookContext(c *gc.C, addMetrics bool) *context.HookContext {
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

	c.Assert(context.HookCommand(hookname), gc.DeepEquals, expected)
	restorer()
}

func (s *WindowsHookSuite) TestHookCommandNotPowerShellScripts(c *gc.C) {
	restorer := envtesting.PatchValue(&version.Current.OS, version.Windows)

	cmdhook := "somehook.cmd"
	c.Assert(context.HookCommand(cmdhook), gc.DeepEquals, []string{cmdhook})

	bathook := "somehook.bat"
	c.Assert(context.HookCommand(bathook), gc.DeepEquals, []string{bathook})

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

	expected, err := context.LookPath(filepath.Join(charmDir, "hooks", "something-happened"))
	c.Assert(err, gc.IsNil)
	obtained, err := context.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
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
	obtained, err := context.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
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
	obtained, err := context.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
	c.Assert(err, gc.ErrorMatches, "hooks/something-happened does not exist")
	c.Assert(obtained, gc.Equals, "")
}

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
			c.Assert(context.IsMissingHookError(err), jc.IsTrue)
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
