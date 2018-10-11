// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
The existing machinemanager_test.go uses a home grown mocking mechanism.
I wanted to establish the new suffixed file to have a place to start systematically moving those tests to use gomock.
There are two benefits to this

1) We can work piecemeal
2) We don't have to mix two mocking styles (in attempt to preserve one file) when transitioning between mocking styles

The plan is to start moving those old style tests and when finished delete the old file and mv the new file.
*/

package machinemanager_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/machinemanager"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&NewMachineManagerSuite{})

type NewMachineManagerSuite struct {
	jujutesting.BaseSuite

	clientFacade *mocks.MockClientFacade
	facade       *mocks.MockFacadeCaller

	tag    names.Tag
	args   params.Entities
	client *machinemanager.Client
}

func (s *NewMachineManagerSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("0")
	s.args = params.Entities{Entities: []params.Entity{{Tag: s.tag.String()}}}

	s.BaseSuite.SetUpTest(c)
}

func (s *NewMachineManagerSuite) TestUpgradeSeriesValidate(c *gc.C) {
	defer s.setup(c).Finish()

	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{
			{
				Entity: params.Entity{Tag: names.NewMachineTag(s.tag.String()).String()},
				Series: "xenial",
			},
		},
	}
	result := params.UpgradeSeriesUnitsResult{
		UnitNames: []string{"ubuntu/0", "ubuntu/1"},
	}
	results := params.UpgradeSeriesUnitsResults{Results: []params.UpgradeSeriesUnitsResult{result}}
	s.facade.EXPECT().FacadeCall("UpgradeSeriesValidate", args, gomock.Any()).SetArg(2, results)

	unitNames, err := s.client.UpgradeSeriesValidate(s.tag.String(), "xenial")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitNames, gc.DeepEquals, result.UnitNames)
}

func (s *NewMachineManagerSuite) TestApplicationsSuccess(c *gc.C) {
	defer s.setup(c).Finish()

	apps := []string{"redis"}
	resultSource := params.StringsResults{Results: []params.StringsResult{{Result: apps}}}
	s.facade.EXPECT().FacadeCall("Applications", s.args, gomock.Any()).SetArg(2, resultSource)

	result, err := s.client.Applications(s.tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, apps)
}

func (s *NewMachineManagerSuite) TestApplicationsError(c *gc.C) {
	defer s.setup(c).Finish()

	msg := "boom"
	resultSource := params.StringsResults{Results: []params.StringsResult{{Error: &params.Error{Message: msg}}}}
	s.facade.EXPECT().FacadeCall("Applications", s.args, gomock.Any()).SetArg(2, resultSource)

	_, err := s.client.Applications(s.tag.Id())
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *NewMachineManagerSuite) TestApplicationsMultiResultError(c *gc.C) {
	defer s.setup(c).Finish()

	resultSource := params.StringsResults{Results: []params.StringsResult{
		{}, {},
	}}
	s.facade.EXPECT().FacadeCall("Applications", s.args, gomock.Any()).SetArg(2, resultSource)

	_, err := s.client.Applications(s.tag.Id())
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *NewMachineManagerSuite) TestUpgradeSeriesPrepareAlreadyInProgress(c *gc.C) {
	defer s.setup(c).Finish()

	arg := params.UpdateSeriesArg{
		Entity: params.Entity{Tag: s.tag.String()},
		Series: "xenial",
		Force:  true,
	}
	resultSource := params.ErrorResult{
		Error: &params.Error{
			Message: "lock already exists",
			Code:    params.CodeAlreadyExists,
		},
	}
	s.facade.EXPECT().FacadeCall("UpgradeSeriesPrepare", arg, gomock.Any()).SetArg(2, resultSource)

	err := s.client.UpgradeSeriesPrepare(s.tag.Id(), "xenial", true)
	c.Assert(errors.IsAlreadyExists(err), jc.IsTrue)
}

func (s *NewMachineManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clientFacade = mocks.NewMockClientFacade(ctrl)
	s.facade = mocks.NewMockFacadeCaller(ctrl)

	s.clientFacade.EXPECT().BestAPIVersion().Return(5)

	s.client = machinemanager.ConstructClient(s.clientFacade, s.facade)

	return ctrl
}
