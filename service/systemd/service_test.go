// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd_test

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	"github.com/juju/utils/shell"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
	systemdtesting "github.com/juju/juju/service/systemd/testing"
	coretesting "github.com/juju/juju/testing"
)

var renderer = &shell.BashRenderer{}

const confStr = `
[Unit]
Description=juju agent for %s
After=syslog.target
After=network.target
After=systemd-user-sessions.service

[Service]
ExecStart=%s
Restart=on-failure

[Install]
WantedBy=multi-user.target

`

const jujud = "/var/lib/juju/bin/jujud"

var listCmdArg = exec.RunParams{
	Commands: `/bin/systemctl list-unit-files --no-legend --no-page -l -t service | grep -o -P '^\w[\S]*(?=\.service)'`,
}

var errFailure = errors.New("you-failed")

type initSystemSuite struct {
	coretesting.BaseSuite

	dataDir string
	ch      chan string
	dBus    *MockDBusAPI
	fops    *MockFileSystemOps
	exec    *systemd.MockShimExec

	name    string
	tag     names.Tag
	conf    common.Conf
	confStr string
}

var _ = gc.Suite(&initSystemSuite{})

func (s *initSystemSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	dataDir, err := paths.DataDir("vivid")
	c.Assert(err, jc.ErrorIsNil)
	s.dataDir = dataDir

	// Set up the service config.
	tagStr := "machine-0"
	tag, err := names.ParseTag(tagStr)
	c.Assert(err, jc.ErrorIsNil)
	s.tag = tag
	s.name = "jujud-" + tagStr
	s.conf = common.Conf{
		Desc:      "juju agent for " + tagStr,
		ExecStart: jujud + " " + tagStr,
	}
}

func (s *initSystemSuite) patch(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.fops = NewMockFileSystemOps(ctrl)
	s.dBus = NewMockDBusAPI(ctrl)

	s.ch = systemd.PatchNewChan(s)
	s.exec = systemd.PatchExec(s, ctrl)

	return ctrl
}

func (s *initSystemSuite) newService(c *gc.C) *systemd.Service {
	var fac systemd.DBusAPIFactory
	if s.dBus == nil {
		fac = func() (systemd.DBusAPI, error) {
			return nil, errors.New("Prior call to initSystemSuite.patch required before attempting DBusAPI connection")
		}
	} else {
		fac = func() (systemd.DBusAPI, error) { return s.dBus, nil }
	}

	svc, err := systemd.NewService(s.name, s.conf, systemd.EtcSystemdDir, fac, s.fops, renderer.Join(s.dataDir, "init"))
	c.Assert(err, jc.ErrorIsNil)
	return svc
}

func (s *initSystemSuite) expectConf(c *gc.C, conf common.Conf) *gomock.Call {
	data, err := systemd.Serialize(s.name, conf, renderer)
	c.Assert(err, jc.ErrorIsNil)

	return s.exec.EXPECT().RunCommands(
		exec.RunParams{
			Commands: "cat /etc/systemd/system/jujud-machine-0.service",
		},
	).Return(&exec.ExecResponse{Stdout: data}, nil)
}

func (s *initSystemSuite) newConfStr(name string) string {
	return s.newConfStrCmd(name, "")
}

func (s *initSystemSuite) newConfStrCmd(name, cmd string) string {
	tag := name[len("jujud-"):]
	if cmd == "" {
		cmd = jujud + " " + tag
	}
	return fmt.Sprintf(confStr[1:], tag, cmd)
}

func (s *initSystemSuite) newConfStrEnv(name, env string) string {
	const replace = "[Service]\n"
	result := s.newConfStr(name)
	result = strings.Replace(
		result, replace,
		fmt.Sprintf("%sEnvironment=%s\n", replace, env),
		1,
	)
	return result
}

func (s *initSystemSuite) TestListServices(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{
		Stdout: []byte("jujud-machine-0\njujud-unit-wordpress-0"),
	}, nil)

	services, err := systemd.ListServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(services, jc.SameContents, []string{"jujud-machine-0", "jujud-unit-wordpress-0"})
}

func (s *initSystemSuite) TestListServicesEmpty(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{Stdout: []byte("")}, nil)

	services, err := systemd.ListServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(services, gc.HasLen, 0)
}

