// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/kubernetes/provider/resources"
	"github.com/juju/juju/caas/kubernetes/provider/resources/mocks"
	coretesting "github.com/juju/juju/testing"
)

type applierSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&applierSuite{})

func (s *applierSuite) TestRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r1 := mocks.NewMockResource(ctrl)
	r2 := mocks.NewMockResource(ctrl)

	applier := resources.NewApplierForTest()
	c.Assert(len(applier.Operations()), gc.DeepEquals, 0)
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
	c.Assert(applier.Run(context.TODO(), nil, false), jc.ErrorIsNil)
}

func (s *applierSuite) TestRunApplyFailedWithRollBackForNewResource(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r1 := mocks.NewMockResource(ctrl)

	applier := resources.NewApplierForTest()
	c.Assert(len(applier.Operations()), gc.DeepEquals, 0)
	applier.Apply(r1)

	existingR1 := mocks.NewMockResource(ctrl)

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
	c.Assert(applier.Run(context.TODO(), nil, false), gc.ErrorMatches, `something was wrong`)
}

func (s *applierSuite) TestRunApplyFailedWithRollBackForExistingResource(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r1 := mocks.NewMockResource(ctrl)

	applier := resources.NewApplierForTest()
	c.Assert(len(applier.Operations()), gc.DeepEquals, 0)
	applier.Apply(r1)

	existingR1 := mocks.NewMockResource(ctrl)

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
	c.Assert(applier.Run(context.TODO(), nil, false), gc.ErrorMatches, `something was wrong`)
}

func (s *applierSuite) TestRunDeleteFailedWithRollBack(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r1 := mocks.NewMockResource(ctrl)

	applier := resources.NewApplierForTest()
	c.Assert(len(applier.Operations()), gc.DeepEquals, 0)
	applier.Delete(r1)

	existingR1 := mocks.NewMockResource(ctrl)

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
	c.Assert(applier.Run(context.TODO(), nil, false), gc.ErrorMatches, `something was wrong`)
}
