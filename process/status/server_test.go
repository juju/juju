package status

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
)

type serverSuite struct{}

var _ = gc.Suite(&serverSuite{})

func (*serverSuite) TestGood(c *gc.C) {
	input := []process.Info{{
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
		Details: process.Details{
			ID: "idfoo",
			Status: process.PluginStatus{
				Label: "process status",
			},
		},
	}}

	i, err := UnitStatus(input)
	c.Assert(err, jc.ErrorIsNil)
	procs, ok := i.([]api.Process)
	if !ok {
		c.Fatalf("Expected []api.Process, got %#v", i)
	}
	expected := []api.Process{api.Proc2api(input[0])}
	c.Assert(procs, gc.DeepEquals, expected)
}
