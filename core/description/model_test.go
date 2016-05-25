// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
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

func (*ModelSerializationSuite) TestUpdateConfig(c *gc.C) {
	model := NewModel(ModelArgs{Config: map[string]interface{}{
		"name": "awesome",
		"uuid": "some-uuid",
	}})
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
	addMinimalService(initial)
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
	services := model.Services()
	c.Assert(services, gc.HasLen, 1)
	c.Assert(services[0].Name(), gc.Equals, "ubuntu")
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
	initial.SetSequence("service-foo", 3)
	initial.SetSequence("service-bar", 1)
	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(model.Sequences(), jc.DeepEquals, map[string]int{
		"machine":     4,
		"service-foo": 3,
		"service-bar": 1,
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
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
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
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	s.addMachineToModel(model, "0")
	err := model.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelSerializationSuite) TestModelValidationChecksOpenPortsUnits(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
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

func (*ModelSerializationSuite) TestModelValidationChecksServices(c *gc.C) {
	model := NewModel(ModelArgs{Owner: names.NewUserTag("owner")})
	model.AddService(ServiceArgs{})
	err := model.Validate()
	c.Assert(err, gc.ErrorMatches, "service missing name not valid")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *ModelSerializationSuite) addServiceToModel(model Model, name string, numUnits int) Service {
	service := model.AddService(ServiceArgs{
		Tag:                names.NewServiceTag(name),
		Settings:           map[string]interface{}{},
		LeadershipSettings: map[string]interface{}{},
	})
	service.SetStatus(minimalStatusArgs())
	for i := 0; i < numUnits; i++ {
		// The index i is used as both the machine id and the unit id.
		// A happy coincidence.
		machine := s.addMachineToModel(model, fmt.Sprint(i))
		unit := service.AddUnit(UnitArgs{
			Tag:     names.NewUnitTag(fmt.Sprintf("%s/%d", name, i)),
			Machine: machine.Tag(),
		})
		unit.SetTools(minimalAgentToolsArgs())
		unit.SetAgentStatus(minimalStatusArgs())
		unit.SetWorkloadStatus(minimalStatusArgs())
	}

	return service
}

func (s *ModelSerializationSuite) wordpressModel() (Model, Endpoint, Endpoint) {
	model := NewModel(ModelArgs{
		Owner: names.NewUserTag("owner"),
		Config: map[string]interface{}{
			"uuid": "some-uuid",
		}})
	s.addServiceToModel(model, "wordpress", 2)
	s.addServiceToModel(model, "mysql", 1)

	// Add a relation between wordpress and mysql.
	rel := model.AddRelation(RelationArgs{
		Id:  42,
		Key: "special key",
	})
	wordpressEndpoint := rel.AddEndpoint(EndpointArgs{
		ServiceName: "wordpress",
		Name:        "db",
		// Ignoring other aspects of endpoints.
	})
	mysqlEndpoint := rel.AddEndpoint(EndpointArgs{
		ServiceName: "mysql",
		Name:        "mysql",
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
