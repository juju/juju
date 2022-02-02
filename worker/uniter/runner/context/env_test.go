// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"path/filepath"
	"runtime"
	"sort"

	"github.com/juju/names/v4"
	osseries "github.com/juju/os/v2/series"
	"github.com/juju/proxy"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/keyvalues"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter/runner/context"
	runnertesting "github.com/juju/juju/worker/uniter/runner/testing"
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

func (s *EnvSuite) setSecret(ctx *context.HookContext) (expectVars []string) {
	url := secrets.NewSimpleURL("app/mariadb/password")
	context.SetEnvironmentHookContextSecret(ctx, url.ID())
	return []string{
		"JUJU_SECRET_URL=" + url.ID(),
	}
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

func (s *EnvSuite) setStorage(ctx *context.HookContext) (expectVars []string) {
	tag := names.NewStorageTag("data/0")
	context.SetEnvironmentHookContextStorage(ctx, &runnertesting.StorageContextAccessor{
		map[names.StorageTag]*runnertesting.ContextStorage{
			tag: {
				tag,
				storage.StorageKindBlock,
				"/dev/sdb",
			},
		},
	}, tag)
	return []string{
		"JUJU_STORAGE_ID=data/0",
		"JUJU_STORAGE_KIND=block",
		"JUJU_STORAGE_LOCATION=/dev/sdb",
	}
}

func (s *EnvSuite) TestEnvSetsPath(c *gc.C) {
	paths := context.OSDependentEnvVars(MockEnvPaths{}, context.NewHostEnvironmenter())
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

	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(k string) string {
			switch k {
			case "Path":
				return "foo;bar"
			case "PSModulePath":
				return "ping;pong"
			}
			return ""
		},
		func(k string) (string, bool) {
			switch k {
			case "Path":
				return "foo;bar", true
			case "PSModulePath":
				return "ping;pong", true
			}
			return "", false
		},
	)

	ctx, contextVars := s.getContext(false)
	paths, pathsVars := s.getPaths()
	actualVars, err := ctx.HookVars(paths, false, environmenter)
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, windowsVars)

	relationVars := s.setRelation(ctx)
	secretVars := s.setSecret(ctx)
	storageVars := s.setStorage(ctx)
	actualVars, err = ctx.HookVars(paths, false, environmenter)
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, windowsVars, relationVars, secretVars, storageVars)
}

func (s *EnvSuite) TestEnvUbuntu(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Ubuntu })
	s.PatchValue(&jujuversion.Current, version.MustParse("1.2.3"))

	// TERM is different for trusty.
	for _, testSeries := range []string{"trusty", "focal"} {
		s.PatchValue(&osseries.HostSeries, func() (string, error) { return testSeries, nil })
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

		environmenter := context.NewRemoteEnvironmenter(
			func() []string { return []string{} },
			func(k string) string {
				switch k {
				case "PATH":
					return "foo:bar"
				}
				return ""
			},
			func(k string) (string, bool) {
				switch k {
				case "PATH":
					return "foo:bar", true
				}
				return "", false
			},
		)

		ctx, contextVars := s.getContext(false)
		paths, pathsVars := s.getPaths()
		actualVars, err := ctx.HookVars(paths, false, environmenter)
		c.Assert(err, jc.ErrorIsNil)
		s.assertVars(c, actualVars, contextVars, pathsVars, ubuntuVars)

		relationVars := s.setDepartingRelation(ctx)
		secretVars := s.setSecret(ctx)
		storageVars := s.setStorage(ctx)
		actualVars, err = ctx.HookVars(paths, false, environmenter)
		c.Assert(err, jc.ErrorIsNil)
		s.assertVars(c, actualVars, contextVars, pathsVars, ubuntuVars, relationVars, secretVars, storageVars)
	}
}

func (s *EnvSuite) TestEnvCentos(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.CentOS })
	s.PatchValue(&jujuversion.Current, version.MustParse("1.2.3"))

	// TERM is different for centos7.
	for _, testSeries := range []string{"centos7", "centos8"} {
		s.PatchValue(&osseries.HostSeries, func() (string, error) { return testSeries, nil })
		centosVars := []string{
			"LANG=C.UTF-8",
			"PATH=path-to-tools:foo:bar",
		}

		if testSeries == "centos7" {
			centosVars = append(centosVars, "TERM=screen-256color")
		} else {
			centosVars = append(centosVars, "TERM=tmux-256color")
		}

		environmenter := context.NewRemoteEnvironmenter(
			func() []string { return []string{} },
			func(k string) string {
				switch k {
				case "PATH":
					return "foo:bar"
				}
				return ""
			},
			func(k string) (string, bool) {
				switch k {
				case "PATH":
					return "foo:bar", true
				}
				return "", false
			},
		)

		ctx, contextVars := s.getContext(false)
		paths, pathsVars := s.getPaths()
		actualVars, err := ctx.HookVars(paths, false, environmenter)
		c.Assert(err, jc.ErrorIsNil)
		s.assertVars(c, actualVars, contextVars, pathsVars, centosVars)

		relationVars := s.setRelation(ctx)
		secretVars := s.setSecret(ctx)
		actualVars, err = ctx.HookVars(paths, false, environmenter)
		c.Assert(err, jc.ErrorIsNil)
		s.assertVars(c, actualVars, contextVars, pathsVars, centosVars, relationVars, secretVars)
	}
}

