// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"bytes"

	"github.com/juju/charm/v12"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/naturalsort"
)

type modelSuite struct{}

var _ = gc.Suite(&modelSuite{})

func (*modelSuite) TestEmtpyModel(c *gc.C) {
	model := &Model{}
	c.Check(model.GetApplication("foo"), gc.IsNil)
	c.Check(model.HasRelation("a", "b", "c", "d"), jc.IsFalse)
	machines := model.unitMachinesWithoutApp("foo", "bar", "")
	c.Check(machines, gc.HasLen, 0)
	c.Check(machines, gc.NotNil)
}

func (*modelSuite) TestGetApplication(c *gc.C) {
	app := &Application{Name: "foo"}
	model := &Model{Applications: map[string]*Application{"foo": app}}
	c.Assert(model.GetApplication("foo"), jc.DeepEquals, app)
}

func (*modelSuite) TestHasCharmNilApplications(c *gc.C) {
	model := &Model{}
	c.Assert(model.hasCharm("foo", -1), jc.IsFalse)
}

func (*modelSuite) TestHasCharm(c *gc.C) {
	app := &Application{
		Name:     "foo",
		Charm:    "ch:foo",
		Revision: -1,
	}
	model := &Model{
		Applications: map[string]*Application{
			"foo": app},
	}
	// Match must be exact.
	c.Assert(model.hasCharm("foo", -1), jc.IsFalse)
	c.Assert(model.hasCharm("ch:foo", -1), jc.IsTrue)
}

func (*modelSuite) TestHasRelation(c *gc.C) {
	model := &Model{
		Relations: []Relation{
			{
				App1:      "django",
				Endpoint1: "pgsql",
				App2:      "postgresql",
				Endpoint2: "db",
			},
		},
	}
	c.Check(model.HasRelation("django", "pgsql", "postgresql", "db"), jc.IsTrue)
	c.Check(model.HasRelation("django", "pgsql", "mysql", "db"), jc.IsFalse)
	c.Check(model.HasRelation("postgresql", "db", "django", "pgsql"), jc.IsTrue)
}

func (*modelSuite) TestUnitMachinesWithoutAppSourceNoTarget(c *gc.C) {
	model := &Model{
		Applications: map[string]*Application{
			"django": {
				Units: []Unit{
					{"django/0", "0"},
					{"django/1", "10"},
					{"django/2", "2"},
				},
			},
		},
	}
	machines := model.unitMachinesWithoutApp("django", "nginx", "")
	// Also tests sorting.
	c.Check(machines, jc.DeepEquals, []string{"0", "2", "10"})
}

func (*modelSuite) TestUnitMachinesWithoutAppSourceAllTarget(c *gc.C) {
	model := &Model{
		Applications: map[string]*Application{
			"django": {
				Units: []Unit{
					{"django/0", "0"},
					{"django/1", "1"},
					{"django/2", "2"},
				},
			},
			"nginx": {
				Units: []Unit{
					{"nginx/0", "0"},
					{"nginx/1", "1"},
					{"nginx/2", "2"},
					{"nginx/3", "3"},
				},
			},
		},
	}
	machines := model.unitMachinesWithoutApp("django", "nginx", "")
	c.Check(machines, gc.HasLen, 0)
	c.Check(machines, gc.NotNil)
}

func (*modelSuite) TestMachineHasApp(c *gc.C) {
	model := &Model{
		Applications: map[string]*Application{
			"django": {
				Units: []Unit{
					{"django/0", "0"},
				},
			},
			"nginx": {
				Units: []Unit{
					{"nginx/0", "0/lxd/3"},
					{"nginx/2", "2/kvm/2"},
				},
			},
		},
	}
	c.Check(model.machineHasApp("0", "django", ""), jc.IsTrue)
	c.Check(model.machineHasApp("0", "django", "lxd"), jc.IsFalse)
	c.Check(model.machineHasApp("4", "django", ""), jc.IsFalse)

	c.Check(model.machineHasApp("0", "nginx", ""), jc.IsFalse)
	c.Check(model.machineHasApp("0", "nginx", "lxd"), jc.IsTrue)
	c.Check(model.machineHasApp("0", "nginx", "kvm"), jc.IsFalse)

	c.Check(model.machineHasApp("2", "nginx", ""), jc.IsFalse)
	c.Check(model.machineHasApp("2", "nginx", "lxd"), jc.IsFalse)
	c.Check(model.machineHasApp("2", "nginx", "kvm"), jc.IsTrue)
}

