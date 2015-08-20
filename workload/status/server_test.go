// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api"
)

type serverSuite struct{}

var _ = gc.Suite(&serverSuite{})

func (*serverSuite) TestGood(c *gc.C) {
	input := []workload.Info{{
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
		Details: workload.Details{
			ID: "idfoo",
			Status: workload.PluginStatus{
				State: "workload status",
			},
		},
	}}

	i, err := UnitStatus(input)
	c.Assert(err, jc.ErrorIsNil)
	workloads, ok := i.([]api.Workload)
	if !ok {
		c.Fatalf("Expected []api.Workload, got %#v", i)
	}
	expected := []api.Workload{api.Workload2api(input[0])}
	c.Assert(workloads, gc.DeepEquals, expected)
}
