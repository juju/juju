// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloadshookcontext_test

import (
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	unitfacade "github.com/juju/juju/apiserver/facades/agent/payloadshookcontext"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
)

var _ = gc.Suite(&suite{})

type suite struct {
	testing.IsolationSuite

	stub  *testing.Stub
	state *FakeState
}

func (s *suite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.state = &FakeState{stub: s.stub}
}

func (s *suite) TestTrack(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	s.state.stateIDs = []string{id}

	a := unitfacade.NewUnitFacade(s.state)

	args := params.TrackPayloadArgs{
		Payloads: []params.Payload{{
			Class:  "idfoo",
			Type:   "type",
			ID:     "bar",
			Status: payload.StateRunning,
		}},
	}

	res, err := a.Track(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(res, jc.DeepEquals, params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
			Payload:  nil,
			NotFound: false,
			Error:    nil,
		}},
	})

	c.Check(s.state.payload, jc.DeepEquals, payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "idfoo",
			Type: "type",
		},
		Status: payload.StateRunning,
		Labels: []string{},
		ID:     "bar",
	})
}

func (s *suite) TestListOne(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	pl := payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "foobar",
			Type: "type",
		},
		ID:     "idfoo",
		Status: payload.StateRunning,
		Unit:   "a-application/0",
	}
	s.state.payloads = []payload.Result{{
		ID: id,
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}

	a := unitfacade.NewUnitFacade(s.state)
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewPayloadTag(id).String(),
		}},
	}
	results, err := a.List(args)
	c.Assert(err, jc.ErrorIsNil)

	expected := params.Payload{
		Class:   "foobar",
		Type:    "type",
		ID:      "idfoo",
		Status:  payload.StateRunning,
		Labels:  []string{},
		Unit:    "unit-a-application-0",
		Machine: "machine-1",
	}

	c.Check(results, jc.DeepEquals, params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
			Payload:  &expected,
			NotFound: false,
			Error:    nil,
		}},
	})
}

func (s *suite) TestListAll(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	s.state.stateIDs = []string{id}
	pl := payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "foobar",
			Type: "type",
		},
		ID:     "idfoo",
		Status: payload.StateRunning,
		Unit:   "a-application/0",
	}
	s.state.payloads = []payload.Result{{
		ID: id,
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: "1",
		},
	}}

	a := unitfacade.NewUnitFacade(s.state)
	args := params.Entities{}
	results, err := a.List(args)
	c.Assert(err, jc.ErrorIsNil)

	expected := params.Payload{
		Class:   "foobar",
		Type:    "type",
		ID:      "idfoo",
		Status:  payload.StateRunning,
		Labels:  []string{},
		Unit:    "unit-a-application-0",
		Machine: "machine-1",
	}
	c.Check(results, jc.DeepEquals, params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
			Payload:  &expected,
			NotFound: false,
			Error:    nil,
		}},
	})
}

func (s *suite) TestLookUpOkay(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	s.state.stateIDs = []string{id}

	a := unitfacade.NewUnitFacade(s.state)
	args := params.LookUpPayloadArgs{
		Args: []params.LookUpPayloadArg{{
			Name: "fooID",
			ID:   "bar",
		}},
	}
	res, err := a.LookUp(args)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "LookUp",
		Args:     []interface{}{"fooID", "bar"},
	}})

	c.Check(res, jc.DeepEquals, params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
			NotFound: false,
			Error:    nil,
		}},
	})
}

func (s *suite) TestLookUpMixed(c *gc.C) {
	s.state.stateIDs = []string{
		"ce5bc2a7-65d8-4800-8199-a7c3356ab309",
		"",
		"ce5bc2a7-65d8-4800-8199-a7c3356ab311",
	}
	notFound := errors.NotFoundf("payload")
	s.stub.SetErrors(nil, notFound, nil)

	a := unitfacade.NewUnitFacade(s.state)
	args := params.LookUpPayloadArgs{
		Args: []params.LookUpPayloadArg{{
			Name: "fooID",
			ID:   "bar",
		}, {
			Name: "bazID",
			ID:   "bam",
		}, {
			Name: "spam",
			ID:   "eggs",
		}},
	}
	res, err := a.LookUp(args)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "LookUp", "LookUp", "LookUp")
	c.Check(res, jc.DeepEquals, params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag("ce5bc2a7-65d8-4800-8199-a7c3356ab309").String(),
			},
			NotFound: false,
			Error:    nil,
		}, {
			Entity: params.Entity{
				Tag: "",
			},
			NotFound: true,
			Error:    common.ServerError(notFound),
		}, {
			Entity: params.Entity{
				Tag: names.NewPayloadTag("ce5bc2a7-65d8-4800-8199-a7c3356ab311").String(),
			},
			NotFound: false,
			Error:    nil,
		}},
	})
}

func (s *suite) TestSetStatus(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	s.state.stateIDs = []string{id}
	s.state.stateIDs = []string{"ce5bc2a7-65d8-4800-8199-a7c3356ab309"}

	a := unitfacade.NewUnitFacade(s.state)
	args := params.SetPayloadStatusArgs{
		Args: []params.SetPayloadStatusArg{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
			Status: payload.StateRunning,
		}},
	}
	res, err := a.SetStatus(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.state.id, gc.Equals, id)
	c.Assert(s.state.status, gc.Equals, payload.StateRunning)

	expected := params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
			Error: nil,
		}},
	}
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *suite) TestUntrack(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	s.state.stateIDs = []string{id}

	a := unitfacade.NewUnitFacade(s.state)
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewPayloadTag(id).String(),
		}},
	}
	res, err := a.Untrack(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.state.id, gc.Equals, id)

	expected := params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: names.NewPayloadTag(id).String(),
			},
			Error: nil,
		}},
	}
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *suite) TestUntrackEmptyID(c *gc.C) {
	a := unitfacade.NewUnitFacade(s.state)
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: "",
		}},
	}
	res, err := a.Untrack(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.state.id, gc.Equals, "")

	expected := params.PayloadResults{
		Results: []params.PayloadResult{{
			Entity: params.Entity{
				Tag: "",
			},
			Error: nil,
		}},
	}
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *suite) TestUntrackNoIDs(c *gc.C) {
	id := "ce5bc2a7-65d8-4800-8199-a7c3356ab309"
	s.state.id = id

	a := unitfacade.NewUnitFacade(s.state)
	args := params.Entities{
		Entities: []params.Entity{},
	}
	res, err := a.Untrack(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.state.id, gc.Equals, id)

	expected := params.PayloadResults{}
	c.Assert(res, gc.DeepEquals, expected)
}

type FakeState struct {
	stub *testing.Stub

	// inputs
	id      string
	ids     []string
	status  string
	payload payload.Payload

	//outputs
	stateIDs []string
	payloads []payload.Result
}

func (f *FakeState) nextID() string {
	if len(f.stateIDs) == 0 {
		return ""
	}
	id := f.stateIDs[0]
	f.stateIDs = f.stateIDs[1:]
	return id
}

func (f *FakeState) Track(pl payload.Payload) error {
	f.payload = pl
	if err := f.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (f *FakeState) List(ids ...string) ([]payload.Result, error) {
	f.ids = ids
	if err := f.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return f.payloads, nil
}

func (f *FakeState) SetStatus(id, status string) error {
	f.id = id
	f.status = status
	if err := f.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (f *FakeState) LookUp(name, rawID string) (string, error) {
	f.stub.AddCall("LookUp", name, rawID)
	id := f.nextID()
	if err := f.stub.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	return id, nil
}

func (f *FakeState) Untrack(id string) error {
	f.id = id
	if err := f.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
