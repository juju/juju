// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/caasoperator"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/workertest"
)

var _ = gc.Suite(&CAASOperatorSuite{})

type CAASOperatorSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	facade     *caasoperator.Facade
	st         *mockState
}

func (s *CAASOperatorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewApplicationTag("gitlab"),
	}

	s.st = newMockState()
	s.AddCleanup(func(c *gc.C) {
		workertest.CleanKill(c, s.st.app.settingsWatcher)
	})

	facade, err := caasoperator.NewFacade(s.resources, s.authorizer, s.st)
	c.Assert(err, jc.ErrorIsNil)
	s.facade = facade
}

func (s *CAASOperatorSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasoperator.NewFacade(s.resources, s.authorizer, s.st)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *CAASOperatorSuite) TestSetStatus(c *gc.C) {
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "application-gitlab",
			Status: "bar",
			Info:   "baz",
			Data: map[string]interface{}{
				"qux": "quux",
			},
		}, {
			Tag:    "machine-0",
			Status: "nope",
		}},
	}

	results, err := s.facade.SetStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{&params.Error{Message: `"machine-0" is not a valid application tag`}},
		},
	})

	s.st.CheckCallNames(c, "Model", "Application")
	s.st.CheckCall(c, 1, "Application", "gitlab")
	s.st.app.CheckCallNames(c, "SetStatus")
	s.st.app.CheckCall(c, 0, "SetStatus", status.StatusInfo{
		Status:  "bar",
		Message: "baz",
		Data: map[string]interface{}{
			"qux": "quux",
		},
	})
}

func (s *CAASOperatorSuite) TestCharm(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "application-other"},
			{Tag: "machine-0"},
		},
	}

	results, err := s.facade.Charm(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ApplicationCharmResults{
		Results: []params.ApplicationCharmResult{{
			Result: &params.ApplicationCharm{
				URL:          "cs:gitlab-1",
				ForceUpgrade: false,
				SHA256:       "fake-sha256",
			},
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}, {
			Error: &params.Error{Message: `"machine-0" is not a valid application tag`},
		}},
	})

	s.st.CheckCallNames(c, "Model", "Application")
	s.st.CheckCall(c, 1, "Application", "gitlab")
	s.st.app.CheckCallNames(c, "Charm")
}

func (s *CAASOperatorSuite) TestApplicationConfig(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "application-other"},
			{Tag: "machine-0"},
		},
	}

	results, err := s.facade.ApplicationConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ConfigSettingsResults{
		Results: []params.ConfigSettingsResult{{
			Settings: params.ConfigSettings{"k": 123},
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}, {
			Error: &params.Error{Message: `"machine-0" is not a valid application tag`},
		}},
	})

	s.st.CheckCallNames(c, "Model", "Application")
	s.st.CheckCall(c, 1, "Application", "gitlab")
	s.st.app.CheckCallNames(c, "ConfigSettings")
}

func (s *CAASOperatorSuite) TestWatchApplicationConfig(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "application-other"},
			{Tag: "machine-0"},
		},
	}

	results, err := s.facade.WatchApplicationConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "1",
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}, {
			Error: &params.Error{Message: `"machine-0" is not a valid application tag`},
		}},
	})

	s.st.CheckCallNames(c, "Model", "Application")
	s.st.CheckCall(c, 1, "Application", "gitlab")
	s.st.app.CheckCallNames(c, "WatchConfigSettings")
	c.Assert(s.resources.Get("1"), gc.Equals, s.st.app.settingsWatcher)
}

func (s *CAASOperatorSuite) TestSetContainerSpec(c *gc.C) {
	args := params.SetContainerSpecParams{
		Entities: []params.EntityString{
			{Tag: "application-gitlab", Value: "foo"},
			{Tag: "unit-gitlab-0", Value: "bar"},
			{Tag: "unit-gitlab-1", Value: "baz"},
			{Tag: "application-other"},
			{Tag: "unit-other-0"},
			{Tag: "machine-0"},
		},
	}

	s.st.model.SetErrors(nil, nil, errors.New("bloop"))

	results, err := s.facade.SetContainerSpec(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}, {
			Error: nil,
		}, {
			Error: &params.Error{
				Message: "bloop",
			},
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}},
	})

	s.st.CheckCallNames(c, "Model")
	s.st.model.CheckCallNames(c, "SetContainerSpec", "SetContainerSpec", "SetContainerSpec")
	s.st.model.CheckCall(c, 0, "SetContainerSpec", names.NewApplicationTag("gitlab"), "foo")
	s.st.model.CheckCall(c, 1, "SetContainerSpec", names.NewUnitTag("gitlab/0"), "bar")
	s.st.model.CheckCall(c, 2, "SetContainerSpec", names.NewUnitTag("gitlab/1"), "baz")
}

func (s *CAASOperatorSuite) TestModelName(c *gc.C) {
	result, err := s.facade.ModelName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Result, gc.Equals, "some-model")
}
