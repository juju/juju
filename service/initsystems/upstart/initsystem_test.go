// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/initsystems"
	"github.com/juju/juju/service/initsystems/upstart"
	"github.com/juju/juju/testing"
)

type initSystemSuite struct {
	testing.BaseSuite

	//testPath string
	initDir string
	files   *fakeFiles
	init    initsystems.InitSystem
	conf    initsystems.Conf
}

var _ = gc.Suite(&initSystemSuite{})

func (s *initSystemSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	//s.testPath = c.MkDir()
	s.initDir = c.MkDir()
	s.files = &fakeFiles{}
	s.init = &upstart{
		name:    "upstart",
		initDir: s.initDir,
		fops:    s.files,
	}
	s.conf = initsystems.Conf{
		Desc: "some service",
		Cmd:  "some command",
	}

	//s.PatchEnvPathPrepend(s.testPath)
	s.PatchValue(&upstart.ConfDir, s.initDir)
	s.PatchValue(&initsystems.RetryAttempts, utils.AttemptStrategy{})
}

func (s *initSystemSuite) TestInitSystemName(c *gc.C) {
	name := s.init.Name()

	c.Check(name, gc.Equals, "upstart")
}

func (s *initSystemSuite) TestInitSystemList(c *gc.C) {
	names, err := s.init.List()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, jc.SameContents, []string{
		"jujud-machine-0",
		"jujud-unit-wordpress-0",
	})
}

