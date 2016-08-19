// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
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

func (*ModelSerializationSuite) TestUpdateConfig(c *gc.C) {
	model := NewModel(ModelArgs{
		Config: map[string]interface{}{
			"name": "awesome",
			"uuid": "some-uuid",
		},
		CloudRegion: "some-region",
	})
	model.UpdateConfig(map[string]interface{}{
		"name": "something else",
		"key":  "value",
	})
	c.Assert(model.Config(), jc.DeepEquals, map[string]interface{}{
		"name": "something else",
		"uuid": "some-uuid",
		"key":  "value",
	})
}

func (s *ModelSerializationSuite) exportImport(c *gc.C, initial Model) Model {
	bytes, err := Serialize(initial)
	c.Assert(err, jc.ErrorIsNil)
	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	return model
}

func (s *ModelSerializationSuite) TestParsingYAML(c *gc.C) {
	args := ModelArgs{
		Owner: names.NewUserTag("magic"),
		Config: map[string]interface{}{
			"name": "awesome",
			"uuid": "some-uuid",
		},
		LatestToolsVersion: version.MustParse("2.0.1"),
		Blocks: map[string]string{
			"all-changes": "locked down",
		},
	}
	initial := NewModel(args)
	adminUser := names.NewUserTag("admin@local")
	initial.AddUser(UserArgs{
		Name:        adminUser,
		CreatedBy:   adminUser,
		DateCreated: time.Date(2015, 10, 9, 12, 34, 56, 0, time.UTC),
	})
	addMinimalMachine(initial, "0")
	addMinimalApplication(initial)
	model := s.exportImport(c, initial)

	c.Assert(model.Owner(), gc.Equals, args.Owner)
	c.Assert(model.Tag().Id(), gc.Equals, "some-uuid")
	c.Assert(model.Config(), jc.DeepEquals, args.Config)
	c.Assert(model.LatestToolsVersion(), gc.Equals, args.LatestToolsVersion)
	c.Assert(model.Blocks(), jc.DeepEquals, args.Blocks)
	users := model.Users()
	c.Assert(users, gc.HasLen, 1)
	c.Assert(users[0].Name(), gc.Equals, adminUser)
	machines := model.Machines()
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Id(), gc.Equals, "0")
	applications := model.Applications()
	c.Assert(applications, gc.HasLen, 1)
	c.Assert(applications[0].Name(), gc.Equals, "ubuntu")
}

func (s *ModelSerializationSuite) TestParsingOptionals(c *gc.C) {
	args := ModelArgs{
		Owner: names.NewUserTag("magic"),
		Config: map[string]interface{}{
			"name": "awesome",
			"uuid": "some-uuid",
		},
	}
	initial := NewModel(args)
	model := s.exportImport(c, initial)
	c.Assert(model.LatestToolsVersion(), gc.Equals, version.Zero)
}

func (s *ModelSerializationSuite) TestAnnotations(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	annotations := map[string]string{
		"string":  "value",
		"another": "one",
	}
	initial.SetAnnotations(annotations)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Annotations(), jc.DeepEquals, annotations)
}

func (s *ModelSerializationSuite) TestSequences(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	initial.SetSequence("machine", 4)
	initial.SetSequence("application-foo", 3)
	initial.SetSequence("application-bar", 1)
	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(model.Sequences(), jc.DeepEquals, map[string]int{
		"machine":         4,
		"application-foo": 3,
		"application-bar": 1,
	})
}

func (s *ModelSerializationSuite) TestConstraints(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := ConstraintsArgs{
		Architecture: "amd64",
		Memory:       8 * gig,
	}
	initial.SetConstraints(args)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Constraints(), jc.DeepEquals, newConstraints(args))
}

