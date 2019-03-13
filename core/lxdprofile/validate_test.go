// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/lxdprofile/mocks"
)

type LXDProfileSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LXDProfileSuite{})

func (*LXDProfileSuite) TestValidateWithNoProfiler(c *gc.C) {
	err := lxdprofile.ValidateLXDProfile(nil)
	c.Assert(err, gc.IsNil)
}

func (*LXDProfileSuite) TestValidateWithEmptyConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := mocks.NewMockLXDProfile(ctrl)
	profile.EXPECT().ValidateConfigDevices().Return(nil)

	lxdprofiler := mocks.NewMockLXDProfiler(ctrl)
	lxdprofiler.EXPECT().LXDProfile().Return(profile)

	err := lxdprofile.ValidateLXDProfile(lxdprofiler)
	c.Assert(err, gc.IsNil)
}

func (*LXDProfileSuite) TestValidateWithInvalidConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := mocks.NewMockLXDProfile(ctrl)
	profile.EXPECT().ValidateConfigDevices().Return(errors.New("bad"))

	lxdprofiler := mocks.NewMockLXDProfiler(ctrl)
	lxdprofiler.EXPECT().LXDProfile().Return(profile)

	err := lxdprofile.ValidateLXDProfile(lxdprofiler)
	c.Assert(err, gc.NotNil)
}

func (*LXDProfileSuite) TestNotEmpty(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := mocks.NewMockLXDProfile(ctrl)
	profile.EXPECT().Empty().Return(true)

	lxdprofiler := mocks.NewMockLXDProfiler(ctrl)
	lxdprofiler.EXPECT().LXDProfile().Return(profile)

	result := lxdprofile.NotEmpty(lxdprofiler)
	c.Assert(result, jc.IsFalse)
}

func (*LXDProfileSuite) TestNewLXDCharmProfilerEmpty(c *gc.C) {
	profiler := lxdprofile.NewLXDCharmProfiler(
		lxdprofile.Profile{})
	profile := profiler.LXDProfile()
	c.Assert(profile.Empty(), jc.IsTrue)
}

func (*LXDProfileSuite) TestNewLXDCharmProfilerNotEmpty(c *gc.C) {
	profiler := lxdprofile.NewLXDCharmProfiler(
		lxdprofile.Profile{
			Config: map[string]string{
				"hello": "testing",
			},
		})
	profile := profiler.LXDProfile()
	c.Assert(profile.Empty(), jc.IsFalse)
}

func (*LXDProfileSuite) TestValidateLXDProfileWithNewLXDCharmProfilerSuccess(c *gc.C) {
	profiler := lxdprofile.NewLXDCharmProfiler(
		lxdprofile.Profile{
			Config: map[string]string{
				"hello": "testing",
			},
		})
	c.Assert(lxdprofile.ValidateLXDProfile(profiler), jc.ErrorIsNil)
}

func (*LXDProfileSuite) TestValidateLXDProfileWithNewLXDCharmProfilerErrorDevice(c *gc.C) {
	profiler := lxdprofile.NewLXDCharmProfiler(
		lxdprofile.Profile{
			Devices: map[string]map[string]string{
				"bdisk": {
					"type":   "unix-disk",
					"source": "/dev/loop0",
				},
			},
		})
	c.Assert(lxdprofile.ValidateLXDProfile(profiler), gc.ErrorMatches, "invalid lxd-profile: contains device type \"unix-disk\"")
}

func (*LXDProfileSuite) TestValidateLXDProfileWithNewLXDCharmProfilerErrorConfig(c *gc.C) {
	profiler := lxdprofile.NewLXDCharmProfiler(
		lxdprofile.Profile{
			Config: map[string]string{
				"boot": "testing",
			},
		})
	c.Assert(lxdprofile.ValidateLXDProfile(profiler), gc.ErrorMatches, "invalid lxd-profile: contains config value \"boot\"")
}
