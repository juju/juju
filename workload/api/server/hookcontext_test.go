// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func (suite) TestTrack(c *gc.C) {
	st := &FakeState{}
	a := HookContextAPI{st}

	args := api.TrackArgs{
		Workloads: []api.Workload{{
			Definition: api.WorkloadDefinition{
				Name:        "foobar",
				Description: "desc",
				Type:        "type",
				TypeOptions: map[string]string{"foo": "bar"},
				Command:     "cmd",
				Image:       "img",
				Ports: []api.WorkloadPort{{
					External: 8080,
					Internal: 80,
					Endpoint: "endpoint",
				}},
				Volumes: []api.WorkloadVolume{{
					ExternalMount: "/foo/bar",
					InternalMount: "/baz/bat",
					Mode:          "ro",
					Name:          "volname",
				}},
				EnvVars: map[string]string{"envfoo": "bar"},
			},
			Status: api.WorkloadStatus{
				State:   workload.StateRunning,
				Message: "okay",
			},
			Details: api.WorkloadDetails{
				ID: "idfoo",
				Status: api.PluginStatus{
					State: "running",
				},
			},
		}},
	}

	res, err := a.Track(args)
	c.Assert(err, jc.ErrorIsNil)

	expectedResults := api.WorkloadResults{
		Results: []api.WorkloadResult{{
			ID:    "foobar/idfoo",
			Error: nil,
		}},
	}

	c.Assert(res, gc.DeepEquals, expectedResults)

	expected := workload.Info{
		Workload: charm.Workload{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []charm.WorkloadPort{
				{
					External: 8080,
					Internal: 80,
					Endpoint: "endpoint",
				},
			},
			Volumes: []charm.WorkloadVolume{
				{
					ExternalMount: "/foo/bar",
					InternalMount: "/baz/bat",
					Mode:          "ro",
					Name:          "volname",
				},
			},
			EnvVars: map[string]string{"envfoo": "bar"},
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

	c.Assert(st.info, gc.DeepEquals, expected)
}

func (suite) TestListOne(c *gc.C) {
	wl := workload.Info{
		Workload: charm.Workload{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []charm.WorkloadPort{
				{
					External: 8080,
					Internal: 80,
					Endpoint: "endpoint",
				},
			},
			Volumes: []charm.WorkloadVolume{
				{
					ExternalMount: "/foo/bar",
					InternalMount: "/baz/bat",
					Mode:          "ro",
					Name:          "volname",
				},
			},
			EnvVars: map[string]string{"envfoo": "bar"},
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
	args := api.ListArgs{
		IDs: []string{"foobar/idfoo"},
	}
	results, err := a.List(args)
	c.Assert(err, jc.ErrorIsNil)

	expected := api.Workload{
		Definition: api.WorkloadDefinition{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []api.WorkloadPort{
				{
					External: 8080,
					Internal: 80,
					Endpoint: "endpoint",
				},
			},
			Volumes: []api.WorkloadVolume{
				{
					ExternalMount: "/foo/bar",
					InternalMount: "/baz/bat",
					Mode:          "ro",
					Name:          "volname",
				},
			},
			EnvVars: map[string]string{"envfoo": "bar"},
		},
		Status: api.WorkloadStatus{
			State:   workload.StateRunning,
			Message: "okay",
		},
		Details: api.WorkloadDetails{
			ID: "idfoo",
			Status: api.PluginStatus{
				State: "running",
			},
		},
	}

	expectedResults := api.ListResults{
		Results: []api.ListResult{{
			ID:    "foobar/idfoo",
			Info:  expected,
			Error: nil,
		}},
	}

	c.Assert(results, gc.DeepEquals, expectedResults)
}

func (suite) TestListAll(c *gc.C) {
	wl := workload.Info{
		Workload: charm.Workload{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []charm.WorkloadPort{
				{
					External: 8080,
					Internal: 80,
					Endpoint: "endpoint",
				},
			},
			Volumes: []charm.WorkloadVolume{
				{
					ExternalMount: "/foo/bar",
					InternalMount: "/baz/bat",
					Mode:          "ro",
					Name:          "volname",
				},
			},
			EnvVars: map[string]string{"envfoo": "bar"},
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
	args := api.ListArgs{}
	results, err := a.List(args)
	c.Assert(err, jc.ErrorIsNil)

	expected := api.Workload{
		Definition: api.WorkloadDefinition{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []api.WorkloadPort{
				{
					External: 8080,
					Internal: 80,
					Endpoint: "endpoint",
				},
			},
			Volumes: []api.WorkloadVolume{
				{
					ExternalMount: "/foo/bar",
					InternalMount: "/baz/bat",
					Mode:          "ro",
					Name:          "volname",
				},
			},
			EnvVars: map[string]string{"envfoo": "bar"},
		},
		Status: api.WorkloadStatus{
			State:   workload.StateRunning,
			Message: "okay",
		},
		Details: api.WorkloadDetails{
			ID: "idfoo",
			Status: api.PluginStatus{
				State: "running",
			},
		},
	}

	expectedResults := api.ListResults{
		Results: []api.ListResult{{
			ID:    "foobar/idfoo",
			Info:  expected,
			Error: nil,
		}},
	}

	c.Assert(results, gc.DeepEquals, expectedResults)
}

func (suite) TestDefinitions(c *gc.C) {
	definition := charm.Workload{
		Name:        "foobar",
		Description: "desc",
		Type:        "type",
		TypeOptions: map[string]string{"foo": "bar"},
		Command:     "cmd",
		Image:       "img",
		Ports: []charm.WorkloadPort{
			{
				External: 8080,
				Internal: 80,
				Endpoint: "endpoint",
			},
		},
		Volumes: []charm.WorkloadVolume{
			{
				ExternalMount: "/foo/bar",
				InternalMount: "/baz/bat",
				Mode:          "ro",
				Name:          "volname",
			},
		},
		EnvVars: map[string]string{"envfoo": "bar"},
	}
	st := &FakeState{defs: []charm.Workload{definition}}
	a := HookContextAPI{st}

	results, err := a.Definitions()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.DefinitionsResults{
		Results: []api.WorkloadDefinition{{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []api.WorkloadPort{
				{
					External: 8080,
					Internal: 80,
					Endpoint: "endpoint",
				},
			},
			Volumes: []api.WorkloadVolume{
				{
					ExternalMount: "/foo/bar",
					InternalMount: "/baz/bat",
					Mode:          "ro",
					Name:          "volname",
				},
			},
			EnvVars: map[string]string{"envfoo": "bar"},
		}},
	})
}

func (suite) TestSetStatus(c *gc.C) {
	st := &FakeState{}
	a := HookContextAPI{st}
	args := api.SetStatusArgs{
		Args: []api.SetStatusArg{{
			ID: "fooID/bar",
			Status: api.WorkloadStatus{
				State:   workload.StateRunning,
				Message: "okay",
			},
			PluginStatus: api.PluginStatus{
				State: "statusfoo",
			},
		}},
	}
	res, err := a.SetStatus(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(st.id, gc.Equals, "fooID/bar")
	c.Assert(st.status, jc.DeepEquals, workload.CombinedStatus{
		Status: workload.Status{
			State:   workload.StateRunning,
			Message: "okay",
		},
		PluginStatus: workload.PluginStatus{
			State: "statusfoo",
		},
	})

	expected := api.WorkloadResults{
		Results: []api.WorkloadResult{{
			ID:    "fooID/bar",
			Error: nil,
		}},
	}
	c.Assert(res, gc.DeepEquals, expected)
}

func (suite) TestUntrack(c *gc.C) {
	st := &FakeState{}
	a := HookContextAPI{st}
	args := api.UntrackArgs{
		IDs: []string{"fooID/bar"},
	}
	res, err := a.Untrack(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(st.id, gc.Equals, "fooID/bar")

	expected := api.WorkloadResults{
		Results: []api.WorkloadResult{{
			ID:    "fooID/bar",
			Error: nil,
		}},
	}
	c.Assert(res, gc.DeepEquals, expected)
}

func (suite) TestUntrackEmptyID(c *gc.C) {
	st := &FakeState{}
	a := HookContextAPI{st}
	args := api.UntrackArgs{
		IDs: []string{""},
	}
	res, err := a.Untrack(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(st.id, gc.Equals, "")

	expected := api.WorkloadResults{Results: []api.WorkloadResult{api.WorkloadResult{ID: ""}}}
	c.Assert(res, gc.DeepEquals, expected)
}

func (suite) TestUntrackEmpty(c *gc.C) {
	st := &FakeState{}
	st.id = "foo"
	a := HookContextAPI{st}
	args := api.UntrackArgs{
		IDs: []string{},
	}
	res, err := a.Untrack(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(st.id, gc.Equals, "foo")

	expected := api.WorkloadResults{}
	c.Assert(res, gc.DeepEquals, expected)
}

type FakeState struct {
	// inputs
	id     string
	ids    []string
	status workload.CombinedStatus

	// info is used as input and output
	info workload.Info

	//outputs
	workloads []workload.Info
	defs      []charm.Workload
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

func (f *FakeState) Definitions() ([]charm.Workload, error) {
	return f.defs, f.err
}

func (f *FakeState) SetStatus(id string, status workload.CombinedStatus) error {
	f.id = id
	f.status = status
	return f.err
}

func (f *FakeState) Untrack(id string) error {
	f.id = id
	return f.err
}
