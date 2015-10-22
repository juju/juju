// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

type internalHelpersSuite struct{}

var _ = gc.Suite(&internalHelpersSuite{})

func (internalHelpersSuite) TestAPI2Workload(c *gc.C) {
	p := Workload{
		Definition: WorkloadDefinition{
			Name: "foobar",
			Type: "type",
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

func (internalHelpersSuite) TestWorkload2API(c *gc.C) {
	wl := workload.Info{
		PayloadClass: charm.PayloadClass{
			Name: "foobar",
			Type: "type",
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
