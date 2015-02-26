// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd_test

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
	coretesting "github.com/juju/juju/testing"
)

const confStr = `
[Unit]
Description=juju agent for %s
After=syslog.target
After=network.target
After=systemd-user-sessions.service

[Service]
Type=forking
ExecStart=%s
RemainAfterExit=yes
Restart=always

[Install]
WantedBy=multi-user.target

`

const jujud = "/var/lib/juju/bin/jujud"

type initSystemSuite struct {
	coretesting.BaseSuite

	dataDir string
	ch      chan string
	stub    *testing.Stub
	conn    *systemd.StubDbusAPI
	fops    *systemd.StubFileOps

	name    string
	tag     names.Tag
	conf    common.Conf
	confStr string
	service *systemd.Service
}

var _ = gc.Suite(&initSystemSuite{})

func (s *initSystemSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	// Patch things out.
	s.dataDir = c.MkDir()
	systemd.PatchFindDataDir(s, s.dataDir)
	s.ch = systemd.PatchNewChan(s)

	s.stub = &testing.Stub{}
	s.conn = systemd.PatchNewConn(s, s.stub)
	s.fops = systemd.PatchFileOps(s, s.stub)
	c.Logf("%+v", s.fops)

	// Set up the service.
	tagStr := "machine-0"
	tag, err := names.ParseTag(tagStr)
	c.Assert(err, jc.ErrorIsNil)
	s.tag = tag
	s.name = "jujud-" + tagStr
	s.conf = common.Conf{
		Desc:      "juju agent for " + tagStr,
		ExecStart: jujud + " " + tagStr,
	}
	s.service, err = systemd.NewService(s.name, s.conf)
	c.Assert(err, jc.ErrorIsNil)

	// Reset any incidental calls.
	s.stub.Calls = nil
}

func (s *initSystemSuite) newConfStr(name, cmd string) string {
	tag := name[len("jujud-"):]
	if cmd == "" {
		cmd = jujud + " " + tag
	}
	return fmt.Sprintf(confStr[1:], tag, cmd)
}

func (s *initSystemSuite) addService(name, status string) {
	tag := name[len("jujud-"):]
	desc := "juju agent for " + tag
	s.conn.AddService(name, desc, status)
}

func (s *initSystemSuite) setConf(conf common.Conf) {
	s.conn.SetProperty("", "Description", conf.Desc)

	s.conn.SetProperty("Service", "Description", conf.Desc)

	parts := strings.Fields(conf.ExecStart)
	var args []interface{}
	for _, arg := range parts[1:] {
		args = append(args, arg)
	}
	s.conn.SetProperty("Service", "ExecStart", []interface{}{
		parts[0],
		args,
		false, 0, 0, 0, 0, 0, 0, 0,
	})

	if len(conf.Env) > 0 || len(conf.Limit) > 0 {
		// For now none of our tests need this.
		panic("not supported yet")
	}

	s.conn.SetProperty("Service", "StandardOutput", conf.Out)
	s.conn.SetProperty("Service", "StandardError", conf.Out)
}

func (s *initSystemSuite) checkCreateFileCall(c *gc.C, index int, filename, content string, perm os.FileMode) {
	if content == "" {
		name := filename
		filename = fmt.Sprintf("%s/init/%s/%s.service", s.dataDir, name, name)
		content = s.newConfStr(name, "")
	}

	call := s.stub.Calls[index]
	if !c.Check(call.FuncName, gc.Equals, "CreateFile") {
		return
	}
	if !c.Check(call.Args, gc.HasLen, 3) {
		return
	}

	callFilename, callData, callPerm := call.Args[0], call.Args[1], call.Args[2]
	c.Check(callFilename, gc.Equals, filename)
	c.Check(string(callData.([]byte)), gc.Equals, content)
	c.Check(callPerm, gc.Equals, perm)
}

