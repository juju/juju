// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corebase "github.com/juju/juju/core/base"
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
	c.Assert(bases, jc.DeepEquals, []corebase.Base{
		corebase.MustParseBaseFromString("ubuntu@18.04"),
		corebase.MustParseBaseFromString("ubuntu@20.04"),
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
	c.Assert(bases, jc.DeepEquals, []corebase.Base{
		corebase.MustParseBaseFromString("ubuntu@18.04"),
	})
}

func (s *computedBaseSuite) TestComputedBaseNilManifestKubernetes(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Meta().Return(&charm.Meta{
		Name:        "a",
		Summary:     "b",
		Description: "c",
		Series:      []string{"kubernetes"},
	}).AnyTimes()
	cm.EXPECT().Manifest().Return(nil).AnyTimes()
	bases, err := ComputedBases(cm)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bases, jc.DeepEquals, []corebase.Base{
		corebase.LegacyKubernetesBase(),
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
	c.Assert(errors.IsNotValid(err), jc.IsTrue)
}

func (s *computedBaseSuite) TestBaseToUse(c *gc.C) {
	trusty := corebase.MustParseBaseFromString("ubuntu@16.04")
	jammy := corebase.MustParseBaseFromString("ubuntu@22.04")
	focal := corebase.MustParseBaseFromString("ubuntu@20.04")
	tests := []struct {
		base           corebase.Base
		supportedBases []corebase.Base
		baseToUse      corebase.Base
		err            string
	}{{
		base: corebase.Base{},
		err:  "base not specified and charm does not define any",
	}, {
		base:      trusty,
		baseToUse: trusty,
	}, {
		base:           trusty,
		supportedBases: []corebase.Base{focal, trusty},
		baseToUse:      trusty,
	}, {
		base:           corebase.LatestLTSBase(),
		supportedBases: []corebase.Base{focal, corebase.LatestLTSBase(), trusty},
		baseToUse:      corebase.LatestLTSBase(),
	}, {
		base:           trusty,
		supportedBases: []corebase.Base{jammy, focal},
		err:            `base "ubuntu@16.04" not supported by charm.*`,
	}}
	for _, test := range tests {
		base, err := BaseForCharm(test.base, test.supportedBases)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
			continue
		}
		c.Check(err, jc.ErrorIsNil)
		c.Check(base.IsCompatible(test.baseToUse), jc.IsTrue)
	}
}

func (s *computedBaseSuite) TestIsMissingBaseError(c *gc.C) {
	c.Assert(IsMissingBaseError(errMissingBase), jc.IsTrue)
	c.Assert(IsMissingBaseError(errors.New("foo")), jc.IsFalse)
}
