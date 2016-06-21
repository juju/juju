// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/logfwd"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type LastSentSuite struct {
	jujutesting.JujuConnSuite

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	rawMachine *state.Machine
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&LastSentSuite{})

func (s *LastSentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	// Create a machine to work with
	var err error
	s.rawMachine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	// The default auth is as the machine agent
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.rawMachine.Tag(),
	}
}

func (s *LastSentSuite) TestAuthRefusesUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = s.AdminUserTag(c)

	_, err := logfwd.NewLogForwardingAPI(s.State, s.resources, anAuthorizer)

	c.Check(err, gc.ErrorMatches, "permission denied")
}

func (s *LastSentSuite) TestGetLastSentFound(c *gc.C) {
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model).String()
	s.addModel(c, "other-model", model)
	ts := time.Unix(12345, 0)
	s.setLastSent(c, modelTag, "spam", ts)
	api, err := logfwd.NewLogForwardingAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	res := api.GetLastSent(params.LogForwardingGetLastSentParams{
		IDs: []params.LogForwardingID{{
			ModelTag: modelTag,
			Sink:     "spam",
		}},
	})

	c.Check(res, jc.DeepEquals, params.LogForwardingGetLastSentResults{
		Results: []params.LogForwardingGetLastSentResult{{
			Timestamp: ts.UTC(),
		}},
	})
}

func (s *LastSentSuite) TestGetLastSentBulk(c *gc.C) {
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model).String()
	s.addModel(c, "other-model", model)
	tsSpam := time.Unix(12345, 0)
	tsEggs := time.Unix(12345, 54321)
	s.setLastSent(c, modelTag, "spam", tsSpam)
	s.setLastSent(c, modelTag, "eggs", tsEggs)
	api, err := logfwd.NewLogForwardingAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	res := api.GetLastSent(params.LogForwardingGetLastSentParams{
		IDs: []params.LogForwardingID{{
			ModelTag: modelTag,
			Sink:     "spam",
		}, {
			ModelTag: modelTag,
			Sink:     "eggs",
		}, {
			ModelTag: modelTag,
			Sink:     "ham",
		}},
	})

	c.Check(res, jc.DeepEquals, params.LogForwardingGetLastSentResults{
		Results: []params.LogForwardingGetLastSentResult{{
			Timestamp: tsSpam.UTC(),
		}, {
			Timestamp: tsEggs.UTC(),
		}, {
			Error: &params.Error{
				Message: `cannot find timestamp of the last forwarded record`,
				Code:    params.CodeNotFound,
			},
		}},
	})
}

func (s *LastSentSuite) TestGetLastSentSinkNotFound(c *gc.C) {
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model).String()
	s.addModel(c, "other-model", model)
	api, err := logfwd.NewLogForwardingAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	res := api.GetLastSent(params.LogForwardingGetLastSentParams{
		IDs: []params.LogForwardingID{{
			ModelTag: modelTag,
			Sink:     "spam",
		}},
	})

	c.Check(res, jc.DeepEquals, params.LogForwardingGetLastSentResults{
		Results: []params.LogForwardingGetLastSentResult{{
			Error: &params.Error{
				Message: `cannot find timestamp of the last forwarded record`,
				Code:    params.CodeNotFound,
			},
		}},
	})
}

func (s *LastSentSuite) TestGetLastSentBadModel(c *gc.C) {
	api, err := logfwd.NewLogForwardingAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	res := api.GetLastSent(params.LogForwardingGetLastSentParams{
		IDs: []params.LogForwardingID{{
			ModelTag: "deadbeef-2f18-4fd2-967d-db9663db7bea",
			Sink:     "spam",
		}},
	})

	c.Check(res, jc.DeepEquals, params.LogForwardingGetLastSentResults{
		Results: []params.LogForwardingGetLastSentResult{{
			Error: &params.Error{
				Message: `"deadbeef-2f18-4fd2-967d-db9663db7bea" is not a valid tag`,
			},
		}},
	})
}

func (s *LastSentSuite) TestGetLastSentModelNotFound(c *gc.C) {
	api, err := logfwd.NewLogForwardingAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	res := api.GetLastSent(params.LogForwardingGetLastSentParams{
		IDs: []params.LogForwardingID{{
			ModelTag: "model-deadbeef-2f18-4fd2-967d-db9663db7bea",
			Sink:     "spam",
		}},
	})

	c.Check(res, jc.DeepEquals, params.LogForwardingGetLastSentResults{
		Results: []params.LogForwardingGetLastSentResult{{
			Error: &params.Error{
				Message: `model not found`,
				Code:    params.CodeNotFound,
			},
		}},
	})
}

