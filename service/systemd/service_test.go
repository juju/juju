// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd_test

import (
	"fmt"
	"os"

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
		Desc: "juju agent for " + tagStr,
		Cmd:  "jujud " + tagStr,
	}
	s.service, err = systemd.NewService(s.name, s.conf)
	c.Assert(err, jc.ErrorIsNil)

	// Reset any incidental calls.
	s.stub.Calls = nil
}

func (s *initSystemSuite) newConfStr(name, cmd string) string {
	tag := name[len("jujud-"):]
	if cmd == "" {
		cmd = "jujud " + tag
	}
	return fmt.Sprintf(confStr[1:], tag, cmd)
}

func (s *initSystemSuite) addUnit(name, status string) {
	tag := name[len("jujud-"):]
	desc := "juju agent for " + tag
	s.conn.AddUnit(name, desc, status)
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
	s.addUnit("jujud-machine-0", "active")
	s.addUnit("something-else", "error")
	s.addUnit("jujud-unit-wordpress-0", "active")
	s.addUnit("another", "inactive")

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
		Name:     s.name,
		Conf:     s.conf,
		Dirname:  fmt.Sprintf("%s/init/%s", s.dataDir, s.name),
		ConfName: s.name + ".service",
	})
	s.stub.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestUpdateConfig(c *gc.C) {
	s.conf.Cmd = "<some other command>"
	c.Assert(s.service.Conf.Cmd, gc.Equals, "jujud machine-0")

	s.service.UpdateConfig(s.conf)

	c.Check(s.service, jc.DeepEquals, &systemd.Service{
		Name:     s.name,
		Conf:     s.conf,
		Dirname:  fmt.Sprintf("%s/init/%s", s.dataDir, s.name),
		ConfName: s.name + ".service",
	})
	s.stub.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestUpdateConfigExtraScript(c *gc.C) {
	s.conf.ExtraScript = "<some other command>"

	s.service.UpdateConfig(s.conf)

	dirname := fmt.Sprintf("%s/init/%s", s.dataDir, s.name)
	c.Check(s.service, jc.DeepEquals, &systemd.Service{
		Name: s.name,
		Conf: common.Conf{
			Desc: s.conf.Desc,
			Cmd:  dirname + "/exec-start.sh",
		},
		Dirname:  dirname,
		ConfName: s.name + ".service",
		Script:   []byte("<some other command>\njujud machine-0"),
	})
	s.stub.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestUpdateConfigMultiline(c *gc.C) {
	s.conf.Cmd = "a\nb\nc"

	s.service.UpdateConfig(s.conf)

	dirname := fmt.Sprintf("%s/init/%s", s.dataDir, s.name)
	c.Check(s.service, jc.DeepEquals, &systemd.Service{
		Name: s.name,
		Conf: common.Conf{
			Desc: s.conf.Desc,
			Cmd:  dirname + "/exec-start.sh",
		},
		Dirname:  dirname,
		ConfName: s.name + ".service",
		Script:   []byte("a\nb\nc"),
	})
	s.stub.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestInstalledTrue(c *gc.C) {
	s.addUnit("jujud-machine-0", "active")
	s.addUnit("something-else", "error")
	s.addUnit("juju-mongod", "active")

	installed := s.service.Installed()

	c.Check(installed, jc.IsTrue)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestInstalledFalse(c *gc.C) {
	s.addUnit("something-else", "error")

	installed := s.service.Installed()

	c.Check(installed, jc.IsFalse)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestInstalledError(c *gc.C) {
	s.addUnit("jujud-machine-0", "active")
	s.addUnit("something-else", "error")
	s.addUnit("juju-mongod", "active")
	failure := errors.New("<failed>")
	s.stub.SetErrors(failure)

	installed := s.service.Installed()

	c.Check(installed, jc.IsFalse)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestExistsTrue(c *gc.C) {
	// TODO(ericsnow) Finish!
}

func (s *initSystemSuite) TestExistsFalse(c *gc.C) {
	// TODO(ericsnow) Finish!
}

func (s *initSystemSuite) TestExistsError(c *gc.C) {
	// TODO(ericsnow) Finish!
}

func (s *initSystemSuite) TestRunningTrue(c *gc.C) {
	s.addUnit("jujud-machine-0", "active")
	s.addUnit("something-else", "error")
	s.addUnit("juju-mongod", "active")

	running := s.service.Running()

	c.Check(running, jc.IsTrue)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestRunningFalse(c *gc.C) {
	s.addUnit("jujud-machine-0", "inactive")
	s.addUnit("something-else", "error")
	s.addUnit("juju-mongod", "active")

	running := s.service.Running()

	c.Check(running, jc.IsFalse)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestRunningNotEnabled(c *gc.C) {
	s.addUnit("something-else", "active")

	running := s.service.Running()

	c.Check(running, jc.IsFalse)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestRunningError(c *gc.C) {
	s.addUnit("jujud-machine-0", "active")
	s.addUnit("something-else", "error")
	s.addUnit("juju-mongod", "active")
	failure := errors.New("<failed>")
	s.stub.SetErrors(failure)

	running := s.service.Running()

	c.Check(running, jc.IsFalse)
	s.stub.CheckCallNames(c, "ListUnits", "Close")
}

func (s *initSystemSuite) TestStart(c *gc.C) {
	s.ch <- "done"

	err := s.service.Start()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "StartUnit", "Close")
}

func (s *initSystemSuite) TestStop(c *gc.C) {
	s.ch <- "done"

	err := s.service.Stop()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "StopUnit", "Close")
}

func (s *initSystemSuite) TestStopAndRemove(c *gc.C) {
	s.ch <- "done"

	err := s.service.StopAndRemove()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "StopUnit", "Close", "DisableUnitFiles", "RemoveAll", "Close")
}

func (s *initSystemSuite) TestRemove(c *gc.C) {
	err := s.service.Remove()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "DisableUnitFiles", "RemoveAll", "Close")
}

func (s *initSystemSuite) TestInstall(c *gc.C) {
	err := s.service.Install()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "MkdirAll", "CreateFile", "EnableUnitFiles", "Close")
	s.checkCreateFileCall(c, 1, s.name, "", 0644)
}

func (s *initSystemSuite) TestInstallMultiline(c *gc.C) {
	scriptPath := fmt.Sprintf("%s/init/%s/exec-start.sh", s.dataDir, s.name)
	cmd := "a\nb\nc"
	s.service.Conf.Cmd = scriptPath
	s.service.Script = []byte(cmd)

	err := s.service.Install()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "MkdirAll", "CreateFile", "CreateFile", "EnableUnitFiles", "Close")
	s.checkCreateFileCall(c, 1, scriptPath, cmd, 0755)
	filename := fmt.Sprintf("%s/init/%s/%s.service", s.dataDir, s.name, s.name)
	content := s.newConfStr(s.name, scriptPath)
	s.checkCreateFileCall(c, 2, filename, content, 0644)
}

func (s *initSystemSuite) TestInstallCommands(c *gc.C) {
	// TODO(ericsnow) Finish.
}

///////////////////////////////////////////////////////////

/*
func (s *initSystemSuite) TestInitSystemStart(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.addUnit(name, "inactive")
	s.ch <- "done"

	err := s.init.Start(name)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListUnits", "Close", "StartUnit", "Close")
}

func (s *initSystemSuite) TestInitSystemStartAlreadyRunning(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.addUnit(name, "active")

	err := s.init.Start(name)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *initSystemSuite) TestInitSystemStartNotEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	err := s.init.Start(name)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemStop(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.addUnit(name, "active")
	s.ch <- "done"

	err := s.init.Stop(name)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListUnits", "Close", "StopUnit", "Close")
}

func (s *initSystemSuite) TestInitSystemStopNotRunning(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.addUnit(name, "inactive")

	err := s.init.Stop(name)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemStopNotEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	err := s.init.Stop(name)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemEnable(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	filename := "/var/lib/juju/init/" + name + "/systemd.conf"
	err := s.init.Enable(name, filename)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "ListUnits",
	}, {
		FuncName: "Close",
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

func (s *initSystemSuite) TestInitSystemEnableAlreadyEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.addUnit(name, "inactive")

	filename := "/var/lib/juju/init/" + name + "/systemd.conf"
	err := s.init.Enable(name, filename)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *initSystemSuite) TestInitSystemDisable(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.addUnit(name, "inactive")

	err := s.init.Disable(name)
	c.Assert(err, jc.ErrorIsNil)

	filename := "/var/lib/juju/init/" + name + "/systemd.conf"
	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "ListUnits",
	}, {
		FuncName: "Close",
	}, {
		FuncName: "DisableUnitFiles",
		Args: []interface{}{
			[]string{filename},
			false,
		},
	}, {
		FuncName: "Close",
	}})
}

func (s *initSystemSuite) TestInitSystemDisableNotEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"

	err := s.init.Disable(name)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemIsEnabledTrue(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.addUnit(name, "inactive")

	enabled, err := s.init.IsEnabled(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(enabled, jc.IsTrue)

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "ListUnits",
	}, {
		FuncName: "Close",
	}})
}

func (s *initSystemSuite) TestInitSystemIsEnabledFalse(c *gc.C) {
	name := "jujud-unit-wordpress-0"

	enabled, err := s.init.IsEnabled(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(enabled, jc.IsFalse)
}

func (s *initSystemSuite) TestInitSystemInfoRunning(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.addUnit(name, "active")

	info, err := s.init.Info(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, &initsystems.ServiceInfo{
		Name:        name,
		Description: "juju agent for unit-wordpress-0",
		Status:      "active",
	})

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "ListUnits",
	}, {
		FuncName: "Close",
	}})
}

func (s *initSystemSuite) TestInitSystemInfoNotRunning(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.addUnit(name, "inactive")

	info, err := s.init.Info(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, &initsystems.ServiceInfo{
		Name:        name,
		Description: "juju agent for unit-wordpress-0",
		Status:      "inactive",
	})
}

func (s *initSystemSuite) TestInitSystemInfoNotEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	_, err := s.init.Info(name)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemConf(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	s.addUnit(name, "inactive")

	conf, err := s.init.Conf(name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc: `juju agent for unit-wordpress-0`,
		Cmd:  "jujud unit-wordpress-0",
	})

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "ListUnits",
	}, {
		FuncName: "Close",
	}})
}

func (s *initSystemSuite) TestInitSystemConfNotEnabled(c *gc.C) {
	name := "jujud-unit-wordpress-0"

	_, err := s.init.Conf(name)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestInitSystemValidate(c *gc.C) {
	confName, err := s.init.Validate("jujud-machine-0", s.conf)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(confName, gc.Equals, "jujud-machine-0.service")
	s.stub.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestInitSystemValidateFull(c *gc.C) {
	s.conf.Env = map[string]string{
		"x": "y",
	}
	s.conf.Limit = map[string]string{
		"nofile": "10",
	}
	s.conf.Out = "syslog"

	_, err := s.init.Validate("jujud-machine-0", s.conf)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestInitSystemValidateInvalid(c *gc.C) {
	s.conf.Cmd = ""

	_, err := s.init.Validate("jujud-machine-0", s.conf)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *initSystemSuite) TestInitSystemValidateInvalidOut(c *gc.C) {
	s.conf.Out = "/var/log/juju/machine-0.log"

	_, err := s.init.Validate("jujud-machine-0", s.conf)

	expected := errors.NotValidf("Out")
	c.Check(errors.Cause(err), gc.FitsTypeOf, expected)
}

func (s *initSystemSuite) TestInitSystemValidateInvalidLimit(c *gc.C) {
	s.conf.Limit = map[string]string{
		"x": "y",
	}

	_, err := s.init.Validate("jujud-machine-0", s.conf)

	expected := errors.NotValidf("Limit")
	c.Check(errors.Cause(err), gc.FitsTypeOf, expected)
}

func (s *initSystemSuite) TestInitSystemSerialize(c *gc.C) {
	data, err := s.init.Serialize("jujud-machine-0", s.conf)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, s.confStr)

	s.stub.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestInitSystemSerializeUnsupported(c *gc.C) {
	tag := "unit-wordpress-0"
	name := "jujud-unit-wordpress-0"
	conf := common.Conf{
		Desc: "juju agent for " + tag,
		Cmd:  "jujud " + tag,
		Out:  "/var/log/juju/" + tag,
	}
	_, err := s.init.Serialize(name, conf)

	expected := errors.NotValidf("Out")
	c.Check(errors.Cause(err), gc.FitsTypeOf, expected)
}

func (s *initSystemSuite) TestInitSystemDeserialize(c *gc.C) {
	name := "jujud-unit-wordpress-0"
	data := s.newConfStr(name)
	conf, err := s.init.Deserialize([]byte(data), name)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc: "juju agent for unit-wordpress-0",
		Cmd:  "jujud unit-wordpress-0",
	})

	s.stub.CheckCalls(c, nil)
}

func (s *initSystemSuite) TestInitSystemDeserializeUnsupported(c *gc.C) {
	name := "jujud-machine-0"
	data := `
[Unit]
Description=juju agent for machine-0
After=syslog.target
After=network.target
After=systemd-user-sessions.service

[Service]
Type=forking
StandardOutput=/var/log/juju/machine-0.log
ExecStart=jujud machine-0
RemainAfterExit=yes
Restart=always

[Install]
WantedBy=multi-user.target

`[1:]
	_, err := s.init.Deserialize([]byte(data), name)

	expected := errors.NotValidf("Out")
	c.Check(errors.Cause(err), gc.FitsTypeOf, expected)
}
*/
