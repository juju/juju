// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"

	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/keyvalues"
	jujuos "github.com/juju/utils/os"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/worker/uniter/runner/context"
)

type EnvSuite struct {
	envtesting.IsolationSuite
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
		"JUJU_AGENT_SOCKET=path-to-jujuc.socket",
	}
}

func (s *EnvSuite) getContext() (ctx *context.HookContext, expectVars []string) {
	return context.NewModelHookContext(
			"some-context-id",
			"model-uuid-deadbeef",
			"some-model-name",
			"this-unit/123",
			"PURPLE",
			"proceed with care",
			"essential",
			"some-zone",
			[]string{"he.re:12345", "the.re:23456"},
			proxy.Settings{
				Http:    "some-http-proxy",
				Https:   "some-https-proxy",
				Ftp:     "some-ftp-proxy",
				NoProxy: "some-no-proxy",
			},
			names.NewMachineTag("42"),
		), []string{
			"JUJU_CONTEXT_ID=some-context-id",
			"JUJU_MODEL_UUID=model-uuid-deadbeef",
			"JUJU_MODEL_NAME=some-model-name",
			"JUJU_UNIT_NAME=this-unit/123",
			"JUJU_METER_STATUS=PURPLE",
			"JUJU_METER_INFO=proceed with care",
			"JUJU_SLA=essential",
			"JUJU_API_ADDRESSES=he.re:12345 the.re:23456",
			"JUJU_MACHINE_ID=42",
			"JUJU_AVAILABILITY_ZONE=some-zone",
			"http_proxy=some-http-proxy",
			"HTTP_PROXY=some-http-proxy",
			"https_proxy=some-https-proxy",
			"HTTPS_PROXY=some-https-proxy",
			"ftp_proxy=some-ftp-proxy",
			"FTP_PROXY=some-ftp-proxy",
			"no_proxy=some-no-proxy",
			"NO_PROXY=some-no-proxy",
		}
}

func (s *EnvSuite) setRelation(ctx *context.HookContext) (expectVars []string) {
	context.SetEnvironmentHookContextRelation(
		ctx, 22, "an-endpoint", "that-unit/456",
	)
	return []string{
		"JUJU_RELATION=an-endpoint",
		"JUJU_RELATION_ID=an-endpoint:22",
		"JUJU_REMOTE_UNIT=that-unit/456",
	}
}

func (s *EnvSuite) TestEnvSetsPath(c *gc.C) {
	paths := context.OSDependentEnvVars(MockEnvPaths{})
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
	os.Setenv("Path", "foo;bar")
	os.Setenv("PSModulePath", "ping;pong")
	windowsVars := []string{
		"Path=path-to-tools;foo;bar",
		"PSModulePath=ping;pong;" + filepath.FromSlash("path-to-charm/lib/Modules"),
	}

	ctx, contextVars := s.getContext()
	paths, pathsVars := s.getPaths()
	actualVars, err := ctx.HookVars(paths)
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, windowsVars)

	relationVars := s.setRelation(ctx)
	actualVars, err = ctx.HookVars(paths)
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, windowsVars, relationVars)
}

func (s *EnvSuite) TestEnvUbuntu(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Ubuntu })
	os.Setenv("PATH", "foo:bar")
	ubuntuVars := []string{
		"PATH=path-to-tools:foo:bar",
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
	}

	ctx, contextVars := s.getContext()
	paths, pathsVars := s.getPaths()
	actualVars, err := ctx.HookVars(paths)
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, ubuntuVars)

	relationVars := s.setRelation(ctx)
	actualVars, err = ctx.HookVars(paths)
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, ubuntuVars, relationVars)
}
