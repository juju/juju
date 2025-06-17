// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/machinemanager"
	"github.com/juju/juju/apiserver/facades/client/machinemanager/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

var over9kCPUCores uint64 = 9001

type instanceTypesSuite struct {
	authorizer *apiservertesting.FakeAuthorizer
	st         *mocks.MockBackend
	leadership *mocks.MockLeadership
	api        *machinemanager.MachineManagerAPI
}

var _ = gc.Suite(&instanceTypesSuite{})

func (s *instanceTypesSuite) SetUpTest(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin"), Controller: true}
}

func (s *instanceTypesSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockBackend(ctrl)
	s.leadership = mocks.NewMockLeadership(ctrl)
	var err error
	s.api, err = machinemanager.NewMachineManagerAPI(s.st,
		nil,
		nil,
		machinemanager.ModelAuthorizer{
			Authorizer: s.authorizer,
			ModelTag:   names.NewModelTag("beef1beef1-0000-0000-000011112222"),
		},
		context.NewEmptyCloudCallContext(),
		common.NewResources(),
		s.leadership,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *instanceTypesSuite) TestInstanceTypes(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	model.EXPECT().ControllerUUID().Return(testing.ControllerTag.Id())
	s.st.EXPECT().Model().Return(model, nil)

	itCons := constraints.Value{CpuCores: &over9kCPUCores}
	failureCons := constraints.Value{}

	env := mocks.NewMockEnviron(ctrl)
	env.EXPECT().InstanceTypes(gomock.Any(), itCons).Return(instances.InstanceTypesWithCostMetadata{
		CostUnit:     "USD/h",
		CostCurrency: "USD",
		InstanceTypes: []instances.InstanceType{
			{Name: "instancetype-1"},
			{Name: "instancetype-2"}},
	}, nil).MinTimes(1)

	env.EXPECT().InstanceTypes(gomock.Any(), failureCons).Return(
		instances.InstanceTypesWithCostMetadata{},
		errors.NotFoundf("Instances matching constraint %v", failureCons),
	).MinTimes(1)

	fakeEnvironGet := func(st environs.EnvironConfigGetter,
		newEnviron environs.NewEnvironFunc,
	) (environs.Environ, error) {
		c.Assert(st.ControllerUUID(), gc.Equals, testing.ControllerTag.Id())
		return env, nil
	}

	cons := params.ModelInstanceTypesConstraints{
		Constraints: []params.ModelInstanceTypesConstraint{{Value: &itCons}, {Value: &failureCons}, {}},
	}

	r, err := machinemanager.InstanceTypes(s.api, fakeEnvironGet, cons)
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
