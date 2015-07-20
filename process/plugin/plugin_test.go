package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/testing"
)

type suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&suite{})

const exitstatus1 = "exit status 1: "

func (s *suite) TestLaunch(c *gc.C) {
	f := &fakeRunner{
		out: []byte(`{ "id" : "foo", "status": { "label" : "bar" } }`),
	}
	s.PatchValue(&runCmd, f.runCmd)

	p := Plugin{"Name", "Path"}
	proc := charm.Process{Image: "img"}

	pd, err := p.Launch(proc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pd, gc.Equals, process.Details{
		ID: "foo",
		Status: process.PluginStatus{
			Label: "bar",
		},
	})

	c.Assert(f.name, gc.DeepEquals, p.Name)
	c.Assert(f.cmd.Path, gc.Equals, p.Executable)
	c.Assert(f.cmd.Args[1], gc.Equals, "launch")
	c.Assert(f.cmd.Args[2:], gc.HasLen, 1)
	// fix this to be more stringent when we fix json serialization for charm.Process
	c.Assert(f.cmd.Args[2], gc.Matches, `.*"Image":"img".*`)
}

func (s *suite) TestLaunchBadOutput(c *gc.C) {
	f := &fakeRunner{
		out: []byte(`not json`),
	}
	s.PatchValue(&runCmd, f.runCmd)

	p := Plugin{"Name", "Path"}
	proc := charm.Process{Image: "img"}

	_, err := p.Launch(proc)
	c.Assert(err, gc.ErrorMatches, `error parsing data for workload process details.*`)
}

func (s *suite) TestLaunchNoId(c *gc.C) {
	f := &fakeRunner{
		out: []byte(`{ "status" : { "status" : "bar" } }`),
	}
	s.PatchValue(&runCmd, f.runCmd)

	p := Plugin{"Name", "Path"}
	proc := charm.Process{Image: "img"}

	_, err := p.Launch(proc)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *suite) TestLaunchNoStatus(c *gc.C) {
	f := &fakeRunner{
		out: []byte(`{ "id" : "foo" }`),
	}
	s.PatchValue(&runCmd, f.runCmd)

	p := Plugin{"Name", "Path"}
	proc := charm.Process{Image: "img"}

	_, err := p.Launch(proc)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *suite) TestLaunchErr(c *gc.C) {
	f := &fakeRunner{
		err: errors.New("foo"),
	}
	s.PatchValue(&runCmd, f.runCmd)

	p := Plugin{"Name", "Path"}
	proc := charm.Process{Image: "img"}

	_, err := p.Launch(proc)
	c.Assert(errors.Cause(err), gc.Equals, f.err)
}

func (s *suite) TestStatus(c *gc.C) {
	f := &fakeRunner{
		out: []byte(`{ "label" : "status!" }`),
	}
	s.PatchValue(&runCmd, f.runCmd)

	p := Plugin{"Name", "Path"}

	status, err := p.Status("id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, process.PluginStatus{
		Label: "status!",
	})
	c.Assert(f.name, gc.DeepEquals, p.Name)
	c.Assert(f.cmd.Path, gc.Equals, p.Executable)
	c.Assert(f.cmd.Args[1], gc.Equals, "status")
	c.Assert(f.cmd.Args[2:], gc.DeepEquals, []string{"id"})
}

func (s *suite) TestStatusErr(c *gc.C) {
	f := &fakeRunner{
		err: errors.New("foo"),
	}
	s.PatchValue(&runCmd, f.runCmd)

	p := Plugin{"Name", "Path"}

	_, err := p.Status("id")
	c.Assert(errors.Cause(err), gc.Equals, f.err)
}

func (s *suite) TestDestroy(c *gc.C) {
	f := &fakeRunner{}
	s.PatchValue(&runCmd, f.runCmd)

	p := Plugin{"Name", "Path"}

	err := p.Destroy("id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.name, gc.DeepEquals, p.Name)
	c.Assert(f.cmd.Path, gc.Equals, p.Executable)
	c.Assert(f.cmd.Args[1], gc.Equals, "destroy")
	c.Assert(f.cmd.Args[2:], gc.DeepEquals, []string{"id"})
}

func (s *suite) TestDestroyErr(c *gc.C) {
	f := &fakeRunner{
		err: errors.New("foo"),
	}
	s.PatchValue(&runCmd, f.runCmd)

	p := Plugin{"Name", "Path"}

	err := p.Destroy("id")
	c.Assert(errors.Cause(err), gc.Equals, f.err)
}

func (s *suite) TestRunCmd(c *gc.C) {
	f := &fakeLogger{c: c}
	s.PatchValue(&getLogger, f.getLogger)
	m := maker{
		stdout: "foo!",
		stderr: "bar!\nbaz!",
	}
	cmd := m.make()
	out, err := runCmd("name", cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.TrimSpace(string(out)), gc.DeepEquals, m.stdout)
	c.Assert(f.name, gc.Equals, "juju.process.plugin.name")
	c.Assert(f.logs, gc.DeepEquals, []string{"bar!", "baz!"})
}

func (s *suite) TestRunCmdErr(c *gc.C) {
	f := &fakeLogger{c: c}
	s.PatchValue(&getLogger, f.getLogger)
	m := maker{
		exit:   1,
		stdout: "foo!",
	}
	cmd := m.make()
	_, err := runCmd("name", cmd)
	c.Assert(err, gc.ErrorMatches, "exit status 1: foo!")
}

type fakeLogger struct {
	logs []string
	name string
	c    *gc.C
}

func (f *fakeLogger) getLogger(name string) infoLogger {
	f.name = name
	return f
}

func (f *fakeLogger) Infof(s string, args ...interface{}) {
	f.logs = append(f.logs, s)
	f.c.Assert(args, gc.IsNil)
}

type fakeRunner struct {
	name string
	cmd  *exec.Cmd
	out  []byte
	err  error
}

func (f *fakeRunner) runCmd(name string, cmd *exec.Cmd) ([]byte, error) {
	f.name = name
	f.cmd = cmd
	return f.out, f.err
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
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = []string{
		fmt.Sprintf("%s=%s", isHelperProc, "1"),
		fmt.Sprintf("%s=%s", helperStdout, m.stdout),
		fmt.Sprintf("%s=%s", helperStderr, m.stderr),
		fmt.Sprintf("%s=%d", helperExit, m.exit),
	}
	return cmd
}

func TestHelperProcess(*stdtesting.T) {
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
