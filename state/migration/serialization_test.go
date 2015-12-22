// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/testing"
)

type ModelSerializationSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ModelSerializationSuite{})

func (*ModelSerializationSuite) TestNil(c *gc.C) {
	_, err := importModel(nil)
	c.Check(err, gc.ErrorMatches, "version: expected int, got nothing")
}

func (*ModelSerializationSuite) TestMissingVersion(c *gc.C) {
	_, err := importModel(map[string]interface{}{})
	c.Check(err, gc.ErrorMatches, "version: expected int, got nothing")
}

func (*ModelSerializationSuite) TestNonIntVersion(c *gc.C) {
	_, err := importModel(map[string]interface{}{
		"version": "hello",
	})
	c.Check(err.Error(), gc.Equals, `version: expected int, got string("hello")`)
}

func (*ModelSerializationSuite) TestUnknownVersion(c *gc.C) {
	_, err := importModel(map[string]interface{}{
		"version": 42,
	})
	c.Check(err.Error(), gc.Equals, `version 42 not valid`)
}

type MachineSerializationSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&MachineSerializationSuite{})

func (*MachineSerializationSuite) TestNil(c *gc.C) {
	_, err := importMachines(nil)
	c.Check(err, gc.ErrorMatches, "machines version schema check failed: .*")
}

func (*MachineSerializationSuite) TestMissingVersion(c *gc.C) {
	_, err := importMachines(map[string]interface{}{
		"machines": []interface{}{},
	})
	c.Check(err.Error(), gc.Equals, "machines version schema check failed: version: expected int, got nothing")
}

func (*MachineSerializationSuite) TestMissingMachines(c *gc.C) {
	_, err := importMachines(map[string]interface{}{
		"version": 1,
	})
	c.Check(err.Error(), gc.Equals, "machines version schema check failed: machines: expected list, got nothing")
}

func (*MachineSerializationSuite) TestNonIntVersion(c *gc.C) {
	_, err := importMachines(map[string]interface{}{
		"version":  "hello",
		"machines": []interface{}{},
	})
	c.Check(err.Error(), gc.Equals, `machines version schema check failed: version: expected int, got string("hello")`)
}

func (*MachineSerializationSuite) TestUnknownVersion(c *gc.C) {
	_, err := importMachines(map[string]interface{}{
		"version":  42,
		"machines": []interface{}{},
	})
	c.Check(err.Error(), gc.Equals, `version 42 not valid`)
}

func (*MachineSerializationSuite) TestMachinesIsMap(c *gc.C) {
	_, err := importMachines(map[string]interface{}{
		"version":  42,
		"machines": []interface{}{"hello"},
	})
	c.Check(err.Error(), gc.Equals, `machines version schema check failed: machines[0]: expected map, got string("hello")`)
}

func (*MachineSerializationSuite) TestNestedParsing(c *gc.C) {
	machines, err := importMachines(map[string]interface{}{
		"version": 1,
		"machines": []interface{}{
			map[string]interface{}{
				"id":         "0",
				"containers": []interface{}{},
			},
			map[string]interface{}{
				"id": "1",
				"containers": []interface{}{
					map[string]interface{}{
						"id":         "1/lxc/0",
						"containers": []interface{}{},
					},
					map[string]interface{}{
						"id":         "1/lxc/1",
						"containers": []interface{}{},
					},
				},
			},
			map[string]interface{}{
				"id": "2",
				"containers": []interface{}{
					map[string]interface{}{
						"id": "2/kvm/0",
						"containers": []interface{}{
							map[string]interface{}{
								"id":         "2/kvm/0/lxc/0",
								"containers": []interface{}{},
							},
							map[string]interface{}{
								"id":         "2/kvm/0/lxc/1",
								"containers": []interface{}{},
							},
						},
					},
				},
			},
		}})
	c.Assert(err, jc.ErrorIsNil)
	expected := []*machine{
		&machine{
			Id_: "0",
		},
		&machine{
			Id_: "1",
			Containers_: []*machine{
				&machine{
					Id_: "1/lxc/0",
				},
				&machine{
					Id_: "1/lxc/1",
				},
			},
		},
		&machine{
			Id_: "2",
			Containers_: []*machine{
				&machine{
					Id_: "2/kvm/0",
					Containers_: []*machine{
						&machine{
							Id_: "2/kvm/0/lxc/0",
						},
						&machine{
							Id_: "2/kvm/0/lxc/1",
						},
					},
				},
			},
		},
	}
	c.Assert(machines, jc.DeepEquals, expected)
}

func (*MachineSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := machines{
		Version: 1,
		Machines_: []*machine{
			&machine{
				Id_: "0",
			},
			&machine{
				Id_: "1",
				Containers_: []*machine{
					&machine{
						Id_: "1/lxc/0",
					},
					&machine{
						Id_: "1/lxc/1",
					},
				},
			},
			&machine{
				Id_: "2",
				Containers_: []*machine{
					&machine{
						Id_: "2/kvm/0",
						Containers_: []*machine{
							&machine{
								Id_: "2/kvm/0/lxc/0",
							},
							&machine{
								Id_: "2/kvm/0/lxc/1",
							},
						},
					},
				},
			},
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	machines, err := importMachines(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(machines, jc.DeepEquals, initial.Machines_)
}
