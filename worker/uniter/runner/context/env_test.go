// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"

	jujuos "github.com/juju/os"
	"github.com/juju/os/series"
	"github.com/juju/proxy"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/keyvalues"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter/runner/context"
)

type EnvSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&EnvSuite{})

func (s *EnvSuite) assertVars(c *gc.C, actual []string, expect ...[]string) {
	var fullExpect []string
	for _, someExpect := range expect {
		fullExpect = append(fullExpect, someExpect...)
	}
	sort.Strings(actual)
	sort.Strings(fullExpect)
	c.Assert(actual, jc.DeepEquals, fullExpect)
}

func (s *EnvSuite) getPaths() (paths context.Paths, expectVars []string) {
	// note: path-munging is os-dependent, not included in expectVars
	return MockEnvPaths{}, []string{
		"CHARM_DIR=path-to-charm",
		"JUJU_CHARM_DIR=path-to-charm",
		"JUJU_AGENT_SOCKET_ADDRESS=path-to-jujuc.socket",
		"JUJU_AGENT_SOCKET_NETWORK=unix",
	}
}

func (s *EnvSuite) getContext(newProxyOnly bool) (ctx *context.HookContext, expectVars []string) {
	var (
		legacyProxy proxy.Settings
		jujuProxy   proxy.Settings
		proxy       = proxy.Settings{
			Http:    "some-http-proxy",
			Https:   "some-https-proxy",
			Ftp:     "some-ftp-proxy",
			NoProxy: "some-no-proxy",
		}
	)
	if newProxyOnly {
		jujuProxy = proxy
	} else {
		legacyProxy = proxy
	}

	expected := []string{
		"JUJU_CONTEXT_ID=some-context-id",
		"JUJU_HOOK_NAME=some-hook-name",
		"JUJU_MODEL_UUID=model-uuid-deadbeef",
		"JUJU_PRINCIPAL_UNIT=this-unit/123",
		"JUJU_MODEL_NAME=some-model-name",
		"JUJU_UNIT_NAME=this-unit/123",
		"JUJU_METER_STATUS=PURPLE",
		"JUJU_METER_INFO=proceed with care",
		"JUJU_SLA=essential",
		"JUJU_API_ADDRESSES=he.re:12345 the.re:23456",
		"JUJU_MACHINE_ID=42",
		"JUJU_AVAILABILITY_ZONE=some-zone",
		"JUJU_VERSION=1.2.3",
		"CLOUD_API_VERSION=6.66",
	}
	if newProxyOnly {
		expected = append(expected,
			"JUJU_CHARM_HTTP_PROXY=some-http-proxy",
			"JUJU_CHARM_HTTPS_PROXY=some-https-proxy",
			"JUJU_CHARM_FTP_PROXY=some-ftp-proxy",
			"JUJU_CHARM_NO_PROXY=some-no-proxy",
		)
	} else {
		expected = append(expected,
			"http_proxy=some-http-proxy",
			"HTTP_PROXY=some-http-proxy",
			"https_proxy=some-https-proxy",
			"HTTPS_PROXY=some-https-proxy",
			"ftp_proxy=some-ftp-proxy",
			"FTP_PROXY=some-ftp-proxy",
			"no_proxy=some-no-proxy",
			"NO_PROXY=some-no-proxy",
			// JUJU_CHARM prefixed proxy values are always specified
			// even if empty.
			"JUJU_CHARM_HTTP_PROXY=",
			"JUJU_CHARM_HTTPS_PROXY=",
			"JUJU_CHARM_FTP_PROXY=",
			"JUJU_CHARM_NO_PROXY=",
		)
	}
	// It doesn't make sense that we set both legacy and juju proxy
	// settings, but by setting both to different values, we can see
	// what the environment values are.
	return context.NewModelHookContext(context.ModelHookContextParams{
		ID:                  "some-context-id",
		HookName:            "some-hook-name",
		ModelUUID:           "model-uuid-deadbeef",
		ModelName:           "some-model-name",
		UnitName:            "this-unit/123",
		MeterCode:           "PURPLE",
		MeterInfo:           "proceed with care",
		SLALevel:            "essential",
		AvailZone:           "some-zone",
		APIAddresses:        []string{"he.re:12345", "the.re:23456"},
		LegacyProxySettings: legacyProxy,
		JujuProxySettings:   jujuProxy,
		MachineTag:          names.NewMachineTag("42"),
	}), expected
}

