// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/plugin"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func (suite) TestAPI2Proc(c *gc.C) {
	p := ProcessInfo{
		Process: Process{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []ProcessPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "endpoint",
			}},
			Volumes: []ProcessVolume{{
				ExternalMount: "/foo/bar",
				InternalMount: "/baz/bat",
				Mode:          "ro",
				Name:          "volname",
			}},
			EnvVars: map[string]string{"envfoo": "bar"},
		},
		Status: 5,
		Details: ProcDetails{
			ID:         "idfoo",
			ProcStatus: ProcStatus{Status: "process status"},
		},
	}

	proc := API2Proc(p)
	p2 := Proc2api(proc)
	c.Assert(p2, gc.DeepEquals, p)
	proc2 := API2Proc(p2)
	c.Assert(proc2, gc.DeepEquals, proc)
}

func (suite) TestProc2API(c *gc.C) {
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

	p := Proc2api(proc)
	proc2 := API2Proc(p)
	c.Assert(proc2, gc.DeepEquals, proc)
	p2 := Proc2api(proc2)
	c.Assert(p2, gc.DeepEquals, p)
}
