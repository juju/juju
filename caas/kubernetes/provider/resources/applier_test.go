// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
	"github.com/juju/juju/caas/kubernetes/provider/resources/mocks"
	coretesting "github.com/juju/juju/internal/testing"
)

type applierSuite struct {
	coretesting.BaseSuite
}

func TestApplierSuite(t *stdtesting.T) {
	tc.Run(t, &applierSuite{})
}

func (s *applierSuite) TestRun(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r1 := mocks.NewMockResource(ctrl)
	r2 := mocks.NewMockResource(ctrl)

	applier := resources.NewApplierForTest()
	c.Assert(len(applier.Operations()), tc.DeepEquals, 0)
	applier.Apply(r1)
	applier.Delete(r2)

	gomock.InOrder(
		r1.EXPECT().Clone().Return(r1),
		r1.EXPECT().Get(gomock.Any(), gomock.Any()).Return(errors.NewNotFound(nil, "")),
		r1.EXPECT().Apply(gomock.Any(), gomock.Any()).Return(nil),

		r2.EXPECT().Clone().Return(r2),
		r2.EXPECT().Get(gomock.Any(), gomock.Any()).Return(errors.NewNotFound(nil, "")),
		r2.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil),
	)
	c.Assert(applier.Run(c.Context(), nil, false), tc.ErrorIsNil)
}

func (s *applierSuite) TestRunApplyFailedWithRollBackForNewResource(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r1 := mocks.NewMockResource(ctrl)
	r1Meta := &metav1.ObjectMeta{}
	r1.EXPECT().GetObjectMeta().AnyTimes().Return(r1Meta)

	applier := resources.NewApplierForTest()
	c.Assert(len(applier.Operations()), tc.DeepEquals, 0)
	applier.Apply(r1)

	existingR1 := mocks.NewMockResource(ctrl)
	existingR1Meta := &metav1.ObjectMeta{}
	existingR1.EXPECT().GetObjectMeta().AnyTimes().Return(existingR1Meta)

	gomock.InOrder(
		r1.EXPECT().Clone().Return(existingR1),
		existingR1.EXPECT().Get(gomock.Any(), gomock.Any()).Return(errors.NewNotFound(nil, "")),
		r1.EXPECT().Apply(gomock.Any(), gomock.Any()).Return(errors.New("something was wrong")),

		// rollback.
		r1.EXPECT().Clone().Return(r1),
		r1.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil),
		// delete the new resource was just created.
		r1.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil),
	)
	c.Assert(applier.Run(c.Context(), nil, false), tc.ErrorMatches, `something was wrong`)
}

func (s *applierSuite) TestRunApplyResourceVersionChanged(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r1 := mocks.NewMockResource(ctrl)
	r1Meta := &metav1.ObjectMeta{
		ResourceVersion: "1",
	}
	r1.EXPECT().ID().AnyTimes().Return(resources.ID{"A", "r1", "namespace"})
	r1.EXPECT().GetObjectMeta().AnyTimes().Return(r1Meta)

	applier := resources.NewApplierForTest()
	c.Assert(len(applier.Operations()), tc.DeepEquals, 0)
	applier.Apply(r1)

	existingR1 := mocks.NewMockResource(ctrl)
	existingR1Meta := &metav1.ObjectMeta{
		ResourceVersion: "2",
	}
	existingR1.EXPECT().GetObjectMeta().AnyTimes().Return(existingR1Meta)

	gomock.InOrder(
		r1.EXPECT().Clone().Return(existingR1),
		existingR1.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil),
	)
	c.Assert(applier.Run(c.Context(), nil, false), tc.ErrorMatches, `A r1: resource version conflict`)
}

