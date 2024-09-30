// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type instanceIdGetterSuite struct {
	machineService *MockMachineService
}

var _ = gc.Suite(&instanceIdGetterSuite{})

type fakeInstanceIdGetter struct {
	state.Entity
	instanceId string
	err        string
	fetchError
}

func (f *fakeInstanceIdGetter) InstanceId() (instance.Id, error) {
	if f.err != "" {
		return "", fmt.Errorf("%s", f.err)
	}
	return instance.Id(f.instanceId), nil
}

func (s *instanceIdGetterSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machineService = NewMockMachineService(ctrl)
	return ctrl
}

func (s *instanceIdGetterSuite) TestInstanceId(c *gc.C) {
	defer s.setupMocks(c).Finish()

	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			u("x/0"): &fakeInstanceIdGetter{instanceId: "foo"},
			u("x/1"): &fakeInstanceIdGetter{instanceId: "bar"},
			u("x/2"): &fakeInstanceIdGetter{instanceId: "baz", err: "x2 error"},
			u("x/3"): &fakeInstanceIdGetter{fetchError: "x3 error"},
		},
	}
	getCanRead := func() (common.AuthFunc, error) {
		x0 := u("x/0")
		x2 := u("x/2")
		x3 := u("x/3")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x2 || tag == x3
		}, nil
	}
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("x/0")).Return("uuid-0", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "uuid-0").Return("foo", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("x/2")).Return("uuid-2", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "uuid-2").Return("", errors.New("x2 error"))
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("x/3")).Return("uuid-3", nil)
	s.machineService.EXPECT().InstanceID(gomock.Any(), "uuid-3").Return("", errors.New("x3 error"))
	ig := common.NewInstanceIdGetter(st, s.machineService, getCanRead)
	entities := params.Entities{Entities: []params.Entity{
		{Tag: "unit-x-0"}, {Tag: "unit-x-1"}, {Tag: "unit-x-2"}, {Tag: "unit-x-3"}, {Tag: "unit-x-4"},
	}}
	results, err := ig.InstanceId(context.Background(), entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "foo"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Message: "x2 error"}},
			{Error: &params.Error{Message: "x3 error"}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *instanceIdGetterSuite) TestInstanceIdError(c *gc.C) {
	getCanRead := func() (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	ig := common.NewInstanceIdGetter(&fakeState{}, s.machineService, getCanRead)
	_, err := ig.InstanceId(context.Background(), params.Entities{Entities: []params.Entity{{Tag: "unit-x-0"}}})
	c.Assert(err, gc.ErrorMatches, "pow")
}