func (*modelSuite) TestUnsatisfiedMachineAndUnitPlacement(c *gc.C) {
	model := &Model{
		Applications: map[string]*Application{
			"django": {
				Units: []Unit{
					{"django/0", "0/lxd/0"},
					{"django/1", "1/lxd/0"},
					{"django/2", "2/lxd/0"},
				},
			},
			"nginx": {
				Units: []Unit{
					{"nginx/0", "0"},
					{"nginx/2", "2"},
					{"nginx/3", "3"},
				},
			},
		},
	}
	checkPlacement := func(app string, placements, expected []string) {
		result := model.unsatisfiedMachineAndUnitPlacements(app, placements)
		if expected == nil {
			c.Check(result, gc.IsNil)
		} else {
			c.Check(result, jc.DeepEquals, expected)
		}
	}

	placements := []string{"other-app", "new", "lxd:new", "lxd:app-name"}
	checkPlacement("unknown", placements, placements)
	checkPlacement("nginx", placements, placements)
	checkPlacement("nginx", []string{"0", "2", "3"}, nil)
	placements = []string{"lxd:0", "lxd:2", "lxd:3"}
	checkPlacement("nginx", placements, placements)
	checkPlacement("nginx", []string{"0", "1", "2"}, []string{"1"})
	checkPlacement("nginx", []string{"0", "5", "4", "2"}, []string{"5", "4"})
	placements = []string{"0", "1", "2"}
	checkPlacement("django", placements, placements)
	checkPlacement("django", []string{"lxd:0", "lxd:1", "lxd:2"}, nil)
	checkPlacement("django", []string{"lxd:0", "lxd:4", "lxd:2"}, []string{"lxd:4"})
	checkPlacement("django", []string{"lxd:nginx/0", "lxd:nginx/1", "lxd:nginx/2"}, []string{"lxd:nginx/1"})
	checkPlacement("django", []string{"lxd:nginx/0", "lxd:nginx/2", "lxd:nginx/3"}, []string{"lxd:nginx/3"})
}

func (*modelSuite) TestUnitMachinesWithoutAppSourceSomeTarget(c *gc.C) {
	model := &Model{
		Applications: map[string]*Application{
			"django": {
				Units: []Unit{
					{"django/0", "0"},
					{"django/1", "1"},
					{"django/2", "2"},
				},
			},
			"nginx": {
				Units: []Unit{
					{"nginx/0", "0"},
					{"nginx/2", "2/lxd/0"},
					{"nginx/3", "3"},
				},
			},
		},
	}
	machines := model.unitMachinesWithoutApp("django", "nginx", "")
	// Machine 2 is shown because the nginx isn't next to the django unit, but
	// instead in a container.
	c.Check(machines, jc.DeepEquals, []string{"1", "2"})
}

func (*modelSuite) TestUnitMachinesWithoutAppSourceSomeTargetContainer(c *gc.C) {
	model := &Model{
		Applications: map[string]*Application{
			"django": {
				Units: []Unit{
					{"django/0", "0"},
					{"django/1", "1"},
					{"django/2", "2"},
					{"django/3", "3"},
					{"django/4", "4"},
					{"django/5", "4"}, // Yes also on machine 4.
				},
			},
			"nginx": {
				Units: []Unit{
					{"nginx/0", "0"},
					{"nginx/1", "1/lxd/3"},
					{"nginx/2", "2/lxd/0"},
					{"nginx/3", "1/lxd/2"},
					{"nginx/4", "3/kvm/2"},
				},
			},
		},
	}
	machines := model.unitMachinesWithoutApp("django", "nginx", "lxd")
	// Machine 2 is shown because the nginx isn't next to the django unit, but
	// instead in a container.
	c.Check(machines, jc.DeepEquals, []string{"0", "3", "4"})
}

func (*modelSuite) TestBundleMachineMapped(c *gc.C) {
	model := &Model{
		Applications: map[string]*Application{
			"mysql": {
				Charm: "ch:mysql",
				Units: []Unit{
					{"mysql/0", "0/lxd/0"},
				},
			},
		},
		Machines: map[string]*Machine{
			// We don't actually look at the content of the machines
			// for this test, just the keys.
			"0": nil, "0/lxd/0": nil, "2": {ID: "2"},
		},
		MachineMap: map[string]string{
			"0": "2", // 0 in bundle is machine 2 in existing.
		},
	}
	machine := model.BundleMachine("0")
	c.Assert(machine, gc.NotNil)
	c.Assert(machine.ID, gc.Equals, "2")
}

