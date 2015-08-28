// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	devVersion := version.Current
	devVersion.Build = 1234
	s.PatchValue(&version.Current, devVersion)
	env := newEnviron("foo", useDefaultKeys, nil)
	arch := arch.PPC64EL
	err := bootstrap.ValidateUploadAllowed(env, &arch)
	c.Assert(err, gc.ErrorMatches, `cannot build tools for "ppc64el" using a machine running on "amd64"`)
}

func (s *toolsSuite) TestValidateUploadAllowedIncompatibleTargetArch(c *gc.C) {
	// Host runs ppc64el, environment only supports amd64, arm64.
	s.PatchValue(&arch.HostArch, func() string { return arch.PPC64EL })
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
	s.PatchValue(&arch.HostArch, func() string { return arm64 })
	err := bootstrap.ValidateUploadAllowed(env, &arm64)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *toolsSuite) TestFindBootstrapTools(c *gc.C) {
	var called int
	var filter tools.Filter
	var findStream string
	s.PatchValue(bootstrap.FindTools, func(_ environs.Environ, major, minor int, stream string, f tools.Filter) (tools.List, error) {
		called++
		c.Check(major, gc.Equals, version.Current.Major)
		c.Check(minor, gc.Equals, version.Current.Minor)
		findStream = stream
		filter = f
		return nil, nil
	})

	vers := version.MustParse("1.2.1")
	devVers := version.MustParse("1.2-beta1")
	arm64 := "arm64"

	type test struct {
		version *version.Number
		arch    *string
		dev     bool
		filter  tools.Filter
		stream  string
	}
	tests := []test{{
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
		version: &vers,
		arch:    &arm64,
		filter:  tools.Filter{Arch: arm64, Number: vers},
	}, {
		version: &vers,
		arch:    &arm64,
		dev:     true,
		filter:  tools.Filter{Arch: arm64, Number: vers},
	}, {
		version: &devVers,
		arch:    &arm64,
		filter:  tools.Filter{Arch: arm64, Number: devVers},
	}, {
		version: &devVers,
		arch:    &arm64,
		filter:  tools.Filter{Arch: arm64, Number: devVers},
		stream:  "devel",
	}}

	for i, test := range tests {
		c.Logf("test %d: %#v", i, test)
		extra := map[string]interface{}{"development": test.dev}
		if test.stream != "" {
			extra["agent-stream"] = test.stream
		}
		env := newEnviron("foo", useDefaultKeys, extra)
		bootstrap.FindBootstrapTools(env, test.version, test.arch)
		c.Assert(called, gc.Equals, i+1)
		c.Assert(filter, gc.Equals, test.filter)
		if test.stream != "" {
			c.Check(findStream, gc.Equals, test.stream)
		} else {
			if test.dev || test.version.IsDev() {
				c.Check(findStream, gc.Equals, "devel")
			} else {
				c.Check(findStream, gc.Equals, "released")
			}
		}
	}
}

func (s *toolsSuite) TestFindAvailableToolsError(c *gc.C) {
	s.PatchValue(bootstrap.FindTools, func(_ environs.Environ, major, minor int, stream string, f tools.Filter) (tools.List, error) {
		return nil, errors.New("splat")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	_, err := bootstrap.FindAvailableTools(env, nil, nil, false)
	c.Assert(err, gc.ErrorMatches, "splat")
}

func (s *toolsSuite) TestFindAvailableToolsNoUpload(c *gc.C) {
	s.PatchValue(bootstrap.FindTools, func(_ environs.Environ, major, minor int, stream string, f tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
		"agent-version": "1.17.1",
	})
	_, err := bootstrap.FindAvailableTools(env, nil, nil, false)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *toolsSuite) TestFindAvailableToolsForceUpload(c *gc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	var findToolsCalled int
	s.PatchValue(bootstrap.FindTools, func(_ environs.Environ, major, minor int, stream string, f tools.Filter) (tools.List, error) {
		findToolsCalled++
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	uploadedTools, err := bootstrap.FindAvailableTools(env, nil, nil, true)
	c.Assert(err, jc.ErrorIsNil)
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
	s.PatchValue(bootstrap.FindTools, func(_ environs.Environ, major, minor int, stream string, f tools.Filter) (tools.List, error) {
		findToolsCalled++
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	_, err := bootstrap.FindAvailableTools(env, nil, nil, true)
	c.Assert(err, gc.ErrorMatches, `environment "foo" of type dummy does not support instances running on "i386"`)
	c.Assert(findToolsCalled, gc.Equals, 0)
}

func (s *toolsSuite) TestFindAvailableToolsSpecificVersion(c *gc.C) {
	currentVersion := version.Current
	currentVersion.Major = 2
	currentVersion.Minor = 3
	s.PatchValue(&version.Current, currentVersion)
	var findToolsCalled int
	s.PatchValue(bootstrap.FindTools, func(_ environs.Environ, major, minor int, stream string, f tools.Filter) (tools.List, error) {
		c.Assert(f.Number.Major, gc.Equals, 10)
		c.Assert(f.Number.Minor, gc.Equals, 11)
		c.Assert(f.Number.Patch, gc.Equals, 12)
		c.Assert(stream, gc.Equals, "released")
		findToolsCalled++
		return []*tools.Tools{
			&tools.Tools{
				Version: currentVersion,
				URL:     "http://testing.invalid/tools.tar.gz",
			},
		}, nil
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	toolsVersion := version.MustParse("10.11.12")
	result, err := bootstrap.FindAvailableTools(env, &toolsVersion, nil, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(findToolsCalled, gc.Equals, 1)
	c.Assert(result, jc.DeepEquals, tools.List{
		&tools.Tools{
			Version: currentVersion,
			URL:     "http://testing.invalid/tools.tar.gz",
		},
	})
}

func (s *toolsSuite) TestFindAvailableToolsAutoUpload(c *gc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	trustyTools := &tools.Tools{
		Version: version.MustParseBinary("1.2.3-trusty-amd64"),
		URL:     "http://testing.invalid/tools.tar.gz",
	}
	s.PatchValue(bootstrap.FindTools, func(_ environs.Environ, major, minor int, stream string, f tools.Filter) (tools.List, error) {
		return tools.List{trustyTools}, nil
	})
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
		"agent-stream": "proposed"})
	availableTools, err := bootstrap.FindAvailableTools(env, nil, nil, false)
	c.Assert(err, jc.ErrorIsNil)
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
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	var allTools tools.List
	for _, series := range version.SupportedSeries() {
		binary := version.Binary{
			Number: version.Current.Number,
			Series: series,
			Arch:   arch.HostArch(),
			OS:     version.Current.OS,
		}
		allTools = append(allTools, &tools.Tools{
			Version: binary,
			URL:     "http://testing.invalid/tools.tar.gz",
		})
	}

	s.PatchValue(bootstrap.FindTools, func(_ environs.Environ, major, minor int, stream string, f tools.Filter) (tools.List, error) {
		return allTools, nil
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	availableTools, err := bootstrap.FindAvailableTools(env, nil, nil, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(availableTools, gc.HasLen, len(allTools))
	c.Assert(env.supportedArchitecturesCount, gc.Equals, 0)
}
