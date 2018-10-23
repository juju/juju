// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/caasoperator"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/status"
	coretesting "github.com/juju/juju/testing"
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
		workertest.CleanKill(c, s.st.app.unitsWatcher)
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
	s.st.app.CheckCallNames(c, "SetOperatorStatus")
	s.st.app.CheckCall(c, 0, "SetOperatorStatus", status.StatusInfo{
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
				URL:                  "cs:gitlab-1",
				ForceUpgrade:         false,
				SHA256:               "fake-sha256",
				CharmModifiedVersion: 666,
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
	s.st.app.CheckCallNames(c, "Charm", "CharmModifiedVersion")
}

func (s *CAASOperatorSuite) TestWatchUnits(c *gc.C) {
	s.st.app.unitsChanges <- []string{"gitlab/0", "gitlab/1"}

	results, err := s.facade.WatchUnits(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: `"unit-gitlab-0" is not a valid application tag`,
	})

	c.Assert(results.Results[0].StringsWatcherId, gc.Equals, "1")
	c.Assert(results.Results[0].Changes, jc.DeepEquals, []string{"gitlab/0", "gitlab/1"})
	resource := s.resources.Get("1")
	c.Assert(resource, gc.Equals, s.st.app.unitsWatcher)
}

func (s *CAASOperatorSuite) TestLife(c *gc.C) {
	results, err := s.facade.Life(params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-gitlab-0"},
			{Tag: "application-gitlab"},
			{Tag: "machine-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{{
			Life: params.Dying,
		}, {
			Life: params.Alive,
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}},
	})
}

func (s *CAASOperatorSuite) TestRemove(c *gc.C) {
	results, err := s.facade.Remove(params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-gitlab-0"},
			{Tag: "machine-0"},
			{Tag: "unit-mysql-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{
				Error: &params.Error{
					Code:    "unauthorized access",
					Message: "permission denied",
				},
			},
			{
				Error: &params.Error{
					Code:    "unauthorized access",
					Message: "permission denied",
				},
			}},
	})
}

func (s *CAASOperatorSuite) TestSetPodSpec(c *gc.C) {
	validSpecStr := `
containers:
  - name: gitlab
    image: gitlab/latest
`[1:]

	args := params.SetPodSpecParams{
		Specs: []params.EntityString{
			{Tag: "application-gitlab", Value: validSpecStr},
			{Tag: "application-gitlab", Value: validSpecStr},
			{Tag: "application-gitlab", Value: "bad spec"},
			{Tag: "unit-gitlab-0"},
			{Tag: "application-other"},
			{Tag: "unit-other-0"},
			{Tag: "machine-0"},
		},
	}

	s.st.model.SetErrors(nil, errors.New("bloop"))

	results, err := s.facade.SetPodSpec(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}, {
			Error: &params.Error{
				Message: "bloop",
			},
		}, {
			Error: &params.Error{
				Message: "invalid pod spec",
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
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}},
	})

	s.st.CheckCallNames(c, "Model")
	s.st.model.CheckCallNames(c, "SetPodSpec", "SetPodSpec")
	s.st.model.CheckCall(c, 0, "SetPodSpec", names.NewApplicationTag("gitlab"), validSpecStr)
}

func (s *CAASOperatorSuite) TestModel(c *gc.C) {
	result, err := s.facade.CurrentModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ModelResult{
		Name: "some-model",
		UUID: "deadbeef",
		Type: "iaas",
	})
}

func (s *CAASOperatorSuite) TestWatch(c *gc.C) {
	s.st.app.appChanges <- struct{}{}

	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "application-gitlab"},
		{Tag: "application-mysql"},
		{Tag: "unit-mysql-0"},
	}}
	result, err := s.facade.Watch(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.NotFoundError("application mysql")},
			{Error: apiservertesting.NotFoundError("unit mysql/0")},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	c.Assert(result.Results[0].NotifyWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	c.Assert(resource, gc.Equals, s.st.app.watcher)
}

func (s *CAASOperatorSuite) TestSetTools(c *gc.C) {
	vers := version.MustParseBinary("2.99.0-bionic-amd64")
	results, err := s.facade.SetTools(params.EntitiesVersion{
		AgentTools: []params.EntityVersion{
			{Tag: "application-gitlab", Tools: &params.Version{Version: vers}},
			{Tag: "machine-0", Tools: &params.Version{Version: vers}},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{
				Error: &params.Error{
					Code:    "unauthorized access",
					Message: "permission denied",
				},
			}},
	})
	s.st.app.CheckCall(c, 0, "SetAgentVersion", vers)
}

func (s *CAASOperatorSuite) TestAddresses(c *gc.C) {
	_, err := s.facade.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "Model", "APIHostPortsForAgents")
}

func (s *CAASOperatorSuite) TestWatchAPIHostPorts(c *gc.C) {
	_, err := s.facade.WatchAPIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c, "Model", "WatchAPIHostPortsForAgents")
}
