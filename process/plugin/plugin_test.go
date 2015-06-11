package plugin

import (
	"fmt"
	"io/ioutil"
	"math"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&suite{})

func (*suite) TestOutput(c *gc.C) {
	output := "foo!"

	cmd := makeCmd(output, 0, 0, c)
	out, err := outputWithTimeout(cmd, time.Second)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, []byte(output+"\n"))
}

type hasTimeout interface {
	IsTimeout() bool
}

func (*suite) TestOutputWithTimeout(c *gc.C) {
	cmd := makeCmd("foo!", 0, time.Second*2, c)
	out, err := outputWithTimeout(cmd, time.Millisecond*100)
	c.Assert(err, gc.NotNil)
	c.Assert(out, gc.IsNil)
	if e, ok := err.(hasTimeout); !ok {
		c.Errorf("Error caused by timeout does not have Timeout function")
	} else {
		c.Assert(e.IsTimeout(), jc.IsTrue)
	}
}

func (*suite) TestOutputError(c *gc.C) {
	output := "foooo"

	cmd := makeCmd(output, 1, time.Duration(0), c)
	out, err := outputWithTimeout(cmd, time.Second)
	c.Assert(err, gc.ErrorMatches, output+"\n")
	c.Assert(out, gc.IsNil)
}

func (s *suite) TestLaunch(c *gc.C) {
	f := fakeOutput{out: []byte(`
id: foo
details:
  foo: bar
  baz: 5`[1:])}
	s.PatchValue(&outputWithTimeout, f.fakeOut)
	p, err := Launch("plugin", "image")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.ID, gc.Equals, "foo")
	c.Assert(p.Details, gc.DeepEquals, map[string]interface{}{"foo": "bar", "baz": 5})
	c.Assert(f.cmd.Path, gc.Equals, "plugin")
	c.Assert(f.cmd.Args, gc.DeepEquals, []string{"plugin", "launch", "image"})
}

func (s *suite) TestLaunchBadOutput(c *gc.C) {
	f := fakeOutput{out: []byte("not yaml")}
	s.PatchValue(&outputWithTimeout, f.fakeOut)
	_, err := Launch("plugin", "image")
	c.Assert(err, gc.NotNil)
	msg := strings.Replace(err.Error(), "\n", " ", -1)
	c.Assert(msg, gc.Matches, `error parsing data returned from "plugin".*`)
}

func (s *suite) TestLaunchNoId(c *gc.C) {
	f := fakeOutput{out: []byte("foo: bar")}
	s.PatchValue(&outputWithTimeout, f.fakeOut)
	_, err := Launch("plugin", "image")
	c.Assert(err, gc.ErrorMatches, `no id set by plugin "plugin"`)
}

func (s *suite) TestLaunchErr(c *gc.C) {
	f := fakeOutput{out: []byte("id: foo"), err: fmt.Errorf("foo")}
	s.PatchValue(&outputWithTimeout, f.fakeOut)
	_, err := Launch("plugin", "image")
	c.Assert(err, gc.Equals, f.err)
}

func (s *suite) TestStatus(c *gc.C) {
	f := fakeOutput{out: []byte("some data")}
	s.PatchValue(&outputWithTimeout, f.fakeOut)
	status, err := Status("plugin", "id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, "some data")
	c.Assert(f.cmd.Path, gc.Equals, "plugin")
	c.Assert(f.cmd.Args, gc.DeepEquals, []string{"plugin", "status", "id"})
}

func (s *suite) TestStatusErr(c *gc.C) {
	f := fakeOutput{out: []byte("id: foo"), err: fmt.Errorf("foo")}
	s.PatchValue(&outputWithTimeout, f.fakeOut)
	_, err := Status("plugin", "id")
	c.Assert(err, gc.Equals, f.err)
}

func (s *suite) TestStop(c *gc.C) {
	f := fakeOutput{out: []byte("some data")}
	s.PatchValue(&outputWithTimeout, f.fakeOut)
	err := Stop("plugin", "id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.cmd.Path, gc.Equals, "plugin")
	c.Assert(f.cmd.Args, gc.DeepEquals, []string{"plugin", "stop", "id"})
}

func (s *suite) TestStopErr(c *gc.C) {
	f := fakeOutput{out: []byte("some data"), err: fmt.Errorf("foo")}
	s.PatchValue(&outputWithTimeout, f.fakeOut)
	err := Stop("plugin", "id")
	c.Assert(err, gc.Equals, f.err)
}

func makeCmd(output string, exit int, timeout time.Duration, c *gc.C) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return winCmd(output, exit, timeout, c)
	} else {
		return nixCmd(output, exit, timeout, c)
	}
}

func winCmd(output string, exit int, timeout time.Duration, c *gc.C) *exec.Cmd {
	var data string
	if timeout > 0 {
		secs := int(math.Ceil(timeout.Seconds()))
		data = fmt.Sprintf("timeout /t %d\necho %s\nexit %d", secs, output, exit)
	} else {
		data = fmt.Sprintf("echo %s\nexit %d", output, exit)
	}

	path := filepath.Join(c.MkDir(), "foo.bat")
	err := ioutil.WriteFile(path, []byte(data), 0744)
	c.Assert(err, jc.ErrorIsNil)
	return exec.Command(path)
}

func nixCmd(output string, exit int, timeout time.Duration, c *gc.C) *exec.Cmd {
	var data string
	if timeout > 0 {
		secs := int(math.Ceil(timeout.Seconds()))
		data = fmt.Sprintf("#!/bin/sh\nsleep %d\necho %s\nexit %d", secs, output, exit)
	} else {
		data = fmt.Sprintf("#!/bin/sh\necho %s\nexit %d", output, exit)
	}

	path := filepath.Join(c.MkDir(), "foo.sh")
	err := ioutil.WriteFile(path, []byte(data), 0744)
	c.Assert(err, jc.ErrorIsNil)
	return exec.Command(path)
}

type fakeOutput struct {
	out     []byte
	err     error
	cmd     *exec.Cmd
	timeout time.Duration
}

func (f *fakeOutput) fakeOut(cmd *exec.Cmd, timeout time.Duration) ([]byte, error) {
	f.cmd = cmd
	f.timeout = timeout
	return f.out, f.err
}
