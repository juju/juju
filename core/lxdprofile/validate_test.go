// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/lxdprofile/mocks"
	"github.com/juju/juju/internal/errors"
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
