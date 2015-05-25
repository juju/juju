// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/juju/names"
	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter/runner"
)

type MergeEnvSuite struct {
	envtesting.IsolationSuite
}

var _ = gc.Suite(&MergeEnvSuite{})

func (e *MergeEnvSuite) TestMergeEnviron(c *gc.C) {
	// environment does not get fully cleared on Windows
	// when using testing.IsolationSuite
	origEnv := os.Environ()
	extraExpected := []string{
		"DUMMYVAR=foo",
		"DUMMYVAR2=bar",
		"NEWVAR=ImNew",
	}
	expectEnv := make([]string, 0, len(origEnv)+len(extraExpected))

	// os.Environ prepends some garbage on Windows that we need to strip out.
	// All the garbage starts and ends with = (for example "=C:=").
	for _, v := range origEnv {
		if !(strings.HasPrefix(v, "=") && strings.HasSuffix(v, "=")) {
			expectEnv = append(expectEnv, v)
		}
	}
	expectEnv = append(expectEnv, extraExpected...)
	os.Setenv("DUMMYVAR2", "ChangeMe")
	os.Setenv("DUMMYVAR", "foo")

	newEnv := make([]string, 0, len(expectEnv))
	for _, v := range runner.MergeWindowsEnvironment([]string{"dummyvar2=bar", "NEWVAR=ImNew"}, os.Environ()) {
		if !(strings.HasPrefix(v, "=") && strings.HasSuffix(v, "=")) {
			newEnv = append(newEnv, v)
		}
	}
	c.Assert(expectEnv, jc.SameContents, newEnv)
}

func (s *MergeEnvSuite) TestMergeEnvWin(c *gc.C) {
	initial := []string{"a=foo", "b=bar", "foo=val"}
	newValues := []string{"a=baz", "c=omg", "FOO=val2", "d=another"}

	created := runner.MergeWindowsEnvironment(newValues, initial)
	expected := []string{"a=baz", "b=bar", "c=omg", "foo=val2", "d=another"}
	c.Check(created, jc.SameContents, expected)
}

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

func (s *EnvSuite) getPaths() (paths runner.Paths, expectVars []string) {
	// note: path-munging is os-dependent, not included in expectVars
	return MockEnvPaths{}, []string{
		"CHARM_DIR=path-to-charm",
		"JUJU_CHARM_DIR=path-to-charm",
		"JUJU_AGENT_SOCKET=path-to-jujuc.socket",
	}
}

func (s *EnvSuite) getContext() (ctx *runner.HookContext, expectVars []string) {
	return runner.NewEnvironmentHookContext(
			"some-context-id",
			"env-uuid-deadbeef",
			"some-env-name",
			"this-unit/123",
			"PURPLE",
			"proceed with care",
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
			"JUJU_ENV_UUID=env-uuid-deadbeef",
			"JUJU_ENV_NAME=some-env-name",
			"JUJU_UNIT_NAME=this-unit/123",
			"JUJU_METER_STATUS=PURPLE",
			"JUJU_METER_INFO=proceed with care",
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

func (s *EnvSuite) setRelation(ctx *runner.HookContext) (expectVars []string) {
	runner.SetEnvironmentHookContextRelation(
		ctx, 22, "an-endpoint", "that-unit/456",
	)
	return []string{
		"JUJU_RELATION=an-endpoint",
		"JUJU_RELATION_ID=an-endpoint:22",
		"JUJU_REMOTE_UNIT=that-unit/456",
	}
}

func (s *EnvSuite) TestEnvWindows(c *gc.C) {
	s.PatchValue(&version.Current.OS, version.Windows)
	os.Setenv("Path", "foo;bar")
	os.Setenv("PSModulePath", "ping;pong")
	windowsVars := []string{
		"Path=path-to-tools;foo;bar",
		"PSModulePath=ping;pong;" + filepath.FromSlash("path-to-charm/lib/Modules"),
	}

	ctx, contextVars := s.getContext()
	paths, pathsVars := s.getPaths()
	actualVars := ctx.HookVars(paths)
	s.assertVars(c, actualVars, contextVars, pathsVars, windowsVars)

	relationVars := s.setRelation(ctx)
	actualVars = ctx.HookVars(paths)
	s.assertVars(c, actualVars, contextVars, pathsVars, windowsVars, relationVars)
}

func (s *EnvSuite) TestEnvUbuntu(c *gc.C) {
	s.PatchValue(&version.Current.OS, version.Ubuntu)
	os.Setenv("PATH", "foo:bar")
	ubuntuVars := []string{
		"PATH=path-to-tools:foo:bar",
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
	}

	ctx, contextVars := s.getContext()
	paths, pathsVars := s.getPaths()
	actualVars := ctx.HookVars(paths)
	s.assertVars(c, actualVars, contextVars, pathsVars, ubuntuVars)

	relationVars := s.setRelation(ctx)
	actualVars = ctx.HookVars(paths)
	s.assertVars(c, actualVars, contextVars, pathsVars, ubuntuVars, relationVars)
}