func (*ModelSerializationSuite) TestModelValidation(c *gc.C) {
	model := NewModel(ModelArgs{})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, "missing model owner not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (*ModelSerializationSuite) TestModelValidationChecksMachines(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner"), CloudRegion: "some-region"})
	model.AddMachine(MachineArgs{})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, "machine missing id not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *ModelSerializationSuite) addMachineToModel(model Model, id string) Machine {
	machine := model.AddMachine(MachineArgs{Id: names.NewMachineTag(id)})
	machine.SetInstance(CloudInstanceArgs{InstanceId: "magic"})
	machine.SetTools(minimalAgentToolsArgs())
	machine.SetStatus(minimalStatusArgs())
	return machine
}

func (s *ModelSerializationSuite) TestModelValidationChecksMachinesGood(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner"), CloudRegion: "some-region"})
	s.addMachineToModel(model, "0")
	err := model.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSerializationSuite) TestModelValidationChecksOpenPortsUnits(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner"), CloudRegion: "some-region"})
	machine := s.addMachineToModel(model, "0")
	machine.AddOpenedPorts(OpenedPortsArgs{
		OpenedPorts: []PortRangeArgs{
			{
				UnitName: "missing/0",
				FromPort: 8080,
				ToPort:   8080,
				Protocol: "tcp",
			},
		},
	})
	err := model.Validate()
	c.Assert(err.Error(), gc.Equals, "unknown unit names in open ports: [missing/0]")
}

func (*ModelSerializationSuite) TestModelValidationChecksApplications(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner"), CloudRegion: "some-region"})
	model.AddApplication(ApplicationArgs{})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, "application missing name not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *ModelSerializationSuite) addApplicationToModel(model Model, name string, numUnits int) Application {
	application := model.AddApplication(ApplicationArgs{
		Tag:                names.NewApplicationTag(name),
		Settings:           map[string]interface{}{},
		LeadershipSettings: map[string]interface{}{},
	})
	application.SetStatus(minimalStatusArgs())
	for i := 0; i < numUnits; i++ {
		// The index i is used as both the machine id and the unit id.
		// A happy coincidence.
		machine := s.addMachineToModel(model, fmt.Sprint(i))
		unit := application.AddUnit(UnitArgs{
			Tag:     names.NewUnitTag(fmt.Sprintf("%s/%d", name, i)),
			Machine: machine.Tag(),
		})
		unit.SetTools(minimalAgentToolsArgs())
		unit.SetAgentStatus(minimalStatusArgs())
		unit.SetWorkloadStatus(minimalStatusArgs())
	}

	return application
}

func (s *ModelSerializationSuite) wordpressModel() (Model, Endpoint, Endpoint) {
	model := NewModel(ModelArgs{
		Owner: names.NewUserTag("owner"),
		Config: map[string]interface{}{
			"uuid": "some-uuid",
		},
		CloudRegion: "some-region",
	})
	s.addApplicationToModel(model, "wordpress", 2)
	s.addApplicationToModel(model, "mysql", 1)

	// Add a relation between wordpress and mysql.
	rel := model.AddRelation(RelationArgs{
		Id:  42,
		Key: "special key",
	})
	wordpressEndpoint := rel.AddEndpoint(EndpointArgs{
		ApplicationName: "wordpress",
		Name:            "db",
		// Ignoring other aspects of endpoints.
	})
	mysqlEndpoint := rel.AddEndpoint(EndpointArgs{
		ApplicationName: "mysql",
		Name:            "mysql",
		// Ignoring other aspects of endpoints.
	})
	return model, wordpressEndpoint, mysqlEndpoint
}

func (s *ModelSerializationSuite) wordpressModelWithSettings() Model {
	model, wordpressEndpoint, mysqlEndpoint := s.wordpressModel()

	wordpressEndpoint.SetUnitSettings("wordpress/0", map[string]interface{}{
		"key": "value",
	})
	wordpressEndpoint.SetUnitSettings("wordpress/1", map[string]interface{}{
		"key": "value",
	})
	mysqlEndpoint.SetUnitSettings("mysql/0", map[string]interface{}{
		"key": "value",
	})
	return model
}

func (s *ModelSerializationSuite) TestModelValidationChecksRelationsMissingSettings(c *gc.C) {
	model, _, _ := s.wordpressModel()
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, "missing relation settings for units \\[wordpress/0 wordpress/1\\] in relation 42")
}

