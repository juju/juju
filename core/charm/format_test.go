// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v11"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
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
	cm.EXPECT().Meta().Return(&charm.Meta{})
	cm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{Name: "ubuntu", Channel: charm.Channel{
				Track: "20.04",
				Risk:  "stable",
			}},
		},
	})

	c.Assert(Format(cm), gc.Equals, FormatV2)
}

func (s formatSuite) TestFormatV1EmptyManifest(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{})

	c.Assert(Format(cm), gc.Equals, FormatV1)
}

func (s formatSuite) TestFormatV1Series(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{}},
	})
	cm.EXPECT().Meta().Return(&charm.Meta{
		Series: []string{"kubernetes"},
	})

	c.Assert(Format(cm), gc.Equals, FormatV1)
}