func (s *initSystemSuite) TestNewService(c *gc.C) {
	svc := s.newService(c)
	c.Check(svc.Service, jc.DeepEquals, common.Service{Name: s.name, Conf: s.conf})
	c.Check(svc.ConfName, gc.Equals, s.name+".service")
	c.Check(svc.UnitName, gc.Equals, s.name+".service")
	c.Check(svc.DirName, gc.Equals, systemd.EtcSystemdDir)
}

func (s *initSystemSuite) TestNewServiceLogfile(c *gc.C) {
	s.conf.Logfile = "/var/log/juju/machine-0.log"
	svc := s.newService(c)

	user, group := paths.SyslogUserGroup()
	script := `
#!/usr/bin/env bash

# Set up logging.
touch '/var/log/juju/machine-0.log'
chown `[1:] + user + `:` + group + ` '/var/log/juju/machine-0.log'
chmod 0640 '/var/log/juju/machine-0.log'
exec >> '/var/log/juju/machine-0.log'
exec 2>&1

# Run the script.
` + jujud + " machine-0"

	c.Check(svc.Service, jc.DeepEquals, common.Service{
		Name: s.name,
		Conf: common.Conf{
			Desc:      s.conf.Desc,
			ExecStart: path.Join(svc.DirName, svc.Name()+"-exec-start.sh"),
			Logfile:   "/var/log/juju/machine-0.log",
		},
	})

	c.Check(svc.ConfName, gc.Equals, s.name+".service")
	c.Check(svc.UnitName, gc.Equals, s.name+".service")
	c.Check(svc.DirName, gc.Equals, systemd.EtcSystemdDir)

	// This gives us a more readable output if they aren't equal.
	c.Check(string(svc.Script), gc.Equals, script)
	c.Check(strings.Split(string(svc.Script), "\n"), jc.DeepEquals, strings.Split(script, "\n"))
}

func (s *initSystemSuite) TestNewServiceEmptyConf(c *gc.C) {
	svc, err := systemd.NewService(
		s.name, common.Conf{}, systemd.EtcSystemdDir, systemd.NewDBusAPI, s.fops, renderer.Join(s.dataDir, "init"))
	c.Assert(err, gc.IsNil)
	c.Check(svc.Service, jc.DeepEquals, common.Service{Name: s.name})
	c.Check(svc.ConfName, gc.Equals, s.name+".service")
	c.Check(svc.UnitName, gc.Equals, s.name+".service")
	c.Check(svc.DirName, gc.Equals, systemd.EtcSystemdDir)
}

func (s *initSystemSuite) TestNewServiceBasic(c *gc.C) {
	s.conf.ExecStart = "/path/to/some/other/command"
	svc := s.newService(c)
	c.Check(svc.Service, jc.DeepEquals, common.Service{Name: s.name, Conf: s.conf})
	c.Check(svc.ConfName, gc.Equals, s.name+".service")
	c.Check(svc.UnitName, gc.Equals, s.name+".service")
	c.Check(svc.DirName, gc.Equals, systemd.EtcSystemdDir)
}

func (s *initSystemSuite) TestNewServiceExtraScript(c *gc.C) {
	s.conf.ExtraScript = "'/path/to/another/command'"
	svc := s.newService(c)

	script := `
#!/usr/bin/env bash

'/path/to/another/command'
`[1:] + jujud + " machine-0"

	c.Check(svc.Service, jc.DeepEquals, common.Service{
		Name: s.name,
		Conf: common.Conf{
			Desc:      s.conf.Desc,
			ExecStart: path.Join(svc.DirName, svc.Name()+"-exec-start.sh"),
		},
	})

	c.Check(svc.ConfName, gc.Equals, s.name+".service")
	c.Check(svc.UnitName, gc.Equals, s.name+".service")
	c.Check(svc.DirName, gc.Equals, systemd.EtcSystemdDir)
	c.Check(string(svc.Script), gc.Equals, script)
}

