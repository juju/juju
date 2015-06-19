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

	"github.com/juju/juju/testing"
)

type suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&suite{})

const exitstatus1 = "exit status 1: "

func (s *suite) TestLaunch(c *gc.C) {
	f := &fakeLogger{c: c}
	s.PatchValue(&getLogger, f.getLogger)
	p := maker{
		c:      c,
		stdout: `{ "id" : "foo", "status" : "bar" }`,
	}.make()

	proc := charm.Process{Image: "img"}

	pd, err := p.Launch(proc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pd, gc.Equals, ProcDetails{"foo", "bar"})

	// re-enable once we put struct tags on charm.Process
	// c.Assert(f.logs, gc.DeepEquals, []string{`launch { "image" : "img" }`})
	c.Assert(f.name, gc.Equals, "juju.process.plugin."+p.Name)
}

func (s *suite) TestLaunchBadOutput(c *gc.C) {
	f := &fakeLogger{c: c}
	s.PatchValue(&getLogger, f.getLogger)
	p := maker{
		c:      c,
		stdout: `not json`,
	}.make()

	proc := charm.Process{Image: "img"}

	_, err := p.Launch(proc)
	c.Assert(err, gc.NotNil)
	msg := strings.Replace(err.Error(), "\n", " ", -1)
	c.Assert(msg, gc.Matches, `error parsing data returned from "Name".*`)
}

func (s *suite) TestLaunchNoId(c *gc.C) {
	f := &fakeLogger{c: c}
	s.PatchValue(&getLogger, f.getLogger)
	p := maker{
		c:      c,
		stdout: `{ "status" : "bar" }`,
	}.make()

	proc := charm.Process{Image: "img"}

	_, err := p.Launch(proc)
	c.Assert(errors.Cause(err), jc.Satisfies, IsInvalid)
}

func (s *suite) TestLaunchErr(c *gc.C) {
	f := &fakeLogger{
		c: c,
	}
	s.PatchValue(&getLogger, f.getLogger)
	p := maker{
		c:      c,
		exit:   1,
		stdout: `nope`,
	}.make()

	proc := charm.Process{Image: "img"}

	_, err := p.Launch(proc)
	c.Assert(err, gc.ErrorMatches, exitstatus1+"nope")
}

func (s *suite) TestStatus(c *gc.C) {
	f := &fakeLogger{c: c}
	s.PatchValue(&getLogger, f.getLogger)
	p := maker{
		c:      c,
		stdout: `status!`,
	}.make()

	status, err := p.Status("id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.TrimSpace(status), gc.Equals, "status!")
	c.Assert(f.logs, gc.DeepEquals, []string{"status id"})
	c.Assert(f.name, gc.Equals, "juju.process.plugin."+p.Name)
}

func (s *suite) TestStatusErr(c *gc.C) {
	f := &fakeLogger{c: c}
	s.PatchValue(&getLogger, f.getLogger)
	p := maker{
		c:      c,
		exit:   1,
		stdout: `status!`,
	}.make()

	_, err := p.Status("id")
	c.Assert(err, gc.ErrorMatches, exitstatus1+"status!")
}

func (s *suite) TestDestroy(c *gc.C) {
	f := &fakeLogger{c: c}
	s.PatchValue(&getLogger, f.getLogger)
	p := maker{
		c: c,
	}.make()

	err := p.Destroy("id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f.logs, gc.DeepEquals, []string{"destroy id"})
	c.Assert(f.name, gc.Equals, "juju.process.plugin."+p.Name)
}

func (s *suite) TestDestroyErr(c *gc.C) {
	f := &fakeLogger{c: c}
	s.PatchValue(&getLogger, f.getLogger)
	p := maker{
		exit:   1,
		stdout: "nope",
		c:      c,
	}.make()

	err := p.Destroy("id")
	c.Assert(err, gc.ErrorMatches, exitstatus1+"nope")
}

// maker makes a script that outputs the arguments passed to it as stderr and
// the string in stdout to stdout.
type maker struct {
	stdout string
	exit   int
	c      *gc.C
}

func (m maker) make() Plugin {
	if runtime.GOOS == "windows" {
		return m.winCmd()
	}
	return m.nixCmd()
}

func (m maker) winCmd() Plugin {
	data := fmt.Sprintf(`
echo %* 1>&2
echo "%s"
exit %d`[1:], m.stdout, m.exit)

	path := filepath.Join(m.c.MkDir(), "foo.bat")
	err := ioutil.WriteFile(path, []byte(data), 0744)
	m.c.Assert(err, jc.ErrorIsNil)
	return Plugin{"Name", path}
}

func (m maker) nixCmd() Plugin {
	data := fmt.Sprintf(`
#!/bin/sh
>&2 echo $@
echo '%s'
exit %d
`[1:], m.stdout, m.exit)

	path := filepath.Join(m.c.MkDir(), "foo.sh")
	err := ioutil.WriteFile(path, []byte(data), 0744)
	m.c.Assert(err, jc.ErrorIsNil)
	return Plugin{"Name", path}

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
