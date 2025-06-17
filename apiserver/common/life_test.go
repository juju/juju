// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
)

type lifeSuite struct {
	applicationService *mocks.MockApplicationService
	machineService     *mocks.MockMachineService
}

func TestLifeSuite(t *testing.T) {
	tc.Run(t, &lifeSuite{})
}

func (s *lifeSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.applicationService = mocks.NewMockApplicationService(ctrl)
	s.machineService = mocks.NewMockMachineService(ctrl)

	c.Cleanup(func() {
		s.applicationService = nil
		s.machineService = nil
	})

	return ctrl
}

func (s *lifeSuite) TestUnitLife(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), unit.Name("x/0")).Return(life.Alive, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), unit.Name("x/1")).Return(life.Dead, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), unit.Name("x/2")).Return("", applicationerrors.UnitNotFound)

	getCanRead := func(ctx context.Context) (common.AuthFunc, error) {
		return func(names.Tag) bool {
			return true
		}, nil
	}
	lg := common.NewLifeGetter(s.applicationService, s.machineService, nil, getCanRead, loggertesting.WrapCheckLog(c))
	entities := params.Entities{Entities: []params.Entity{
		{Tag: "unit-x-0"}, {Tag: "unit-x-1"}, {Tag: "unit-x-2"},
	}}
	results, err := lg.Life(c.Context(), entities)
	c.Check(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Dead},
			{Error: apiservertesting.NotFoundError(`unit x/2`)},
		},
	})
}

func (s *lifeSuite) TestApplicationLife(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	s.applicationService.EXPECT().GetApplicationLifeByName(gomock.Any(), "x").Return(life.Alive, nil)
	s.applicationService.EXPECT().GetApplicationLifeByName(gomock.Any(), "y").Return(life.Dead, nil)
	s.applicationService.EXPECT().GetApplicationLifeByName(gomock.Any(), "z").Return("", applicationerrors.ApplicationNotFound)

	getCanRead := func(ctx context.Context) (common.AuthFunc, error) {
		return func(names.Tag) bool {
			return true
		}, nil
	}
	lg := common.NewLifeGetter(s.applicationService, s.machineService, nil, getCanRead, loggertesting.WrapCheckLog(c))
	entities := params.Entities{Entities: []params.Entity{
		{Tag: "application-x"}, {Tag: "application-y"}, {Tag: "application-z"},
	}}
	results, err := lg.Life(c.Context(), entities)
	c.Check(err, tc.ErrorIsNil)
	c.Check(results, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Life: life.Dead},
			{Error: apiservertesting.NotFoundError(`application "z"`)},
		},
	})
}

func (s *lifeSuite) TestMachineLife(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	getCanRead := func(ctx context.Context) (common.AuthFunc, error) {
		x0 := names.NewMachineTag("0")
		x2 := names.NewMachineTag("2")
		x3 := names.NewMachineTag("3")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x2 || tag == x3
		}, nil
	}

	s.machineService.EXPECT().GetMachineLife(gomock.Any(), machine.Name("0")).Return(life.Alive, nil)
	s.machineService.EXPECT().GetMachineLife(gomock.Any(), machine.Name("2")).Return(life.Dead, nil)
	s.machineService.EXPECT().GetMachineLife(gomock.Any(), machine.Name("3")).Return("", fmt.Errorf("3 error"))

	lg := common.NewLifeGetter(s.applicationService, s.machineService, nil, getCanRead, loggertesting.WrapCheckLog(c))
	entities := params.Entities{Entities: []params.Entity{
		{Tag: "machine-0"}, {Tag: "machine-1"}, {Tag: "machine-2"}, {Tag: "machine-3"}, {Tag: "machine-4"},
	}}
	results, err := lg.Life(c.Context(), entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: life.Dead},
			{Error: &params.Error{Message: "3 error"}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *lifeSuite) TestLifeError(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	getCanRead := func(ctx context.Context) (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	lg := common.NewLifeGetter(s.applicationService, s.machineService, nil, getCanRead, loggertesting.WrapCheckLog(c))
	_, err := lg.Life(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "x0"}}})
	c.Assert(err, tc.ErrorMatches, "pow")
}

func (s *lifeSuite) TestLifeNoArgsNoError(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	getCanRead := func(ctx context.Context) (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	lg := common.NewLifeGetter(s.applicationService, s.machineService, nil, getCanRead, loggertesting.WrapCheckLog(c))
	result, err := lg.Life(c.Context(), params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}