func (s *EnvSuite) TestEnvOpenSUSE(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.OpenSUSE })
	s.PatchValue(&jujuversion.Current, version.MustParse("1.2.3"))

	// TERM is different for opensuseleap.
	for _, testSeries := range []string{"opensuseleap", "opensuse"} {
		s.PatchValue(&osseries.HostSeries, func() (string, error) { return testSeries, nil })
		openSUSEVars := []string{
			"LANG=C.UTF-8",
			"PATH=path-to-tools:foo:bar",
		}

		if testSeries == "opensuseleap" {
			openSUSEVars = append(openSUSEVars, "TERM=screen-256color")
		} else {
			openSUSEVars = append(openSUSEVars, "TERM=tmux-256color")
		}

		environmenter := context.NewRemoteEnvironmenter(
			func() []string { return []string{} },
			func(k string) string {
				switch k {
				case "PATH":
					return "foo:bar"
				}
				return ""
			},
			func(k string) (string, bool) {
				switch k {
				case "PATH":
					return "foo:bar", true
				}
				return "", false
			},
		)

		ctx, contextVars := s.getContext(false)
		paths, pathsVars := s.getPaths()
		actualVars, err := ctx.HookVars(paths, false, environmenter)
		c.Assert(err, jc.ErrorIsNil)
		s.assertVars(c, actualVars, contextVars, pathsVars, openSUSEVars)

		relationVars := s.setRelation(ctx)
		secretVars := s.setSecret(ctx)
		actualVars, err = ctx.HookVars(paths, false, environmenter)
		c.Assert(err, jc.ErrorIsNil)
		s.assertVars(c, actualVars, contextVars, pathsVars, openSUSEVars, relationVars, secretVars)
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

	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(k string) string {
			switch k {
			case "PATH":
				return "foo:bar"
			}
			return ""
		},
		func(k string) (string, bool) {
			switch k {
			case "PATH":
				return "foo:bar", true
			}
			return "", false
		},
	)

	ctx, contextVars := s.getContext(false)
	paths, pathsVars := s.getPaths()
	actualVars, err := ctx.HookVars(paths, false, environmenter)
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, genericLinuxVars)

	relationVars := s.setRelation(ctx)
	secretVars := s.setSecret(ctx)
	actualVars, err = ctx.HookVars(paths, false, environmenter)
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, genericLinuxVars, relationVars, secretVars)
}

func (s *EnvSuite) TestHostEnv(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.GenericLinux })
	s.PatchValue(&jujuversion.Current, version.MustParse("1.2.3"))

	genericLinuxVars := []string{
		"LANG=C.UTF-8",
		"PATH=path-to-tools:foo:bar",
		"TERM=screen",
	}

	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return []string{"KUBERNETES_SERVICE=test"} },
		func(k string) string {
			switch k {
			case "PATH":
				return "foo:bar"
			}
			return ""
		},
		func(k string) (string, bool) {
			switch k {
			case "KUBERNETES_SERVICE":
				return "test", true
			case "PATH":
				return "foo:bar", true
			}
			return "", false
		},
	)

	ctx, contextVars := s.getContext(false)
	paths, pathsVars := s.getPaths()
	actualVars, err := ctx.HookVars(paths, false, environmenter)
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, genericLinuxVars, []string{"KUBERNETES_SERVICE=test"})

	relationVars := s.setRelation(ctx)
	secretVars := s.setSecret(ctx)
	actualVars, err = ctx.HookVars(paths, false, environmenter)
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, genericLinuxVars, relationVars, secretVars, []string{"KUBERNETES_SERVICE=test"})
}

func (s *EnvSuite) TestContextDependentDoesNotIncludeUnSet(c *gc.C) {
	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(_ string) string { return "" },
		func(_ string) (string, bool) { return "", false },
	)

	c.Assert(len(context.ContextDependentEnvVars(environmenter)), gc.Equals, 0)
}

func (s *EnvSuite) TestContextDependentDoesIncludeAll(c *gc.C) {
	counter := 0
	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(_ string) string { return "" },
		func(_ string) (string, bool) {
			counter = counter + 1
			return "dummy-val", true
		},
	)
	c.Assert(len(context.ContextDependentEnvVars(environmenter)), gc.Equals, counter)
}

func (s *EnvSuite) TestContextDependentParitalInclude(c *gc.C) {
	counter := 0
	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(_ string) string { return "" },
		func(_ string) (string, bool) {
			// We are just going to include the first two env vars here to make
			// sure that both true and false statements work
			if counter < 2 {
				counter = counter + 1
				return "dummy-val", true
			}
			return "", false
		},
	)

	c.Assert(len(context.ContextDependentEnvVars(environmenter)), gc.Equals, counter)
	c.Assert(counter, gc.Equals, 2)
}

func (s *EnvSuite) TestContextDependentCallsAllVarKeys(c *gc.C) {
	queriedVars := map[string]bool{}
	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(_ string) string { return "" },
		func(k string) (string, bool) {
			for _, envKey := range context.ContextAllowedEnvVars {
				if envKey == k && queriedVars[k] == false {
					queriedVars[k] = true
					return "dummy-val", true
				} else if envKey == k && queriedVars[envKey] == true {
					c.Errorf("key %q has been queried more than once", k)
					return "", false
				}
			}
			c.Errorf("unexpected key %q has been queried for", k)
			return "", false
		},
	)

	rval := context.ContextDependentEnvVars(environmenter)
	c.Assert(len(rval), gc.Equals, len(queriedVars))
	c.Assert(len(queriedVars), gc.Equals, len(context.ContextAllowedEnvVars))
}
