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

	wl := API2Workload(p)
	p2 := Workload2api(wl)
	c.Assert(p2, gc.DeepEquals, p)
	wl2 := API2Workload(p2)
	c.Assert(wl2, gc.DeepEquals, wl)
}

func (suite) TestWorkload2API(c *gc.C) {
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

	w := Workload2api(wl)
	wl2 := API2Workload(w)
	c.Assert(wl2, gc.DeepEquals, wl)
	w2 := Workload2api(wl2)
	c.Assert(w2, gc.DeepEquals, w)
}