func (s *ModelSerializationSuite) TestModelValidationChecksRelationsMissingSettings2(c *gc.C) {
	model, wordpressEndpoint, _ := s.wordpressModel()

	wordpressEndpoint.SetUnitSettings("wordpress/0", map[string]interface{}{
		"key": "value",
	})
	wordpressEndpoint.SetUnitSettings("wordpress/1", map[string]interface{}{
		"key": "value",
	})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, "missing relation settings for units \\[mysql/0\\] in relation 42")
}

func (s *ModelSerializationSuite) TestModelValidationChecksRelations(c *gc.C) {
	model := s.wordpressModelWithSettings()
	err := model.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSerializationSuite) TestModelSerializationWithRelations(c *gc.C) {
	initial := s.wordpressModelWithSettings()
	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)
	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model, jc.DeepEquals, initial)
}

func (s *ModelSerializationSuite) TestModelValidationChecksSubnets(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	model.AddSubnet(SubnetArgs{CIDR: "10.0.0.0/24", SpaceName: "foo"})
	model.AddSubnet(SubnetArgs{CIDR: "10.0.1.0/24"})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `subnet "10.0.0.0/24" references non-existent space "foo"`)
	model.AddSpace(SpaceArgs{Name: "foo"})
	err = model.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSerializationSuite) TestModelValidationChecksAddressMachineID(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	model.AddIPAddress(IPAddressArgs{Value: "192.168.1.0", MachineID: "42"})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `ip address "192.168.1.0" references non-existent machine "42"`)
}

func (s *ModelSerializationSuite) TestModelValidationChecksAddressDeviceName(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := IPAddressArgs{Value: "192.168.1.0", MachineID: "42", DeviceName: "foo"}
	model.AddIPAddress(args)
	s.addMachineToModel(model, "42")
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `ip address "192.168.1.0" references non-existent device "foo"`)
}

func (s *ModelSerializationSuite) TestModelValidationChecksAddressValueEmpty(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := IPAddressArgs{MachineID: "42", DeviceName: "foo"}
	model.AddIPAddress(args)
	s.addMachineToModel(model, "42")
	model.AddLinkLayerDevice(LinkLayerDeviceArgs{Name: "foo", MachineID: "42"})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `ip address has invalid value ""`)
}

func (s *ModelSerializationSuite) TestModelValidationChecksAddressValueInvalid(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := IPAddressArgs{MachineID: "42", DeviceName: "foo", Value: "foobar"}
	model.AddIPAddress(args)
	s.addMachineToModel(model, "42")
	model.AddLinkLayerDevice(LinkLayerDeviceArgs{Name: "foo", MachineID: "42"})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `ip address has invalid value "foobar"`)
}

func (s *ModelSerializationSuite) TestModelValidationChecksAddressSubnetEmpty(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := IPAddressArgs{MachineID: "42", DeviceName: "foo", Value: "192.168.1.1"}
	model.AddIPAddress(args)
	s.addMachineToModel(model, "42")
	model.AddLinkLayerDevice(LinkLayerDeviceArgs{Name: "foo", MachineID: "42"})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `ip address "192.168.1.1" has empty subnet CIDR`)
}

func (s *ModelSerializationSuite) TestModelValidationChecksAddressSubnetInvalid(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := IPAddressArgs{
		MachineID:  "42",
		DeviceName: "foo",
		Value:      "192.168.1.1",
		SubnetCIDR: "foo",
	}
	model.AddIPAddress(args)
	s.addMachineToModel(model, "42")
	model.AddLinkLayerDevice(LinkLayerDeviceArgs{Name: "foo", MachineID: "42"})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `ip address "192.168.1.1" has invalid subnet CIDR "foo"`)
}