func (s *initSystemSuite) TestListServices(c *gc.C) {
	s.addService("jujud-machine-0", "active")
	s.addService("something-else", "error")
	s.addService("jujud-unit-wordpress-0", "active")
	s.addService("another", "inactive")

	names, err := systemd.ListServices()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, jc.SameContents, []string{
		"jujud-machine-0",
		"something-else",
		"jujud-unit-wordpress-0",
		"another",
	})
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestListServicesEmpty(c *gc.C) {
	names, err := systemd.ListServices()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(names, gc.HasLen, 0)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestNewService(c *gc.C) {
	service, err := systemd.NewService(s.name, s.conf)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(service, jc.DeepEquals, &systemd.Service{
		Service: common.Service{
			Name: s.name,
			Conf: s.conf,
		},
		ConfName: s.name + ".service",
		UnitName: s.name + ".service",
		Dirname:  fmt.Sprintf("%s/init/%s", s.dataDir, s.name),
	})
	s.stub.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestUpdateConfig(c *gc.C) {
	s.conf.ExecStart = "/path/to/some/other/command"
	c.Assert(s.service.Service.Conf.ExecStart, gc.Equals, jujud+" machine-0")

	s.service.UpdateConfig(s.conf)

	c.Check(s.service, jc.DeepEquals, &systemd.Service{
		Service: common.Service{
			Name: s.name,
			Conf: s.conf,
		},
		ConfName: s.name + ".service",
		UnitName: s.name + ".service",
		Dirname:  fmt.Sprintf("%s/init/%s", s.dataDir, s.name),
	})
	s.stub.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestUpdateConfigExtraScript(c *gc.C) {
	s.conf.ExtraScript = "/path/to/another/command"

	s.service.UpdateConfig(s.conf)

	dirname := fmt.Sprintf("%s/init/%s", s.dataDir, s.name)
	script := "/path/to/another/command\n" + jujud + " machine-0"
	c.Check(s.service, jc.DeepEquals, &systemd.Service{
		Service: common.Service{
			Name: s.name,
			Conf: common.Conf{
				Desc:      s.conf.Desc,
				ExecStart: dirname + "/exec-start.sh",
			},
		},
		UnitName: s.name + ".service",
		ConfName: s.name + ".service",
		Dirname:  dirname,
		Script:   []byte(script),
	})
	c.Check(string(s.service.Script), gc.Equals, script)
	s.stub.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestUpdateConfigMultiline(c *gc.C) {
	s.conf.ExecStart = "a\nb\nc"

	s.service.UpdateConfig(s.conf)

	dirname := fmt.Sprintf("%s/init/%s", s.dataDir, s.name)
	c.Check(s.service, jc.DeepEquals, &systemd.Service{
		Service: common.Service{
			Name: s.name,
			Conf: common.Conf{
				Desc:      s.conf.Desc,
				ExecStart: dirname + "/exec-start.sh",
			},
		},
		UnitName: s.name + ".service",
		ConfName: s.name + ".service",
		Dirname:  dirname,
		Script:   []byte("a\nb\nc"),
	})
	s.stub.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestInstalledTrue(c *gc.C) {
	s.addService("jujud-machine-0", "active")
	s.addService("something-else", "error")
	s.addService("juju-mongod", "active")

	installed := s.service.Installed()

	c.Check(installed, jc.IsTrue)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestInstalledFalse(c *gc.C) {
	s.addService("something-else", "error")

	installed := s.service.Installed()

	c.Check(installed, jc.IsFalse)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestInstalledError(c *gc.C) {
	s.addService("jujud-machine-0", "active")
	s.addService("something-else", "error")
	s.addService("juju-mongod", "active")
	failure := errors.New("<failed>")
	s.stub.SetErrors(failure)

	installed := s.service.Installed()

	c.Check(installed, jc.IsFalse)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestExistsTrue(c *gc.C) {
	s.setConf(s.conf)

	exists := s.service.Exists()

	c.Check(exists, jc.IsTrue)
	s.stub.CheckCallNames(c,
		"GetUnitProperties",
		"GetUnitTypeProperties",
		"Close",
	)
}

func (s *initSystemSuite) TestExistsFalse(c *gc.C) {
	s.setConf(common.Conf{
		Desc:      s.conf.Desc,
		ExecStart: s.conf.ExecStart,
		Out:       "syslog",
	})

	exists := s.service.Exists()

	c.Check(exists, jc.IsFalse)
	s.stub.CheckCallNames(c,
		"GetUnitProperties",
		"GetUnitTypeProperties",
		"Close",
	)
}

func (s *initSystemSuite) TestExistsError(c *gc.C) {
	failure := errors.New("<failed>")
	s.stub.SetErrors(failure)

	exists := s.service.Exists()

	c.Check(exists, jc.IsFalse)
	s.stub.CheckCallNames(c,
		"GetUnitProperties",
		"Close",
	)
}

func (s *initSystemSuite) TestRunningTrue(c *gc.C) {
	s.addService("jujud-machine-0", "active")
	s.addService("something-else", "error")
	s.addService("juju-mongod", "active")

	running := s.service.Running()

	c.Check(running, jc.IsTrue)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestRunningFalse(c *gc.C) {
	s.addService("jujud-machine-0", "inactive")
	s.addService("something-else", "error")
	s.addService("juju-mongod", "active")

	running := s.service.Running()

	c.Check(running, jc.IsFalse)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestRunningNotEnabled(c *gc.C) {
	s.addService("something-else", "active")

	running := s.service.Running()

	c.Check(running, jc.IsFalse)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestRunningError(c *gc.C) {
	s.addService("jujud-machine-0", "active")
	s.addService("something-else", "error")
	s.addService("juju-mongod", "active")
	failure := errors.New("<failed>")
	s.stub.SetErrors(failure)

	running := s.service.Running()

	c.Check(running, jc.IsFalse)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestStart(c *gc.C) {
	s.addService("jujud-machine-0", "inactive")
	s.ch <- "done"

	err := s.service.Start()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "ListUnits",
	}, {
		FuncName: "Close",
	}, {
		FuncName: "ListUnits",
	}, {
		FuncName: "Close",
	}, {
		FuncName: "StartUnit",
		Args: []interface{}{
			s.name + ".service",
			"fail",
			(chan<- string)(s.ch),
		},
	}, {
		FuncName: "Close",
	}})
}

func (s *initSystemSuite) TestStartAlreadyRunning(c *gc.C) {
	s.addService("jujud-machine-0", "active")
	s.ch <- "done" // just in case

	err := s.service.Start()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"ListUnits",
		"Close",
		"ListUnits",
		"Close",
	)
}

func (s *initSystemSuite) TestStartNotInstalled(c *gc.C) {
	s.ch <- "done" // just in case

	err := s.service.Start()

	c.Check(err, jc.Satisfies, errors.IsNotFound)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestStop(c *gc.C) {
	s.addService("jujud-machine-0", "active")
	s.ch <- "done"

	err := s.service.Stop()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "ListUnits",
	}, {
		FuncName: "Close",
	}, {
		FuncName: "StopUnit",
		Args: []interface{}{
			s.name + ".service",
			"fail",
			(chan<- string)(s.ch),
		},
	}, {
		FuncName: "Close",
	}})
}

func (s *initSystemSuite) TestStopNotRunning(c *gc.C) {
	s.addService("jujud-machine-0", "inactive")
	s.ch <- "done" // just in case

	err := s.service.Stop()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestStopNotInstalled(c *gc.C) {
	s.ch <- "done" // just in case

	err := s.service.Stop()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestStopAndRemove(c *gc.C) {
	s.addService("jujud-machine-0", "active")
	s.ch <- "done"

	err := s.service.StopAndRemove()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"ListUnits",
		"Close",
		"StopUnit",
		"Close",
		"ListUnits",
		"Close",
		"DisableUnitFiles",
		"RemoveAll",
		"Close",
	)
}

func (s *initSystemSuite) TestRemove(c *gc.C) {
	s.addService("jujud-machine-0", "inactive")

	err := s.service.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"ListUnits",
		"Close",
		"DisableUnitFiles",
		"RemoveAll",
		"Close",
	)
	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "ListUnits",
	}, {
		FuncName: "Close",
	}, {
		FuncName: "DisableUnitFiles",
		Args: []interface{}{
			[]string{s.name + ".service"},
			false,
		},
	}, {
		FuncName: "RemoveAll",
		Args: []interface{}{
			fmt.Sprintf("%s/init/%s", s.dataDir, s.name),
		},
	}, {
		FuncName: "Close",
	}})
}

func (s *initSystemSuite) TestRemoveNotInstalled(c *gc.C) {
	err := s.service.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestInstall(c *gc.C) {
	err := s.service.Install()
	c.Assert(err, jc.ErrorIsNil)

	dirname := fmt.Sprintf("%s/init/%s", s.dataDir, s.name)
	filename := fmt.Sprintf("%s/%s.service", dirname, s.name)
	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "ListUnits",
	}, {
		FuncName: "Close",
	}, {
		FuncName: "MkdirAll",
		Args: []interface{}{
			dirname,
		},
	}, {
		FuncName: "CreateFile",
		Args: []interface{}{
			filename,
			[]byte(s.newConfStr(s.name, "")),
			os.FileMode(0644),
		},
	}, {
		FuncName: "EnableUnitFiles",
		Args: []interface{}{
			[]string{filename},
			false,
			true,
		},
	}, {
		FuncName: "Close",
	}})
}

func (s *initSystemSuite) TestInstallAlreadyInstalled(c *gc.C) {
	s.addService("jujud-machine-0", "inactive")
	s.setConf(s.conf)

	err := s.service.Install()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"ListUnits",
		"Close",
		"GetUnitProperties",
		"GetUnitTypeProperties",
		"Close",
	)
}

