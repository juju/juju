// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtools "github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
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
	devVersion := jujuversion.Current
	devVersion.Build = 1234
	s.PatchValue(&jujuversion.Current, devVersion)
	env := newEnviron("foo", useDefaultKeys, nil)
	arch := arch.PPC64EL
	validator, err := env.ConstraintsValidator(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, jc.ErrorIsNil)
	err = bootstrap.ValidateUploadAllowed(env, &arch, nil, validator)
	c.Assert(err, gc.ErrorMatches, `cannot use agent built for "ppc64el" using a machine running on "amd64"`)
}

func (s *toolsSuite) TestValidateUploadAllowedIncompatibleTargetArch(c *gc.C) {
	// Host runs ppc64el, environment only supports amd64, arm64.
	s.PatchValue(&arch.HostArch, func() string { return arch.PPC64EL })
	// Force a dev version by having a non zero build number.
	// This is because we have not uploaded any tools and auto
	// upload is only enabled for dev versions.
	devVersion := jujuversion.Current
	devVersion.Build = 1234
	s.PatchValue(&jujuversion.Current, devVersion)
	env := newEnviron("foo", useDefaultKeys, nil)
	validator, err := env.ConstraintsValidator(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, jc.ErrorIsNil)
	err = bootstrap.ValidateUploadAllowed(env, nil, nil, validator)
	c.Assert(err, gc.ErrorMatches, `model "foo" of type dummy does not support instances running on "ppc64el"`)
}

func (s *toolsSuite) TestValidateUploadAllowed(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	// Host runs arm64, environment supports arm64.
	arm64 := "arm64"
	ubuntuFocal := corebase.MustParseBaseFromString("ubuntu@20.04")
	s.PatchValue(&arch.HostArch, func() string { return arm64 })
	s.PatchValue(&os.HostOS, func() ostype.OSType { return ostype.Ubuntu })
	validator, err := env.ConstraintsValidator(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Assert(err, jc.ErrorIsNil)
	err = bootstrap.ValidateUploadAllowed(env, &arm64, &ubuntuFocal, validator)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *toolsSuite) TestFindBootstrapTools(c *gc.C) {
	var called int
	var filter tools.Filter
	var findStreams []string
	s.PatchValue(bootstrap.FindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, _ environs.BootstrapEnviron, major, minor int, streams []string, f tools.Filter) (tools.List, error) {
		called++
		c.Check(major, gc.Equals, jujuversion.Current.Major)
		c.Check(minor, gc.Equals, jujuversion.Current.Minor)
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
		bootstrap.FindBootstrapTools(context.Background(), env, ss, test.version, test.arch, test.base)
		c.Assert(called, gc.Equals, i+1)
		c.Assert(filter, gc.Equals, test.filter)
		if test.streams != nil {
			c.Check(findStreams, gc.DeepEquals, test.streams)
		} else {
			if test.dev || jujuversion.IsDev(*test.version) {
				c.Check(findStreams, gc.DeepEquals, []string{"devel", "proposed", "released"})
			} else {
				c.Check(findStreams, gc.DeepEquals, []string{"released"})
			}
		}
	}
}

func (s *toolsSuite) TestFindAvailableToolsError(c *gc.C) {
	// TODO (stickupkid): Remove the patch and pass in a valid mock.
	s.PatchValue(bootstrap.FindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, _ environs.BootstrapEnviron, major, minor int, streams []string, f tools.Filter) (tools.List, error) {
		return nil, errors.New("splat")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	_, err := bootstrap.FindPackagedTools(context.Background(), env, ss, nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, "splat")
}

func (s *toolsSuite) TestFindAvailableToolsNoUpload(c *gc.C) {
	s.PatchValue(bootstrap.FindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, _ environs.BootstrapEnviron, major, minor int, streams []string, f tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
		"agent-version": "1.17.1",
	})
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	_, err := bootstrap.FindPackagedTools(context.Background(), env, ss, nil, nil, nil)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *toolsSuite) TestFindAvailableToolsSpecificVersion(c *gc.C) {
	currentVersion := coretesting.CurrentVersion()
	currentVersion.Major = 2
	currentVersion.Minor = 3
	s.PatchValue(&jujuversion.Current, currentVersion.Number)
	var findToolsCalled int
	s.PatchValue(bootstrap.FindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, _ environs.BootstrapEnviron, major, minor int, streams []string, f tools.Filter) (tools.List, error) {
		c.Assert(f.Number.Major, gc.Equals, 10)
		c.Assert(f.Number.Minor, gc.Equals, 11)
		c.Assert(f.Number.Patch, gc.Equals, 12)
		c.Assert(streams, gc.DeepEquals, []string{"released"})
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
	result, err := bootstrap.FindPackagedTools(context.Background(), env, ss, &toolsVersion, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(findToolsCalled, gc.Equals, 1)
	c.Assert(result, jc.DeepEquals, tools.List{
		&tools.Tools{
			Version: currentVersion,
			URL:     "http://testing.invalid/tools.tar.gz",
		},
	})
}

func (s *toolsSuite) TestFindAvailableToolsCompleteNoValidate(c *gc.C) {
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
	availableTools, err := bootstrap.FindPackagedTools(context.Background(), env, ss, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(availableTools, gc.HasLen, len(allTools))
	c.Assert(env.constraintsValidatorCount, gc.Equals, 0)
}