func (s *initSystemSuite) TestNewServiceMultiLine(c *gc.C) {
	s.conf.ExecStart = "a\nb\nc"
	svc := s.newService(c)

	script := `
#!/usr/bin/env bash

a
b
c`[1:]

	c.Check(svc.Service, jc.DeepEquals, common.Service{
		Name: s.name,
		Conf: common.Conf{
			Desc:      s.conf.Desc,
			ExecStart: path.Join(svc.DirName, svc.Name()+"-exec-start.sh"),
		},
	})

	c.Check(svc.ConfName, gc.Equals, s.name+".service")
	c.Check(svc.UnitName, gc.Equals, s.name+".service")
	c.Check(svc.DirName, gc.Equals, systemd.EtcSystemdDir)

	// This gives us a more readable output if they aren't equal.
	c.Check(string(svc.Script), gc.Equals, script)
}

func (s *initSystemSuite) TestInstalledTrue(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{
		Stdout: []byte("jujud-machine-0\njuju-mongod"),
	}, nil)

	installed, err := s.newService(c).Installed()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(installed, jc.IsTrue)
}

func (s *initSystemSuite) TestInstalledFalse(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{
		Stdout: []byte("some-other-service"),
	}, nil)

	installed, err := s.newService(c).Installed()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(installed, jc.IsFalse)
}

func (s *initSystemSuite) TestInstalledError(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	s.exec.EXPECT().RunCommands(listCmdArg).Return(nil, errFailure)

	installed, err := s.newService(c).Installed()
	c.Assert(errors.Cause(err), gc.Equals, errFailure)
	c.Check(installed, jc.IsFalse)
}

func (s *initSystemSuite) TestExistsTrue(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()
	s.expectConf(c, s.conf)

	exists, err := s.newService(c).Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, jc.IsTrue)
}

func (s *initSystemSuite) TestExistsFalse(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	// We force the systemd API to return a slightly different conf.
	// In this case we simply set Conf.Env, which s.conf does not set.
	// This causes Service.Exists to return false.
	s.expectConf(c, common.Conf{
		Desc:      s.conf.Desc,
		ExecStart: s.conf.ExecStart,
		Env:       map[string]string{"a": "b"},
	})

	exists, err := s.newService(c).Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(exists, jc.IsFalse)
}

func (s *initSystemSuite) TestExistsError(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	s.exec.EXPECT().RunCommands(
		exec.RunParams{
			Commands: "cat /etc/systemd/system/jujud-machine-0.service",
		},
	).Return(nil, errFailure)

	exists, err := s.newService(c).Exists()
	c.Assert(errors.Cause(err), gc.Equals, errFailure)
	c.Check(exists, jc.IsFalse)
}

func (s *initSystemSuite) TestExistsEmptyConf(c *gc.C) {
	svc := s.newService(c)
	svc.Service.Conf = common.Conf{}
	_, err := svc.Exists()
	c.Check(err, gc.ErrorMatches, `.*no conf expected.*`)
}

func (s *initSystemSuite) TestRunningTrue(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.dBus.EXPECT().ListUnits().Return([]dbus.UnitStatus{
			{Name: "jujud-machine-0.service", LoadState: "loaded", ActiveState: "active"},
			{Name: "juju-mongod.service", LoadState: "loaded", ActiveState: "active"},
		}, nil),
		s.dBus.EXPECT().Close(),
	)

	running, err := s.newService(c).Running()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(running, jc.IsTrue)
}

func (s *initSystemSuite) TestRunningFalse(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.dBus.EXPECT().ListUnits().Return([]dbus.UnitStatus{
			{Name: "jujud-machine-0.service", LoadState: "loaded", ActiveState: "inactive"},
			{Name: "juju-mongod.service", LoadState: "loaded", ActiveState: "active"},
		}, nil),
		s.dBus.EXPECT().Close(),
	)

	running, err := s.newService(c).Running()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(running, jc.IsFalse)
}

func (s *initSystemSuite) TestRunningNotEnabled(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.dBus.EXPECT().ListUnits().Return([]dbus.UnitStatus{
			{Name: "random-thing.service", LoadState: "loaded", ActiveState: "active"},
		}, nil),
		s.dBus.EXPECT().Close(),
	)

	running, err := s.newService(c).Running()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(running, jc.IsFalse)
}