func (s *ModelSerializationSuite) TestModelValidationChecksAddressSucceeds(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := IPAddressArgs{
		MachineID:  "42",
		DeviceName: "foo",
		Value:      "192.168.1.1",
		SubnetCIDR: "192.168.1.0/24",
	}
	model.AddIPAddress(args)
	s.addMachineToModel(model, "42")
	model.AddLinkLayerDevice(LinkLayerDeviceArgs{Name: "foo", MachineID: "42"})
	err := model.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSerializationSuite) TestModelValidationChecksAddressGatewayAddressInvalid(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := IPAddressArgs{
		MachineID:      "42",
		DeviceName:     "foo",
		Value:          "192.168.1.1",
		SubnetCIDR:     "192.168.1.0/24",
		GatewayAddress: "foo",
	}
	model.AddIPAddress(args)
	s.addMachineToModel(model, "42")
	model.AddLinkLayerDevice(LinkLayerDeviceArgs{Name: "foo", MachineID: "42"})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `ip address "192.168.1.1" has invalid gateway address "foo"`)
}

func (s *ModelSerializationSuite) TestModelValidationChecksAddressGatewayAddressValid(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := IPAddressArgs{
		MachineID:      "42",
		DeviceName:     "foo",
		Value:          "192.168.1.2",
		SubnetCIDR:     "192.168.1.0/24",
		GatewayAddress: "192.168.1.1",
	}
	model.AddIPAddress(args)
	s.addMachineToModel(model, "42")
	model.AddLinkLayerDevice(LinkLayerDeviceArgs{Name: "foo", MachineID: "42"})
	err := model.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSerializationSuite) TestModelValidationChecksLinkLayerDeviceMachineId(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	model.AddLinkLayerDevice(LinkLayerDeviceArgs{Name: "foo", MachineID: "42"})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `device "foo" references non-existent machine "42"`)
	s.addMachineToModel(model, "42")
	err = model.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSerializationSuite) TestModelValidationChecksLinkLayerName(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	model.AddLinkLayerDevice(LinkLayerDeviceArgs{MachineID: "42"})
	s.addMachineToModel(model, "42")
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, "device has empty name.*")
}

func (s *ModelSerializationSuite) TestModelValidationChecksLinkLayerMACAddress(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := LinkLayerDeviceArgs{MachineID: "42", Name: "foo", MACAddress: "DEADBEEF"}
	model.AddLinkLayerDevice(args)
	s.addMachineToModel(model, "42")
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `device "foo" has invalid MACAddress "DEADBEEF"`)
}

func (s *ModelSerializationSuite) TestModelValidationChecksParentExists(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := LinkLayerDeviceArgs{MachineID: "42", Name: "foo", ParentName: "bar", MACAddress: "01:23:45:67:89:ab"}
	model.AddLinkLayerDevice(args)
	s.addMachineToModel(model, "42")
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `device "foo" has non-existent parent "bar"`)
	model.AddLinkLayerDevice(LinkLayerDeviceArgs{Name: "bar", MachineID: "42"})
	err = model.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSerializationSuite) TestModelValidationChecksParentIsNotItself(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := LinkLayerDeviceArgs{MachineID: "42", Name: "foo", ParentName: "foo"}
	model.AddLinkLayerDevice(args)
	s.addMachineToModel(model, "42")
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `device "foo" is its own parent`)
}

func (s *ModelSerializationSuite) TestModelValidationChecksParentIsABridge(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := LinkLayerDeviceArgs{MachineID: "42", Name: "foo", ParentName: "m#43#d#bar"}
	model.AddLinkLayerDevice(args)
	args2 := LinkLayerDeviceArgs{MachineID: "43", Name: "bar"}
	model.AddLinkLayerDevice(args2)
	s.addMachineToModel(model, "42")
	s.addMachineToModel(model, "43")
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `device "foo" on a container but not a bridge`)
}