func (s *EnvSuite) setRelation(ctx *context.HookContext) (expectVars []string) {
	context.SetEnvironmentHookContextRelation(ctx, 22, "an-endpoint", "that-unit/456", "that-app", "")
	return []string{
		"JUJU_RELATION=an-endpoint",
		"JUJU_RELATION_ID=an-endpoint:22",
		"JUJU_REMOTE_UNIT=that-unit/456",
		"JUJU_REMOTE_APP=that-app",
	}
}

func (s *EnvSuite) setDepartingRelation(ctx *context.HookContext) (expectVars []string) {
	context.SetEnvironmentHookContextRelation(ctx, 22, "an-endpoint", "that-unit/456", "that-app", "that-unit/456")
	return []string{
		"JUJU_RELATION=an-endpoint",
		"JUJU_RELATION_ID=an-endpoint:22",
		"JUJU_REMOTE_UNIT=that-unit/456",
		"JUJU_REMOTE_APP=that-app",
		"JUJU_DEPARTING_UNIT=that-unit/456",
	}
}

func (s *EnvSuite) TestEnvSetsPath(c *gc.C) {
	paths := context.OSDependentEnvVars(MockEnvPaths{}, os.Getenv)
	c.Assert(paths, gc.Not(gc.HasLen), 0)
	vars, err := keyvalues.Parse(paths, true)
	c.Assert(err, jc.ErrorIsNil)
	key := "PATH"
	if runtime.GOOS == "windows" {
		key = "Path"
	}
	c.Assert(vars[key], gc.Not(gc.Equals), "")
}

func (s *EnvSuite) TestEnvWindows(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Windows })
	s.PatchValue(&jujuversion.Current, version.MustParse("1.2.3"))
	windowsVars := []string{
		"Path=path-to-tools;foo;bar",
		"PSModulePath=ping;pong;" + filepath.FromSlash("path-to-charm/lib/Modules"),
	}

	ctx, contextVars := s.getContext(false)
	paths, pathsVars := s.getPaths()
	actualVars, err := ctx.HookVars(paths, false, func(k string) string {
		switch k {
		case "Path":
			return "foo;bar"
		case "PSModulePath":
			return "ping;pong"
		default:
			c.Errorf("unexpected get env call for %q", k)
		}
		return ""
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, windowsVars)

	relationVars := s.setRelation(ctx)
	actualVars, err = ctx.HookVars(paths, false, func(k string) string {
		switch k {
		case "Path":
			return "foo;bar"
		case "PSModulePath":
			return "ping;pong"
		default:
			c.Errorf("unexpected get env call for %q", k)
		}
		return ""
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, windowsVars, relationVars)
}

func (s *EnvSuite) TestEnvUbuntu(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Ubuntu })
	s.PatchValue(&jujuversion.Current, version.MustParse("1.2.3"))

	// As TERM is series-specific we need to make sure all supported versions are covered.
	for _, testSeries := range series.OSSupportedSeries(jujuos.Ubuntu) {
		s.PatchValue(&series.MustHostSeries, func() string { return testSeries })
		ubuntuVars := []string{
			"APT_LISTCHANGES_FRONTEND=none",
			"DEBIAN_FRONTEND=noninteractive",
			"LANG=C.UTF-8",
			"PATH=path-to-tools:foo:bar",
		}

		if testSeries == "trusty" {
			ubuntuVars = append(ubuntuVars, "TERM=screen-256color")
		} else {
			ubuntuVars = append(ubuntuVars, "TERM=tmux-256color")
		}

		ctx, contextVars := s.getContext(false)
		paths, pathsVars := s.getPaths()
		actualVars, err := ctx.HookVars(paths, false, func(k string) string {
			switch k {
			case "PATH":
				return "foo:bar"
			default:
				c.Errorf("unexpected get env call for %q", k)
			}
			return ""
		})
		c.Assert(err, jc.ErrorIsNil)
		s.assertVars(c, actualVars, contextVars, pathsVars, ubuntuVars)

		relationVars := s.setDepartingRelation(ctx)
		actualVars, err = ctx.HookVars(paths, false, func(k string) string {
			switch k {
			case "PATH":
				return "foo:bar"
			default:
				c.Errorf("unexpected get env call for %q", k)
			}
			return ""
		})
		c.Assert(err, jc.ErrorIsNil)
		s.assertVars(c, actualVars, contextVars, pathsVars, ubuntuVars, relationVars)
	}
}