func (s *initSystemSuite) TestRunningError(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.dBus.EXPECT().ListUnits().Return(nil, errFailure),
		s.dBus.EXPECT().Close(),
	)

	_, err := s.newService(c).Running()
	c.Check(errors.Cause(err), gc.Equals, errFailure)
}

func (s *initSystemSuite) TestStart(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	svc := s.newService(c)

	gomock.InOrder(
		s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{
			Stdout: []byte("jujud-machine-0\njuju-mongod"),
		}, nil),
		s.dBus.EXPECT().ListUnits().Return([]dbus.UnitStatus{
			{Name: svc.UnitName, LoadState: "loaded", ActiveState: "inactive"},
		}, nil),
		s.dBus.EXPECT().Close(),

		// Equality check for the channel fails here, so we use Any().
		// We know this is safe, because we notify on the channel we got from
		// the patched call and everything proceeds happily.
		s.dBus.EXPECT().StartUnit(svc.UnitName, "fail", gomock.Any()).Return(1, nil).Do(
			func(_ ...interface{}) { s.ch <- "done" },
		),
		s.dBus.EXPECT().Close(),
	)

	err := svc.Start()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestStartAlreadyRunning(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	svc := s.newService(c)

	gomock.InOrder(
		s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{
			Stdout: []byte("jujud-machine-0\njuju-mongod"),
		}, nil),
		s.dBus.EXPECT().ListUnits().Return([]dbus.UnitStatus{
			{Name: svc.UnitName, LoadState: "loaded", ActiveState: "active"},
		}, nil),
		s.dBus.EXPECT().Close(),
	)

	err := svc.Start()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestStartNotInstalled(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{Stdout: []byte("")}, nil)

	err := s.newService(c).Start()
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *initSystemSuite) TestStop(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	svc := s.newService(c)

	gomock.InOrder(
		s.dBus.EXPECT().ListUnits().Return([]dbus.UnitStatus{
			{Name: svc.UnitName, LoadState: "loaded", ActiveState: "active"},
		}, nil),
		s.dBus.EXPECT().Close(),

		// Equality check for the channel fails here, so we use Any().
		// We know this is safe, because we notify on the channel we got from
		// the patched call and everything proceeds happily.
		s.dBus.EXPECT().StopUnit(svc.UnitName, "fail", gomock.Any()).Return(1, nil).Do(
			func(_ ...interface{}) { s.ch <- "done" },
		),
		s.dBus.EXPECT().Close(),
	)

	err := svc.Stop()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestStopNotRunning(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	svc := s.newService(c)

	gomock.InOrder(
		s.dBus.EXPECT().ListUnits().Return([]dbus.UnitStatus{
			{Name: svc.UnitName, LoadState: "loaded", ActiveState: "inactive"},
		}, nil),
		s.dBus.EXPECT().Close(),
	)

	err := svc.Stop()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestStopNotInstalled(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.dBus.EXPECT().ListUnits().Return(nil, nil),
		s.dBus.EXPECT().Close(),
	)

	err := s.newService(c).Stop()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestRemove(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	svc := s.newService(c)

	gomock.InOrder(
		s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{Stdout: []byte(svc.Name())}, nil),
		s.dBus.EXPECT().DisableUnitFiles([]string{svc.UnitName}, false).Return(nil, nil),
		s.dBus.EXPECT().Reload().Return(nil),
		s.fops.EXPECT().Remove(path.Join(svc.DirName, svc.ConfName)).Return(nil),
		s.fops.EXPECT().Remove(path.Join(svc.DirName, svc.Name()+"-exec-start.sh")).Return(nil),
		s.dBus.EXPECT().Close(),
	)

	err := svc.Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestRemoveNotInstalled(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{Stdout: []byte("")}, nil)

	err := s.newService(c).Remove()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestInstall(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	fileName := fmt.Sprintf("%s/%s.service", systemd.EtcSystemdDir, s.name)

	gomock.InOrder(
		s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{Stdout: []byte("")}, nil),
		s.fops.EXPECT().WriteFile(fileName, []byte(s.newConfStr(s.name)), os.FileMode(0644)).Return(nil),
		s.dBus.EXPECT().LinkUnitFiles([]string{fileName}, false, true).Return(nil, nil),
		s.dBus.EXPECT().Reload().Return(nil),
		s.dBus.EXPECT().EnableUnitFiles([]string{fileName}, false, true).Return(true, nil, nil),
		s.dBus.EXPECT().Close(),
	)

	err := s.newService(c).Install()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestRemoveOld(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	svc := s.newService(c)

	s.fops.EXPECT().RemoveAll(path.Join(systemd.LibSystemdDir, svc.Name())).Return(nil)

	err := svc.RemoveOldService()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestInstallAlreadyInstalled(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	s.expectConf(c, s.conf)
	svc := s.newService(c)

	s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{Stdout: []byte(svc.Name())}, nil)

	err := svc.Install()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestInstallZombie(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	// We force the systemd API to return a slightly different conf.
	// In this case we simply set a different Env value between the
	// conf we are installing and the conf returned by the systemd API.
	// This causes Service.Exists to return false.
	conf := common.Conf{
		Desc:      s.conf.Desc,
		ExecStart: s.conf.ExecStart,
		Env:       map[string]string{"a": "b"},
	}
	s.expectConf(c, conf)
	conf.Env["a"] = "c"
	svc, err := systemd.NewService(
		s.name,
		conf,
		systemd.EtcSystemdDir,
		func() (systemd.DBusAPI, error) { return s.dBus, nil },
		s.fops,
		renderer.Join(s.dataDir, "init"),
	)
	c.Assert(err, jc.ErrorIsNil)

	fileName := fmt.Sprintf("%s/%s.service", systemd.EtcSystemdDir, s.name)

	s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{Stdout: []byte(svc.Name())}, nil).Times(2)
	s.dBus.EXPECT().Close().Times(3)

	s.dBus.EXPECT().ListUnits().Return([]dbus.UnitStatus{
		{Name: svc.Name(), LoadState: "loaded", ActiveState: "active"},
	}, nil)
	s.dBus.EXPECT().DisableUnitFiles([]string{svc.UnitName}, false).Return(nil, nil)
	s.dBus.EXPECT().Reload().Return(nil)
	s.fops.EXPECT().Remove(path.Join(svc.DirName, svc.ConfName)).Return(nil)
	s.fops.EXPECT().Remove(path.Join(svc.DirName, svc.Name()+"-exec-start.sh")).Return(nil)
	s.fops.EXPECT().WriteFile(fileName, []byte(s.newConfStrEnv(s.name, `"a=c"`)), os.FileMode(0644)).Return(nil)
	s.dBus.EXPECT().LinkUnitFiles([]string{fileName}, false, true).Return(nil, nil)
	s.dBus.EXPECT().Reload().Return(nil)
	s.dBus.EXPECT().EnableUnitFiles([]string{fileName}, false, true).Return(true, nil, nil)

	err = svc.Install()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestInstallMultiLine(c *gc.C) {
	ctrl := s.patch(c)
	defer ctrl.Finish()

	fileName := fmt.Sprintf("%s/%s.service", systemd.EtcSystemdDir, s.name)
	scriptPath := fmt.Sprintf("%s/%s-exec-start.sh", systemd.EtcSystemdDir, s.name)
	cmd := "a\nb\nc"

	svc := s.newService(c)
	svc.Service.Conf.ExecStart = scriptPath
	svc.Script = []byte(cmd)

	gomock.InOrder(
		s.exec.EXPECT().RunCommands(listCmdArg).Return(&exec.ExecResponse{Stdout: []byte("")}, nil),
		s.fops.EXPECT().WriteFile(scriptPath, []byte(cmd), os.FileMode(0755)).Return(nil),
		s.fops.EXPECT().WriteFile(fileName, []byte(s.newConfStrCmd(s.name, scriptPath)), os.FileMode(0644)).Return(nil),
		s.dBus.EXPECT().LinkUnitFiles([]string{fileName}, false, true).Return(nil, nil),
		s.dBus.EXPECT().Reload().Return(nil),
		s.dBus.EXPECT().EnableUnitFiles([]string{fileName}, false, true).Return(true, nil, nil),
		s.dBus.EXPECT().Close(),
	)

	err := svc.Install()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *initSystemSuite) TestInstallEmptyConf(c *gc.C) {
	svc := s.newService(c)
	svc.Service.Conf = common.Conf{}
	err := svc.Install()
	c.Check(err, gc.ErrorMatches, `.*missing conf.*`)
}

func (s *initSystemSuite) TestInstallCommands(c *gc.C) {
	name := "jujud-machine-0"
	commands, err := s.newService(c).InstallCommands()
	c.Assert(err, jc.ErrorIsNil)

	test := systemdtesting.WriteConfTest{
		Service:  name,
		DataDir:  systemd.EtcSystemdDir,
		Expected: s.newConfStr(name),
	}
	test.CheckCommands(c, commands)
}

func (s *initSystemSuite) TestInstallCommandsLogfile(c *gc.C) {
	name := "jujud-machine-0"
	s.conf.Logfile = "/var/log/juju/machine-0.log"
	svc := s.newService(c)
	commands, err := svc.InstallCommands()
	c.Assert(err, jc.ErrorIsNil)

	user, group := paths.SyslogUserGroup()
	test := systemdtesting.WriteConfTest{
		Service: name,
		DataDir: systemd.EtcSystemdDir,
		Expected: strings.Replace(
			s.newConfStr(name),
			"ExecStart=/var/lib/juju/bin/jujud machine-0",
			"ExecStart=/etc/systemd/system/jujud-machine-0-exec-start.sh",
			-1),
		Script: `
# Set up logging.
touch '/var/log/juju/machine-0.log'
chown `[1:] + user + `:` + group + ` '/var/log/juju/machine-0.log'
chmod 0640 '/var/log/juju/machine-0.log'
exec >> '/var/log/juju/machine-0.log'
exec 2>&1

# Run the script.
` + jujud + " machine-0",
	}

	test.CheckCommands(c, commands)
}

func (s *initSystemSuite) TestInstallCommandsShutdown(c *gc.C) {
	name := "juju-shutdown-job"
	conf, err := service.ShutdownAfterConf("cloud-final")
	c.Assert(err, jc.ErrorIsNil)

	svc, err := systemd.NewService(
		name, conf, systemd.EtcSystemdDir, systemd.NewDBusAPI, s.fops, renderer.Join(s.dataDir, "init"))
	c.Assert(err, jc.ErrorIsNil)

	commands, err := svc.InstallCommands()
	c.Assert(err, jc.ErrorIsNil)

	test := systemdtesting.WriteConfTest{
		Service: name,
		DataDir: systemd.EtcSystemdDir,
		Expected: `
[Unit]
Description=juju shutdown job
After=syslog.target
After=network.target
After=systemd-user-sessions.service
After=cloud-final

[Service]
ExecStart=/sbin/shutdown -h now
ExecStopPost=/bin/systemctl disable juju-shutdown-job.service

[Install]
WantedBy=multi-user.target
`[1:],
	}

	test.CheckCommands(c, commands)
}

func (s *initSystemSuite) TestInstallCommandsEmptyConf(c *gc.C) {
	svc := s.newService(c)
	svc.Service.Conf = common.Conf{}
	_, err := svc.InstallCommands()
	c.Check(err, gc.ErrorMatches, `.*missing conf.*`)
}

func (s *initSystemSuite) TestStartCommands(c *gc.C) {
	commands, err := s.newService(c).StartCommands()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(commands, jc.DeepEquals, []string{"/bin/systemctl start jujud-machine-0.service"})
}

func (s *initSystemSuite) TestInstallLimits(c *gc.C) {
	name := "juju-job"
	conf := common.Conf{
		Desc:      "juju agent for juju-job",
		ExecStart: "/usr/bin/jujud juju-job",
		Limit: map[string]string{
			"fsize":   "unlimited",
			"cpu":     "unlimited",
			"as":      "12345",
			"memlock": "unlimited",
			"nofile":  "64000",
		},
	}
	data, err := systemd.Serialize(name, conf, renderer)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Equals, `
[Unit]
Description=juju agent for juju-job
After=syslog.target
After=network.target
After=systemd-user-sessions.service

[Service]
LimitAS=12345
LimitCPU=infinity
LimitFSIZE=infinity
LimitMEMLOCK=infinity
LimitNOFILE=64000
ExecStart=/usr/bin/jujud juju-job
Restart=on-failure

[Install]
WantedBy=multi-user.target

`[1:])
}
