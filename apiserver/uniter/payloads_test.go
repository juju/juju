// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/uniter"
	"github.com/juju/juju/payload"
)

var _ = gc.Suite(&PayloadsSuite{})

type PayloadsSuite struct {
	testing.IsolationSuite

	state *FakeState
	api   *uniter.PayloadsAPI
}

func (s *PayloadsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.state = &FakeState{&testing.Stub{}}
	s.api = uniter.NewTestPayloadsAPI(s.state)
}

func (s *PayloadsSuite) TestTrackEmpty(c *gc.C) {
	res, err := s.api.TrackPayloads(params.TrackPayloadsParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{})
}

func (s *PayloadsSuite) TestTrackSuccess(c *gc.C) {
	args := params.TrackPayloadsParams{
		Payloads: []params.TrackPayloadParams{{
			Class:  "idfoo",
			Type:   "type",
			ID:     "bar",
			Status: payload.StateRunning,
		}},
	}

	res, err := s.api.TrackPayloads(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}

func (s *PayloadsSuite) TestTrackFailure(c *gc.C) {
	args := params.TrackPayloadsParams{
		Payloads: []params.TrackPayloadParams{{
			Class:  "idfoo",
			Type:   "type",
			ID:     "bar",
			Status: payload.StateRunning,
		}},
	}
	s.state.SetErrors(errors.New("fail"))

	res, err := s.api.TrackPayloads(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{params.ErrorResult{Error: &params.Error{Message: "fail", Code: ""}}}})
}

func (s *PayloadsSuite) TestUntrackEmpty(c *gc.C) {
	res, err := s.api.UntrackPayloads(params.UntrackPayloadsParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{})
}

func (s *PayloadsSuite) TestUntrackSuccess(c *gc.C) {
	args := params.UntrackPayloadsParams{
		Payloads: []params.UntrackPayloadParams{{
			Class: "idfoo",
			ID:    "bar",
		}},
	}

	res, err := s.api.UntrackPayloads(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}

func (s *PayloadsSuite) TestUntrackFailure(c *gc.C) {
	args := params.UntrackPayloadsParams{
		Payloads: []params.UntrackPayloadParams{{
			Class: "idfoo",
			ID:    "bar",
		}},
	}
	s.state.SetErrors(errors.New("fail"))

	res, err := s.api.UntrackPayloads(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{params.ErrorResult{Error: &params.Error{Message: "fail", Code: ""}}}})
}

func (s *PayloadsSuite) TestSetPayloadStatusEmpty(c *gc.C) {
	res, err := s.api.SetPayloadsStatus(params.PayloadsStatusParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{})
}

func (s *PayloadsSuite) TestSetPayloadStatusSuccess(c *gc.C) {
	args := params.PayloadsStatusParams{
		Payloads: []params.PayloadStatusParams{{
			Class:  "idfoo",
			ID:     "bar",
			Status: payload.StateRunning,
		}},
	}

	res, err := s.api.SetPayloadsStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}

func (s *PayloadsSuite) TestSetPayloadStatusFailure(c *gc.C) {
	args := params.PayloadsStatusParams{
		Payloads: []params.PayloadStatusParams{{
			Class:  "idfoo",
			ID:     "bar",
			Status: payload.StateRunning,
		}},
	}
	s.state.SetErrors(errors.New("fail"))

	res, err := s.api.SetPayloadsStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{params.ErrorResult{Error: &params.Error{Message: "fail", Code: ""}}}})
}

type FakeState struct {
	*testing.Stub
}

func (f *FakeState) Track(pl payload.Payload) error {
	f.AddCall("Track", pl)
	return f.NextErr()
}

func (f *FakeState) SetStatus(id, status string) error {
	f.AddCall("SetStatus", id, status)
	return f.NextErr()
}

func (f *FakeState) Untrack(id string) error {
	f.AddCall("Untrack", id)
	return f.NextErr()
}