func (*modelSuite) TestBundleMachineNotMapped(c *gc.C) {
	model := &Model{
		Applications: map[string]*Application{
			"mysql": {
				Charm: "ch:mysql",
				Units: []Unit{
					{"mysql/0", "0/lxd/0"},
				},
			},
		},
		Machines: map[string]*Machine{
			// We don't actually look at the content of the machines
			// for this test, just the keys.
			"0": nil, "0/lxd/0": nil, "2": {ID: "2"},
		},
	}
	machine := model.BundleMachine("0")
	c.Assert(machine, gc.IsNil)
}

type inferMachineMapSuite struct {
	testing.IsolationSuite

	data *charm.BundleData
}

var _ = gc.Suite(&inferMachineMapSuite{})

func (s *inferMachineMapSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	loggo.ConfigureLoggers("bundlechanges=trace")

	bundle := `
        applications:
            django:
                charm: ch:django
                revision: 42
                channel: stable
                series: trusty
                num_units: 5
                to:
                    - new
                    - 4
                    - kvm:8
                    - lxc:new
        machines:
            4:
                constraints: "cpu-cores=4"
            8:
                constraints: "cpu-cores=8"
    `
	s.data = s.parseBundle(c, bundle)
}

func (s *inferMachineMapSuite) parseBundle(c *gc.C, bundle string) *charm.BundleData {
	reader := bytes.NewBufferString(bundle)
	data, err := charm.ReadBundleData(reader)
	c.Assert(err, jc.ErrorIsNil)
	return data
}

func (s *inferMachineMapSuite) TestInferMachineMapEmptyModel(c *gc.C) {
	model := &Model{logger: loggo.GetLogger("bundlechanges")}
	model.InferMachineMap(s.data)
	// MachineMap is empty and not nil.
	c.Assert(model.MachineMap, gc.HasLen, 0)
	c.Assert(model.MachineMap, gc.NotNil)
}

func (s *inferMachineMapSuite) TestInferMachineMapSuppliedMapping(c *gc.C) {
	userSpecifiedMap := map[string]string{
		"4": "0", "8": "2",
	}
	model := &Model{
		logger:     loggo.GetLogger("bundlechanges"),
		MachineMap: userSpecifiedMap,
	}
	// If the user specified a mapping for those machines, use those.
	model.InferMachineMap(s.data)
	c.Assert(model.MachineMap, jc.DeepEquals, userSpecifiedMap)
}

func (s *inferMachineMapSuite) TestInferMachineMapPartial(c *gc.C) {
	userSpecifiedMap := map[string]string{
		"4": "1",
	}
	model := &Model{
		Applications: map[string]*Application{
			"django": {
				Units: []Unit{
					{"django/0", "1"},
					{"django/1", "2"},
					{"django/2", "3"},
				},
			},
		},
		MachineMap: userSpecifiedMap,
		logger:     loggo.GetLogger("bundlechanges"),
	}
	model.InferMachineMap(s.data)
	// Since the user specified a mapping for machine 4 we use that, and
	// machine 8 effectively gets the first django unit that isn't a target
	// in the supplied machine map.
	c.Assert(model.MachineMap, jc.DeepEquals, map[string]string{
		"4": "1", "8": "2",
	})
}

func (s *inferMachineMapSuite) TestInferMachineMapDeployedUnits(c *gc.C) {
	model := &Model{
		Applications: map[string]*Application{
			"django": {
				Units: []Unit{
					{"django/0", "0"},
					{"django/1", "1"},
					{"django/2", "2/kvm/0"},
					{"django/3", "3/lxc/0"},
					{"django/4", "4/lxc/0"},
				},
			},
		},
		logger: loggo.GetLogger("bundlechanges"),
	}
	model.InferMachineMap(s.data)
	// Since the placement directives use a mix of new and non-new, this
	// does make the inference harder. The first two machines identified
	// map the bundle machine ids.
	c.Assert(model.MachineMap, jc.DeepEquals, map[string]string{
		"4": "0", "8": "1",
	})
}

func (s *inferMachineMapSuite) TestOffest(c *gc.C) {
	data := s.parseBundle(c, `
        applications:
            django:
                charm: ch:django
                num_units: 3
                to: [1, 2, 3]
        machines:
            1:
            2:
            3:
`)
	model := &Model{
		Applications: map[string]*Application{
			"django": {
				Units: []Unit{
					{"django/0", "0"},
					{"django/1", "1"},
					{"django/2", "2"},
				},
			},
		},
		logger: loggo.GetLogger("bundlechanges"),
	}
	model.InferMachineMap(data)
	c.Assert(model.MachineMap, jc.DeepEquals, map[string]string{
		"1": "1", "2": "2", "3": "0",
	})
}

