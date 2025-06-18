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
	"github.com/juju/juju/core/unit"
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

func (l *lifeSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	l.applicationService = mocks.NewMockApplicationService(ctrl)
	l.machineService = mocks.NewMockMachineService(ctrl)

	c.Cleanup(func() {
		l.applicationService = nil
		l.machineService = nil
	})

	return ctrl
}

func (s *lifeSuite) TestLife(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	st := &fakeState{}
	getCanRead := func(ctx context.Context) (common.AuthFunc, error) {
		x0 := u("x/0")
		x2 := u("x/2")
		x3 := u("x/3")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x2 || tag == x3
		}, nil
	}

	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), unit.Name("x/0")).Return(life.Alive, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), unit.Name("x/2")).Return(life.Dead, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), unit.Name("x/3")).Return("", fmt.Errorf("x3 error"))

	lg := common.NewLifeGetter(s.applicationService, s.machineService, st, getCanRead, loggertesting.WrapCheckLog(c))
	entities := params.Entities{Entities: []params.Entity{
		{Tag: "unit-x-0"}, {Tag: "unit-x-1"}, {Tag: "unit-x-2"}, {Tag: "unit-x-3"}, {Tag: "unit-x-4"},
	}}
	results, err := lg.Life(c.Context(), entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: life.Dead},
			{Error: &params.Error{Message: "x3 error"}},
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