func (s *ModelSerializationSuite) TestModelValidationChecksChildDeviceContained(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := LinkLayerDeviceArgs{MachineID: "42", Name: "foo", ParentName: "m#43#d#bar"}
	model.AddLinkLayerDevice(args)
	args2 := LinkLayerDeviceArgs{MachineID: "43", Name: "bar", Type: "bridge"}
	model.AddLinkLayerDevice(args2)
	s.addMachineToModel(model, "42")
	s.addMachineToModel(model, "43")
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `ParentName "m#43#d#bar" for non-container machine "42"`)
}

func (s *ModelSerializationSuite) TestModelValidationChecksParentOnHost(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := LinkLayerDeviceArgs{MachineID: "41/lxd/0", Name: "foo", ParentName: "m#43#d#bar"}
	model.AddLinkLayerDevice(args)
	args2 := LinkLayerDeviceArgs{MachineID: "43", Name: "bar", Type: "bridge"}
	model.AddLinkLayerDevice(args2)
	machine := s.addMachineToModel(model, "41")
	container := machine.AddContainer(MachineArgs{Id: names.NewMachineTag("41/lxd/0")})
	container.SetInstance(CloudInstanceArgs{InstanceId: "magic"})
	container.SetTools(minimalAgentToolsArgs())
	container.SetStatus(minimalStatusArgs())
	s.addMachineToModel(model, "43")
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `parent machine of device "foo" not host machine "41"`)
}

func (s *ModelSerializationSuite) TestModelValidationLinkLayerDeviceContainer(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	args := LinkLayerDeviceArgs{MachineID: "43/lxd/0", Name: "foo", ParentName: "m#43#d#bar"}
	model.AddLinkLayerDevice(args)
	args2 := LinkLayerDeviceArgs{MachineID: "43", Name: "bar", Type: "bridge"}
	model.AddLinkLayerDevice(args2)
	machine := s.addMachineToModel(model, "43")
	container := machine.AddContainer(MachineArgs{Id: names.NewMachineTag("43/lxd/0")})
	container.SetInstance(CloudInstanceArgs{InstanceId: "magic"})
	container.SetTools(minimalAgentToolsArgs())
	container.SetStatus(minimalStatusArgs())
	err := model.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSerializationSuite) TestSpaces(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	space := initial.AddSpace(SpaceArgs{Name: "special"})
	c.Assert(space.Name(), gc.Equals, "special")
	spaces := initial.Spaces()
	c.Assert(spaces, gc.HasLen, 1)
	c.Assert(spaces[0], gc.Equals, space)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Spaces(), jc.DeepEquals, spaces)

}

func (s *ModelSerializationSuite) TestLinkLayerDevice(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	device := initial.AddLinkLayerDevice(LinkLayerDeviceArgs{Name: "foo"})
	c.Assert(device.Name(), gc.Equals, "foo")
	devices := initial.LinkLayerDevices()
	c.Assert(devices, gc.HasLen, 1)
	c.Assert(devices[0], jc.DeepEquals, device)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.LinkLayerDevices(), jc.DeepEquals, devices)
}

func (s *ModelSerializationSuite) TestSubnets(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	subnet := initial.AddSubnet(SubnetArgs{CIDR: "10.0.0.0/24"})
	c.Assert(subnet.CIDR(), gc.Equals, "10.0.0.0/24")
	subnets := initial.Subnets()
	c.Assert(subnets, gc.HasLen, 1)
	c.Assert(subnets[0], jc.DeepEquals, subnet)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Subnets(), jc.DeepEquals, subnets)
}

func (s *ModelSerializationSuite) TestIPAddress(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	addr := initial.AddIPAddress(IPAddressArgs{Value: "10.0.0.4"})
	c.Assert(addr.Value(), gc.Equals, "10.0.0.4")
	addresses := initial.IPAddresses()
	c.Assert(addresses, gc.HasLen, 1)
	c.Assert(addresses[0], jc.DeepEquals, addr)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.IPAddresses(), jc.DeepEquals, addresses)
}

func (s *ModelSerializationSuite) TestSSHHostKey(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	key := initial.AddSSHHostKey(SSHHostKeyArgs{MachineID: "foo"})
	c.Assert(key.MachineID(), gc.Equals, "foo")
	keys := initial.SSHHostKeys()
	c.Assert(keys, gc.HasLen, 1)
	c.Assert(keys[0], jc.DeepEquals, key)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.SSHHostKeys(), jc.DeepEquals, keys)
}

