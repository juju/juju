// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package service_test

import (
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
	"github.com/juju/juju/version"
)

const unknownExecutable = "/sbin/unknown/init/system"

type discoveryTest struct {
	os       version.OSType
	series   string
	expected string
}

func (dt discoveryTest) version() version.Binary {
	return version.Binary{
		OS:     dt.os,
		Series: dt.series,
	}
}

func (dt discoveryTest) goos() string {
	switch dt.os {
	case version.Windows:
		return "windows"
	default:
		return "non-windows"
	}
}

func (dt discoveryTest) executable(c *gc.C) string {
	switch dt.expected {
	case service.InitSystemUpstart:
		return "/sbin/upstart"
	case service.InitSystemSystemd:
		return "/lib/systemd/systemd"
	case service.InitSystemWindows:
		return unknownExecutable
	case "":
		return unknownExecutable
	default:
		c.Errorf("unknown expected init system %q", dt.expected)
		return unknownExecutable
	}
}

func (dt discoveryTest) disableLocalDiscovery(c *gc.C, s *discoverySuite) {
	service.PatchGOOS(s, "<another OS>")
	service.PatchPid1File(c, s, unknownExecutable)
}

func (dt discoveryTest) disableVersionDiscovery(s *discoverySuite) {
	service.PatchVersion(s, version.Binary{
		OS: version.Unknown,
	})
}

func (dt discoveryTest) setLocal(c *gc.C, s *discoverySuite) {
	service.PatchGOOS(s, dt.goos())
	service.PatchPid1File(c, s, dt.executable(c))
}

func (dt discoveryTest) setVersion(s *discoverySuite) version.Binary {
	vers := dt.version()
	service.PatchVersion(s, vers)
	return vers
}

func (dt discoveryTest) checkService(c *gc.C, svc service.Service, err error, name string, conf common.Conf) {
	if dt.expected == "" {
		c.Check(err, jc.Satisfies, errors.IsNotFound)
		return
	}

	// Check the success case.
	if !c.Check(err, jc.ErrorIsNil) {
		return
	}
	switch dt.expected {
	case service.InitSystemUpstart:
		if conf.InitDir == "" {
			conf.InitDir = "/etc/init"
		}
		c.Check(svc, gc.FitsTypeOf, &upstart.Service{})
	case service.InitSystemSystemd:
		c.Check(svc, gc.FitsTypeOf, &systemd.Service{})
	case service.InitSystemWindows:
		c.Check(svc, gc.FitsTypeOf, &windows.Service{})
	default:
		c.Errorf("unknown expected init system %q", dt.expected)
		return
	}
	if svc == nil {
		return
	}

	c.Check(svc.Name(), gc.Equals, name)
	c.Check(svc.Conf(), jc.DeepEquals, conf)
}

func (dt discoveryTest) checkInitSystem(c *gc.C, name string, ok bool) {
	if dt.expected == "" {
		if !c.Check(ok, jc.IsFalse) {
			c.Logf("found init system %q", name)
		}
	} else {
		c.Check(ok, jc.IsTrue)
		c.Check(name, gc.Equals, dt.expected)
	}
}

var discoveryTests = []discoveryTest{{
	os:       version.Windows,
	expected: service.InitSystemWindows,
}, {
	os:       version.Ubuntu,
	series:   "oneiric",
	expected: "",
}, {
	os:       version.Ubuntu,
	series:   "precise",
	expected: service.InitSystemUpstart,
}, {
	os:       version.Ubuntu,
	series:   "utopic",
	expected: service.InitSystemUpstart,
}, {
	os:       version.Ubuntu,
	series:   "vivid",
	expected: service.InitSystemSystemd,
}, {
	os:       version.CentOS,
	expected: "",
}, {
	os:       version.Unknown,
	expected: "",
}}

type discoverySuite struct {
	testing.IsolationSuite

	name string
	conf common.Conf
}

var _ = gc.Suite(&discoverySuite{})

func (s *discoverySuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.name = "a-service"
	s.conf = common.Conf{
		Desc:      "some service",
		ExecStart: "/path/to/some-command",
	}
}

func (s *discoverySuite) TestDiscoverServiceLocalHost(c *gc.C) {
	var localInitSystem string
	switch runtime.GOOS {
	case "windows":
		localInitSystem = service.InitSystemWindows
	case "linux":
		// TODO(ericsnow) Drop the vivid special-case once systemd is
		// turned on there.
		if version.Current.Series == "vivid" {
			return
		}
		localInitSystem, _ = service.VersionInitSystem(version.Current)
	}
	test := discoveryTest{
		os:       version.Current.OS,
		series:   version.Current.Series,
		expected: localInitSystem,
	}
	test.disableVersionDiscovery(s)

	svc, err := service.DiscoverService(s.name, s.conf)
	c.Assert(err, jc.ErrorIsNil)

	test.checkService(c, svc, err, s.name, s.conf)
}

func (s *discoverySuite) TestDiscoverServiceVersionLocal(c *gc.C) {
	for _, test := range discoveryTests {
		c.Logf("testing {%q, %q}...", test.os, test.series)

		test.setLocal(c, s)
		test.disableVersionDiscovery(s)

		svc, err := service.DiscoverService(s.name, s.conf)

		test.checkService(c, svc, err, s.name, s.conf)
	}
}

func (s *discoverySuite) TestDiscoverServiceVersionFallback(c *gc.C) {
	for _, test := range discoveryTests {
		c.Logf("testing {%q, %q}...", test.os, test.series)

		test.disableLocalDiscovery(c, s)
		test.setVersion(s)

		svc, err := service.DiscoverService(s.name, s.conf)

		test.checkService(c, svc, err, s.name, s.conf)
	}
}

func (s *discoverySuite) TestVersionInitSystem(c *gc.C) {
	for _, test := range discoveryTests {
		c.Logf("testing {%q, %q}...", test.os, test.series)

		vers := test.setVersion(s)

		initSystem, ok := service.VersionInitSystem(vers)

		test.checkInitSystem(c, initSystem, ok)
	}
}
