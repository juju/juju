// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/coreos/go-systemd/dbus"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
	coretesting "github.com/juju/juju/testing"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type SystemdSuite struct {
	coretesting.BaseSuite
	service *systemd.Service
	initDir string
}

var _ = gc.Suite(&SystemdSuite{})

func (s *SystemdSuite) SetUpTest(c *gc.C) {
	s.initDir = c.MkDir()
	s.PatchValue(&systemd.InitDir, s.initDir)

	env := make(map[string]string)
	env["DIR"] = "dir"
	env["VAR"] = "val"
	s.service = systemd.NewService(
		"dummy-service",
		common.Conf{
			Desc:        "dummy service for testing",
			Cmd:         "some-command --execute",
			Env:         env,
			ExtraScript: "some -t extra --script",
		},
	)
}

// TestInitDirDefaulting tests wether the InitDir of a new service is properly
// set to its default value if it is not directly provided.
func (s *SystemdSuite) TestInitDirDefaulting(c *gc.C) {
	service := systemd.NewService("service", common.Conf{})
	c.Assert(service.Conf.InitDir, gc.Equals, s.initDir)
}

// TestServiceFilePath tests that servicePath() properly returns the full path of
// the service file associated to a service.
func (s *SystemdSuite) TestServiceFilePath(c *gc.C) {
	c.Assert(s.service.ServiceFilePath(), gc.Equals, path.Join(s.initDir, "dummy-service.service"))
}

// TestExtraScriptPath tests that extraScriptPath() properly returns the full
// path of the extra script file associated to a service.
func (s *SystemdSuite) TestExtraScriptPath(c *gc.C) {
	c.Assert(s.service.ExtraScriptPath(), gc.Equals, path.Join(s.initDir, "dummy-service-extra.sh"))
}

// buildDummyRunCommand returns a function with an identical signature to
// systemd.RunCommand that returns the strings given by args and a nil error.
// the returned function remembers the commands it recieved in *issuedcmds.
func (s *SystemdSuite) buildDummyRunCommand(issuedcmds *[]string, args ...string) func(string, ...string) (string, error) {
	*issuedcmds = []string{}
	return func(cmd string, cmdargs ...string) (string, error) {
		*issuedcmds = append(*issuedcmds, fmt.Sprintf("%s %s", cmd, strings.Join(cmdargs, " ")))
		return strings.Join(args, "\n"), nil
	}
}

// buildDummyListUnits returns a function with an identical signature to
// listUnits() that returns a list dbus.UnitStatus structs which (depending on
// the value of the parameter given) will return a list with our services
// in/not in it, and a nil error.
func (s *SystemdSuite) buildDummyListUnits(good bool) func() ([]dbus.UnitStatus, error) {
	if good {
		return func() ([]dbus.UnitStatus, error) {
			return []dbus.UnitStatus{
				dbus.UnitStatus{
					Name:        s.service.ServiceName(),
					ActiveState: "active",
				},
			}, nil
		}
	}

	return func() ([]dbus.UnitStatus, error) {
		return []dbus.UnitStatus{}, nil
	}
}

// dummyReloadDaemon is a simple function which always returns a nil error.
func dummyReloadDaemon() error {
	return nil
}

// dummyFunctionProto is a simple function which takes the address of a boolean
// and returns a function which takes a string and will set that boolean to true
// if called and a nil error.
func dummyFunctionProto(boolean *bool) func(string) error {
	return func(string) error {
		*boolean = true
		return nil
	}
}

// writeValidServiceFile writes a proper service file to the InitDir.
func (s *SystemdSuite) writeValidServiceFile(c *gc.C) {
	contents, err := s.service.Render()
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(s.service.ServiceFilePath(), contents, 0644)
	c.Assert(err, jc.ErrorIsNil)
}

// writeInvalidServiceFile writes an unrelated service file to the InitDir.
func (s *SystemdSuite) writeInvalidServiceFile(c *gc.C) {
	err := ioutil.WriteFile(s.service.ServiceFilePath(), []byte("nothing relevant"), 0644)
	c.Assert(err, jc.ErrorIsNil)
}