func (s *applierSuite) TestRunApplyFailedWithRollBackForExistingResource(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r1 := mocks.NewMockResource(ctrl)
	r1Meta := &metav1.ObjectMeta{}
	r1.EXPECT().GetObjectMeta().AnyTimes().Return(r1Meta)

	applier := resources.NewApplierForTest()
	c.Assert(len(applier.Operations()), tc.DeepEquals, 0)
	applier.Apply(r1)

	existingR1 := mocks.NewMockResource(ctrl)
	existingR1Meta := &metav1.ObjectMeta{}
	existingR1.EXPECT().GetObjectMeta().AnyTimes().Return(existingR1Meta)

	gomock.InOrder(
		r1.EXPECT().Clone().Return(existingR1),
		existingR1.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil),
		r1.EXPECT().Apply(gomock.Any(), gomock.Any()).Return(errors.New("something was wrong")),

		// rollback.
		existingR1.EXPECT().Clone().Return(existingR1),
		existingR1.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil),
		// re-apply the old resource.
		existingR1.EXPECT().Apply(gomock.Any(), gomock.Any()).Return(nil),
	)
	c.Assert(applier.Run(c.Context(), nil, false), tc.ErrorMatches, `something was wrong`)
}

func (s *applierSuite) TestRunDeleteFailedWithRollBack(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r1 := mocks.NewMockResource(ctrl)
	r1Meta := &metav1.ObjectMeta{}
	r1.EXPECT().GetObjectMeta().AnyTimes().Return(r1Meta)

	applier := resources.NewApplierForTest()
	c.Assert(len(applier.Operations()), tc.DeepEquals, 0)
	applier.Delete(r1)

	existingR1 := mocks.NewMockResource(ctrl)
	existingR1Meta := &metav1.ObjectMeta{}
	existingR1.EXPECT().GetObjectMeta().AnyTimes().Return(existingR1Meta)

	gomock.InOrder(
		r1.EXPECT().Clone().Return(existingR1),
		existingR1.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil),
		r1.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(errors.New("something was wrong")),

		// rollback.
		existingR1.EXPECT().Clone().Return(existingR1),
		existingR1.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil),
		// re-apply the old resource.
		existingR1.EXPECT().Apply(gomock.Any(), gomock.Any()).Return(nil),
	)
	c.Assert(applier.Run(c.Context(), nil, false), tc.ErrorMatches, `something was wrong`)
}

func (s *applierSuite) TestApplySet(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r1 := mocks.NewMockResource(ctrl)
	r1.EXPECT().ID().AnyTimes().Return(resources.ID{"A", "r1", "namespace"})
	r1Meta := &metav1.ObjectMeta{}
	r1.EXPECT().GetObjectMeta().AnyTimes().Return(r1Meta)
	r1.EXPECT().Clone().AnyTimes().Return(r1)
	r2 := mocks.NewMockResource(ctrl)
	r2.EXPECT().ID().AnyTimes().Return(resources.ID{"B", "r2", "namespace"})
	r2Meta := &metav1.ObjectMeta{}
	r2.EXPECT().GetObjectMeta().AnyTimes().Return(r2Meta)
	r2.EXPECT().Clone().AnyTimes().Return(r2)
	r2Copy := mocks.NewMockResource(ctrl)
	r2Copy.EXPECT().ID().AnyTimes().Return(resources.ID{"B", "r2", "namespace"})
	r2CopyMeta := &metav1.ObjectMeta{}
	r2Copy.EXPECT().GetObjectMeta().AnyTimes().Return(r2CopyMeta)
	r2Copy.EXPECT().Clone().AnyTimes().Return(r2)
	r3 := mocks.NewMockResource(ctrl)
	r3.EXPECT().ID().AnyTimes().Return(resources.ID{"A", "r3", "namespace"})
	r3Meta := &metav1.ObjectMeta{}
	r3.EXPECT().GetObjectMeta().AnyTimes().Return(r3Meta)
	r3.EXPECT().Clone().AnyTimes().Return(r3)

	applier := resources.NewApplierForTest()
	c.Assert(len(applier.Operations()), tc.DeepEquals, 0)
	applier.ApplySet([]resources.Resource{r1, r2}, []resources.Resource{r2Copy, r3})

	gomock.InOrder(
		r1.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil),
		r1.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil),
		r2.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil),
		r2Copy.EXPECT().Apply(gomock.Any(), gomock.Any()).Return(nil),
		r3.EXPECT().Get(gomock.Any(), gomock.Any()).Return(errors.NotFoundf("missing aye")),
		r3.EXPECT().Apply(gomock.Any(), gomock.Any()).Return(nil),
	)
	c.Assert(applier.Run(c.Context(), nil, false), tc.ErrorIsNil)
}
