// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v11"
	charmresource "github.com/juju/charm/v11/resource"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type computedSeriesSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&computedSeriesSuite{})

func (s *computedSeriesSuite) TestComputedSeriesNilManifest(c *gc.C) {
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
	series, err := ComputedSeries(cm)
	c.Assert(err, gc.IsNil)
	c.Assert(series, jc.DeepEquals, []string{"bionic"})
}

func (s *computedSeriesSuite) TestComputedSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{Name: "ubuntu", Channel: charm.Channel{
				Track: "18.04",
				Risk:  "stable",
			}}, {Name: "ubuntu", Channel: charm.Channel{
				Track: "20.04",
				Risk:  "stable",
			}},
		},
	}).AnyTimes()
	cm.EXPECT().Meta().Return(&charm.Meta{
		Name:        "a",
		Summary:     "b",
		Description: "c",
	}).AnyTimes()
	series, err := ComputedSeries(cm)
	c.Assert(err, gc.IsNil)
	c.Assert(series, jc.DeepEquals, []string{"bionic", "focal"})
}

func (s *computedSeriesSuite) TestComputedSeriesKubernetes(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{Name: "ubuntu", Channel: charm.Channel{
				Track: "18.04",
				Risk:  "stable",
			}},
		},
	}).AnyTimes()
	cm.EXPECT().Meta().Return(&charm.Meta{
		Name:        "a",
		Summary:     "b",
		Description: "c",
		Containers: map[string]charm.Container{
			"redis": {Resource: "redis-container-resource"},
		},
		Resources: map[string]charmresource.Meta{
			"redis-container-resource": {
				Name: "redis-container",
				Type: charmresource.TypeContainerImage,
			},
		},
	}).AnyTimes()
	series, err := ComputedSeries(cm)
	c.Assert(err, gc.IsNil)
	c.Assert(series, jc.DeepEquals, []string{"bionic"})
}

func (s *computedSeriesSuite) TestComputedSeriesError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Manifest().Return(&charm.Manifest{
		Bases: []charm.Base{
			{Name: "ubuntu", Channel: charm.Channel{
				Track: "18.04",
			}}, {Name: "ubuntu", Channel: charm.Channel{
				Track: "testme",
			}},
		},
	}).AnyTimes()
	cm.EXPECT().Meta().Return(&charm.Meta{
		Name:        "a",
		Summary:     "b",
		Description: "c",
	}).AnyTimes()
	_, err := ComputedSeries(cm)
	c.Assert(err, gc.ErrorMatches, `os "ubuntu" version "testme" not found`)
}

func (s *computedSeriesSuite) TestSeriesToUse(c *gc.C) {
	tests := []struct {
		series          string
		supportedSeries []string
		seriesToUse     string
		err             string
	}{{
		series: "",
		err:    "series not specified and charm does not define any",
	}, {
		series:      "trusty",
		seriesToUse: "trusty",
	}, {
		series:          "trusty",
		supportedSeries: []string{"precise", "trusty"},
		seriesToUse:     "trusty",
	}, {
		series:          "",
		supportedSeries: []string{"precise", "trusty"},
		seriesToUse:     "precise",
	}, {
		series:          "wily",
		supportedSeries: []string{"precise", "trusty"},
		err:             `series "wily" not supported by charm.*`,
	}}
	for _, test := range tests {
		series, err := SeriesForCharm(test.series, test.supportedSeries)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(series, jc.DeepEquals, test.seriesToUse)
	}
}

func (s *computedSeriesSuite) TestIsMissingSeriesError(c *gc.C) {
	c.Assert(IsMissingSeriesError(errMissingSeries), jc.IsTrue)
	c.Assert(IsMissingSeriesError(errors.New("foo")), jc.IsFalse)
}
