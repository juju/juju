// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/rpc/params"
)

var over9kCPUCores uint64 = 9001

type instanceTypesSuite struct {
	instanceTypesFetcher *MockInstanceTypesFetcher
}

var _ = gc.Suite(&instanceTypesSuite{})

func (s *instanceTypesSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.instanceTypesFetcher = NewMockInstanceTypesFetcher(ctrl)
	return ctrl
}

func (s *instanceTypesSuite) TestInstanceTypes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	itCons := constraints.Value{CpuCores: &over9kCPUCores}
	failureCons := constraints.Value{}

	s.instanceTypesFetcher.EXPECT().InstanceTypes(gomock.Any(), itCons).Return(instances.InstanceTypesWithCostMetadata{
		CostUnit:     "USD/h",
		CostCurrency: "USD",
		InstanceTypes: []instances.InstanceType{
			{Name: "instancetype-1"},
			{Name: "instancetype-2"}},
	}, nil).MinTimes(1)

	s.instanceTypesFetcher.EXPECT().InstanceTypes(gomock.Any(), failureCons).Return(
		instances.InstanceTypesWithCostMetadata{},
		errors.NotFoundf("Instances matching constraint %v", failureCons),
	).MinTimes(1)

	cons := params.ModelInstanceTypesConstraints{
		Constraints: []params.ModelInstanceTypesConstraint{{Value: &itCons}, {Value: &failureCons}, {}},
	}

	r, err := instanceTypes(context.Background(), s.instanceTypesFetcher, cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Results, gc.HasLen, 3)
	expected := []params.InstanceTypesResult{
		{
			InstanceTypes: []params.InstanceType{
				{Name: "instancetype-1"},
				{Name: "instancetype-2"},
			},
			CostUnit:     "USD/h",
			CostCurrency: "USD",
		},
		{
			Error: &params.Error{
				Message: "Instances matching constraint  not found", Code: "not found",
			},
		},
		{
			Error: &params.Error{
				Message: "Instances matching constraint  not found", Code: "not found",
			},
		},
	}
	c.Assert(r.Results, gc.DeepEquals, expected)
}
