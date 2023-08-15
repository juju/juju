// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"github.com/juju/charm/v11"
	charmresource "github.com/juju/charm/v11/resource"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type kubernetesSuite struct {
	testing.CleanupSuite
}

var _ = gc.Suite(&kubernetesSuite{})

func (s *kubernetesSuite) TestMetadataV1NoKubernetes(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Meta().Return(&charm.Meta{Series: []string{"bionic"}}).MinTimes(2)
	cm.EXPECT().Manifest().Return(nil).AnyTimes()

	c.Assert(IsKubernetes(cm), jc.IsFalse)
}

func (s *kubernetesSuite) TestMetadataV1Kubernetes(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Meta().Return(&charm.Meta{Series: []string{"kubernetes"}}).MinTimes(2)
	cm.EXPECT().Manifest().Return(nil).AnyTimes()

	c.Assert(IsKubernetes(cm), jc.IsTrue)
}

func (s *kubernetesSuite) TestMetadataV2NoKubernetes(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Meta().Return(&charm.Meta{}).AnyTimes()
	cm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{
		{
			Name: "ubuntu",
			Channel: charm.Channel{
				Risk:  "stable",
				Track: "20.04",
			},
		},
	}}).AnyTimes()

	c.Assert(IsKubernetes(cm), jc.IsFalse)
}

func (s *kubernetesSuite) TestMetadataV2Kubernetes(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cm := NewMockCharmMeta(ctrl)
	cm.EXPECT().Meta().Return(&charm.Meta{
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
	cm.EXPECT().Manifest().Return(&charm.Manifest{Bases: []charm.Base{
		{
			Name: "ubuntu",
			Channel: charm.Channel{
				Risk:  "stable",
				Track: "20.04",
			},
		},
	}}).AnyTimes()

	c.Assert(IsKubernetes(cm), jc.IsTrue)
}
