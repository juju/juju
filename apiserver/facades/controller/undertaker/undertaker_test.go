// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/undertaker"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type undertakerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&undertakerSuite{})

func (s *undertakerSuite) setupStateAndAPI(c *gc.C, isSystem bool, modelName string) (*mockState, *undertaker.UndertakerAPI) {
	machineNo := "1"
	if isSystem {
		machineNo = "0"
	}

	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag(machineNo),
		Controller: true,
	}

	st := newMockState(names.NewUserTag("admin"), modelName, isSystem)
	api, err := undertaker.NewUndertaker(st, nil, authorizer)
	c.Assert(err, jc.ErrorIsNil)
	return st, api
}

func (s *undertakerSuite) TestNoPerms(c *gc.C) {
	for _, authorizer := range []apiservertesting.FakeAuthorizer{{
		Tag: names.NewMachineTag("0"),
	}, {
		Tag: names.NewUserTag("bob"),
	}} {
		st := newMockState(names.NewUserTag("admin"), "admin", true)
		_, err := undertaker.NewUndertaker(
			st,
			nil,
			authorizer,
		)
		c.Assert(err, gc.ErrorMatches, "permission denied")
	}
}

func (s *undertakerSuite) TestModelInfo(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel")
	st, api := s.setupStateAndAPI(c, true, "admin")
	for _, test := range []struct {
		st        *mockState
		api       *undertaker.UndertakerAPI
		isSystem  bool
		modelName string
	}{
		{otherSt, hostedAPI, false, "hostedmodel"},
		{st, api, true, "admin"},
	} {
		test.st.model.life = state.Dying
		test.st.model.forced = true

		result, err := test.api.ModelInfo()
		c.Assert(err, jc.ErrorIsNil)

		info := result.Result
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Error, gc.IsNil)

		c.Assert(info.UUID, gc.Equals, test.st.model.UUID())
		c.Assert(info.GlobalName, gc.Equals, "user-admin/"+test.modelName)
		c.Assert(info.Name, gc.Equals, test.modelName)
		c.Assert(info.IsSystem, gc.Equals, test.isSystem)
		c.Assert(info.Life, gc.Equals, life.Dying)
		c.Assert(info.ForceDestroyed, gc.Equals, true)
	}
}

func (s *undertakerSuite) TestProcessDyingModel(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel")
	model, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = hostedAPI.ProcessDyingModel()
	c.Assert(err, gc.ErrorMatches, "model is not dying")
	c.Assert(model.Life(), gc.Equals, state.Alive)

	otherSt.model.life = state.Dying
	err = hostedAPI.ProcessDyingModel()
	c.Assert(err, gc.IsNil)
	c.Assert(model.Life(), gc.Equals, state.Dead)
}

func (s *undertakerSuite) TestRemoveAliveModel(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel")
	_, err := otherSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = hostedAPI.RemoveModel()
	c.Assert(err, gc.ErrorMatches, "model not dying or dead")
}

func (s *undertakerSuite) TestRemoveDyingModel(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel")

	// Set model to dying
	otherSt.model.life = state.Dying

	c.Assert(hostedAPI.RemoveModel(), jc.ErrorIsNil)
}

func (s *undertakerSuite) TestDeadRemoveModel(c *gc.C) {
	otherSt, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel")

	// Set model to dead
	otherSt.model.life = state.Dying
	err := hostedAPI.ProcessDyingModel()
	c.Assert(err, gc.IsNil)

	err = hostedAPI.RemoveModel()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(otherSt.removed, jc.IsTrue)
}

func (s *undertakerSuite) TestModelConfig(c *gc.C) {
	_, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel")

	cfg, err := hostedAPI.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, gc.NotNil)
}

func (s *undertakerSuite) TestSetStatus(c *gc.C) {
	mock, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel")

	results, err := hostedAPI.SetStatus(params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			mock.model.Tag().String(), status.Destroying.String(),
			"woop", map[string]interface{}{"da": "ta"},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(mock.model.status, gc.Equals, status.Destroying)
	c.Assert(mock.model.statusInfo, gc.Equals, "woop")
	c.Assert(mock.model.statusData, jc.DeepEquals, map[string]interface{}{"da": "ta"})
}

func (s *undertakerSuite) TestSetStatusControllerPermissions(c *gc.C) {
	_, hostedAPI := s.setupStateAndAPI(c, true, "hostedmodel")
	results, err := hostedAPI.SetStatus(params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			"model-6ada782f-bcd4-454b-a6da-d1793fbcb35e", status.Destroying.String(),
			"woop", map[string]interface{}{"da": "ta"},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, ".*not found")
}

func (s *undertakerSuite) TestSetStatusNonControllerPermissions(c *gc.C) {
	_, hostedAPI := s.setupStateAndAPI(c, false, "hostedmodel")
	results, err := hostedAPI.SetStatus(params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			"model-6ada782f-bcd4-454b-a6da-d1793fbcb35e", status.Destroying.String(),
			"woop", map[string]interface{}{"da": "ta"},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "permission denied")
}
