// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart_test

import (
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/initsystems"
	iupstart "github.com/juju/juju/service/initsystems/upstart"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/testing"
)

type UpstartSuite struct {
	testing.BaseSuite
	testPath string
	service  *upstart.Service
	initDir  string
}

var _ = gc.Suite(&UpstartSuite{})

func (s *UpstartSuite) SetUpTest(c *gc.C) {
	s.testPath = c.MkDir()
	s.initDir = c.MkDir()
	s.PatchEnvPathPrepend(s.testPath)
	s.PatchValue(&iupstart.ConfDir, s.initDir)
	s.service = &upstart.Service{
		Name: "some-service",
		Conf: service.Conf{Conf: initsystems.Conf{
			Desc: "some service",
			Cmd:  "some command",
		}},
	}
}

func (s *UpstartSuite) TestMachineAgentUpstartService(c *gc.C) {
	svc := upstart.MachineAgentUpstartService(
		"jujud-machine-0",
		"/var/lib/juju/tools/machine-0",
		"/var/lib/juju",
		"/var/log/juju",
		"machine-0",
		"0",
		map[string]string{
			"w": "x",
			"y": "z",
		},
	)

	c.Check(svc.Name, gc.Equals, "jujud-machine-0")
	c.Check(svc.Conf, jc.DeepEquals, service.Conf{
		Conf: initsystems.Conf{
			Desc: "juju machine/0 agent",
			Cmd: "/var/lib/juju/.../machine-0/jujud" +
				" machine" +
				` --data-dir "/var/lib/juju"` +
				" --machine-id 0" +
				" --debug",
			Env: map[string]string{
				"w": "x",
				"y": "z",
			},
			Limit: map[string]string{
				"nofile": "20000 20000",
			},
			Out: "/var/log/juju/machine-0.log",
		},
		ExtraScript: "",
	})
}

var checkargs = `
#!/bin/bash --norc
if [ "$1" != "--system" ]; then
  exit 255
fi
if [ "$2" != "some-service" ]; then
  exit 255
fi
if [ "$3" != "" ]; then
  exit 255
fi
`[1:]

func (s *UpstartSuite) MakeTool(c *gc.C, name, script string) {
	path := filepath.Join(s.testPath, name)
	err := ioutil.WriteFile(path, []byte(checkargs+script), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

const expectStart = `description "this is an upstart service"
author "Juju Team <juju@lists.ubuntu.com>"
start on runlevel [2345]
stop on runlevel [!2345]
respawn
normal exit 0
`

func (s *UpstartSuite) dummyConf(c *gc.C) service.Conf {
	return service.Conf{Conf: initsystems.Conf{
		Desc: "this is an upstart service",
		Cmd:  "do something",
	}}
}

func (s *UpstartSuite) assertInstall(c *gc.C, conf service.Conf, expectEnd string) {
	expectContent := expectStart + expectEnd
	expectPath := filepath.Join(iupstart.ConfDir, "some-service.conf")

	s.service.Conf = conf
	cmds, err := s.service.InstallCommands()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmds, gc.DeepEquals, []string{
		"cat >> " + expectPath + " << 'EOF'\n" + expectContent + "EOF\n",
		"start some-service",
	})
}

func (s *UpstartSuite) TestServiceInstallCommands(c *gc.C) {
}

func (s *UpstartSuite) TestInstallCommandsErrors(c *gc.C) {
	check := func(msg string) {
		_, err := s.service.InstallCommands()
		c.Assert(err, gc.ErrorMatches, msg)
	}
	conf := service.Conf{}
	s.service.Conf = conf

	s.service.Name = ""
	check("missing Name")
	s.service.Name = "some-service"
	check("missing Desc")
	s.service.Conf.Desc = "this is an upstart service"
	check("missing Cmd")
}

func (s *UpstartSuite) TestInstallSimple(c *gc.C) {
	conf := s.dummyConf(c)
	s.assertInstall(c, conf, "\n\nscript\n\n\n  exec do something\nend script\n")
}

func (s *UpstartSuite) TestInstallExtraScript(c *gc.C) {
	conf := s.dummyConf(c)
	conf.ExtraScript = "extra lines of script"
	s.assertInstall(c, conf, "\n\nscript\nextra lines of script\n\n  exec do something\nend script\n")
}

func (s *UpstartSuite) TestInstallOutput(c *gc.C) {
	conf := s.dummyConf(c)
	conf.Out = "/some/output/path"
	s.assertInstall(c, conf, "\n\nscript\n\n\n  # Ensure log files are properly protected\n  touch /some/output/path\n  chown syslog:syslog /some/output/path\n  chmod 0600 /some/output/path\n\n  exec do something >> /some/output/path 2>&1\nend script\n")
}

func (s *UpstartSuite) TestInstallEnv(c *gc.C) {
	conf := s.dummyConf(c)
	conf.Env = map[string]string{"FOO": "bar baz", "QUX": "ping pong"}
	s.assertInstall(c, conf, `env FOO="bar baz"
env QUX="ping pong"


script


  exec do something
end script
`)
}

func (s *UpstartSuite) TestInstallLimit(c *gc.C) {
	conf := s.dummyConf(c)
	conf.Limit = map[string]string{"nofile": "65000 65000", "nproc": "20000 20000"}
	s.assertInstall(c, conf, `
limit nofile 65000 65000
limit nproc 20000 20000

script


  exec do something
end script
`)
}
