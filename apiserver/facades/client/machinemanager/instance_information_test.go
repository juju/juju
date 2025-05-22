// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/rpc/params"
)

var over9kCPUCores uint64 = 9001

type instanceTypesSuite struct {
	instanceTypesFetcher *MockInstanceTypesFetcher
}

func TestInstanceTypesSuite(t *stdtesting.T) {
	tc.Run(t, &instanceTypesSuite{})
}

func (s *instanceTypesSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.instanceTypesFetcher = NewMockInstanceTypesFetcher(ctrl)
	return ctrl
}

func (s *instanceTypesSuite) TestInstanceTypes(c *tc.C) {
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

	r, err := instanceTypes(c.Context(), s.instanceTypesFetcher, cons)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r.Results, tc.HasLen, 3)
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
	c.Assert(r.Results, tc.DeepEquals, expected)
}
