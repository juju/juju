// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api"
	"github.com/juju/juju/workload/api/internal"
)

var _ = gc.Suite(&suite{})

type suite struct {
	stub  *testing.Stub
	state *FakeState
}

func (s *suite) SetUpTest(c *gc.C) {
	s.stub = &testing.Stub{}
	s.state = &FakeState{stub: s.stub}
}

func (s *suite) TestTrack(c *gc.C) {
	a := HookContextAPI{s.state}

	args := internal.TrackArgs{
		Workloads: []internal.Workload{{
			Definition: internal.WorkloadDefinition{
				Name: "foobar",
				Type: "type",
			},
			Status: internal.WorkloadStatus{
				State:   workload.StateRunning,
				Message: "okay",
			},
			Details: internal.WorkloadDetails{
				ID: "idfoo",
				Status: internal.PluginStatus{
					State: "running",
				},
			},
		}},
	}

	res, err := a.Track(args)
	c.Assert(err, jc.ErrorIsNil)

	expectedResults := internal.WorkloadResults{
		Results: []internal.WorkloadResult{{
			ID: internal.FullID{
				Class: "foobar",
				ID:    "idfoo",
			},
			Error: nil,
		}},
	}

	c.Assert(res, gc.DeepEquals, expectedResults)

	expected := workload.Info{
		PayloadClass: charm.PayloadClass{
			Name: "foobar",
			Type: "type",
		},
		Status: workload.Status{
			State:   workload.StateRunning,
			Message: "okay",
		},
		Tags: []string{},
		Details: workload.Details{
			ID: "idfoo",
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}

	c.Check(s.state.info, jc.DeepEquals, expected)
}

func (s *suite) TestListOne(c *gc.C) {
	wl := workload.Info{
		PayloadClass: charm.PayloadClass{
			Name: "foobar",
			Type: "type",
		},
		Status: workload.Status{
			State:   workload.StateRunning,
			Message: "okay",
		},
		Details: workload.Details{
			ID: "idfoo",
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}
	s.state.workloads = []workload.Info{wl}

	a := HookContextAPI{s.state}
	args := internal.ListArgs{
		IDs: []internal.FullID{{
			Class: "foobar",
			ID:    "idfoo",
		}},
	}
	results, err := a.List(args)
	c.Assert(err, jc.ErrorIsNil)

	expected := internal.Workload{
		Definition: internal.WorkloadDefinition{
			Name: "foobar",
			Type: "type",
		},
		Status: internal.WorkloadStatus{
			State:   workload.StateRunning,
			Message: "okay",
		},
		Tags: []string{},
		Details: internal.WorkloadDetails{
			ID: "idfoo",
			Status: internal.PluginStatus{
				State: "running",
			},
		},
	}

	expectedResults := internal.ListResults{
		Results: []internal.ListResult{{
			ID: internal.FullID{
				Class: "foobar",
				ID:    "idfoo",
			},
			Info:  expected,
			Error: nil,
		}},
	}

	c.Assert(results, gc.DeepEquals, expectedResults)
}

func (s *suite) TestListAll(c *gc.C) {
	wl := workload.Info{
		PayloadClass: charm.PayloadClass{
			Name: "foobar",
			Type: "type",
		},
		Status: workload.Status{
			State:   workload.StateRunning,
			Message: "okay",
		},
		Details: workload.Details{
			ID: "idfoo",
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}
	s.state.workloads = []workload.Info{wl}

	a := HookContextAPI{s.state}
	args := internal.ListArgs{}
	results, err := a.List(args)
	c.Assert(err, jc.ErrorIsNil)

	expected := internal.Workload{
		Definition: internal.WorkloadDefinition{
			Name: "foobar",
			Type: "type",
		},
		Status: internal.WorkloadStatus{
			State:   workload.StateRunning,
			Message: "okay",
		},
		Tags: []string{},
		Details: internal.WorkloadDetails{
			ID: "idfoo",
			Status: internal.PluginStatus{
				State: "running",
			},
		},
	}

	expectedResults := internal.ListResults{
		Results: []internal.ListResult{{
			ID: internal.FullID{
				Class: "foobar",
				ID:    "idfoo",
			},
			Info:  expected,
			Error: nil,
		}},
	}

	c.Assert(results, gc.DeepEquals, expectedResults)
}

func (s *suite) TestLookUpOkay(c *gc.C) {
	s.state.stateIDs = []string{"ce5bc2a7-65d8-4800-8199-a7c3356ab309"}

	a := HookContextAPI{s.state}
	args := internal.LookUpArgs{
		Args: []internal.LookUpArg{{
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

	c.Check(res, jc.DeepEquals, internal.LookUpResults{
		Results: []internal.LookUpResult{{
			ID:       names.NewPayloadTag("ce5bc2a7-65d8-4800-8199-a7c3356ab309"),
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
	notFound := errors.NotFoundf("workload")
	s.stub.SetErrors(nil, notFound, nil)

	a := HookContextAPI{s.state}
	args := internal.LookUpArgs{
		Args: []internal.LookUpArg{{
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
	c.Check(res, jc.DeepEquals, internal.LookUpResults{
		Results: []internal.LookUpResult{{
			ID:       names.NewPayloadTag("ce5bc2a7-65d8-4800-8199-a7c3356ab309"),
			NotFound: false,
			Error:    nil,
		}, {
			ID:       names.PayloadTag{},
			NotFound: true,
			Error:    common.ServerError(notFound),
		}, {
			ID:       names.NewPayloadTag("ce5bc2a7-65d8-4800-8199-a7c3356ab311"),
			NotFound: false,
			Error:    nil,
		}},
		Error: common.ServerError(api.BulkFailure),
	})
}

func (s *suite) TestSetStatus(c *gc.C) {
	s.state.stateIDs = []string{"ce5bc2a7-65d8-4800-8199-a7c3356ab309"}

	a := HookContextAPI{s.state}
	args := internal.SetStatusArgs{
		Args: []internal.SetStatusArg{{
			ID: internal.FullID{
				Class: "fooID",
				ID:    "bar",
			},
			Status: workload.StateRunning,
		}},
	}
	res, err := a.SetStatus(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.state.id, gc.Equals, "ce5bc2a7-65d8-4800-8199-a7c3356ab309")
	c.Assert(s.state.status, gc.Equals, workload.StateRunning)

	expected := internal.WorkloadResults{
		Results: []internal.WorkloadResult{{
			ID: internal.FullID{
				Class: "fooID",
				ID:    "bar",
			},
			Error: nil,
		}},
	}
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *suite) TestUntrack(c *gc.C) {
	s.state.stateIDs = []string{"ce5bc2a7-65d8-4800-8199-a7c3356ab309"}

	a := HookContextAPI{s.state}
	args := internal.UntrackArgs{
		IDs: []internal.FullID{{
			Class: "fooID",
			ID:    "bar",
		}},
	}
	res, err := a.Untrack(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.state.id, gc.Equals, "ce5bc2a7-65d8-4800-8199-a7c3356ab309")

	expected := internal.WorkloadResults{
		Results: []internal.WorkloadResult{{
			ID: internal.FullID{
				Class: "fooID",
				ID:    "bar",
			},
			Error: nil,
		}},
	}
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *suite) TestUntrackEmptyID(c *gc.C) {
	a := HookContextAPI{s.state}
	args := internal.UntrackArgs{
		IDs: []internal.FullID{
			{},
		},
	}
	res, err := a.Untrack(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.state.id, gc.Equals, "")

	expected := internal.WorkloadResults{
		Results: []internal.WorkloadResult{{
			ID: internal.FullID{
				Class: "",
				ID:    "",
			},
			Error: nil,
		}},
	}
	c.Assert(res, gc.DeepEquals, expected)
}

func (s *suite) TestUntrackNoIDs(c *gc.C) {
	s.state.id = "foo"

	a := HookContextAPI{s.state}
	args := internal.UntrackArgs{
		IDs: []internal.FullID{},
	}
	res, err := a.Untrack(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.state.id, gc.Equals, "foo")

	expected := internal.WorkloadResults{}
	c.Assert(res, gc.DeepEquals, expected)
}

type FakeState struct {
	stub *testing.Stub

	// inputs
	id     string
	ids    []string
	status string

	// info is used as input and output
	info workload.Info

	//outputs
	stateIDs  []string
	workloads []workload.Info
}

func (f *FakeState) nextID() string {
	if len(f.stateIDs) == 0 {
		return ""
	}
	id := f.stateIDs[0]
	f.stateIDs = f.stateIDs[1:]
	return id
}

func (f *FakeState) Track(info workload.Info) error {
	f.info = info
	if err := f.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (f *FakeState) List(ids ...string) ([]workload.Info, error) {
	f.ids = ids
	if err := f.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return f.workloads, nil
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
