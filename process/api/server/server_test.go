// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
	"github.com/juju/juju/process/plugin"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func (suite) TestNewAPI(c *gc.C) {
	st := &FakeState{}
	a, err := NewAPI(st, fakeAuth{b: true})
	c.Assert(err, jc.ErrorIsNil)
	_, ok := a.st.(*FakeState)

	c.Assert(ok, jc.IsTrue)
}

func (suite) TestNewAPIError(c *gc.C) {
	st := &FakeState{}
	_, err := NewAPI(st, fakeAuth{b: false})
	c.Assert(errors.Cause(err), gc.Equals, common.ErrPerm)
}

func (suite) TestRegisterProcess(c *gc.C) {
	st := &FakeState{}
	a, err := NewAPI(st, fakeAuth{b: true})
	c.Assert(err, jc.ErrorIsNil)

	args := api.RegisterProcessArgs{
		UnitTag: "foo/0",
		ProcessInfo: api.ProcessInfo{
			Process: api.Process{
				Name:        "foo/0",
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
		},
	}

	err = a.RegisterProcess(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(st.unit, gc.Equals, names.NewUnitTag(args.UnitTag))

	expected := process.Info{
		Process: charm.Process{
			Name:        "foo/0",
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
			Name:        "foo/0",
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
	st := &FakeState{infos: []process.Info{proc}}
	a, err := NewAPI(st, fakeAuth{b: true})
	c.Assert(err, jc.ErrorIsNil)
	args := api.ListProcessesArgs{
		UnitTag: "foo/0",
		IDs:     []string{"foo", "bar"},
	}
	procs, err := a.ListProcesses(args)
	c.Assert(err, jc.ErrorIsNil)

	expected := api.ProcessInfo{
		Process: api.Process{
			Name:        "foo/0",
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
	c.Assert(procs, gc.DeepEquals, []api.ProcessInfo{expected})
}

func (suite) TestSetProcessStatus(c *gc.C) {
	st := &FakeState{}
	a, err := NewAPI(st, fakeAuth{b: true})
	c.Assert(err, jc.ErrorIsNil)
	args := api.SetProcessStatusArgs{
		UnitTag: "foo/0",
		ID:      "fooID",
		Status:  api.ProcStatus{Status: "statusfoo"},
	}
	err = a.SetProcessStatus(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(st.id, gc.Equals, args.ID)
	c.Assert(st.unit, gc.Equals, names.NewUnitTag(args.UnitTag))
	c.Assert(st.status, gc.Equals, args.Status.Status)
}

func (suite) TestUnregisterTransaction(c *gc.C) {
	st := &FakeState{}
	a, err := NewAPI(st, fakeAuth{b: true})
	c.Assert(err, jc.ErrorIsNil)
	args := api.UnregisterProcessArgs{
		UnitTag: "foo/0",
		ID:      "fooID",
	}
	err = a.UnregisterProcess(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(st.id, gc.Equals, args.ID)
	c.Assert(st.unit, gc.Equals, names.NewUnitTag(args.UnitTag))
}

type FakeState struct {
	// inputs
	info   process.Info
	unit   names.UnitTag
	ids    []string
	id     string
	status string
	//outputs
	err   error
	infos []process.Info
}

func (f *FakeState) RegisterProcess(unit names.UnitTag, info process.Info) error {
	f.unit = unit
	f.info = info
	return f.err
}
func (f *FakeState) ListProcesses(unit names.UnitTag, ids []string) ([]process.Info, error) {
	f.unit = unit
	f.ids = ids
	return f.infos, f.err
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

type fakeAuth struct {
	b bool
	common.Authorizer
}

func (f fakeAuth) AuthUnitAgent() bool {
	return f.b
}
