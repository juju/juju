// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtools "github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
)

type toolsSuite struct {
	coretesting.BaseSuite
}

func TestToolsSuite(t *stdtesting.T) { tc.Run(t, &toolsSuite{}) }
func (s *toolsSuite) TestValidateUploadAllowedIncompatibleHostArch(c *tc.C) {
	// Host runs amd64, want ppc64 tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	devVersion := jujuversion.Current
	devVersion.Build = 1234
	s.PatchValue(&jujuversion.Current, devVersion)
	env := newEnviron("foo", useDefaultKeys, nil)
	arch := arch.PPC64EL
	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	err = bootstrap.ValidateUploadAllowed(env, &arch, nil, validator)
	c.Assert(err, tc.ErrorMatches, `cannot use agent built for "ppc64el" using a machine running on "amd64"`)
}

func (s *toolsSuite) TestValidateUploadAllowedIncompatibleTargetArch(c *tc.C) {
	// Host runs ppc64el, environment only supports amd64, arm64.
	s.PatchValue(&arch.HostArch, func() string { return arch.PPC64EL })
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	devVersion := jujuversion.Current
	devVersion.Build = 1234
	s.PatchValue(&jujuversion.Current, devVersion)
	env := newEnviron("foo", useDefaultKeys, nil)
	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	err = bootstrap.ValidateUploadAllowed(env, nil, nil, validator)
	c.Assert(err, tc.ErrorMatches, `model "foo" of type dummy does not support instances running on "ppc64el"`)
}

func (s *toolsSuite) TestValidateUploadAllowed(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	// Host runs arm64, environment supports arm64.
	arm64 := "arm64"
	ubuntuFocal := corebase.MustParseBaseFromString("ubuntu@20.04")
	s.PatchValue(&arch.HostArch, func() string { return arm64 })
	s.PatchValue(&os.HostOS, func() ostype.OSType { return ostype.Ubuntu })
	validator, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	err = bootstrap.ValidateUploadAllowed(env, &arm64, &ubuntuFocal, validator)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *toolsSuite) TestFindBootstrapTools(c *tc.C) {
	var called int
	var filter tools.Filter
	var findStreams []string
	s.PatchValue(bootstrap.FindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, _ environs.BootstrapEnviron, major, minor int, streams []string, f tools.Filter) (tools.List, error) {
		called++
		c.Check(major, tc.Equals, jujuversion.Current.Major)
		c.Check(minor, tc.Equals, jujuversion.Current.Minor)
		findStreams = streams
		filter = f
		return nil, nil
	})

	vers := semversion.MustParse("1.2.1")
	devVers := semversion.MustParse("1.2-beta1")
	arm64 := "arm64"

	type test struct {
		version *semversion.Number
		arch    *string
		base    *corebase.Base
		dev     bool
		filter  tools.Filter
		streams []string
	}
	tests := []test{{
		version: nil,
		arch:    nil,
		base:    nil,
		dev:     true,
		filter:  tools.Filter{},
	}, {
		version: &vers,
		arch:    nil,
		base:    nil,
		dev:     false,
		filter:  tools.Filter{Number: vers},
	}, {
		version: &vers,
		arch:    &arm64,
		base:    nil,
		filter:  tools.Filter{Arch: arm64, Number: vers},
	}, {
		version: &vers,
		arch:    &arm64,
		base:    nil,
		dev:     true,
		filter:  tools.Filter{Arch: arm64, Number: vers},
	}, {
		version: &devVers,
		arch:    &arm64,
		base:    nil,
		filter:  tools.Filter{Arch: arm64, Number: devVers},
	}, {
		version: &devVers,
		arch:    &arm64,
		base:    nil,
		filter:  tools.Filter{Arch: arm64, Number: devVers},
		streams: []string{"devel", "proposed", "released"},
	}}

	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	for i, test := range tests {
		c.Logf("test %d: %#v", i, test)
		extra := map[string]interface{}{"development": test.dev}
		if test.streams != nil {
			extra["agent-stream"] = test.streams[0]
		}
		env := newEnviron("foo", useDefaultKeys, extra)
		bootstrap.FindBootstrapTools(c.Context(), env, ss, test.version, test.arch, test.base)
		c.Assert(called, tc.Equals, i+1)
		c.Assert(filter, tc.Equals, test.filter)
		if test.streams != nil {
			c.Check(findStreams, tc.DeepEquals, test.streams)
		} else {
			if test.dev || jujuversion.IsDev(*test.version) {
				c.Check(findStreams, tc.DeepEquals, []string{"devel", "proposed", "released"})
			} else {
				c.Check(findStreams, tc.DeepEquals, []string{"released"})
			}
		}
	}
}

func (s *toolsSuite) TestFindAvailableToolsError(c *tc.C) {
	// TODO (stickupkid): Remove the patch and pass in a valid mock.
	s.PatchValue(bootstrap.FindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, _ environs.BootstrapEnviron, major, minor int, streams []string, f tools.Filter) (tools.List, error) {
		return nil, errors.New("splat")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	_, err := bootstrap.FindPackagedTools(c.Context(), env, ss, nil, nil, nil)
	c.Assert(err, tc.ErrorMatches, "splat")
}

func (s *toolsSuite) TestFindAvailableToolsNoUpload(c *tc.C) {
	s.PatchValue(bootstrap.FindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, _ environs.BootstrapEnviron, major, minor int, streams []string, f tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
		"agent-version": "1.17.1",
	})
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	_, err := bootstrap.FindPackagedTools(c.Context(), env, ss, nil, nil, nil)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *toolsSuite) TestFindAvailableToolsSpecificVersion(c *tc.C) {
	currentVersion := coretesting.CurrentVersion()
	currentVersion.Major = 2
	currentVersion.Minor = 3
	s.PatchValue(&jujuversion.Current, currentVersion.Number)
	var findToolsCalled int
	s.PatchValue(bootstrap.FindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, _ environs.BootstrapEnviron, major, minor int, streams []string, f tools.Filter) (tools.List, error) {
		c.Assert(f.Number.Major, tc.Equals, 10)
		c.Assert(f.Number.Minor, tc.Equals, 11)
		c.Assert(f.Number.Patch, tc.Equals, 12)
		c.Assert(streams, tc.DeepEquals, []string{"released"})
		findToolsCalled++
		return []*tools.Tools{
			{
				Version: currentVersion,
				URL:     "http://testing.invalid/tools.tar.gz",
			},
		}, nil
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	toolsVersion := semversion.MustParse("10.11.12")
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	result, err := bootstrap.FindPackagedTools(c.Context(), env, ss, &toolsVersion, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(findToolsCalled, tc.Equals, 1)
	c.Assert(result, tc.DeepEquals, tools.List{
		&tools.Tools{
			Version: currentVersion,
			URL:     "http://testing.invalid/tools.tar.gz",
		},
	})
}

func (s *toolsSuite) TestFindAvailableToolsCompleteNoValidate(c *tc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	allTools := tools.List{
		&tools.Tools{
			Version: semversion.Binary{
				Number:  jujuversion.Current,
				Release: "ubuntu",
				Arch:    arch.HostArch(),
			},
			URL: "http://testing.invalid/tools.tar.gz",
		},
	}

	s.PatchValue(bootstrap.FindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, _ environs.BootstrapEnviron, major, minor int, streams []string, f tools.Filter) (tools.List, error) {
		return allTools, nil
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	availableTools, err := bootstrap.FindPackagedTools(c.Context(), env, ss, nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(availableTools, tc.HasLen, len(allTools))
	c.Assert(env.constraintsValidatorCount, tc.Equals, 0)
}
