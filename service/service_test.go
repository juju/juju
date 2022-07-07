// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service"
	svctesting "github.com/juju/juju/service/common/testing"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
)

type serviceSuite struct {
	service.BaseSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestNewServiceKnown(c *gc.C) {
	for _, test := range []struct {
		series     string
		initSystem string
	}{
		{
			series:     "vivid",
			initSystem: service.InitSystemSystemd,
		}, {
			series:     "trusty",
			initSystem: service.InitSystemUpstart,
		},
	} {
		svc, err := service.NewService(s.Name, s.Conf, test.series)
		if !c.Check(err, jc.ErrorIsNil) {
			continue
		}

		switch test.initSystem {
		case service.InitSystemSystemd:
			c.Check(svc, gc.FitsTypeOf, &systemd.Service{})
		case service.InitSystemUpstart:
			c.Check(svc, gc.FitsTypeOf, &upstart.Service{})
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

	c.Check(errors.Is(err, errors.NotFound), jc.IsTrue)
}

func (s *serviceSuite) TestListServices(c *gc.C) {
	_, err := service.ListServices()

	c.Check(err, jc.ErrorIsNil)
}

func (*serviceSuite) TestListServicesScript(c *gc.C) {
	script := service.ListServicesScript()

	expected := strings.Split(service.DiscoverInitSystemScript(), "\n")
	expected[0] = "init_system=$(" + expected[0]
	expected[len(expected)-1] += ")"
	expected = append(expected,
		`case "$init_system" in`,
		`systemd)`,
		`    /bin/systemctl list-unit-files --no-legend --no-page -l -t service`+
			` | grep -o -P '^\w[\S]*(?=\.service)'`,
		`    ;;`,
		`upstart)`,
		`    sudo initctl list | awk '{print $1}' | sort | uniq`,
		`    ;;`,
		`*)`,
		`    exit 1`,
		`    ;;`,
		`esac`,
	)
	c.Check(strings.Split(script, "\n"), jc.DeepEquals, expected)
}

func (s *serviceSuite) TestInstallAndStartOkay(c *gc.C) {
	s.PatchAttempts(5)

	err := service.InstallAndStart(s.Service)
	c.Assert(err, jc.ErrorIsNil)

	s.Service.CheckCallNames(c, "Name", "Install", "Stop", "Start")
}

func (s *serviceSuite) TestInstallAndStartRetry(c *gc.C) {
	s.PatchAttempts(5)
	s.Service.SetErrors(nil, s.Failure, s.Failure)

	err := service.InstallAndStart(s.Service)
	c.Assert(err, jc.ErrorIsNil)

	s.Service.CheckCallNames(c, "Name", "Install", "Stop", "Start", "Stop", "Start")
}

func (s *serviceSuite) TestInstallAndStartFail(c *gc.C) {
	s.PatchAttempts(3)
	s.Service.SetErrors(nil, s.Failure, s.Failure, s.Failure, s.Failure, s.Failure, s.Failure)

	err := service.InstallAndStart(s.Service)

	s.CheckFailure(c, err)
	s.Service.CheckCallNames(c, "Name", "Install", "Stop", "Start", "Stop", "Start", "Stop", "Start")
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
	*svctesting.FakeService
}

func (s *restartable) Restart() error {
	s.AddCall("Restart")

	return s.NextErr()
}

func (s *restartSuite) TestRestartRestartable(c *gc.C) {
	s.Patched.Service = &restartable{s.Service}

	err := service.Restart(s.Name)
	c.Assert(err, jc.ErrorIsNil)

	// TODO(tsm): fix service.upstart behaviour to match other implementations,
	// then change the test to
	// s.Stub.CheckCallNames(c, "DiscoverService", "Restart")
	s.Stub.CheckCallNames(c, "DiscoverService", "Stop", "Start")
}

func (s *restartSuite) TestRestartFailDiscovery(c *gc.C) {
	s.Stub.SetErrors(s.Failure)

	err := service.Restart(s.Name)

	s.CheckFailure(c, err)
	s.Stub.CheckCallNames(c, "DiscoverService")
}

func (s *restartSuite) TestRestartFailStop(c *gc.C) {
	s.Stub.SetErrors(nil, s.Failure, nil) // DiscoverService, Stop, Start

	err := service.Restart(s.Name)

	// s.CheckFailure(c, err)
	c.Check(err, jc.ErrorIsNil)

	// s.Stub.CheckCallNames(c, "DiscoverService", "Restart")
	s.Stub.CheckCallNames(c, "DiscoverService", "Stop", "Start")
}

func (s *restartSuite) TestRestartFailStart(c *gc.C) {
	s.Stub.SetErrors(nil, nil, s.Failure) // DiscoverService, Stop, Start

	err := service.Restart(s.Name)

	s.CheckFailure(c, err)
	s.Stub.CheckCallNames(c, "DiscoverService", "Stop", "Start")
}

func (s *restartSuite) TestRestartFailRestart(c *gc.C) {
	// TODO(tsm): fix service.upstart behaviour to match other implementations

	s.Patched.Service = &restartable{s.Service}
	//s.Stub.SetErrors(nil, s.Failure)  // DiscoverService, Restart
	s.Stub.SetErrors(nil, nil, s.Failure) // DiscoverService, Stop, Start

	err := service.Restart(s.Name)

	s.CheckFailure(c, err)
	// s.Stub.CheckCallNames(c, "DiscoverService", "Restart")
	s.Stub.CheckCallNames(c, "DiscoverService", "Stop", "Start")
}