// Fixing LP #1883645
func (s *inferMachineMapSuite) TestBundleMachinesDeterminism(c *gc.C) {
	data := s.parseBundle(c, `
        series: bionic
        machines:
            "0":
                series: bionic
            "1":
                series: bionic
            "2":
                series: bionic
            "10":
                series: bionic
            "11":
                series: bionic
            "12":
                series: bionic
            "20":
                series: bionic
            "21":
                series: bionic
            "22":
                series: bionic
        applications:
            ubuntu:
                num_units: 6
                charm: ubuntu
                to:
                - 0
                - 1
                - 2
                - 10
                - 11
                - 12
            memcached:
                num_units: 6
                charm: ch:memcached
                to:
                - 10
                - 12
                - 13
                - 20
                - 21
                - 22
`)
	model := &Model{
		Applications: map[string]*Application{
			"ubuntu": {
				Units: []Unit{
					{"ubuntu/0", "0"},
					{"ubuntu/1", "1"},
					{"ubuntu/2", "2"},
					{"ubuntu/3", "10"},
					{"ubuntu/4", "11"},
					{"ubuntu/5", "12"},
				},
			},
			"memcached": {
				Units: []Unit{
					{"memcached/0", "10"},
					{"memcached/1", "11"},
					{"memcached/2", "12"},
				},
			},
		},
		Machines: map[string]*Machine{
			"0":  {ID: "0"},
			"1":  {ID: "1"},
			"2":  {ID: "2"},
			"10": {ID: "10"},
			"11": {ID: "11"},
			"12": {ID: "12"},
		},
		logger: loggo.GetLogger("bundlechanges"),
	}

	// Loop through enough times to trigger a potential map ordering bug.
	for i := 0; i < 10; i++ {
		model.initializeSequence()
		model.InferMachineMap(data)
		c.Assert(model.MachineMap, jc.DeepEquals, map[string]string{
			"0": "0", "1": "1", "2": "2", "10": "10", "11": "11", "12": "12",
		})

		names := make([]string, 0, len(data.Machines))
		for name := range data.Machines {
			names = append(names, name)
		}
		naturalsort.Sort(names)

		var got [][]string
		for _, machine := range names {
			if model.BundleMachine(machine) == nil {
				got = append(got, []string{machine, model.nextMachine()})
			}
		}
		c.Assert(got, jc.DeepEquals, [][]string{
			{"20", "13"},
			{"21", "14"},
			{"22", "15"},
		})
	}
}

type applicationSuite struct{}

var _ = gc.Suite(&applicationSuite{})

func (*applicationSuite) TestNilApplication(c *gc.C) {
	var app *Application
	annotations := map[string]string{"a": "b", "c": "d"}
	toChange := app.changedAnnotations(annotations)
	c.Check(toChange, jc.DeepEquals, annotations)
}

func (*applicationSuite) TestEmptyApplication(c *gc.C) {
	app := &Application{}
	annotations := map[string]string{"a": "b", "c": "d"}
	toChange := app.changedAnnotations(annotations)
	c.Assert(toChange, jc.DeepEquals, annotations)
}

func (*applicationSuite) TestChangedAnnotationsSomeChanges(c *gc.C) {
	app := &Application{
		Annotations: map[string]string{"a": "b", "c": "g", "f": "p"},
	}
	annotations := map[string]string{"a": "b", "c": "d"}
	toChange := app.changedAnnotations(annotations)
	c.Assert(toChange, jc.DeepEquals, map[string]string{"c": "d"})
}

func (*applicationSuite) TestChangedOptionsSomeChanges(c *gc.C) {
	app := &Application{
		Options: map[string]interface{}{
			"string": "hello",
			"int":    float64(42), // comes over the API as a float
			"float":  float64(2.5),
			"bool":   true,
		},
	}
	options := map[string]interface{}{"string": "hello", "int": 42}
	toChange := app.changedOptions(options)
	c.Assert(toChange, gc.HasLen, 0)

	options = map[string]interface{}{"string": "world", "int": 24, "float": 3.14, "bool": false}
	toChange = app.changedOptions(options)
	c.Assert(toChange, jc.DeepEquals, options)
}
