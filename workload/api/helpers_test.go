// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/workload"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func (suite) TestAPI2Workload(c *gc.C) {
	p := Workload{
		Definition: WorkloadDefinition{
			Name:        "foobar",
			Description: "desc",
			Type:        "type",
			TypeOptions: map[string]string{"foo": "bar"},
			Command:     "cmd",
			Image:       "img",
			Ports: []WorkloadPort{{
				External: 8080,
				Internal: 80,
				Endpoint: "endpoint",
			}},
			Volumes: []WorkloadVolume{{
				ExternalMount: "/foo/bar",
				InternalMount: "/baz/bat",
				Mode:          "ro",
				Name:          "volname",
			}},
			EnvVars: map[string]string{"envfoo": "bar"},
		},
		Status: WorkloadStatus{
			State:   workload.StateRunning,
			Blocker: "",
			Message: "okay",
		},
		Details: WorkloadDetails{
			ID: "idfoo",
			Status: PluginStatus{
				State: "workload status",
			},
		},
	}

	proc := API2Workload(p)
	p2 := Workload2api(proc)
	c.Assert(p2, gc.DeepEquals, p)
	proc2 := API2Workload(p2)
	c.Assert(proc2, gc.DeepEquals, proc)
}

func (suite) TestWorkload2API(c *gc.C) {
	proc := workload.Info{
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
			Blocker: "",
			Message: "okay",
		},
		Details: workload.Details{
			ID: "idfoo",
			Status: workload.PluginStatus{
				State: "workload status",
			},
		},
	}

	p := Workload2api(proc)
	proc2 := API2Workload(p)
	c.Assert(proc2, gc.DeepEquals, proc)
	p2 := Workload2api(proc2)
	c.Assert(p2, gc.DeepEquals, p)
}
