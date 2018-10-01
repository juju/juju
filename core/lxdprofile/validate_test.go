// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	charm "gopkg.in/juju/charm.v6"

	apicharms "github.com/juju/juju/api/charms"
	"github.com/juju/juju/core/lxdprofile"
)

type LXDProfileSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LXDProfileSuite{})

func (*LXDProfileSuite) TestValidateWithEmptyConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := &charm.LXDProfile{}

	lxdprofiler := NewMockLXDProfiler(ctrl)
	lxdprofiler.EXPECT().LXDProfile().Return(profile)

	err := lxdprofile.ValidateLXDProfile(lxdprofiler)
	c.Assert(err, gc.IsNil)
}

func (*LXDProfileSuite) TestValidateWithInvalidConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := &charm.LXDProfile{
		Config: map[string]string{
			"boot": "foobar",
		},
	}

	lxdprofiler := NewMockLXDProfiler(ctrl)
	lxdprofiler.EXPECT().LXDProfile().Return(profile)

	err := lxdprofile.ValidateLXDProfile(lxdprofiler)
	c.Assert(err, gc.NotNil)
}

func (*LXDProfileSuite) TestValidateCharmInfoWithEmptyConfig(c *gc.C) {
	info := &apicharms.CharmInfo{}

	err := lxdprofile.ValidateCharmInfoLXDProfile(info)
	c.Assert(err, gc.IsNil)
}

func (*LXDProfileSuite) TestValidateCharmInfoWithInvalidConfig(c *gc.C) {
	info := &apicharms.CharmInfo{
		LXDProfile: &charm.LXDProfile{
			Config: map[string]string{
				"boot": "foobar",
			},
		},
	}

	err := lxdprofile.ValidateCharmInfoLXDProfile(info)
	c.Assert(err, gc.IsNil)
}
