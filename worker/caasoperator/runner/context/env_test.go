// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"os"
	"sort"

	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/keyvalues"
	jujuos "github.com/juju/utils/os"
	"github.com/juju/utils/proxy"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/caasoperator/runner/context"
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
	return MockFakePaths{}, []string{
		"CHARM_DIR=path-to-charm",
		"JUJU_CHARM_DIR=path-to-charm",
		"JUJU_AGENT_SOCKET=path-to-hookcommand.socket",
	}
}

func (s *EnvSuite) getContext() (ctx *context.HookContext, expectVars []string) {
	return context.NewModelHookContext(
			"some-context-id",
			"model-uuid-deadbeef",
			"some-model-name",
			"this-app",
			[]string{"he.re:12345", "the.re:23456"},
			proxy.Settings{
				Http:    "some-http-proxy",
				Https:   "some-https-proxy",
				Ftp:     "some-ftp-proxy",
				NoProxy: "some-no-proxy",
			},
		), []string{
			"JUJU_CONTEXT_ID=some-context-id",
			"JUJU_MODEL_UUID=model-uuid-deadbeef",
			"JUJU_MODEL_NAME=some-model-name",
			"JUJU_APPLICATION_NAME=this-app",
			"JUJU_API_ADDRESSES=he.re:12345 the.re:23456",
			"JUJU_VERSION=1.2.3",
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
	paths := context.OSEnvVars(MockFakePaths{})
	c.Assert(paths, gc.Not(gc.HasLen), 0)
	vars, err := keyvalues.Parse(paths, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(vars["PATH"], gc.Not(gc.Equals), "")
}

func (s *EnvSuite) TestEnv(c *gc.C) {
	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Ubuntu })
	s.PatchValue(&jujuversion.Current, version.MustParse("1.2.3"))
	os.Setenv("PATH", "foo:bar")
	vars := []string{
		"PATH=path-to-tools:foo:bar",
	}

	ctx, contextVars := s.getContext()
	paths, pathsVars := s.getPaths()
	actualVars, err := ctx.HookVars(paths)
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, vars)

	relationVars := s.setRelation(ctx)
	actualVars, err = ctx.HookVars(paths)
	c.Assert(err, jc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, vars, relationVars)
}
