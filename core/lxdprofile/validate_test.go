// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/lxdprofile/mocks"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type LXDProfileSuite struct {
	testhelpers.IsolationSuite
}

func TestLXDProfileSuite(t *testing.T) {
	tc.Run(t, &LXDProfileSuite{})
}

func (*LXDProfileSuite) TestValidateWithNoProfiler(c *tc.C) {
	err := lxdprofile.ValidateLXDProfile(nil)
	c.Assert(err, tc.IsNil)
}

func (*LXDProfileSuite) TestValidateWithEmptyConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := mocks.NewMockLXDProfile(ctrl)
	profile.EXPECT().ValidateConfigDevices().Return(nil)

	lxdprofiler := mocks.NewMockLXDProfiler(ctrl)
	lxdprofiler.EXPECT().LXDProfile().Return(profile)

	err := lxdprofile.ValidateLXDProfile(lxdprofiler)
	c.Assert(err, tc.IsNil)
}

func (*LXDProfileSuite) TestValidateWithInvalidConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := mocks.NewMockLXDProfile(ctrl)
	profile.EXPECT().ValidateConfigDevices().Return(errors.New("bad"))

	lxdprofiler := mocks.NewMockLXDProfiler(ctrl)
	lxdprofiler.EXPECT().LXDProfile().Return(profile)

	err := lxdprofile.ValidateLXDProfile(lxdprofiler)
	c.Assert(err, tc.NotNil)
}

func (*LXDProfileSuite) TestNotEmpty(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := mocks.NewMockLXDProfile(ctrl)
	profile.EXPECT().Empty().Return(true)

	lxdprofiler := mocks.NewMockLXDProfiler(ctrl)
	lxdprofiler.EXPECT().LXDProfile().Return(profile)

	result := lxdprofile.NotEmpty(lxdprofiler)
	c.Assert(result, tc.IsFalse)
}