// writeValidScriptFile writes a proper ExtraScript file to the InitDir.
func (s *SystemdSuite) writeValidScriptFile(c *gc.C) {
	contents := fmt.Sprintf(systemd.ExtraScriptTemplate, s.service.Conf.ExtraScript)
	err := ioutil.WriteFile(s.service.ExtraScriptPath(), []byte(contents), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

// writeInvalidScriptFile writes an unrelated ExtraScript file to the InitDir.
func (s *SystemdSuite) writeInvalidScriptFile(c *gc.C) {
	err := ioutil.WriteFile(s.service.ExtraScriptPath(), []byte("nothing relevant"), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

// serviceStatusCommand returns a slice of strings with a single element
// representing the systemctl command of statting the service represented by s.
func (s *SystemdSuite) serviceStatusCommand() []string {
	return []string{"systemctl status " + s.service.ServiceName()}
}

// TestExistsAndMatches tests existsAndMatches under various circumstances.
func (s *SystemdSuite) TestExistsAndMatches(c *gc.C) {
	// non-existent service file or ExtraScript file
	exists, matches, err := s.service.ExistsAndMatches()
	c.Assert(exists, jc.IsFalse)
	c.Assert(matches, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)

	// properly installed service file and ExtraScript file with different content
	s.writeInvalidServiceFile(c)
	s.writeInvalidScriptFile(c)
	exists, matches, err = s.service.ExistsAndMatches()
	c.Assert(exists, jc.IsTrue)
	c.Assert(matches, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)

	// properly installed matching service file with mismatching ExtraScript file
	s.writeValidServiceFile(c)
	s.writeInvalidServiceFile(c)
	exists, matches, err = s.service.ExistsAndMatches()
	c.Assert(exists, jc.IsTrue)
	c.Assert(matches, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)

	// mismatching service file with matching ExtraScript file
	s.writeInvalidServiceFile(c)
	s.writeValidScriptFile(c)
	exists, matches, err = s.service.ExistsAndMatches()
	c.Assert(exists, jc.IsTrue)
	c.Assert(matches, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)

	// properly installed service file with suitable content
	s.writeValidServiceFile(c)
	exists, matches, err = s.service.ExistsAndMatches()
	c.Assert(exists, jc.IsTrue)
	c.Assert(matches, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
}

// TestEnabled tests wether enabled properly recognises the status of the service.
func (s *SystemdSuite) TestEnabled(c *gc.C) {
	var issuedcmds []string

	s.PatchValue(systemd.RunCommand, s.buildDummyRunCommand(&issuedcmds, "nothing about its status"))
	c.Assert(s.service.Enabled(), jc.IsFalse)
	c.Assert(issuedcmds, gc.DeepEquals, s.serviceStatusCommand())

	s.PatchValue(systemd.RunCommand, s.buildDummyRunCommand(&issuedcmds, "the service is enabled"))
	c.Assert(s.service.Enabled(), jc.IsTrue)
	c.Assert(issuedcmds, gc.DeepEquals, s.serviceStatusCommand())
}

// TestInstallFromScratch tests a completely fresh installation of the service
// using Install.
func (s *SystemdSuite) TestInstallFromScratch(c *gc.C) {
	var issuedcmds []string
	var enableCalled, startCalled bool

	s.PatchValue(systemd.RunCommand, s.buildDummyRunCommand(&issuedcmds, "the service is enabled"))
	s.PatchValue(systemd.Reload, dummyReloadDaemon)
	s.PatchValue(systemd.Enable, dummyFunctionProto(&enableCalled))
	s.PatchValue(systemd.Start, dummyFunctionProto(&startCalled))

	c.Assert(s.service.Install(), jc.ErrorIsNil)

	// check that enabled() did not get a chance to get called
	c.Assert(issuedcmds, gc.DeepEquals, []string{})

	// check that enable() and start() were called
	c.Assert(enableCalled, jc.IsTrue)
	c.Assert(startCalled, jc.IsTrue)

	// check service file contents
	found, err := ioutil.ReadFile(s.service.ServiceFilePath())
	c.Assert(err, jc.ErrorIsNil)
	expected, err := s.service.Render()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals, expected)

	// check ExtraScript file contents
	found, err = ioutil.ReadFile(s.service.ExtraScriptPath())
	c.Assert(err, jc.ErrorIsNil)
	expected = []byte(fmt.Sprintf(systemd.ExtraScriptTemplate, s.service.Conf.ExtraScript))
	c.Assert(found, gc.DeepEquals, expected)
}

// TestInstallDefaultsIfAlreadyInstalled tests wether Install promptly returns
// if the service is already installed and enabled.
func (s *SystemdSuite) TestInstallDefaultsIfAlreadyInstalled(c *gc.C) {
	var issuedcmds []string
	var enableCalled, startCalled bool

	s.writeValidScriptFile(c)
	s.writeValidServiceFile(c)
	s.PatchValue(systemd.RunCommand, s.buildDummyRunCommand(&issuedcmds, "the service is enabled"))
	s.PatchValue(systemd.Reload, dummyReloadDaemon)
	s.PatchValue(systemd.Enable, dummyFunctionProto(&enableCalled))
	s.PatchValue(systemd.Start, dummyFunctionProto(&startCalled))

	c.Assert(s.service.Install(), jc.ErrorIsNil)

	// check that enabled() was called
	c.Assert(issuedcmds, gc.DeepEquals, s.serviceStatusCommand())

	// check that enable() and start() were not called
	c.Assert(enableCalled, jc.IsFalse)
	c.Assert(startCalled, jc.IsFalse)
}

// TestInstalled tests Installed's behavior under any set of parameters.
func (s *SystemdSuite) TestInstalled(c *gc.C) {
	var issuedcmds []string

	// non-existent service or ExtraScript files
	c.Assert(s.service.Installed(), jc.IsFalse)

	// existing service and ExtraScript files but disabled service
	s.writeValidScriptFile(c)
	s.writeValidServiceFile(c)
	s.PatchValue(systemd.RunCommand, s.buildDummyRunCommand(&issuedcmds, "nothing about its status"))

	c.Assert(s.service.Installed(), jc.IsFalse)
	c.Assert(issuedcmds, gc.DeepEquals, s.serviceStatusCommand())

	// properly installed and service and ExtraScript files, service enabled
	s.writeValidScriptFile(c)
	s.writeValidServiceFile(c)
	s.PatchValue(systemd.RunCommand, s.buildDummyRunCommand(&issuedcmds, "the service is enabled"))

	c.Assert(s.service.Installed(), jc.IsTrue)
	c.Assert(issuedcmds, gc.DeepEquals, s.serviceStatusCommand())
}

// TestExists tests Exists'behavior under any possible set of parameters.
func (s *SystemdSuite) TestExists(c *gc.C) {
	var issuedcmds []string
	s.PatchValue(systemd.RunCommand, s.buildDummyRunCommand(&issuedcmds, "totally not enabled"))

	// non-existent service or ExtraScript files, service disabled
	c.Assert(s.service.Exists(), jc.IsFalse)

	// existing service and ExtraScript files with improper contents,
	// service still disabled
	s.writeInvalidScriptFile(c)
	s.writeInvalidServiceFile(c)
	c.Assert(s.service.Exists(), jc.IsFalse)

	// existing service and ExtraScript files with improper contents,
	// service now enabled
	s.PatchValue(systemd.RunCommand, s.buildDummyRunCommand(&issuedcmds, "the service is enabled"))
	c.Assert(s.service.Exists(), jc.IsFalse)

	// proper service and ExtraScript files, service disabled
	s.writeValidScriptFile(c)
	s.writeValidServiceFile(c)
	s.PatchValue(systemd.RunCommand, s.buildDummyRunCommand(&issuedcmds, "nothing about its status"))
	c.Assert(s.service.Exists(), jc.IsFalse)

	// proper service and ExtraScript files, service enabled
	s.PatchValue(systemd.RunCommand, s.buildDummyRunCommand(&issuedcmds, "the service is enabled"))
	c.Assert(s.service.Exists(), jc.IsTrue)
}

// TestRunning tests Running under both possible scenarios.
func (s *SystemdSuite) TestRunning(c *gc.C) {
	// service not appearing to be running
	s.PatchValue(systemd.List, s.buildDummyListUnits(false))

	c.Assert(s.service.Running(), jc.IsFalse)

	// `systemctl status` saying service is running
	s.PatchValue(systemd.List, s.buildDummyListUnits(true))

	c.Assert(s.service.Running(), jc.IsTrue)
}

// TestStart tests that Start properly issues the commands to start the service.
func (s *SystemdSuite) TestStart(c *gc.C) {
	var startCalled bool

	s.PatchValue(systemd.Start, dummyFunctionProto(&startCalled))

	c.Assert(s.service.Start(), jc.ErrorIsNil)
}

// TestStop tests that Stop properly issues the command to stop the service.
func (s *SystemdSuite) TestStop(c *gc.C) {
	var stopCalled bool

	s.PatchValue(systemd.Stop, dummyFunctionProto(&stopCalled))

	c.Assert(s.service.Stop(), jc.ErrorIsNil)
	c.Assert(stopCalled, jc.IsTrue)
}

// TestRemove tests if Remove properly cleans up an installed service.
func (s *SystemdSuite) TestRemove(c *gc.C) {
	var disableCalled bool

	s.writeValidScriptFile(c)
	s.writeValidServiceFile(c)

	s.PatchValue(systemd.Disable, dummyFunctionProto(&disableCalled))
	s.PatchValue(systemd.Reload, dummyReloadDaemon)

	c.Assert(s.service.Remove(), jc.ErrorIsNil)

	// check that service file was removed
	_, err := os.Stat(s.service.ServiceFilePath())
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// check that ExtraScript file was removed
	_, err = os.Stat(s.service.ExtraScriptPath())
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// check that the command for disabling the process was properly issued
	c.Assert(disableCalled, jc.IsTrue)
}

// TestStopAndRemove tests StopAndRemove on a properly installed service.
func (s *SystemdSuite) TestStopAndRemove(c *gc.C) {
	var stopCalled, disableCalled bool

	s.writeValidScriptFile(c)
	s.writeValidServiceFile(c)
	s.PatchValue(systemd.Stop, dummyFunctionProto(&stopCalled))
	s.PatchValue(systemd.Disable, dummyFunctionProto(&disableCalled))
	s.PatchValue(systemd.Reload, dummyReloadDaemon)

	c.Assert(s.service.StopAndRemove(), jc.ErrorIsNil)

	// check that the service file was removed
	_, err := os.Stat(s.service.ServiceFilePath())
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// check that the ExtraScript file was removed
	_, err = os.Stat(s.service.ExtraScriptPath())
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	// check that the stopping and disabling commands were properly issued
	c.Assert(stopCalled, jc.IsTrue)
	c.Assert(disableCalled, jc.IsTrue)
}
