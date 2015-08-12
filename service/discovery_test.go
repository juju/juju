// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
	"github.com/juju/juju/version"
)

var maybeSystemd = service.InitSystemSystemd

func init() {
	if featureflag.Enabled(feature.LegacyUpstart) {
		maybeSystemd = service.InitSystemUpstart
	}
}

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

func (dt discoveryTest) log(c *gc.C) {
	c.Logf(" - testing {%q, %q}...", dt.os, dt.series)
}

func (dt discoveryTest) disableLocalDiscovery(c *gc.C, s *discoverySuite) {
	s.PatchLocalDiscoveryDisable()
}

func (dt discoveryTest) disableVersionDiscovery(s *discoverySuite) {
	dt.os = version.Unknown
	dt.setVersion(s)
}

func (dt discoveryTest) setLocal(c *gc.C, s *discoverySuite) {
	s.PatchLocalDiscoveryNoMatch(dt.expected)
}

func (dt discoveryTest) setVersion(s *discoverySuite) version.Binary {
	vers := dt.version()
	s.PatchVersion(vers)
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

func (dt discoveryTest) checkInitSystem(c *gc.C, name string, err error) {
	if dt.expected == "" {
		if !c.Check(err, jc.Satisfies, errors.IsNotFound) {
			c.Logf("found init system %q", name)
		}
	} else {
		c.Check(err, jc.ErrorIsNil)
		c.Check(name, gc.Equals, dt.expected)
	}
}

var discoveryTests = []discoveryTest{{
	os:       version.Windows,
	series:   "win2012",
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
	expected: maybeSystemd,
}, {
	os:       version.CentOS,
	series:   "centos7",
	expected: service.InitSystemSystemd,
}, {
	os:       version.Unknown,
	expected: "",
}}

type discoverySuite struct {
	service.BaseSuite

	name string
	conf common.Conf
}

var _ = gc.Suite(&discoverySuite{})

func (s *discoverySuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.name = "a-service"
	s.conf = common.Conf{
		Desc:      "some service",
		ExecStart: "/path/to/some-command",
	}
}

func (s *discoverySuite) unsetLegacyUpstart(c *gc.C) {
	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, "")
	c.Assert(err, jc.ErrorIsNil)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func (s *discoverySuite) setLegacyUpstart(c *gc.C) {
	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.LegacyUpstart)
	c.Assert(err, jc.ErrorIsNil)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func (s *discoverySuite) TestDiscoverServiceLocalHost(c *gc.C) {
	var localInitSystem string
	var err error
	switch runtime.GOOS {
	case "windows":
		localInitSystem = service.InitSystemWindows
	case "linux":
		localInitSystem, err = service.VersionInitSystem(version.Current.Series)
	}
	c.Assert(err, gc.IsNil)

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

func (s *discoverySuite) TestDiscoverServiceVersionFallback(c *gc.C) {
	for _, test := range discoveryTests {
		test.log(c)

		test.disableLocalDiscovery(c, s)
		test.setVersion(s)

		svc, err := service.DiscoverService(s.name, s.conf)

		test.checkService(c, svc, err, s.name, s.conf)
	}
}

func (s *discoverySuite) TestVersionInitSystem(c *gc.C) {
	for _, test := range discoveryTests {
		test.log(c)
		initSystem, err := service.VersionInitSystem(test.series)
		test.checkInitSystem(c, initSystem, err)
	}
}

func (s *discoverySuite) TestVersionInitSystemLegacyUpstart(c *gc.C) {
	s.setLegacyUpstart(c)
	test := discoveryTest{
		os:       version.Ubuntu,
		series:   "vivid",
		expected: service.InitSystemUpstart,
	}
	vers := test.setVersion(s)

	initSystem, err := service.VersionInitSystem(vers.Series)

	test.checkInitSystem(c, initSystem, err)
}

func (s *discoverySuite) TestVersionInitSystemNoLegacyUpstart(c *gc.C) {
	s.unsetLegacyUpstart(c)
	test := discoveryTest{
		os:       version.Ubuntu,
		series:   "vivid",
		expected: service.InitSystemSystemd,
	}
	vers := test.setVersion(s)

	initSystem, err := service.VersionInitSystem(vers.Series)

	test.checkInitSystem(c, initSystem, err)
}

func (s *discoverySuite) TestDiscoverLocalInitSystemMatchFirst(c *gc.C) {
	s.PatchLocalDiscovery(
		service.NewDiscoveryCheck("initA", true, nil),
		service.NewDiscoveryCheck("initB", false, nil),
	)

	name, err := service.DiscoverLocalInitSystem()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(name, gc.Equals, "initA")
}

func (s *discoverySuite) TestDiscoverLocalInitSystemErrorFirst(c *gc.C) {
	failure := errors.New("<failed>")
	s.PatchLocalDiscovery(
		service.NewDiscoveryCheck("initA", false, failure),
		service.NewDiscoveryCheck("initB", true, nil),
	)

	name, err := service.DiscoverLocalInitSystem()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(name, gc.Equals, "initB")
}

func (s *discoverySuite) TestDiscoverLocalInitSystemMatchFirstError(c *gc.C) {
	failure := errors.New("<failed>")
	s.PatchLocalDiscovery(
		service.NewDiscoveryCheck("initA", true, failure),
		service.NewDiscoveryCheck("initB", false, nil),
	)

	name, err := service.DiscoverLocalInitSystem()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(name, gc.Equals, "initA")
}

func (s *discoverySuite) TestDiscoverLocalInitSystemMatchSecond(c *gc.C) {
	s.PatchLocalDiscovery(
		service.NewDiscoveryCheck("initA", false, nil),
		service.NewDiscoveryCheck("initB", true, nil),
	)

	name, err := service.DiscoverLocalInitSystem()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(name, gc.Equals, "initB")
}

func (s *discoverySuite) TestDiscoverLocalInitSystemMatchNone(c *gc.C) {
	s.PatchLocalDiscovery(
		service.NewDiscoveryCheck("initA", false, nil),
		service.NewDiscoveryCheck("initB", false, nil),
	)

	_, err := service.DiscoverLocalInitSystem()

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *discoverySuite) TestDiscoverLocalInitSystemErrorMixed(c *gc.C) {
	failure := errors.New("<failed>")
	s.PatchLocalDiscovery(
		service.NewDiscoveryCheck("initA", false, failure),
		service.NewDiscoveryCheck("initB", false, nil),
	)

	_, err := service.DiscoverLocalInitSystem()

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *discoverySuite) TestDiscoverLocalInitSystemErrorAll(c *gc.C) {
	failureA := errors.New("<failed A>")
	failureB := errors.New("<failed B>")
	s.PatchLocalDiscovery(
		service.NewDiscoveryCheck("initA", false, failureA),
		service.NewDiscoveryCheck("initB", false, failureB),
	)

	_, err := service.DiscoverLocalInitSystem()

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *discoverySuite) TestDiscoverInitSystemScriptBash(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("not supported on windows")
	}

	script, filename := s.newDiscoverInitSystemScript(c)
	script += filename
	response, err := exec.RunCommands(exec.RunParams{
		Commands: script,
	})
	c.Assert(err, jc.ErrorIsNil)

	initSystem, err := service.DiscoverInitSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(response.Code, gc.Equals, 0)
	c.Check(string(response.Stdout), gc.Equals, initSystem)
	c.Check(string(response.Stderr), gc.Equals, "")
}

func (s *discoverySuite) TestDiscoverInitSystemScriptPosix(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("not supported on windows")
	}

	script, filename := s.newDiscoverInitSystemScript(c)
	script += "sh " + filename
	response, err := exec.RunCommands(exec.RunParams{
		Commands: script,
	})
	c.Assert(err, jc.ErrorIsNil)

	initSystem, err := service.DiscoverInitSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(response.Code, gc.Equals, 0)
	c.Check(string(response.Stdout), gc.Equals, initSystem)
	c.Check(string(response.Stderr), gc.Equals, "")
}

