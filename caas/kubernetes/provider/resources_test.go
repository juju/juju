// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/testing"
)

func TestResourcesSuite(t *stdtesting.T) { tc.Run(t, &ResourcesSuite{}) }

type ResourcesSuite struct {
	BaseSuite
}

func (s *ResourcesSuite) TestAdoptResources(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	modelSelector := "juju-model-uuid==" + testing.ModelTag.Id()

	gomock.InOrder(
		s.mockPods.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: modelSelector}).
			Return(&core.PodList{Items: []core.Pod{
				{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}},
			}}, nil),
		s.mockPods.EXPECT().Update(gomock.Any(), &core.Pod{ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"juju-controller-uuid": "uuid"}}}, v1.UpdateOptions{}).
			Return(nil, nil),

		s.mockPersistentVolumeClaims.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: modelSelector}).
			Return(&core.PersistentVolumeClaimList{Items: []core.PersistentVolumeClaim{
				{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}},
			}}, nil),
		s.mockPersistentVolumeClaims.EXPECT().Update(gomock.Any(), &core.PersistentVolumeClaim{ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"juju-controller-uuid": "uuid"}}}, v1.UpdateOptions{}).
			Return(nil, nil),

		s.mockPersistentVolumes.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: modelSelector}).
			Return(&core.PersistentVolumeList{Items: []core.PersistentVolume{
				{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}},
			}}, nil),
		s.mockPersistentVolumes.EXPECT().Update(gomock.Any(), &core.PersistentVolume{ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"juju-controller-uuid": "uuid"}}}, v1.UpdateOptions{}).
			Return(nil, nil),

		s.mockStatefulSets.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: modelSelector}).
			Return(&apps.StatefulSetList{Items: []apps.StatefulSet{
				{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}},
			}}, nil),
		s.mockStatefulSets.EXPECT().Update(gomock.Any(), &apps.StatefulSet{ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"juju-controller-uuid": "uuid"}}}, v1.UpdateOptions{}).
			Return(nil, nil),

		s.mockDeployments.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: modelSelector}).
			Return(&apps.DeploymentList{Items: []apps.Deployment{
				{ObjectMeta: v1.ObjectMeta{Labels: map[string]string{}}},
			}}, nil),
		s.mockDeployments.EXPECT().Update(gomock.Any(), &apps.Deployment{ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"juju-controller-uuid": "uuid"}}}, v1.UpdateOptions{}).
			Return(nil, nil),
	)

	err := s.broker.AdoptResources(c.Context(), "uuid", semversion.MustParse("1.2.3"))
	c.Assert(err, tc.ErrorIsNil)
}
