// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/errors"
	"github.com/juju/juju/charm"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/base"
)

type computedBaseSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&computedBaseSuite{})

func (s *computedBaseSuite) TestComputedBase(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{
			Name: "ubuntu",
			Channel: charm.Channel{
				Track: "18.04",
				Risk:  "stable",
			},
		}, {
			Name: "ubuntu",
			Channel: charm.Channel{
				Track: "20.04",
				Risk:  "stable",
			},
		}},
	}).AnyTimes()
	bases, err := ComputedBases(cm)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bases, jc.DeepEquals, []base.Base{
		base.MustParseBaseFromString("ubuntu@18.04"),
		base.MustParseBaseFromString("ubuntu@20.04"),
	})
}

func (s *computedBaseSuite) TestComputedBaseNilManifest(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Meta().Return(&charm.Meta{
		Name:        "a",
		Summary:     "b",
		Description: "c",
		Series:      []string{"bionic"},
	}).AnyTimes()
	cm.EXPECT().Manifest().Return(nil).AnyTimes()
	bases, err := ComputedBases(cm)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bases, jc.DeepEquals, []base.Base{
		base.MustParseBaseFromString("ubuntu@18.04"),
	})
}

func (s *computedBaseSuite) TestComputedBaseError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{
			Name: "ubuntu",
			Channel: charm.Channel{
				Track: "18.04",
				Risk:  "stable",
			},
		}, {
			Name: "ubuntu",
		}},
	}).AnyTimes()
	cm.EXPECT().Meta().Return(&charm.Meta{
		Name:        "a",
		Summary:     "b",
		Description: "c",
	}).AnyTimes()
	_, err := ComputedBases(cm)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *computedBaseSuite) TestBaseToUse(c *gc.C) {
	trusty := base.MustParseBaseFromString("ubuntu@16.04")
	jammy := base.MustParseBaseFromString("ubuntu@22.04")
	focal := base.MustParseBaseFromString("ubuntu@20.04")
	tests := []struct {
		series         base.Base
		supportedBases []base.Base
		baseToUse      base.Base
		err            string
	}{{
		series: base.Base{},
		err:    "charm does not define any bases",
	}, {
		series:    trusty,
		baseToUse: trusty,
	}, {
		series:         trusty,
		supportedBases: []base.Base{focal, trusty},
		baseToUse:      trusty,
	}, {
		series:         base.LatestLTSBase(),
		supportedBases: []base.Base{focal, base.LatestLTSBase(), trusty},
		baseToUse:      base.LatestLTSBase(),
	}, {
		series:         trusty,
		supportedBases: []base.Base{jammy, focal},
		err:            `base "ubuntu@16.04" not supported by charm.*`,
	}}
	for _, test := range tests {
		base, err := BaseForCharm(test.series, test.supportedBases)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
			continue
		}
		c.Check(err, jc.ErrorIsNil)
		c.Check(base.IsCompatible(test.baseToUse), jc.IsTrue)
	}
}

func (s *computedBaseSuite) TestBaseIsCompatibleWithCharm(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{
			Name: "ubuntu",
			Channel: charm.Channel{
				Track: "18.04",
				Risk:  "stable",
			},
		}, {
			Name: "ubuntu",
			Channel: charm.Channel{
				Track: "20.04",
				Risk:  "stable",
			},
		}},
	}).AnyTimes()
	cm.EXPECT().Meta().Return(&charm.Meta{
		Name: "my-charm",
	}).AnyTimes()

	focal := base.MustParseBaseFromString("ubuntu@20.04")
	jammy := base.MustParseBaseFromString("ubuntu@22.04")

	c.Assert(BaseIsCompatibleWithCharm(focal, cm), jc.ErrorIsNil)
	c.Assert(BaseIsCompatibleWithCharm(jammy, cm), jc.Satisfies, IsUnsupportedBaseError)
}

func (s *computedBaseSuite) TestOSIsCompatibleWithCharm(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{{
			Name: "ubuntu",
			Channel: charm.Channel{
				Track: "18.04",
				Risk:  "stable",
			},
		}, {
			Name: "ubuntu",
			Channel: charm.Channel{
				Track: "20.04",
				Risk:  "stable",
			},
		}},
	}).AnyTimes()
	cm.EXPECT().Meta().Return(&charm.Meta{
		Name: "my-charm",
	}).AnyTimes()

	c.Assert(OSIsCompatibleWithCharm("ubuntu", cm), jc.ErrorIsNil)
	c.Assert(OSIsCompatibleWithCharm("centos", cm), jc.ErrorIs, errors.NotSupported)
}
