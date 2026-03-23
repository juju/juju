// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/rpc/params"
)

type instanceIdGetterSuite struct {
	machineService *mocks.MockMachineService
}

func TestInstanceIdGetterSuite(t *testing.T) {
	tc.Run(t, &instanceIdGetterSuite{})
}

func (s *instanceIdGetterSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machineService = mocks.NewMockMachineService(ctrl)

	c.Cleanup(func() {
		s.machineService = nil
	})

	return ctrl
}

func (s *instanceIdGetterSuite) TestInstanceId(c *tc.C) {
	defer s.setupMocks(c).Finish()

	getCanRead := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag.Id() == "0" || tag.Id() == "2" || tag.Id() == "3"
		}, nil
	}
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("0")).Return("uuid-0", nil)
	s.machineService.EXPECT().GetInstanceID(gomock.Any(), machine.UUID("uuid-0")).Return("foo", nil)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("uuid-2", nil)
	s.machineService.EXPECT().GetInstanceID(gomock.Any(), machine.UUID("uuid-2")).Return("", errors.New("x2 error"))
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("3")).Return("uuid-3", nil)
	s.machineService.EXPECT().GetInstanceID(gomock.Any(), machine.UUID("uuid-3")).Return("", errors.New("x3 error"))
	ig := common.NewInstanceIdGetter(s.machineService, getCanRead)
	entities := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"}, {Tag: "machine-1"}, {Tag: "machine-2"}, {Tag: "machine-3"}, {Tag: "machine-4"},
	}}
	results, err := ig.InstanceId(c.Context(), entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "foo"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Message: "x2 error"}},
			{Error: &params.Error{Message: "x3 error"}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *instanceIdGetterSuite) TestInstanceIdError(c *tc.C) {
	getCanRead := func(ctx context.Context) (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	ig := common.NewInstanceIdGetter(s.machineService, getCanRead)
	_, err := ig.InstanceId(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "unit-x-0"}}})
	c.Assert(err, tc.ErrorMatches, "pow")
}

func (s *instanceIdGetterSuite) TestInstanceIdErrorNotProvisioned(c *tc.C) {
	defer s.setupMocks(c).Finish()

	getCanRead := func(ctx context.Context) (common.AuthFunc, error) {
		return func(names.Tag) bool { return true }, nil
	}

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("0")).Return("uuid-0", nil)
	s.machineService.EXPECT().GetInstanceID(gomock.Any(), machine.UUID("uuid-0")).Return("", machineerrors.NotProvisioned)

	entities := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
	}}
	ig := common.NewInstanceIdGetter(s.machineService, getCanRead)
	results, err := ig.InstanceId(c.Context(), entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.Satisfies, params.IsCodeNotProvisioned)
}

func (s *instanceIdGetterSuite) TestInstanceIdErrorNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	getCanRead := func(ctx context.Context) (common.AuthFunc, error) {
		return func(names.Tag) bool { return true }, nil
	}

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("0")).Return("", machineerrors.MachineNotFound)

	entities := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"},
	}}
	ig := common.NewInstanceIdGetter(s.machineService, getCanRead)
	results, err := ig.InstanceId(c.Context(), entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}
