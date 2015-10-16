// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api/internal"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func (suite) TestTrack(c *gc.C) {
	st := &FakeState{}
	a := HookContextAPI{st}

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

	c.Assert(st.info, gc.DeepEquals, expected)
}

func (suite) TestListOne(c *gc.C) {
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
	st := &FakeState{workloads: []workload.Info{wl}}
	a := HookContextAPI{st}
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

func (suite) TestListAll(c *gc.C) {
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
	st := &FakeState{workloads: []workload.Info{wl}}
	a := HookContextAPI{st}
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

func (suite) TestSetStatus(c *gc.C) {
	st := &FakeState{}
	a := HookContextAPI{st}
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

	c.Check(st.id, gc.Equals, "fooID/bar")
	c.Assert(st.status, gc.Equals, workload.StateRunning)

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

func (suite) TestUntrack(c *gc.C) {
	st := &FakeState{}
	a := HookContextAPI{st}
	args := internal.UntrackArgs{
		IDs: []internal.FullID{{
			Class: "fooID",
			ID:    "bar",
		}},
	}
	res, err := a.Untrack(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(st.id, gc.Equals, "fooID/bar")

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

func (suite) TestUntrackEmptyID(c *gc.C) {
	st := &FakeState{}
	a := HookContextAPI{st}
	args := internal.UntrackArgs{
		IDs: []internal.FullID{
			{},
		},
	}
	res, err := a.Untrack(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(st.id, gc.Equals, "")

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

func (suite) TestUntrackEmpty(c *gc.C) {
	st := &FakeState{}
	st.id = "foo"
	a := HookContextAPI{st}
	args := internal.UntrackArgs{
		IDs: []internal.FullID{},
	}
	res, err := a.Untrack(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(st.id, gc.Equals, "foo")

	expected := internal.WorkloadResults{}
	c.Assert(res, gc.DeepEquals, expected)
}

type FakeState struct {
	// inputs
	id     string
	ids    []string
	status string

	// info is used as input and output
	info workload.Info

	//outputs
	workloads []workload.Info
	defs      []charm.PayloadClass
	err       error
}

func (f *FakeState) Track(info workload.Info) error {
	f.info = info
	return f.err
}

func (f *FakeState) List(ids ...string) ([]workload.Info, error) {
	f.ids = ids
	return f.workloads, f.err
}

func (f *FakeState) SetStatus(id, status string) error {
	f.id = id
	f.status = status
	return f.err
}

func (f *FakeState) Untrack(id string) error {
	f.id = id
	return f.err
}