func (s *initSystemSuite) TestInitSystemListLimited(c *gc.C) {
	names, err := s.init.List("jujud-machine-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, jc.SameContents, []string{"jujud-machine-0"})
}

func (s *initSystemSuite) TestInitSystemListLimitedEmpty(c *gc.C) {
	names, err := s.init.List("jujud-machine-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, jc.SameContents, []string{})
}

func (s *initSystemSuite) TestInitSystemStart(c *gc.C) {
	err := s.init.Start("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	// TODO(ericsnow) Check underlying calls.
}

func (s *initSystemSuite) TestInitSystemStartAlreadyRunning(c *gc.C) {
	err := s.init.Start("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *initSystemSuite) TestInitSystemStartNotEnabled(c *gc.C) {
	err := s.init.Start("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemStop(c *gc.C) {
	err := s.init.Stop("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	// TODO(ericsnow) Check underlying calls.
}

func (s *initSystemSuite) TestInitSystemStopNotRunning(c *gc.C) {
	err := s.init.Stop("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemStopNotEnabled(c *gc.C) {
	err := s.init.Stop("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemEnable(c *gc.C) {
	err := s.init.Enable("jujud-unit-wordpress-0", s.conf)
	c.Assert(err, jc.ErrorIsNil)

	// TODO(ericsnow) Check underlying calls.
}

func (s *initSystemSuite) TestInitSystemEnableAlreadyEnabled(c *gc.C) {
	err := s.init.Enable("jujud-unit-wordpress-0", s.conf)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *initSystemSuite) TestInitSystemDisable(c *gc.C) {
	err := s.init.Disable("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	// TODO(ericsnow) Check underlying calls.
}

func (s *initSystemSuite) TestInitSystemDisableNotEnabled(c *gc.C) {
	err := s.init.Disable("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemIsEnabledTrue(c *gc.C) {
	enabled, err := s.init.IsEnabled("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(enabled, jc.IsTrue)
	// TODO(ericsnow) Check underlying calls.
}

func (s *initSystemSuite) TestInitSystemIsEnabledFalse(c *gc.C) {
	enabled, err := s.init.IsEnabled("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(enabled, jc.IsFalse)
}

func (s *initSystemSuite) TestInitSystemInfo(c *gc.C) {
	info, err := s.init.Info("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, initsystems.ServiceInfo{
		Name:   "jujud-unit-wordpress-0",
		Desc:   "juju agent for unit-wordpress-0",
		Status: initsystems.StatusRunning,
	})
	// TODO(ericsnow) Check underlying calls.
}

func (s *initSystemSuite) TestInitSystemInfoNotEnabled(c *gc.C) {
	_, err := s.init.Info("jujud-unit-wordpress-0")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemConf(c *gc.C) {
	conf, err := s.init.Conf("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(conf, jc.DeepEquals, &initSystem.Conf{
		Desc: "juju agent for unit-wordpress-0",
	})
	// TODO(ericsnow) Check underlying calls.
}

func (s *initSystemSuite) TestInitSystemConfNotEnabled(c *gc.C) {
	_, err := s.init.Conf("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemValidate(c *gc.C) {
	err := s.init.Validate("jujud-unit-wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	// TODO(ericsnow) Check underlying calls.
}

func (s *initSystemSuite) TestInitSystemValidateInvalid(c *gc.C) {
	err := s.init.Validate("jujud-unit-wordpress-0")

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *initSystemSuite) TestInitSystemSerialize(c *gc.C) {
	data, err := s.init.Serialize("jujud-unit-wordpress-0", s.conf)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, "")
	// TODO(ericsnow) Check underlying calls.
}

func (s *initSystemSuite) TestInitSystemDeserialize(c *gc.C) {
	conf, err := s.init.Deserialize(data)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(conf, jc.DeepEquals, &initSystem.Conf{
		Desc: "juju agent for unit-wordpress-0",
	})
	// TODO(ericsnow) Check underlying calls.
}

func (s *initSystemSuite) TestInitSystemDeserializeUnsupported(c *gc.C) {
	_, err := s.init.Deserialize(data)

	c.Check(err, gc.ErrorMatches, "")
}

// TODO(ericsnow) Port to initsystem_test.go.
/*
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

func (s *initSystemSuite) MakeTool(c *gc.C, name, script string) {
	path := filepath.Join(s.testPath, name)
	err := ioutil.WriteFile(path, []byte(checkargs+script), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) StoppedStatus(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service stop/waiting"`)
}

func (s *initSystemSuite) RunningStatus(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service start/running, process 123"`)
}

func (s *initSystemSuite) TestInitDir(c *gc.C) {
	svc := upstart.NewService("blah", initsystems.Conf{})
	c.Assert(svc.Conf.InitDir, gc.Equals, "")
}

func (s *initSystemSuite) goodInstall(c *gc.C) {
	s.MakeTool(c, "start", "exit 0")
	err := s.service.Install()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestInstalled(c *gc.C) {
	c.Assert(s.service.Installed(), jc.IsFalse)
	s.goodInstall(c)
	c.Assert(s.service.Installed(), jc.IsTrue)
}

func (s *initSystemSuite) TestExists(c *gc.C) {
	// Setup creates the file, but it is empty.
	c.Assert(s.service.Exists(), jc.IsFalse)
	s.goodInstall(c)
	c.Assert(s.service.Exists(), jc.IsTrue)
}

func (s *initSystemSuite) TestExistsNonEmpty(c *gc.C) {
	s.goodInstall(c)
	s.service.Conf.Cmd = "something else"
	c.Assert(s.service.Exists(), jc.IsFalse)
}

func (s *initSystemSuite) TestRunning(c *gc.C) {
	s.MakeTool(c, "status", "exit 1")
	c.Assert(s.service.Running(), jc.IsFalse)
	s.MakeTool(c, "status", `echo "GIBBERISH NONSENSE"`)
	c.Assert(s.service.Running(), jc.IsFalse)
	s.RunningStatus(c)
	c.Assert(s.service.Running(), jc.IsTrue)
}

func (s *initSystemSuite) TestStart(c *gc.C) {
	s.RunningStatus(c)
	s.MakeTool(c, "start", "exit 99")
	c.Assert(s.service.Start(), gc.IsNil)
	s.StoppedStatus(c)
	c.Assert(s.service.Start(), gc.ErrorMatches, ".*exit status 99.*")
	s.MakeTool(c, "start", "exit 0")
	c.Assert(s.service.Start(), gc.IsNil)
}

func (s *initSystemSuite) TestStop(c *gc.C) {
	s.StoppedStatus(c)
	s.MakeTool(c, "stop", "exit 99")
	c.Assert(s.service.Stop(), gc.IsNil)
	s.RunningStatus(c)
	c.Assert(s.service.Stop(), gc.ErrorMatches, ".*exit status 99.*")
	s.MakeTool(c, "stop", "exit 0")
	c.Assert(s.service.Stop(), gc.IsNil)
}

func (s *initSystemSuite) TestRemoveMissing(c *gc.C) {
	c.Assert(s.service.StopAndRemove(), gc.IsNil)
}

func (s *initSystemSuite) TestRemoveStopped(c *gc.C) {
	s.goodInstall(c)
	s.StoppedStatus(c)
	c.Assert(s.service.StopAndRemove(), gc.IsNil)
	_, err := os.Stat(filepath.Join(upstart.ConfDir, "some-service.conf"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *initSystemSuite) TestRemoveRunning(c *gc.C) {
	s.goodInstall(c)
	s.RunningStatus(c)
	s.MakeTool(c, "stop", "exit 99")
	c.Assert(s.service.StopAndRemove(), gc.ErrorMatches, ".*exit status 99.*")
	_, err := os.Stat(filepath.Join(upstart.ConfDir, "some-service.conf"))
	c.Assert(err, jc.ErrorIsNil)
	s.MakeTool(c, "stop", "exit 0")
	c.Assert(s.service.StopAndRemove(), gc.IsNil)
	_, err = os.Stat(filepath.Join(upstart.ConfDir, "some-service.conf"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *initSystemSuite) TestStopAndRemove(c *gc.C) {
	s.goodInstall(c)
	s.RunningStatus(c)
	s.MakeTool(c, "stop", "exit 99")

	// StopAndRemove will fail, as it calls stop.
	c.Assert(s.service.StopAndRemove(), gc.ErrorMatches, ".*exit status 99.*")
	_, err := os.Stat(filepath.Join(upstart.ConfDir, "some-service.conf"))
	c.Assert(err, jc.ErrorIsNil)

	// Plain old Remove will succeed.
	c.Assert(s.service.Remove(), gc.IsNil)
	_, err = os.Stat(filepath.Join(upstart.ConfDir, "some-service.conf"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *initSystemSuite) TestInstallErrors(c *gc.C) {
	conf := initsystems.Conf{
		InitDir: c.MkDir(),
	}
	check := func(msg string) {
		c.Assert(s.service.Install(), gc.ErrorMatches, msg)
		_, err := s.service.InstallCommands()
		c.Assert(err, gc.ErrorMatches, msg)
	}
	s.service.Conf = conf
	s.service.Name = ""
	check("missing Name")
	s.service.Name = "some-service"
	check("unexpected InitDir in conf")
	s.service.Conf.InitDir = ""
	check("missing Desc")
	s.service.Conf.Desc = "this is an upstart service"
	check("missing Cmd")
}

const expectStart = `description "this is an upstart service"
author "Juju Team <juju@lists.ubuntu.com>"
start on runlevel [2345]
stop on runlevel [!2345]
respawn
normal exit 0
`

func (s *initSystemSuite) dummyConf(c *gc.C) initsystems.Conf {
	return initsystems.Conf{
		Desc: "this is an upstart service",
		Cmd:  "do something",
	}
}

func (s *initSystemSuite) assertInstall(c *gc.C, conf initsystems.Conf, expectEnd string) {
	expectContent := expectStart + expectEnd
	expectPath := filepath.Join(upstart.ConfDir, "some-service.conf")

	s.service.Conf = conf
	svc := s.service
	cmds, err := s.service.InstallCommands()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmds, gc.DeepEquals, []string{
		"cat >> " + expectPath + " << 'EOF'\n" + expectContent + "EOF\n",
		"start some-service",
	})

	s.MakeTool(c, "start", "exit 99")
	err = svc.Install()
	c.Assert(err, gc.ErrorMatches, ".*exit status 99.*")
	s.MakeTool(c, "start", "exit 0")
	err = svc.Install()
	c.Assert(err, jc.ErrorIsNil)
	content, err := ioutil.ReadFile(expectPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, expectContent)
}

func (s *initSystemSuite) TestInstallSimple(c *gc.C) {
	conf := s.dummyConf(c)
	s.assertInstall(c, conf, "\n\nscript\n\n\n  exec do something\nend script\n")
}

func (s *initSystemSuite) TestInstallExtraScript(c *gc.C) {
	conf := s.dummyConf(c)
	conf.ExtraScript = "extra lines of script"
	s.assertInstall(c, conf, "\n\nscript\nextra lines of script\n\n  exec do something\nend script\n")
}

func (s *initSystemSuite) TestInstallOutput(c *gc.C) {
	conf := s.dummyConf(c)
	conf.Out = "/some/output/path"
	s.assertInstall(c, conf, "\n\nscript\n\n\n  # Ensure log files are properly protected\n  touch /some/output/path\n  chown syslog:syslog /some/output/path\n  chmod 0600 /some/output/path\n\n  exec do something >> /some/output/path 2>&1\nend script\n")
}

func (s *initSystemSuite) TestInstallEnv(c *gc.C) {
	conf := s.dummyConf(c)
	conf.Env = map[string]string{"FOO": "bar baz", "QUX": "ping pong"}
	s.assertInstall(c, conf, `env FOO="bar baz"
env QUX="ping pong"


script


  exec do something
end script
`)
}

func (s *initSystemSuite) TestInstallLimit(c *gc.C) {
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

func (s *initSystemSuite) TestInstallAlreadyRunning(c *gc.C) {
	pathTo := func(name string) string {
		return filepath.Join(s.testPath, name)
	}
	s.MakeTool(c, "status-stopped", `echo "some-service stop/waiting"`)
	s.MakeTool(c, "status-started", `echo "some-service start/running, process 123"`)
	s.MakeTool(c, "stop", fmt.Sprintf(
		"rm %s; ln -s %s %s",
		pathTo("status"), pathTo("status-stopped"), pathTo("status"),
	))
	s.MakeTool(c, "start", fmt.Sprintf(
		"rm %s; ln -s %s %s",
		pathTo("status"), pathTo("status-started"), pathTo("status"),
	))
	err := symlink.New(pathTo("status-started"), pathTo("status"))
	c.Assert(err, jc.ErrorIsNil)

	conf := s.dummyConf(c)
	s.service.UpdateConfig(conf)
	err = s.service.Install()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.service, jc.Satisfies, (*upstart.Service).Running)
}
*/
