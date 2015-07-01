// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
	"github.com/juju/juju/process/plugin"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func (suite) TestRegisterProcess(c *gc.C) {
	st := &FakeState{}
	a := HookContextAPI{st}

	args := api.RegisterProcessesArgs{
		Processes: []api.RegisterProcessArg{{
			UnitTag: "foo/0",
			ProcessInfo: api.ProcessInfo{
				Process: api.Process{
					Name:        "foobar",
					Description: "desc",
					Type:        "type",
					TypeOptions: map[string]string{"foo": "bar"},
					Command:     "cmd",
					Image:       "img",
					Ports: []api.ProcessPort{{
						External: 8080,
						Internal: 80,
						Endpoint: "endpoint",
					}},
					Volumes: []api.ProcessVolume{{
						ExternalMount: "/foo/bar",
						InternalMount: "/baz/bat",
						Mode:          "ro",
						Name:          "volname",
					}},
					EnvVars: map[string]string{"envfoo": "bar"},
				},
				Status: 5,
				Details: api.ProcDetails{
					ID:         "idfoo",
					ProcStatus: api.ProcStatus{Status: "process status"},
				},
			},
		}},
	}

	res, err := a.RegisterProcesses(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.unit, gc.Equals, names.NewUnitTag("foo/0"))

	expectedResults := api.ProcessResults{
		Results: []api.ProcessResult{{
			ID:    "idfoo",
			Error: nil,
		}},
	}

	c.Assert(res, gc.DeepEquals, expectedResults)

	expected := process.Info{
		Process: charm.Process{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []charm.ProcessPort{
				{
					External: 8080,
					Internal: 80,
					Endpoint: "endpoint",
				},
			},
			Volumes: []charm.ProcessVolume{
				{
					ExternalMount: "/foo/bar",
					InternalMount: "/baz/bat",
					Mode:          "ro",
					Name:          "volname",
				},
			},
			EnvVars: map[string]string{"envfoo": "bar"},
		},
		Status: 5,
		Details: plugin.ProcDetails{
			ID:         "idfoo",
			ProcStatus: plugin.ProcStatus{Status: "process status"},
		},
	}

	c.Assert(st.info, gc.DeepEquals, expected)
}

func (suite) TestListProcesses(c *gc.C) {
	proc := process.Info{
		Process: charm.Process{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []charm.ProcessPort{
				{
					External: 8080,
					Internal: 80,
					Endpoint: "endpoint",
				},
			},
			Volumes: []charm.ProcessVolume{
				{
					ExternalMount: "/foo/bar",
					InternalMount: "/baz/bat",
					Mode:          "ro",
					Name:          "volname",
				},
			},
			EnvVars: map[string]string{"envfoo": "bar"},
		},
		Status: 5,
		Details: plugin.ProcDetails{
			ID:         "idfoo",
			ProcStatus: plugin.ProcStatus{Status: "process status"},
		},
	}
	st := &FakeState{info: proc}
	a := HookContextAPI{st}
	args := api.ListProcessesArgs{
		UnitTag: "foo/0",
		IDs:     []string{"idfoo"},
	}
	results, err := a.ListProcesses(args)
	c.Assert(err, jc.ErrorIsNil)

	expected := api.ProcessInfo{
		Process: api.Process{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []api.ProcessPort{
				{
					External: 8080,
					Internal: 80,
					Endpoint: "endpoint",
				},
			},
			Volumes: []api.ProcessVolume{
				{
					ExternalMount: "/foo/bar",
					InternalMount: "/baz/bat",
					Mode:          "ro",
					Name:          "volname",
				},
			},
			EnvVars: map[string]string{"envfoo": "bar"},
		},
		Status: 5,
		Details: api.ProcDetails{
			ID:         "idfoo",
			ProcStatus: api.ProcStatus{Status: "process status"},
		},
	}

	expectedResults := api.ListProcessesResults{
		Results: []api.ListProcessResult{{
			ID:    "idfoo",
			Info:  expected,
			Error: nil,
		}},
	}

	c.Assert(results, gc.DeepEquals, expectedResults)
}

func (suite) TestSetProcessStatus(c *gc.C) {
	st := &FakeState{}
	a := HookContextAPI{st}
	args := api.SetProcessesStatusArgs{
		Args: []api.SetProcessStatusArg{{
			UnitTag: "foo/0",
			ID:      "fooID",
			Status:  api.ProcStatus{Status: "statusfoo"},
		}},
	}
	res, err := a.SetProcessesStatus(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(st.id, gc.Equals, "fooID")
	c.Assert(st.unit, gc.Equals, names.NewUnitTag("foo/0"))
	c.Assert(st.status, gc.Equals, "statusfoo")

	expected := api.ProcessResults{
		Results: []api.ProcessResult{{
			ID:    "fooID",
			Error: nil,
		}},
	}
	c.Assert(res, gc.DeepEquals, expected)
}

func (suite) TestUnregisterProcesses(c *gc.C) {
	st := &FakeState{}
	a := HookContextAPI{st}
	args := api.UnregisterProcessesArgs{
		UnitTag: "foo/0",
		IDs:     []string{"fooID"},
	}
	res, err := a.UnregisterProcesses(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(st.id, gc.Equals, "fooID")
	c.Assert(st.unit, gc.Equals, names.NewUnitTag("foo/0"))

	expected := api.ProcessResults{
		Results: []api.ProcessResult{{
			ID:    "fooID",
			Error: nil,
		}},
	}
	c.Assert(res, gc.DeepEquals, expected)
}

type FakeState struct {
	// inputs
	unit   names.UnitTag
	id     string
	status string

	// info is used as input and output
	info process.Info

	//outputs
	err error
}

func (f *FakeState) RegisterProcess(unit names.UnitTag, info process.Info) error {
	f.unit = unit
	f.info = info
	return f.err
}
func (f *FakeState) ListProcess(unit names.UnitTag, id string) (process.Info, error) {
	f.unit = unit
	f.id = id
	return f.info, f.err
}

func (f *FakeState) SetProcessStatus(unit names.UnitTag, id string, status string) error {
	f.unit = unit
	f.id = id
	f.status = status
	return f.err
}

func (f *FakeState) UnregisterProcess(unit names.UnitTag, id string) error {
	f.unit = unit
	f.id = id
	return f.err
}
