// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func (suite) TestPayload2api(c *gc.C) {
	apiPayload := Payload2api(workload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "spam",
			Type: "docker",
		},
		ID:      "idspam",
		Status:  workload.StateRunning,
		Tags:    []string{"a-tag"},
		Unit:    "unit-a-service-0",
		Machine: "1",
	})

	c.Check(apiPayload, jc.DeepEquals, Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  workload.StateRunning,
		Tags:    []string{"a-tag"},
		Unit:    "unit-a-service-0",
		Machine: "1",
	})
}

func (suite) TestAPI2Payload(c *gc.C) {
	payload := API2Payload(Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  workload.StateRunning,
		Tags:    []string{"a-tag"},
		Unit:    "unit-a-service-0",
		Machine: "1",
	})

	c.Check(payload, jc.DeepEquals, workload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "spam",
			Type: "docker",
		},
		ID:      "idspam",
		Status:  workload.StateRunning,
		Tags:    []string{"a-tag"},
		Unit:    "unit-a-service-0",
		Machine: "1",
	})
}

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
		Tags: []string{},
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
		Tags: []string{},
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