func (s *ModelSerializationSuite) TestAction(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	enqueued := time.Now().UTC()
	action := initial.AddAction(ActionArgs{
		Name:       "foo",
		Enqueued:   enqueued,
		Parameters: map[string]interface{}{},
		Results:    map[string]interface{}{},
	})
	c.Assert(action.Name(), gc.Equals, "foo")
	c.Assert(action.Enqueued(), gc.Equals, enqueued)
	actions := initial.Actions()
	c.Assert(actions, gc.HasLen, 1)
	c.Assert(actions[0], jc.DeepEquals, action)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Actions(), jc.DeepEquals, actions)
}

func (s *ModelSerializationSuite) TestVolumeValidation(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	model.AddVolume(testVolumeArgs())
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `volume\[0\]: volume "1234" missing status not valid`)
}

func (s *ModelSerializationSuite) TestVolumes(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	volume := initial.AddVolume(testVolumeArgs())
	volume.SetStatus(minimalStatusArgs())
	volumes := initial.Volumes()
	c.Assert(volumes, gc.HasLen, 1)
	c.Assert(volumes[0], gc.Equals, volume)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Volumes(), jc.DeepEquals, volumes)
}

func (s *ModelSerializationSuite) TestFilesystemValidation(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	model.AddFilesystem(testFilesystemArgs())
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, `filesystem\[0\]: filesystem "1234" missing status not valid`)
}

func (s *ModelSerializationSuite) TestFilesystems(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	filesystem := initial.AddFilesystem(testFilesystemArgs())
	filesystem.SetStatus(minimalStatusArgs())
	filesystem.AddAttachment(testFilesystemAttachmentArgs())
	filesystems := initial.Filesystems()
	c.Assert(filesystems, gc.HasLen, 1)
	c.Assert(filesystems[0], gc.Equals, filesystem)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Filesystems(), jc.DeepEquals, filesystems)
}

func (s *ModelSerializationSuite) TestStorage(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	storage := initial.AddStorage(testStorageArgs())
	storages := initial.Storages()
	c.Assert(storages, gc.HasLen, 1)
	c.Assert(storages[0], jc.DeepEquals, storage)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Storages(), jc.DeepEquals, storages)
}

func (s *ModelSerializationSuite) TestStoragePools(c *gc.C) {
	initial := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	poolOne := map[string]interface{}{
		"foo":   42,
		"value": true,
	}
	poolTwo := map[string]interface{}{
		"value": "spanner",
	}
	initial.AddStoragePool(StoragePoolArgs{
		Name: "one", Provider: "sparkles", Attributes: poolOne})
	initial.AddStoragePool(StoragePoolArgs{
		Name: "two", Provider: "spanner", Attributes: poolTwo})

	pools := initial.StoragePools()
	c.Assert(pools, gc.HasLen, 2)
	one, two := pools[0], pools[1]
	c.Check(one.Name(), gc.Equals, "one")
	c.Check(one.Provider(), gc.Equals, "sparkles")
	c.Check(one.Attributes(), jc.DeepEquals, poolOne)
	c.Check(two.Name(), gc.Equals, "two")
	c.Check(two.Provider(), gc.Equals, "spanner")
	c.Check(two.Attributes(), jc.DeepEquals, poolTwo)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)

	pools = model.StoragePools()
	c.Assert(pools, gc.HasLen, 2)
	one, two = pools[0], pools[1]
	c.Check(one.Name(), gc.Equals, "one")
	c.Check(one.Provider(), gc.Equals, "sparkles")
	c.Check(one.Attributes(), jc.DeepEquals, poolOne)
	c.Check(two.Name(), gc.Equals, "two")
	c.Check(two.Provider(), gc.Equals, "spanner")
	c.Check(two.Attributes(), jc.DeepEquals, poolTwo)
}
