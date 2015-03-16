// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package service_test

import (
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
)

type serviceSuite struct {
	service.BaseSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestNewServiceKnown(c *gc.C) {
	initSystems := []string{
		service.InitSystemSystemd,
		service.InitSystemUpstart,
		service.InitSystemWindows,
	}
	for _, initSystem := range initSystems {
		svc, err := service.NewService(s.Name, s.Conf, initSystem)
		if !c.Check(err, jc.ErrorIsNil) {
			continue
		}

		switch initSystem {
		case service.InitSystemSystemd:
			c.Check(svc, gc.FitsTypeOf, &systemd.Service{})
		case service.InitSystemUpstart:
			c.Check(svc, gc.FitsTypeOf, &upstart.Service{})
		case service.InitSystemWindows:
			c.Check(svc, gc.FitsTypeOf, &windows.Service{})
		}
		c.Check(svc.Name(), gc.Equals, s.Name)
		c.Check(svc.Conf(), jc.DeepEquals, s.Conf)
	}
}

func (s *serviceSuite) TestNewServiceMissingName(c *gc.C) {
	_, err := service.NewService("", s.Conf, service.InitSystemUpstart)

	c.Check(err, gc.ErrorMatches, `.*missing name.*`)
}

func (s *serviceSuite) TestNewServiceUnknown(c *gc.C) {
	_, err := service.NewService(s.Name, s.Conf, "<unknown>")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *serviceSuite) TestListServices(c *gc.C) {
	_, err := service.ListServices()

	c.Check(err, jc.ErrorIsNil)
}

func (*serviceSuite) TestListServicesScript(c *gc.C) {
	commands := service.ListServicesScript()

	writeLines := "cat > /tmp/discover_init_system.sh << 'EOF'\n" +
		service.DiscoverInitSystemScript + "\n" +
		"EOF"
	switchLines := "" +
		"init_system=$(/tmp/discover_init_system.sh) " +
		`if [[ $init_system == "systemd" ]]; then ` +
		`/bin/systemctl list-unit-files --no-legend --no-page -t service` +
		` | grep -o -P '^\w[\S]*(?=\.service)'` + "\n" +
		`elif [[ $init_system == "upstart" ]]; then ` +
		`sudo initctl list | awk '{print $1}' | sort | uniq` + "\n" +
		`else exit 1` + "\n" +
		`fi`
	c.Check(commands, jc.DeepEquals, []string{
		writeLines,
		"chmod 0755 /tmp/discover_init_system.sh",
		switchLines,
	})
	c.Check(strings.Split(commands[0], "\n"), jc.DeepEquals, strings.Split(writeLines, "\n"))
	c.Check(strings.Split(commands[2], "\n"), jc.DeepEquals, strings.Split(switchLines, "\n"))
}

func (s *serviceSuite) TestInstallAndStartOkay(c *gc.C) {
	s.PatchAttempts(5)

	err := service.InstallAndStart(s.Service)
	c.Assert(err, jc.ErrorIsNil)

	s.Service.CheckCallNames(c, "Install", "Start")
}

func (s *serviceSuite) TestInstallAndStartRetry(c *gc.C) {
	s.PatchAttempts(5)
	s.Service.SetErrors(nil, s.Failure, s.Failure)

	err := service.InstallAndStart(s.Service)
	c.Assert(err, jc.ErrorIsNil)

	s.Service.CheckCallNames(c, "Install", "Start", "Start", "Start")
}

func (s *serviceSuite) TestInstallAndStartFail(c *gc.C) {
	s.PatchAttempts(3)
	s.Service.SetErrors(nil, s.Failure, s.Failure, s.Failure)

	err := service.InstallAndStart(s.Service)

	s.CheckFailure(c, err)
	s.Service.CheckCallNames(c, "Install", "Start", "Start", "Start")
}

type restartSuite struct {
	service.BaseSuite
}

var _ = gc.Suite(&restartSuite{})

func (s *restartSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.Patched.Service = s.Service
}

func (s *restartSuite) TestRestartStopAndStart(c *gc.C) {
	err := service.Restart(s.Name)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "DiscoverService", "Stop", "Start")
}

type restartable struct {
	*service.FakeService
}

func (s *restartable) Restart() error {
	s.AddCall("Restart")

	return s.NextErr()
}

func (s *restartSuite) TestRestartRestartable(c *gc.C) {
	s.Patched.Service = &restartable{s.Service}

	err := service.Restart(s.Name)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "DiscoverService", "Restart")
}

func (s *restartSuite) TestRestartFailDiscovery(c *gc.C) {
	s.Stub.SetErrors(s.Failure)

	err := service.Restart(s.Name)

	s.CheckFailure(c, err)
	s.Stub.CheckCallNames(c, "DiscoverService")
}

func (s *restartSuite) TestRestartFailStop(c *gc.C) {
	s.Stub.SetErrors(nil, s.Failure) // DiscoverService, Stop

	err := service.Restart(s.Name)

	s.CheckFailure(c, err)
	s.Stub.CheckCallNames(c, "DiscoverService", "Stop")
}

func (s *restartSuite) TestRestartFailStart(c *gc.C) {
	s.Stub.SetErrors(nil, nil, s.Failure) // DiscoverService, Stop, Start

	err := service.Restart(s.Name)

	s.CheckFailure(c, err)
	s.Stub.CheckCallNames(c, "DiscoverService", "Stop", "Start")
}

func (s *restartSuite) TestRestartFailRestart(c *gc.C) {
	s.Patched.Service = &restartable{s.Service}
	s.Stub.SetErrors(nil, s.Failure) // DiscoverService, Restart

	err := service.Restart(s.Name)

	s.CheckFailure(c, err)
	s.Stub.CheckCallNames(c, "DiscoverService", "Restart")
}