func (s *initSystemSuite) TestInstallZombie(c *gc.C) {
	s.addService("jujud-machine-0", "active")
	s.setConf(common.Conf{
		Desc:      s.conf.Desc,
		ExecStart: s.conf.ExecStart,
		Out:       "syslog",
	})
	s.ch <- "done"

	err := s.service.Install()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"ListUnits",
		"Close",
		"GetUnitProperties",
		"GetUnitTypeProperties",
		"Close",
		"ListUnits",
		"Close",
		"StopUnit",
		"Close",
		"ListUnits",
		"Close",
		"DisableUnitFiles",
		"RemoveAll",
		"Close",
		"MkdirAll",
		"CreateFile",
		"EnableUnitFiles",
		"Close",
	)
	s.checkCreateFileCall(c, 15, s.name, "", 0644)
}

func (s *initSystemSuite) TestInstallMultiline(c *gc.C) {
	scriptPath := fmt.Sprintf("%s/init/%s/exec-start.sh", s.dataDir, s.name)
	cmd := "a\nb\nc"
	s.service.Service.Conf.ExecStart = scriptPath
	s.service.Script = []byte(cmd)

	err := s.service.Install()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c,
		"ListUnits",
		"Close",
		"MkdirAll",
		"CreateFile",
		"CreateFile",
		"EnableUnitFiles",
		"Close",
	)
	s.checkCreateFileCall(c, 3, scriptPath, cmd, 0755)
	filename := fmt.Sprintf("%s/init/%s/%s.service", s.dataDir, s.name, s.name)
	content := s.newConfStr(s.name, scriptPath)
	s.checkCreateFileCall(c, 4, filename, content, 0644)
}

func (s *initSystemSuite) TestInstallCommands(c *gc.C) {
	commands, err := s.service.InstallCommands()
	c.Assert(err, jc.ErrorIsNil)

	content := s.newConfStr("jujud-machine-0", "")
	c.Check(commands, jc.DeepEquals, []string{
		"cat >> /tmp/jujud-machine-0.service << 'EOF'\n" + content + "EOF\n",
		"systemd start /tmp/jujud-machine-0.service",
	})
}
