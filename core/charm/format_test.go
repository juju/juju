// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type formatSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&formatSuite{})

func (s formatSuite) TestFormatV2(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{Name: "ubuntu", Channel: charm.Channel{
				Track: "20.04",
				Risk:  "stable",
			}},
		},
	}).AnyTimes()
	format := Format(cm)
	c.Assert(format, gc.Equals, FormatV2)
}

func (s formatSuite) TestFormatV1(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{}).AnyTimes()
	format := Format(cm)
	c.Assert(format, gc.Equals, FormatV1)
}
