package plugin

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"strings"

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
		ID:     "foo",
		Status: process.Status{Label: "bar"},
	})

	c.Assert(f.name, gc.DeepEquals, p.Name)
	c.Assert(f.path, gc.Equals, p.Executable)
	c.Assert(f.subcommand, gc.Equals, "launch")
	c.Assert(f.args, gc.HasLen, 1)
	// fix this to be more stringent when we fix json serialization for charm.Process
	c.Assert(f.args[0], gc.Matches, `.*"Image":"img".*`)
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
	c.Assert(status, gc.Equals, process.Status{Label: "status!"})
	c.Assert(f.name, gc.DeepEquals, p.Name)
	c.Assert(f.path, gc.Equals, p.Executable)
	c.Assert(f.subcommand, gc.Equals, "status")
	c.Assert(f.args, gc.DeepEquals, []string{"id"})
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
	c.Assert(f.path, gc.Equals, p.Executable)
	c.Assert(f.subcommand, gc.Equals, "destroy")
	c.Assert(f.args, gc.DeepEquals, []string{"id"})
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
		c:      c,
		stdout: "foo!",
	}
	path := m.make()
	out, err := runCmd("name", path, "subcommand", "arg1", "arg2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.TrimSpace(string(out)), gc.DeepEquals, m.stdout)
	c.Assert(f.name, gc.Equals, "juju.process.plugin.name")
	c.Assert(f.logs, gc.DeepEquals, []string{"subcommand arg1 arg2"})
}

func (s *suite) TestRunCmdErr(c *gc.C) {
	f := &fakeLogger{c: c}
	s.PatchValue(&getLogger, f.getLogger)
	m := maker{
		c:      c,
		exit:   1,
		stdout: "foo!",
	}
	path := m.make()
	_, err := runCmd("name", path, "command", "arg1", "arg2")
	c.Assert(err, gc.ErrorMatches, "exit status 1: foo!")
}

// maker makes a script that outputs the arguments passed to it as stderr and
// the string in stdout to stdout.
type maker struct {
	stdout string
	exit   int
	c      *gc.C
}

func (m maker) make() string {
	if runtime.GOOS == "windows" {
		return m.winCmd()
	}
	return m.nixCmd()
}

func (m maker) winCmd() string {
	data := fmt.Sprintf(`
echo %* 1>&2
echo "%s"
exit %d`[1:], m.stdout, m.exit)

	path := filepath.Join(m.c.MkDir(), "foo.bat")
	err := ioutil.WriteFile(path, []byte(data), 0744)
	m.c.Assert(err, jc.ErrorIsNil)
	return path
}

func (m maker) nixCmd() string {
	data := fmt.Sprintf(`
#!/bin/sh
>&2 echo $@
echo '%s'
exit %d
`[1:], m.stdout, m.exit)

	path := filepath.Join(m.c.MkDir(), "foo.sh")
	err := ioutil.WriteFile(path, []byte(data), 0744)
	m.c.Assert(err, jc.ErrorIsNil)
	return path

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
	name       string
	path       string
	subcommand string
	args       []string
	out        []byte
	err        error
}

func (f *fakeRunner) runCmd(name, path, subcommand string, args ...string) ([]byte, error) {
	f.name = name
	f.path = path
	f.subcommand = subcommand
	f.args = args
	return f.out, f.err
}
