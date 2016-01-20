// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type MachineSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&MachineSerializationSuite{})

func (s *MachineSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "machines"
	s.sliceName = "machines"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importMachines(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["machines"] = []interface{}{}
	}
}

// TODO MAYBE: move this test into the slice serialization base.
func (*MachineSerializationSuite) TestMachinesIsMap(c *gc.C) {
	_, err := importMachines(map[string]interface{}{
		"version":  42,
		"machines": []interface{}{"hello"},
	})
	c.Check(err.Error(), gc.Equals, `machines version schema check failed: machines[0]: expected map, got string("hello")`)
}

func minimalCloudInstanceMap() map[string]interface{} {
	return map[string]interface{}{
		"version":     1,
		"instance-id": "instance id",
		"status":      "some status",
	}
}

func minimalCloudInstance() *cloudInstance {
	return &cloudInstance{
		Version:     1,
		InstanceId_: "instance id",
		Status_:     "some status",
	}
}

func (*MachineSerializationSuite) TestNestedParsing(c *gc.C) {
	machines, err := importMachines(map[string]interface{}{
		"version": 1,
		"machines": []interface{}{
			map[string]interface{}{
				"id":         "0",
				"instance":   minimalCloudInstanceMap(),
				"containers": []interface{}{},
			},
			map[string]interface{}{
				"id":       "1",
				"instance": minimalCloudInstanceMap(),
				"containers": []interface{}{
					map[string]interface{}{
						"id":         "1/lxc/0",
						"instance":   minimalCloudInstanceMap(),
						"containers": []interface{}{},
					},
					map[string]interface{}{
						"id":         "1/lxc/1",
						"instance":   minimalCloudInstanceMap(),
						"containers": []interface{}{},
					},
				},
			},
			map[string]interface{}{
				"id":       "2",
				"instance": minimalCloudInstanceMap(),
				"containers": []interface{}{
					map[string]interface{}{
						"id":       "2/kvm/0",
						"instance": minimalCloudInstanceMap(),
						"containers": []interface{}{
							map[string]interface{}{
								"id":         "2/kvm/0/lxc/0",
								"instance":   minimalCloudInstanceMap(),
								"containers": []interface{}{},
							},
							map[string]interface{}{
								"id":         "2/kvm/0/lxc/1",
								"instance":   minimalCloudInstanceMap(),
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
			Id_:       "0",
			Instance_: minimalCloudInstance(),
		},
		&machine{
			Id_:       "1",
			Instance_: minimalCloudInstance(),
			Containers_: []*machine{
				&machine{
					Id_:       "1/lxc/0",
					Instance_: minimalCloudInstance(),
				},
				&machine{
					Id_:       "1/lxc/1",
					Instance_: minimalCloudInstance(),
				},
			},
		},
		&machine{
			Id_:       "2",
			Instance_: minimalCloudInstance(),
			Containers_: []*machine{
				&machine{
					Id_:       "2/kvm/0",
					Instance_: minimalCloudInstance(),
					Containers_: []*machine{
						&machine{
							Id_:       "2/kvm/0/lxc/0",
							Instance_: minimalCloudInstance(),
						},
						&machine{
							Id_:       "2/kvm/0/lxc/1",
							Instance_: minimalCloudInstance(),
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
				Id_:       "0",
				Instance_: minimalCloudInstance(),
			},
			&machine{
				Id_:       "1",
				Instance_: minimalCloudInstance(),
				Containers_: []*machine{
					&machine{
						Id_:       "1/lxc/0",
						Instance_: minimalCloudInstance(),
					},
					&machine{
						Id_:       "1/lxc/1",
						Instance_: minimalCloudInstance(),
					},
				},
			},
			&machine{
				Id_:       "2",
				Instance_: minimalCloudInstance(),
				Containers_: []*machine{
					&machine{
						Id_:       "2/kvm/0",
						Instance_: minimalCloudInstance(),
						Containers_: []*machine{
							&machine{
								Id_:       "2/kvm/0/lxc/0",
								Instance_: minimalCloudInstance(),
							},
							&machine{
								Id_:       "2/kvm/0/lxc/1",
								Instance_: minimalCloudInstance(),
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

type CloudInstanceSerializationSuite struct {
	SerializationSuite
}

var _ = gc.Suite(&CloudInstanceSerializationSuite{})

func (s *CloudInstanceSerializationSuite) SetUpTest(c *gc.C) {
	s.importName = "cloudInstance"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importCloudInstance(m)
	}
}

func (s *CloudInstanceSerializationSuite) TestParsingSerializedData(c *gc.C) {
	const MaxUint64 = 1<<64 - 1
	initial := &cloudInstance{
		Version:     1,
		InstanceId_: "instance id",
		Status_:     "some status",
		RootDisk_:   64,
		CpuPower_:   MaxUint64,
	}
	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	instance, err := importCloudInstance(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instance, jc.DeepEquals, initial)
}
