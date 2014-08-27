// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/juju/arch"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

type toolsSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&toolsSuite{})

func (s *toolsSuite) TestValidateUploadAllowedIncompatibleHostArch(c *gc.C) {
	// Host runs amd64, want ppc64 tools.
	s.PatchValue(&arch.HostArch, func() string {
		return "amd64"
	})
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	devVersion := version.Current
	devVersion.Build = 1234
	s.PatchValue(&version.Current, devVersion)
	env := newEnviron("foo", useDefaultKeys, nil)
	arch := "ppc64el"
	err := bootstrap.ValidateUploadAllowed(env, &arch)
	c.Assert(err, gc.ErrorMatches, `cannot build tools for "ppc64el" using a machine running on "amd64"`)
}

func (s *toolsSuite) TestValidateUploadAllowedIncompatibleTargetArch(c *gc.C) {
	// Host runs ppc64el, environment only supports amd64, arm64.
	s.PatchValue(&arch.HostArch, func() string {
		return "ppc64el"
	})
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	devVersion := version.Current
	devVersion.Build = 1234
	s.PatchValue(&version.Current, devVersion)
	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.ValidateUploadAllowed(env, nil)
	c.Assert(err, gc.ErrorMatches, `environment "foo" of type dummy does not support instances running on "ppc64el"`)
}

func (s *toolsSuite) TestValidateUploadAllowed(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	// Host runs arm64, environment supports arm64.
	arm64 := "arm64"
	s.PatchValue(&arch.HostArch, func() string {
		return arm64
	})
	err := bootstrap.ValidateUploadAllowed(env, &arm64)
	c.Assert(err, gc.IsNil)
}

func (s *toolsSuite) TestFindBootstrapTools(c *gc.C) {
	var called int
	var filter tools.Filter
	s.PatchValue(bootstrap.FindTools, func(_ environs.ConfigGetter, major, minor int, f tools.Filter, retry bool) (tools.List, error) {
		called++
		c.Check(major, gc.Equals, version.Current.Major)
		c.Check(minor, gc.Equals, version.Current.Minor)
		c.Check(retry, jc.IsFalse)
		filter = f
		return nil, nil
	})

	vers := version.MustParse("1.2.1")
	arm64 := "arm64"

	type test struct {
		version *version.Number
		arch    *string
		dev     bool
		filter  tools.Filter
	}
	tests := []test{{
		version: nil,
		arch:    nil,
		dev:     false,
		filter:  tools.Filter{Released: true},
	}, {
		version: nil,
		arch:    nil,
		dev:     true,
		filter:  tools.Filter{},
	}, {
		version: &vers,
		arch:    nil,
		dev:     false,
		filter:  tools.Filter{Number: vers},
	}, {
		version: nil,
		arch:    &arm64,
		dev:     false,
		filter:  tools.Filter{Arch: arm64, Released: true},
	}, {
		version: &vers,
		arch:    &arm64,
		dev:     true,
		filter:  tools.Filter{Arch: arm64, Number: vers},
	}}

	for i, test := range tests {
		c.Logf("test %d: %#v", i, test)
		bootstrap.FindBootstrapTools(nil, test.version, test.arch, test.dev)
		c.Assert(called, gc.Equals, i+1)
		c.Assert(filter, gc.Equals, test.filter)
	}
}

