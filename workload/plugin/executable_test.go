// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugin_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/plugin"
)

type executableSuite struct {
	testing.BaseSuite

	stub   *gitjujutesting.Stub
	runner *fakeRunner
}

var _ = gc.Suite(&executableSuite{})

const exitstatus1 = "exit status 1: "

func (s *executableSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &gitjujutesting.Stub{}
	s.runner = &fakeRunner{stub: s.stub}
}

func (s *executableSuite) TestFindExecutablePluginCached(c *gc.C) {
	fops := &stubFops{stub: s.stub}
	paths := &stubPaths{stub: s.stub}
	paths.executable = filepath.Join("some", "dir", "juju-workload-a-plugin")

	found, err := plugin.TestFindExecutablePlugin("a-plugin", paths, fops.LookPath)
	c.Assert(err, jc.ErrorIsNil)

	found.RunCmd = nil
	c.Check(found, jc.DeepEquals, &plugin.ExecutablePlugin{
		Name:       "a-plugin",
		Executable: filepath.Join("some", "dir", "juju-workload-a-plugin"),
	})
	s.stub.CheckCallNames(c, "Executable")
}

func (s *executableSuite) TestFindExecutablePluginLookedUp(c *gc.C) {
	paths := &stubPaths{stub: s.stub}
	fops := &stubFops{stub: s.stub}
	fops.found = filepath.Join("some", "dir", "juju-workload-a-plugin")

	found, err := plugin.TestFindExecutablePlugin("a-plugin", paths, fops.LookPath)
	c.Assert(err, jc.ErrorIsNil)

	found.RunCmd = nil
	c.Check(found, jc.DeepEquals, &plugin.ExecutablePlugin{
		Name:       "a-plugin",
		Executable: filepath.Join("some", "dir", "juju-workload-a-plugin"),
	})
	s.stub.CheckCallNames(c, "Executable", "LookPath", "Init")
}

func (s *executableSuite) TestFindExecutablePluginNotFound(c *gc.C) {
	fops := &stubFops{stub: s.stub}
	paths := &stubPaths{stub: s.stub}

	_, err := plugin.TestFindExecutablePlugin("a-plugin", paths, fops.LookPath)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
	s.stub.CheckCallNames(c, "Executable", "LookPath")
}

func (s *executableSuite) TestPluginInterface(c *gc.C) {
	var _ workload.Plugin = (*plugin.ExecutablePlugin)(nil)
}

func (s *executableSuite) TestLaunch(c *gc.C) {
	s.runner.out = `{ "id" : "foo", "status": { "state" : "bar" } }`

	p := plugin.ExecutablePlugin{
		Name:       "Name",
		Executable: "Path",
		RunCmd:     s.runner.runCmd,
	}
	proc := charm.Workload{Image: "img"}

	pd, err := p.Launch(proc)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(pd, gc.Equals, workload.Details{
		ID: "foo",
		Status: workload.PluginStatus{
			State: "bar",
		},
	})
	s.stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "runCmd",
		Args: []interface{}{
			p.Name,
			exec.Command(p.Executable, "launch", `{"Name":"","Description":"","Type":"","TypeOptions":null,"Command":"","Image":"img","Ports":null,"Volumes":null,"EnvVars":null}`),
		},
	}})
}

func (s *executableSuite) TestLaunchBadOutput(c *gc.C) {
	s.runner.out = `not json`

	p := plugin.ExecutablePlugin{
		Name:       "Name",
		Executable: "Path",
		RunCmd:     s.runner.runCmd,
	}
	proc := charm.Workload{Image: "img"}
	_, err := p.Launch(proc)

	c.Assert(err, gc.ErrorMatches, `error parsing data for workload details.*`)
}

func (s *executableSuite) TestLaunchNoId(c *gc.C) {
	s.runner.out = `{ "status" : { "status" : "bar" } }`

	p := plugin.ExecutablePlugin{
		Name:       "Name",
		Executable: "Path",
		RunCmd:     s.runner.runCmd,
	}
	proc := charm.Workload{Image: "img"}
	_, err := p.Launch(proc)

	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *executableSuite) TestLaunchNoStatus(c *gc.C) {
	s.runner.out = `{ "id" : "foo" }`

	p := plugin.ExecutablePlugin{
		Name:       "Name",
		Executable: "Path",
		RunCmd:     s.runner.runCmd,
	}
	proc := charm.Workload{Image: "img"}
	_, err := p.Launch(proc)

	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *executableSuite) TestLaunchErr(c *gc.C) {
	failure := errors.New("foo")
	s.stub.SetErrors(failure)

	p := plugin.ExecutablePlugin{
		Name:       "Name",
		Executable: "Path",
		RunCmd:     s.runner.runCmd,
	}
	proc := charm.Workload{Image: "img"}
	_, err := p.Launch(proc)

	c.Assert(errors.Cause(err), gc.Equals, failure)
}