func (s *discoverySuite) writeScript(c *gc.C, name, script string) (string, string) {
	filename := filepath.Join(c.MkDir(), name)
	commands := []string{
		fmt.Sprintf(`
cat > %s << 'EOF'
%s
EOF`[1:], filename, script),
		"chmod 0755 " + filename,
	}
	writeScript := strings.Join(commands, "\n") + "\n"
	return writeScript, filename
}

func (s *discoverySuite) newDiscoverInitSystemScript(c *gc.C) (string, string) {
	script := service.DiscoverInitSystemScript()
	return s.writeScript(c, "discover_init_system.sh", script)
}

func (s *discoverySuite) TestNewShellSelectCommandBash(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("not supported on windows")
	}

	discoveryScript := service.DiscoverInitSystemScript()
	handler := func(initSystem string) (string, bool) {
		return "echo -n " + initSystem, true
	}
	script := "init_system=$(" + discoveryScript + ")\n"
	// The script will fail with exit 1 if it cannot match in init system.
	script += service.NewShellSelectCommand("init_system", "exit 1", handler)
	response, err := exec.RunCommands(exec.RunParams{
		Commands: script,
	})
	c.Assert(err, jc.ErrorIsNil)

	initSystem, err := service.DiscoverInitSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(response.Code, gc.Equals, 0)
	c.Check(string(response.Stdout), gc.Equals, initSystem)
	c.Check(string(response.Stderr), gc.Equals, "")
}

func (s *discoverySuite) TestNewShellSelectCommandPosix(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("not supported on windows")
	}

	discoveryScript := service.DiscoverInitSystemScript()
	handler := func(initSystem string) (string, bool) {
		return "echo -n " + initSystem, true
	}
	script := "init_system=$(" + discoveryScript + ")\n"
	// The script will fail with exit 1 if it cannot match in init system.
	script += service.NewShellSelectCommand("init_system", "exit 1", handler)
	commands, filename := s.writeScript(c, "test_shell_select.sh", script)
	commands += "sh " + filename
	response, err := exec.RunCommands(exec.RunParams{
		Commands: script,
	})
	c.Assert(err, jc.ErrorIsNil)

	initSystem, err := service.DiscoverInitSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(response.Code, gc.Equals, 0)
	c.Check(string(response.Stdout), gc.Equals, initSystem)
	c.Check(string(response.Stderr), gc.Equals, "")
}