func (s *toolsSuite) TestFindAvailableToolsError(c *gc.C) {
	s.PatchValue(bootstrap.FindTools, func(_ environs.ConfigGetter, major, minor int, f tools.Filter, retry bool) (tools.List, error) {
		return nil, errors.New("splat")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	_, err := bootstrap.FindAvailableTools(env, nil, false)
	c.Assert(err, gc.ErrorMatches, "splat")
}

func (s *toolsSuite) TestFindAvailableToolsNoUpload(c *gc.C) {
	s.PatchValue(bootstrap.FindTools, func(_ environs.ConfigGetter, major, minor int, f tools.Filter, retry bool) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
		"agent-version": "1.17.1",
	})
	_, err := bootstrap.FindAvailableTools(env, nil, false)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *toolsSuite) TestFindAvailableToolsForceUpload(c *gc.C) {
	s.PatchValue(&arch.HostArch, func() string {
		return "amd64"
	})
	var findToolsCalled int
	s.PatchValue(bootstrap.FindTools, func(_ environs.ConfigGetter, major, minor int, f tools.Filter, retry bool) (tools.List, error) {
		findToolsCalled++
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	uploadedTools, err := bootstrap.FindAvailableTools(env, nil, true)
	c.Assert(err, gc.IsNil)
	c.Assert(uploadedTools, gc.Not(gc.HasLen), 0)
	c.Assert(findToolsCalled, gc.Equals, 0)
	expectedVersion := version.Current.Number
	expectedVersion.Build++
	for _, tools := range uploadedTools {
		c.Assert(tools.Version.Number, gc.Equals, expectedVersion)
		c.Assert(tools.URL, gc.Equals, "")
	}
}

func (s *toolsSuite) TestFindAvailableToolsForceUploadInvalidArch(c *gc.C) {
	s.PatchValue(&arch.HostArch, func() string {
		return arch.I386
	})
	var findToolsCalled int
	s.PatchValue(bootstrap.FindTools, func(_ environs.ConfigGetter, major, minor int, f tools.Filter, retry bool) (tools.List, error) {
		findToolsCalled++
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	_, err := bootstrap.FindAvailableTools(env, nil, true)
	c.Assert(err, gc.ErrorMatches, `environment "foo" of type dummy does not support instances running on "i386"`)
	c.Assert(findToolsCalled, gc.Equals, 0)
}

func (s *toolsSuite) TestFindAvailableToolsAutoUpload(c *gc.C) {
	s.PatchValue(&arch.HostArch, func() string {
		return "amd64"
	})
	s.PatchValue(&version.Current.Arch, "amd64")
	trustyTools := &tools.Tools{
		Version: version.MustParseBinary("1.2.3-trusty-amd64"),
		URL:     "http://testing.invalid/tools.tar.gz",
	}
	s.PatchValue(bootstrap.FindTools, func(_ environs.ConfigGetter, major, minor int, f tools.Filter, retry bool) (tools.List, error) {
		return tools.List{trustyTools}, nil
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	availableTools, err := bootstrap.FindAvailableTools(env, nil, false)
	c.Assert(err, gc.IsNil)
	c.Assert(len(availableTools), jc.GreaterThan, 1)
	c.Assert(env.supportedArchitecturesCount, gc.Equals, 1)
	var trustyToolsFound int
	expectedVersion := version.Current.Number
	expectedVersion.Build++
	for _, tools := range availableTools {
		if tools == trustyTools {
			trustyToolsFound++
		} else {
			c.Assert(tools.Version.Number, gc.Equals, expectedVersion)
			c.Assert(tools.Version.Series, gc.Not(gc.Equals), "trusty")
			c.Assert(tools.URL, gc.Equals, "")
		}
	}
	c.Assert(trustyToolsFound, gc.Equals, 1)
}

func (s *toolsSuite) TestFindAvailableToolsCompleteNoValidate(c *gc.C) {
	s.PatchValue(&arch.HostArch, func() string {
		return "amd64"
	})
	s.PatchValue(&version.Current.Arch, "amd64")

	var allTools tools.List
	for _, series := range version.SupportedSeries() {
		binary := version.Current
		binary.Series = series
		allTools = append(allTools, &tools.Tools{
			Version: binary,
			URL:     "http://testing.invalid/tools.tar.gz",
		})
	}

	s.PatchValue(bootstrap.FindTools, func(_ environs.ConfigGetter, major, minor int, f tools.Filter, retry bool) (tools.List, error) {
		return allTools, nil
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	availableTools, err := bootstrap.FindAvailableTools(env, nil, false)
	c.Assert(err, gc.IsNil)
	c.Assert(availableTools, gc.HasLen, len(allTools))
	c.Assert(env.supportedArchitecturesCount, gc.Equals, 0)
}
