// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type computedSeriesSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&computedSeriesSuite{})

func (s *computedSeriesSuite) TestComputedSeriesLegacy(c *gc.C) {
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
	c.Assert(series, jc.DeepEquals, []string{"bionic", "kubernetes"})
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
	c.Assert(err, gc.ErrorMatches, `unknown series for version: "testme"`)
}
