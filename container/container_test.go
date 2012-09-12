package container_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"os"
	"path/filepath"
	stdtesting "testing"
)

type suite struct {
	testing.JujuConnSuite
}

var _ = Suite(&suite{})

func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

func (s *suite) TestDeploy(c *C) {
	// create a unit to deploy
	dummy := s.AddTestingCharm(c, "dummy")
	service, err := s.State.AddService("dummy", dummy)
	c.Assert(err, IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, IsNil)

	// make sure there's a jujud "executable" in the tools directory
	// for the current version.
	tools := &state.Tools{
		URL:    "unused",
		Binary: version.MustParseBinary("3.2.1-foo-bar"),
	}
	dataDir := c.MkDir()
	toolsDir := environs.ToolsDir(dataDir, tools.Binary)
	err = os.MkdirAll(toolsDir, 0777)
	c.Assert(err, IsNil)
	exe := filepath.Join(toolsDir, "jujud")
	err = ioutil.WriteFile(exe, []byte("#!/bin/sh\n"), 0777)
	c.Assert(err, IsNil)

	initDir := c.MkDir()
	cfg := container.Config{
		DataDir: dataDir,
		InitDir: initDir,
	}

	info := &state.Info{
		Addrs: []string{"a", "b"},
	}

	err = container.Deploy(cfg, unit, info, tools)
	c.Assert(err, ErrorMatches, `(.|\n)+Unknown job(.|\n)+`)

	upstartScript := filepath.Join(cfg.InitDir, "juju-unit-dummy-0.conf")

	data, err := ioutil.ReadFile(upstartScript)
	c.Assert(err, IsNil)

	c.Assert(string(data), Matches, `(.|\n)+`+
		`.*/unit-dummy-0/jujud unit`+
		` --zookeeper-servers 'a,b'`+
		` --log-file /var/log/juju/unit-dummy-0\.log`+
		` --unit-name dummy/0\n`+
		`(.|\n)*`)

	// We can't check that the unit directory is created, because
	// it is removed when the call to Deploy fails, but
	// we can check that it is removed.
	unitDir := filepath.Join(cfg.DataDir, "agents", "unit-dummy-0")
	err = os.MkdirAll(filepath.Join(unitDir, "foo"), 0777)
	c.Assert(err, IsNil)

	err = container.Destroy(cfg, unit)
	c.Assert(err, IsNil)

	_, err = os.Stat(unitDir)
	c.Assert(os.IsNotExist(err), Equals, true, Commentf("%v", err))

	_, err = os.Stat(upstartScript)
	c.Assert(err, NotNil)
}