func (s *LastSentSuite) TestSetLastSentNew(c *gc.C) {
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model).String()
	s.addModel(c, "other-model", model)
	expected := time.Unix(12345, 0)
	api, err := logfwd.NewLogForwardingAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	res := api.SetLastSent(params.LogForwardingSetLastSentParams{
		Params: []params.LogForwardingSetLastSentParam{{
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag,
				Sink:     "spam",
			},
			Timestamp: expected,
		}},
	})

	ts := s.getLastSent(c, modelTag, "spam")
	c.Check(ts, jc.DeepEquals, expected.UTC())
	c.Check(res, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}},
	})
}

func (s *LastSentSuite) TestSetLastSentReplace(c *gc.C) {
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model).String()
	s.addModel(c, "other-model", model)
	expected := time.Unix(12345, 54321)
	s.setLastSent(c, modelTag, "spam", time.Unix(12345, 0))
	api, err := logfwd.NewLogForwardingAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	res := api.SetLastSent(params.LogForwardingSetLastSentParams{
		Params: []params.LogForwardingSetLastSentParam{{
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag,
				Sink:     "spam",
			},
			Timestamp: expected,
		}},
	})

	ts := s.getLastSent(c, modelTag, "spam")
	c.Check(ts, jc.DeepEquals, expected.UTC())
	c.Check(res, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}},
	})
}

func (s *LastSentSuite) TestSetLastSentBulk(c *gc.C) {
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model).String()
	s.addModel(c, "other-model", model)
	expectedSpam := time.Unix(12345, 54321)
	expectedEggs := time.Unix(98765, 0)
	s.setLastSent(c, modelTag, "spam", time.Unix(12345, 0))
	api, err := logfwd.NewLogForwardingAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	res := api.SetLastSent(params.LogForwardingSetLastSentParams{
		Params: []params.LogForwardingSetLastSentParam{{
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag,
				Sink:     "spam",
			},
			Timestamp: expectedSpam,
		}, {
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag,
				Sink:     "eggs",
			},
			Timestamp: expectedEggs,
		}},
	})

	c.Check(res, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}, {
			Error: nil,
		}},
	})
	ts := s.getLastSent(c, modelTag, "spam")
	c.Check(ts, jc.DeepEquals, expectedSpam.UTC())
	ts = s.getLastSent(c, modelTag, "eggs")
	c.Check(ts, jc.DeepEquals, expectedEggs.UTC())
}

func (s *LastSentSuite) TestSetLastSentBadModel(c *gc.C) {
	api, err := logfwd.NewLogForwardingAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	res := api.SetLastSent(params.LogForwardingSetLastSentParams{
		Params: []params.LogForwardingSetLastSentParam{{
			LogForwardingID: params.LogForwardingID{
				ModelTag: "deadbeef-2f18-4fd2-967d-db9663db7bea",
				Sink:     "spam",
			},
			Timestamp: time.Unix(12345, 54321),
		}},
	})

	c.Check(res, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{
				Message: `"deadbeef-2f18-4fd2-967d-db9663db7bea" is not a valid tag`,
			},
		}},
	})
}

func (s *LastSentSuite) TestSetLastSentModelNotFound(c *gc.C) {
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model).String()
	api, err := logfwd.NewLogForwardingAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	res := api.SetLastSent(params.LogForwardingSetLastSentParams{
		Params: []params.LogForwardingSetLastSentParam{{
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag,
				Sink:     "spam",
			},
			Timestamp: time.Unix(12345, 54321),
		}},
	})

	c.Check(res, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{
				Message: `model not found`,
				Code:    params.CodeNotFound,
			},
		}},
	})
}

func (s *LastSentSuite) addModel(c *gc.C, name, uuid string) {
	_, modelState, err := s.State.NewModel(state.ModelArgs{
		Config: testing.CustomModelConfig(c, testing.Attrs{
			"name": name,
			"uuid": uuid,
		}),
		Owner: s.AdminUserTag(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { modelState.Close() })
}

func (s *LastSentSuite) getLastSent(c *gc.C, model, sink string) time.Time {
	tag, err := names.ParseModelTag(model)
	c.Assert(err, jc.ErrorIsNil)
	st, err := s.State.ForModel(tag)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	lsl := state.NewLastSentLogger(st, sink)

	ts, err := lsl.Get()
	c.Assert(err, jc.ErrorIsNil)
	return ts
}

func (s *LastSentSuite) setLastSent(c *gc.C, model, sink string, timestamp time.Time) {
	tag, err := names.ParseModelTag(model)
	c.Assert(err, jc.ErrorIsNil)
	st, err := s.State.ForModel(tag)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	lsl := state.NewLastSentLogger(st, sink)

	err = lsl.Set(timestamp)
	c.Assert(err, jc.ErrorIsNil)
}
