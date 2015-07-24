package server

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
	"github.com/juju/juju/state"
	"github.com/juju/names"
)

type statusSuite struct{}

var _ = gc.Suite(&statusSuite{})

func (*statusSuite) TestGood(c *gc.C) {
	f := &fakeState{
		unitProcs: unitProcs{
			procs: []process.Info{{
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
			}},
		},
	}

	tag := names.NewUnitTag("foo/0")
	i, err := UnitStatus(f, tag)
	c.Assert(err, jc.ErrorIsNil)
	procs, ok := i.([]api.Process)
	if !ok {
		c.Fatalf("Expected []api.Process, got %#v", i)
	}
	expected := []api.Process{api.Proc2api(f.unitProcs.procs[0])}
	c.Assert(procs, gc.DeepEquals, expected)
	c.Assert(f.tag, gc.DeepEquals, tag)

}

type fakeState struct {
	unitProcs unitProcs
	err       error
	tag       names.UnitTag
}

func (f *fakeState) UnitProcesses(unit names.UnitTag) (state.UnitProcesses, error) {
	f.tag = unit
	return f.unitProcs, f.err
}

type unitProcs struct {
	state.UnitProcesses
	procs []process.Info
	err   error
}

func (f unitProcs) List(...string) ([]process.Info, error) {
	return f.procs, f.err
}
