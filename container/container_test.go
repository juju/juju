package container_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"os"
	"path/filepath"
	"regexp"
	stdtesting "testing"
)

type suite struct {
	testing.StateSuite
}

var _ = Suite(&suite{})

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

func (s *suite) TestDeploy(c *C) {
	// make sure there's a jujud "executable" in the path.
	binDir := c.MkDir()
	exe := filepath.Join(binDir, "jujud")
	defer os.Setenv("PATH", os.Getenv("PATH"))
	os.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	err := ioutil.WriteFile(exe, []byte("#!/bin/sh\n"), 0777)
	c.Assert(err, IsNil)

	// create a unit to deploy
	dummy := s.Charm(c, "dummy")
	service, err := s.St.AddService("dummy", dummy)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)

	oldInitDir, oldJujuDir := *container.InitDir, *container.JujuDir
	defer func() {
		*container.InitDir, *container.JujuDir = oldInitDir, oldJujuDir
	}()
	*container.InitDir, *container.JujuDir = c.MkDir(), c.MkDir()

	unitName := "juju-agent-dummy-0"
	upstartScript := filepath.Join(*container.InitDir, unitName+".conf")

	unitDir := filepath.Join(*container.JujuDir, "units", "dummy-0")

	cont := container.Simple
	err = cont.Deploy(unit)
	c.Assert(err, ErrorMatches, `(.|\n)+Unknown job(.|\n)+`)

	data, err := ioutil.ReadFile(upstartScript)
	c.Assert(err, IsNil)
	c.Assert(string(data), Matches, `(.|\n)+`+regexp.QuoteMeta(exe)+` unit --unit-name(.|\n)+`)

	// We can't check that the unit directory is created, because
	// it is removed when the call to Deploy fails, but
	// we can check that it is removed.

	err = os.MkdirAll(filepath.Join(unitDir, "foo"), 0777)
	c.Assert(err, IsNil)

	err = cont.Destroy(unit)
	c.Assert(err, IsNil)

	_, err = os.Stat(unitDir)
	c.Assert(err, NotNil)

	_, err = os.Stat(upstartScript)
	c.Assert(err, NotNil)
}
