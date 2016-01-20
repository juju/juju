// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/juju/version"
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

func minimalMachineMap(id string, containers ...interface{}) map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"id":            id,
		"nonce":         "a-nonce",
		"password-hash": "some-hash",
		"instance":      minimalCloudInstanceMap(),
		"series":        "zesty",
		"tools":         minimalAgentToolsMap(),
		// jobs coming soon...
		"jobs":       []interface{}{},
		"containers": containers,
		// addresses coming soon...
		"provider-addresses":        []interface{}{},
		"machine-addresses":         []interface{}{},
		"preferred-public-address":  nil,
		"preferred-private-address": nil,
	}
}

func minimalMachine(id string, containers ...*machine) *machine {
	return &machine{
		Id_:           id,
		Nonce_:        "a-nonce",
		PasswordHash_: "some-hash",
		Instance_:     minimalCloudInstance(),
		Series_:       "zesty",
		Tools_:        minimalAgentTools(),
		Containers_:   containers,
	}
}

func (s *MachineSerializationSuite) TestMinimalMatches(c *gc.C) {
	bytes, err := yaml.Marshal(minimalMachine("0"))
	c.Assert(err, jc.ErrorIsNil)

	var source map[interface{}]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(source, jc.DeepEquals, minimalMachineMap("0"))
}

func (*MachineSerializationSuite) TestNestedParsing(c *gc.C) {
	machines, err := importMachines(map[string]interface{}{
		"version": 1,
		"machines": []interface{}{
			minimalMachineMap("0"),
			minimalMachineMap("1",
				minimalMachineMap("1/lxc/0"),
				minimalMachineMap("1/lxc/1"),
			),
			minimalMachineMap("2",
				minimalMachineMap("2/kvm/0",
					minimalMachineMap("2/kvm/0/lxc/0"),
					minimalMachineMap("2/kvm/0/lxc/1"),
				),
			),
		}})
	c.Assert(err, jc.ErrorIsNil)
	expected := []*machine{
		minimalMachine("0"),
		minimalMachine("1",
			minimalMachine("1/lxc/0"),
			minimalMachine("1/lxc/1"),
		),
		minimalMachine("2",
			minimalMachine("2/kvm/0",
				minimalMachine("2/kvm/0/lxc/0"),
				minimalMachine("2/kvm/0/lxc/1"),
			),
		),
	}
	c.Assert(machines, jc.DeepEquals, expected)
}

func (*MachineSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := machines{
		Version: 1,
		Machines_: []*machine{
			minimalMachine("0"),
			minimalMachine("1",
				minimalMachine("1/lxc/0"),
				minimalMachine("1/lxc/1"),
			),
			minimalMachine("2",
				minimalMachine("2/kvm/0",
					minimalMachine("2/kvm/0/lxc/0"),
					minimalMachine("2/kvm/0/lxc/1"),
				),
			),
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

func minimalCloudInstanceMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
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

func (s *CloudInstanceSerializationSuite) TestMinimalMatches(c *gc.C) {
	bytes, err := yaml.Marshal(minimalCloudInstance())
	c.Assert(err, jc.ErrorIsNil)

	var source map[interface{}]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(source, jc.DeepEquals, minimalCloudInstanceMap())
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

type AgentToolsSerializationSuite struct {
	SerializationSuite
}

var _ = gc.Suite(&AgentToolsSerializationSuite{})

func (s *AgentToolsSerializationSuite) SetUpTest(c *gc.C) {
	s.importName = "agentTools"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importAgentTools(m)
	}
}

func minimalAgentToolsMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"version":       1,
		"tools-version": "3.4.5-trusty-amd64",
		"url":           "some-url",
		"sha256":        "long-hash",
		"size":          123456789,
	}
}

func minimalAgentTools() *agentTools {
	return &agentTools{
		Version_:      1,
		ToolsVersion_: version.MustParseBinary("3.4.5-trusty-amd64"),
		URL_:          "some-url",
		SHA256_:       "long-hash",
		Size_:         123456789,
	}
}

func (s *AgentToolsSerializationSuite) TestMinimalMatches(c *gc.C) {
	bytes, err := yaml.Marshal(minimalAgentTools())
	c.Assert(err, jc.ErrorIsNil)

	var source map[interface{}]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(source, jc.DeepEquals, minimalAgentToolsMap())
}

func (s *AgentToolsSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := &agentTools{
		Version_:      1,
		ToolsVersion_: version.MustParseBinary("2.0.4-trusty-amd64"),
		URL_:          "some-url",
		SHA256_:       "long-hash",
		Size_:         123456789,
	}
	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	instance, err := importAgentTools(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instance, jc.DeepEquals, initial)
}
