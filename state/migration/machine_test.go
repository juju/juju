// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/version"
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

func (s *MachineSerializationSuite) machineArgs(id string) MachineArgs {
	return MachineArgs{
		Id:            names.NewMachineTag(id),
		Nonce:         "a nonce",
		PasswordHash:  "some-hash",
		Placement:     "placement",
		Series:        "zesty",
		ContainerType: "magic",
		Jobs:          []string{"this", "that"},
	}
}

func (s *MachineSerializationSuite) TestNewMachine(c *gc.C) {
	m := newMachine(s.machineArgs("machine-id"))
	c.Assert(m.Id(), gc.Equals, names.NewMachineTag("machine-id"))
	c.Assert(m.Nonce(), gc.Equals, "a nonce")
	c.Assert(m.PasswordHash(), gc.Equals, "some-hash")
	c.Assert(m.Placement(), gc.Equals, "placement")
	c.Assert(m.Series(), gc.Equals, "zesty")
	c.Assert(m.ContainerType(), gc.Equals, "magic")
	c.Assert(m.Jobs(), jc.DeepEquals, []string{"this", "that"})
	supportedContainers, ok := m.SupportedContainers()
	c.Assert(ok, jc.IsFalse)
	c.Assert(supportedContainers, gc.IsNil)
}

func (s *MachineSerializationSuite) TestNewMachineWithSupportedContainers(c *gc.C) {
	supported := []string{"lxd", "kvm"}
	args := s.machineArgs("id")
	args.SupportedContainers = &supported
	m := newMachine(args)
	supportedContainers, ok := m.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(supportedContainers, jc.DeepEquals, supported)
}

func (s *MachineSerializationSuite) TestNewMachineWithNoSupportedContainers(c *gc.C) {
	supported := []string{}
	args := s.machineArgs("id")
	args.SupportedContainers = &supported
	m := newMachine(args)
	supportedContainers, ok := m.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(supportedContainers, gc.HasLen, 0)
}

func (s *MachineSerializationSuite) TestNewMachineWithNoSupportedContainersNil(c *gc.C) {
	var supported []string
	args := s.machineArgs("id")
	args.SupportedContainers = &supported
	m := newMachine(args)
	supportedContainers, ok := m.SupportedContainers()
	c.Assert(ok, jc.IsTrue)
	c.Assert(supportedContainers, gc.HasLen, 0)
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

func (s *MachineSerializationSuite) TestParsingSerializedData(c *gc.C) {
	// TODO: need to fully specify a machine.
	args := s.machineArgs("0")
	supported := []string{"kvm", "lxd"}
	args.SupportedContainers = &supported
	m := newMachine(args)
	m.SetTools(minimalAgentToolsArgs())

	initial := machines{
		Version:   1,
		Machines_: []*machine{m},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("-- bytes --\n%s\n", bytes)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("-- map --")
	machineMap := source["machines"].([]interface{})[0].(map[interface{}]interface{})
	for key, value := range machineMap {
		c.Logf("%s: %v", key, value)
	}

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

const gig uint64 = 1024 * 1024 * 1024

func (s *CloudInstanceSerializationSuite) TestNewCloudInstance(c *gc.C) {
	args := CloudInstanceArgs{
		InstanceId:       "instance id",
		Status:           "working",
		Architecture:     "amd64",
		Memory:           16 * gig,
		RootDisk:         200 * gig,
		CpuCores:         8,
		CpuPower:         4000,
		Tags:             []string{"much", "strong"},
		AvailabilityZone: "everywhere",
	}

	instance := newCloudInstance(args)

	c.Assert(instance.InstanceId(), gc.Equals, args.InstanceId)
	c.Assert(instance.Status(), gc.Equals, args.Status)
	c.Assert(instance.Architecture(), gc.Equals, args.Architecture)
	c.Assert(instance.Memory(), gc.Equals, args.Memory)
	c.Assert(instance.RootDisk(), gc.Equals, args.RootDisk)
	c.Assert(instance.CpuCores(), gc.Equals, args.CpuCores)
	c.Assert(instance.CpuPower(), gc.Equals, args.CpuPower)
	c.Assert(instance.AvailabilityZone(), gc.Equals, args.AvailabilityZone)

	// Before we check tags, modify args to make sure that the instance ones
	// don't change.

	args.Tags[0] = "weird"
	tags := instance.Tags()
	c.Assert(tags, jc.DeepEquals, []string{"much", "strong"})

	// Also, changing the tags returned, doesn't modify the instance
	tags[0] = "weird"
	c.Assert(instance.Tags(), jc.DeepEquals, []string{"much", "strong"})
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
	initial := newCloudInstance(CloudInstanceArgs{
		InstanceId:   "instance id",
		Status:       "working",
		Architecture: "amd64",
		Memory:       16 * gig,
		CpuPower:     MaxUint64,
	})
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

func (s *AgentToolsSerializationSuite) TestNewAgentTools(c *gc.C) {
	args := AgentToolsArgs{
		Version: version.MustParseBinary("3.4.5-trusty-amd64"),
		URL:     "some-url",
		SHA256:  "long-hash",
		Size:    123456789,
	}
	instance := newAgentTools(args)

	c.Assert(instance.Version(), gc.Equals, args.Version)
	c.Assert(instance.URL(), gc.Equals, args.URL)
	c.Assert(instance.SHA256(), gc.Equals, args.SHA256)
	c.Assert(instance.Size(), gc.Equals, args.Size)
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

func minimalAgentToolsArgs() AgentToolsArgs {
	return AgentToolsArgs{
		Version: version.MustParseBinary("3.4.5-trusty-amd64"),
		URL:     "some-url",
		SHA256:  "long-hash",
		Size:    123456789,
	}
}

func minimalAgentTools() *agentTools {
	return newAgentTools(minimalAgentToolsArgs())
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
	initial := newAgentTools(AgentToolsArgs{
		Version: version.MustParseBinary("2.0.4-trusty-amd64"),
		URL:     "some-url",
		SHA256:  "long-hash",
		Size:    123456789,
	})
	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	instance, err := importAgentTools(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instance, jc.DeepEquals, initial)
}
