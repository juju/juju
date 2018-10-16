// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&ResourcesSuite{})

type ResourcesSuite struct {
	BaseSuite
}

func (s *ResourcesSuite) TestAdoptResources(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	modelSelector := "juju-model-uuid==" + testing.ModelTag.Id()

	gomock.InOrder(
		s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: modelSelector}).Times(1).
			Return(&core.PodList{Items: []core.Pod{
				{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}},
			}}, nil),
		s.mockPods.EXPECT().Update(&core.Pod{ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"juju-controller-uuid": "uuid"}}}).Times(1).
			Return(nil, nil),

		s.mockPersistentVolumeClaims.EXPECT().List(v1.ListOptions{LabelSelector: modelSelector}).Times(1).
			Return(&core.PersistentVolumeClaimList{Items: []core.PersistentVolumeClaim{
				{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}},
			}}, nil),
		s.mockPersistentVolumeClaims.EXPECT().Update(&core.PersistentVolumeClaim{ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"juju-controller-uuid": "uuid"}}}).Times(1).
			Return(nil, nil),

		s.mockPersistentVolumes.EXPECT().List(v1.ListOptions{LabelSelector: modelSelector}).Times(1).
			Return(&core.PersistentVolumeList{Items: []core.PersistentVolume{
				{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}},
			}}, nil),
		s.mockPersistentVolumes.EXPECT().Update(&core.PersistentVolume{ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"juju-controller-uuid": "uuid"}}}).Times(1).
			Return(nil, nil),

		s.mockStatefulSets.EXPECT().List(v1.ListOptions{LabelSelector: modelSelector}).Times(1).
			Return(&apps.StatefulSetList{Items: []apps.StatefulSet{
				{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}},
			}}, nil),
		s.mockStatefulSets.EXPECT().Update(&apps.StatefulSet{ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"juju-controller-uuid": "uuid"}}}).Times(1).
			Return(nil, nil),

		s.mockDeployments.EXPECT().List(v1.ListOptions{LabelSelector: modelSelector}).Times(1).
			Return(&apps.DeploymentList{Items: []apps.Deployment{
				{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}},
			}}, nil),
		s.mockDeployments.EXPECT().Update(&apps.Deployment{ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"juju-controller-uuid": "uuid"}}}).Times(1).
			Return(nil, nil),
	)

	err := s.broker.AdoptResources(context.NewCloudCallContext(), "uuid", version.MustParse("1.2.3"))
	c.Assert(err, jc.ErrorIsNil)
}