func (s *EnvSuite) TestEnvCentos(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.CentOS })
	s.PatchValue(&jujuversion.Current, version.MustParse("1.2.3"))

	// As TERM is series-specific we need to make sure all supported versions are covered.
	for _, testSeries := range series.OSSupportedSeries(jujuos.CentOS) {
		s.PatchValue(&series.MustHostSeries, func() string { return testSeries })
		centosVars := []string{
			"LANG=C.UTF-8",
			"PATH=path-to-tools:foo:bar",
		}

		if testSeries == "centos7" {
			centosVars = append(centosVars, "TERM=screen-256color")
		} else {
			centosVars = append(centosVars, "TERM=tmux-256color")
		}

		ctx, contextVars := s.getContext(false)
		paths, pathsVars := s.getPaths()
		actualVars, err := ctx.HookVars(paths, false, func(k string) string {
			switch k {
			case "PATH":
				return "foo:bar"
			default:
				c.Errorf("unexpected get env call for %q", k)
			}
			return ""
		})
		c.Assert(err, jc.ErrorIsNil)
		s.assertVars(c, actualVars, contextVars, pathsVars, centosVars)

		relationVars := s.setRelation(ctx)
		actualVars, err = ctx.HookVars(paths, false, func(k string) string {
			switch k {
			case "PATH":
				return "foo:bar"
			default:
				c.Errorf("unexpected get env call for %q", k)
			}
			return ""
		})
		c.Assert(err, jc.ErrorIsNil)
		s.assertVars(c, actualVars, contextVars, pathsVars, centosVars, relationVars)
	}
}

func (s *EnvSuite) TestEnvOpenSUSE(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.OpenSUSE })
	s.PatchValue(&jujuversion.Current, version.MustParse("1.2.3"))

	// As TERM is series-specific we need to make sure all supported versions are covered.
	for _, testSeries := range series.OSSupportedSeries(jujuos.OpenSUSE) {
		s.PatchValue(&series.MustHostSeries, func() string { return testSeries })
		openSUSEVars := []string{
			"LANG=C.UTF-8",
			"PATH=path-to-tools:foo:bar",
		}

		if testSeries == "opensuseleap" {
			openSUSEVars = append(openSUSEVars, "TERM=screen-256color")
		} else {
			openSUSEVars = append(openSUSEVars, "TERM=tmux-256color")
		}

		ctx, contextVars := s.getContext(false)
		paths, pathsVars := s.getPaths()
		actualVars, err := ctx.HookVars(paths, false, func(k string) string {
			switch k {
			case "PATH":
				return "foo:bar"
			default:
				c.Errorf("unexpected get env call for %q", k)
			}
			return ""
		})
		c.Assert(err, jc.ErrorIsNil)
		s.assertVars(c, actualVars, contextVars, pathsVars, openSUSEVars)

		relationVars := s.setRelation(ctx)
		actualVars, err = ctx.HookVars(paths, false, func(k string) string {
			switch k {
			case "PATH":
				return "foo:bar"
			default:
				c.Errorf("unexpected get env call for %q", k)
			}
			return ""
		})
		c.Assert(err, jc.ErrorIsNil)
		s.assertVars(c, actualVars, contextVars, pathsVars, openSUSEVars, relationVars)
	}
}

func (s *EnvSuite) TestEnvGenericLinux(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.GenericLinux })
	s.PatchValue(&jujuversion.Current, version.MustParse("1.2.3"))

	genericLinuxVars := []string{
		"LANG=C.UTF-8",
		"PATH=path-to-tools:foo:bar",
		"TERM=screen",
	}

	ctx, contextVars := s.getContext(false)
	paths, pathsVars := s.getPaths()
	actualVars, err := ctx.HookVars(paths, false, func(k string) string {
		switch k {
		case "PATH":
			return "foo:bar"
		default:
			c.Errorf("unexpected get env call for %q", k)
		}
		return ""
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, genericLinuxVars)

	relationVars := s.setRelation(ctx)
	actualVars, err = ctx.HookVars(paths, false, func(k string) string {
		switch k {
		case "PATH":
			return "foo:bar"
		default:
			c.Errorf("unexpected get env call for %q", k)
		}
		return ""
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, genericLinuxVars, relationVars)
}