func (s *executableSuite) TestStatus(c *gc.C) {
	s.runner.out = `{ "state" : "status!" }`

	p := plugin.ExecutablePlugin{
		Name:       "Name",
		Executable: "Path",
		RunCmd:     s.runner.runCmd,
	}
	status, err := p.Status("id")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status, jc.DeepEquals, workload.PluginStatus{
		State: "status!",
	})
	s.stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "runCmd",
		Args: []interface{}{
			p.Name,
			exec.Command(p.Executable, "status", "id"),
		},
	}})
}

func (s *executableSuite) TestStatusErr(c *gc.C) {
	failure := errors.New("foo")
	s.stub.SetErrors(failure)

	p := plugin.ExecutablePlugin{
		Name:       "Name",
		Executable: "Path",
		RunCmd:     s.runner.runCmd,
	}
	_, err := p.Status("id")

	c.Assert(errors.Cause(err), gc.Equals, failure)
}

func (s *executableSuite) TestDestroy(c *gc.C) {
	p := plugin.ExecutablePlugin{
		Name:       "Name",
		Executable: "Path",
		RunCmd:     s.runner.runCmd,
	}
	err := p.Destroy("id")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "runCmd",
		Args: []interface{}{
			p.Name,
			exec.Command(p.Executable, "destroy", "id"),
		},
	}})
}

func (s *executableSuite) TestDestroyErr(c *gc.C) {
	failure := errors.New("foo")
	s.stub.SetErrors(failure)

	p := plugin.ExecutablePlugin{
		Name:       "Name",
		Executable: "Path",
		RunCmd:     s.runner.runCmd,
	}
	err := p.Destroy("id")

	c.Assert(errors.Cause(err), gc.Equals, failure)
}

func (s *executableSuite) TestRunCmd(c *gc.C) {
	c.Skip("this test is an integration test in disguise")

	log := func(b []byte) {
		s.stub.AddCall("log", b)
		s.stub.NextErr() // ignored
	}
	m := maker{
		stdout: "foo!",
		stderr: "bar!\nbaz!",
	}
	cmd := m.make()
	out, err := plugin.RunCmd("name", cmd, log)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(strings.TrimSpace(string(out)), gc.DeepEquals, m.stdout)
	s.stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "log",
		Args:     []interface{}{[]byte("bar!")},
	}, {
		FuncName: "log",
		Args:     []interface{}{[]byte("baz!")},
	}})
}

func (s *executableSuite) TestRunCmdErr(c *gc.C) {
	c.Skip("this test is an integration test in disguise")

	log := func(b []byte) {
		s.stub.AddCall("log", b)
		s.stub.NextErr() // ignored
	}
	m := maker{
		exit:   1,
		stdout: "foo!",
	}
	cmd := m.make()
	_, err := plugin.RunCmd("name", cmd, log)

	c.Check(err, gc.ErrorMatches, "exit status 1: foo!")
	s.stub.CheckCalls(c, nil)
}

type fakeRunner struct {
	stub *gitjujutesting.Stub

	out string
}

func (f *fakeRunner) runCmd(name string, cmd *exec.Cmd) ([]byte, error) {
	f.stub.AddCall("runCmd", name, cmd)
	if err := f.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return []byte(f.out), nil
}

const (
	isHelperProc = "GO_HELPER_PROCESS_OK"
	helperStdout = "GO_HELPER_PROCESS_STDOUT"
	helperStderr = "GO_HELPER_PROCESS_STDERR"
	helperExit   = "GO_HELPER_PROCESS_EXIT_CODE"
)

type maker struct {
	stdout string
	stderr string
	exit   int
}

func (m maker) make() *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperWorkload")
	cmd.Env = []string{
		fmt.Sprintf("%s=%s", isHelperProc, "1"),
		fmt.Sprintf("%s=%s", helperStdout, m.stdout),
		fmt.Sprintf("%s=%s", helperStderr, m.stderr),
		fmt.Sprintf("%s=%d", helperExit, m.exit),
	}
	return cmd
}

func TestHelperWorkload(*stdtesting.T) {
	if os.Getenv(isHelperProc) != "1" {
		return
	}
	exit, err := strconv.Atoi(os.Getenv(helperExit))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error converting exit code: %s", err)
		os.Exit(2)
	}
	defer os.Exit(exit)

	if stderr := os.Getenv(helperStderr); stderr != "" {
		fmt.Fprint(os.Stderr, stderr)
	}

	if stdout := os.Getenv(helperStdout); stdout != "" {
		fmt.Fprint(os.Stdout, stdout)
	}
}

type stubPaths struct {
	stub *gitjujutesting.Stub

	executable string
}

func (s *stubPaths) Executable() (string, error) {
	s.stub.AddCall("Executable")
	if err := s.stub.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	if s.executable == "" {
		return "", errors.NotFoundf("...")
	}
	return s.executable, nil
}

func (s *stubPaths) Init(executable string) error {
	s.stub.AddCall("Init", executable)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
