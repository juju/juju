package state

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/testing"
)

type TopologySuite struct {
	testing.ZkConnSuite
	t *topology
}

var _ = Suite(&TopologySuite{})

func (s *TopologySuite) SetUpTest(c *C) {
	var err error
	s.t, err = readTopology(s.ZkConn)
	c.Assert(err, IsNil)
}

func (s *TopologySuite) TestAddMachine(c *C) {
	// Check that adding machines works correctly.
	err := s.t.AddMachine("machine-0")
	c.Assert(err, IsNil)
	err = s.t.AddMachine("machine-1")
	c.Assert(err, IsNil)
	keys := s.t.MachineKeys()
	c.Assert(keys, DeepEquals, []string{"machine-0", "machine-1"})
}

func (s *TopologySuite) TestAddDuplicatedMachine(c *C) {
	// Check that adding a duplicated machine by key fails.
	err := s.t.AddMachine("machine-0")
	c.Assert(err, IsNil)
	err = s.t.AddMachine("machine-0")
	c.Assert(err, ErrorMatches, `attempted to add duplicated machine "machine-0"`)
}

func (s *TopologySuite) TestRemoveMachine(c *C) {
	// Check that removing machines works correctly.
	err := s.t.AddMachine("machine-0")
	c.Assert(err, IsNil)
	err = s.t.AddMachine("machine-1")
	c.Assert(err, IsNil)
	// Add non-assigned unit. This tests that the logic of
	// checking for assigned units works correctly too.
	err = s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-0", "")
	c.Assert(err, IsNil)

	err = s.t.RemoveMachine("machine-0")
	c.Assert(err, IsNil)

	found := s.t.HasMachine("machine-0")
	c.Assert(found, Equals, false)
	found = s.t.HasMachine("machine-1")
	c.Assert(found, Equals, true)
}

func (s *TopologySuite) TestRemoveNonExistentMachine(c *C) {
	// Check that the removing of a non-existent machine fails.
	err := s.t.RemoveMachine("machine-0")
	c.Assert(err, ErrorMatches, `machine with key "machine-0" not found`)
}

func (s *TopologySuite) TestAssignUnitToMachine(c *C) {
	// Check that we can assign principal units to machines
	err := s.t.AddMachine("machine-0")
	c.Assert(err, IsNil)
	err = s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-0", "")
	c.Assert(err, IsNil)
	err = s.t.AssignUnitToMachine("unit-0-0", "machine-0")
	c.Assert(err, IsNil)

	// Check that we cannot assign subordinate units to machines
	err = s.t.AddMachine("machine-1")
	c.Assert(err, IsNil)
	err = s.t.AddService("service-1", "rsyslog")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-1-1", "unit-0-0")
	c.Assert(err, IsNil)
	err = s.t.AssignUnitToMachine("unit-1-1", "machine-0")
	c.Assert(err, ErrorMatches, "cannot assign subordinate units directly to machines")
}

func (s *TopologySuite) TestRemoveMachineWithAssignedUnits(c *C) {
	// Check that a machine can't be removed when it has assigned units.
	err := s.t.AddMachine("machine-0")
	c.Assert(err, IsNil)
	err = s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-0", "")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-1", "")
	c.Assert(err, IsNil)
	err = s.t.AssignUnitToMachine("unit-0-1", "machine-0")
	c.Assert(err, IsNil)
	err = s.t.RemoveMachine("machine-0")
	c.Assert(err, ErrorMatches, `can't remove machine "machine-0" while units ared assigned`)
}

func (s *TopologySuite) TestMachineHasUnits(c *C) {
	// Check various ways a machine might or might not be assigned
	// to a unit.
	err := s.t.AddMachine("machine-0")
	c.Assert(err, IsNil)
	err = s.t.AddMachine("machine-1")
	c.Assert(err, IsNil)
	err = s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-0", "")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-1", "")
	c.Assert(err, IsNil)
	err = s.t.AssignUnitToMachine("unit-0-1", "machine-0")
	c.Assert(err, IsNil)
	ok, err := s.t.MachineHasUnits("machine-0")
	c.Assert(err, IsNil)
	c.Assert(ok, Equals, true)
	ok, err = s.t.MachineHasUnits("machine-1")
	c.Assert(err, IsNil)
	c.Assert(ok, Equals, false)
	ok, err = s.t.MachineHasUnits("machine-99")
	c.Assert(err, ErrorMatches, `machine with key "machine-99" not found`)
}

func (s *TopologySuite) TestHasMachine(c *C) {
	// Check that the test for a machine works correctly.
	found := s.t.HasMachine("machine-0")
	c.Assert(found, Equals, false)
	err := s.t.AddMachine("machine-0")
	c.Assert(err, IsNil)
	found = s.t.HasMachine("machine-0")
	c.Assert(found, Equals, true)
	found = s.t.HasMachine("machine-1")
	c.Assert(found, Equals, false)
}

func (s *TopologySuite) TestMachineKeys(c *C) {
	// Check that the retrieval of all services keys works correctly.
	keys := s.t.MachineKeys()
	c.Assert(keys, DeepEquals, []string{})
	err := s.t.AddMachine("machine-0")
	c.Assert(err, IsNil)
	err = s.t.AddMachine("machine-1")
	c.Assert(err, IsNil)
	keys = s.t.MachineKeys()
	c.Assert(keys, DeepEquals, []string{"machine-0", "machine-1"})
}

func (s *TopologySuite) TestAddService(c *C) {
	// Check that adding services works correctly.
	c.Assert(s.t.HasService("service-0"), Equals, false)
	err := s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("service-1", "mysql")
	c.Assert(err, IsNil)
	c.Assert(s.t.HasService("service-0"), Equals, true)
	c.Assert(s.t.HasService("service-1"), Equals, true)
}

func (s *TopologySuite) TestAddDuplicatedService(c *C) {
	// Check that adding a duplicated service by key or name fails.
	err := s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("service-0", "mysql")
	c.Assert(err, ErrorMatches, `attempted to add duplicated service "service-0"`)
	err = s.t.AddService("service-1", "wordpress")
	c.Assert(err, ErrorMatches, `service name "wordpress" already in use`)
}

func (s *TopologySuite) TestHasService(c *C) {
	// Check that the test for a service works correctly.
	found := s.t.HasService("service-0")
	c.Assert(found, Equals, false)
	err := s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	found = s.t.HasService("service-0")
	c.Assert(found, Equals, true)
	found = s.t.HasService("service-1")
	c.Assert(found, Equals, false)
}

func (s *TopologySuite) TestServiceKey(c *C) {
	// Check that the key retrieval for a service name works correctly.
	key, err := s.t.ServiceKey("wordpress")
	c.Assert(err, ErrorMatches, `service with name "wordpress" not found`)
	err = s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	key, err = s.t.ServiceKey("wordpress")
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "service-0")
}

func (s *TopologySuite) TestServiceKeys(c *C) {
	// Check that the retrieval of all services keys works correctly.
	keys := s.t.ServiceKeys()
	c.Assert(keys, DeepEquals, []string{})
	err := s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("service-1", "mysql")
	c.Assert(err, IsNil)
	keys = s.t.ServiceKeys()
	c.Assert(keys, DeepEquals, []string{"service-0", "service-1"})
}

func (s *TopologySuite) TestServiceName(c *C) {
	// Check that the name retrieval for a service name works correctly.
	name, err := s.t.ServiceName("service-0")
	c.Assert(err, ErrorMatches, `service with key "service-0" not found`)
	err = s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	name, err = s.t.ServiceName("service-0")
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "wordpress")
}

func (s *TopologySuite) TestRemoveService(c *C) {
	// Check that the removing of a service works correctly.
	err := s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("service-1", "mysql")
	c.Assert(err, IsNil)
	err = s.t.RemoveService("service-0")
	c.Assert(err, IsNil)
	c.Assert(s.t.HasService("service-0"), Equals, false)
	c.Assert(s.t.HasService("service-1"), Equals, true)
}

func (s *TopologySuite) TestRemoveNonExistentService(c *C) {
	// Check that the removing of a non-existent service fails.
	err := s.t.RemoveService("service-99")
	c.Assert(err, ErrorMatches, `service with key "service-99" not found`)
}

func (s *TopologySuite) TestAddUnit(c *C) {
	// Check that the adding of a unit works correctly.
	err := s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("service-1", "mysql")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-05", "")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-12", "")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-1-07", "")
	c.Assert(err, IsNil)
	keys, err := s.t.UnitKeys("service-0")
	c.Assert(err, IsNil)
	c.Assert(keys, DeepEquals, []string{"unit-0-05", "unit-0-12"})
	keys, err = s.t.UnitKeys("service-1")
	c.Assert(err, IsNil)
	c.Assert(keys, DeepEquals, []string{"unit-1-07"})
}

func (s *TopologySuite) TestAddUnitSubordinate(c *C) {
	err := s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("service-1", "rsyslog")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-05", "")
	c.Assert(err, IsNil)
	_, err = s.t.UnitPrincipalKey("unit-0-05")
	c.Assert(err, Equals, unitNotSubordinate)
	err = s.t.AddUnit("unit-0-12", "")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-1-07", "unit-0-05")
	principal, err := s.t.UnitPrincipalKey("unit-1-07")
	c.Assert(err, IsNil)
	c.Assert(principal, Equals, "unit-0-05")
}

func (s *TopologySuite) TestAddDuplicatedUnit(c *C) {
	// Check that it's not possible to add a unit twice.
	err := s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-0", "")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-0", "")
	c.Assert(err, ErrorMatches, `unit "unit-0-0" already in use`)
}

func (s *TopologySuite) TestAddUnitToNonExistingService(c *C) {
	// Check that the adding of a unit to a non-existing services
	// fails correctly.
	err := s.t.AddUnit("unit-0-0", "")
	c.Assert(err, ErrorMatches, `service with key "service-0" not found`)
}

func (s *TopologySuite) TestUnitKeys(c *C) {
	// Check if registered units from a service are returned correctly.
	err := s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("service-1", "mysql")
	c.Assert(err, IsNil)
	units, err := s.t.UnitKeys("service-0")
	c.Assert(err, IsNil)
	c.Assert(units, DeepEquals, []string{})
	err = s.t.AddUnit("unit-0-0", "")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-1", "")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-1-2", "")
	c.Assert(err, IsNil)
	units, err = s.t.UnitKeys("service-0")
	c.Assert(err, IsNil)
	c.Assert(units, DeepEquals, []string{"unit-0-0", "unit-0-1"})
	units, err = s.t.UnitKeys("service-1")
	c.Assert(err, IsNil)
	c.Assert(units, DeepEquals, []string{"unit-1-2"})
}

func (s *TopologySuite) TestUnitKeysWithNonExistingService(c *C) {
	// Check if the retrieving of unit keys from a non-existing
	// service fails correctly.
	_, err := s.t.UnitKeys("service-0")
	c.Assert(err, ErrorMatches, `service with key "service-0" not found`)
}

func (s *TopologySuite) TestHasUnit(c *C) {
	// Check that the test for a unit in a service works correctly.
	err := s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	found := s.t.HasUnit("unit-0-0")
	c.Assert(found, Equals, false)
	err = s.t.AddUnit("unit-0-0", "")
	c.Assert(err, IsNil)
	found = s.t.HasUnit("unit-0-0")
	c.Assert(found, Equals, true)
	found = s.t.HasUnit("unit-0-1")
	c.Assert(found, Equals, false)
}

func (s *TopologySuite) TestUnitName(c *C) {
	// Check that the human readable names are returned correctly.
	err := s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddService("service-1", "mysql")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-0", "")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-1", "")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-1-2", "")
	c.Assert(err, IsNil)
	name, err := s.t.UnitName("unit-0-0")
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "wordpress/0")
	name, err = s.t.UnitName("unit-0-1")
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "wordpress/1")
	name, err = s.t.UnitName("unit-1-2")
	c.Assert(err, IsNil)
	c.Assert(name, Equals, "mysql/2")
}

func (s *TopologySuite) TestUnitNameWithNonExistingServiceOrUnit(c *C) {
	// Check if the retrieval of unit names fails if the service
	// or the unit doesn't exist.
	_, err := s.t.UnitName("unit-0-1")
	c.Assert(err, ErrorMatches, `service with key "service-0" not found`)
	err = s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	_, err = s.t.UnitName("unit-0-1")
	c.Assert(err, ErrorMatches, `unit with key "unit-0-1" not found`)
	err = s.t.AddUnit("unit-0-0", "")
	c.Assert(err, IsNil)
	_, err = s.t.UnitName("unit-0-1")
	c.Assert(err, ErrorMatches, `unit with key "unit-0-1" not found`)
}

func (s *TopologySuite) TestRemoveUnit(c *C) {
	// Check that the removing of a unit works correctly.
	err := s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-0", "")
	c.Assert(err, IsNil)
	err = s.t.AddUnit("unit-0-1", "")
	c.Assert(err, IsNil)
	err = s.t.RemoveUnit("unit-0-0")
	c.Assert(err, IsNil)
	found := s.t.HasUnit("unit-0-0")
	c.Assert(found, Equals, false)
	found = s.t.HasUnit("unit-0-1")
	c.Assert(found, Equals, true)
}

func (s *TopologySuite) TestRemoveNonExistingUnit(c *C) {
	// Check that the removing of non-existing units fails.
	err := s.t.RemoveUnit("unit-0-0")
	c.Assert(err, ErrorMatches, `service with key "service-0" not found`)
	err = s.t.AddService("service-0", "wordpress")
	c.Assert(err, IsNil)
	err = s.t.RemoveUnit("unit-0-0")
	c.Assert(err, ErrorMatches, `unit with key "unit-0-0" not found`)
}

func (s *TopologySuite) TestRelation(c *C) {
	// Check that the retrieving of relations works correctly.
	relation, err := s.t.Relation("relation-1")
	c.Assert(relation, IsNil)
	c.Assert(err, ErrorMatches, `relation "relation-1" does not exist`)
	s.t.AddService("service-p", "riak")
	r := &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{topoEndpoint{"service-p", RolePeer, "cache"}},
	}
	s.t.AddRelation("relation-1", r)
	relation, err = s.t.Relation("relation-1")
	c.Assert(err, IsNil)
	c.Assert(relation, DeepEquals, r)
}

func (s *TopologySuite) TestAddRelation(c *C) {
	// Check that adding a relation works and can only be done once and with 
	// valid services.
	relation, err := s.t.Relation("relation-1")
	c.Assert(relation, IsNil)
	c.Assert(err, ErrorMatches, `relation "relation-1" does not exist`)
	s.t.AddService("service-p", "mysql")
	s.t.AddService("service-r", "wordpress")
	r := &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RoleProvider, "db"},
			topoEndpoint{"service-r", RoleRequirer, "db"},
		},
	}
	err = s.t.AddRelation("relation-1", r)
	c.Assert(err, IsNil)
	relation, err = s.t.Relation("relation-1")
	c.Assert(err, IsNil)
	c.Assert(relation, DeepEquals, r)

	err = s.t.AddRelation("relation-2", &topoRelation{
		Interface: "",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RoleProvider, "db"},
			topoEndpoint{"service-r", RoleRequirer, "db"},
		},
	})
	c.Assert(err, ErrorMatches, `relation interface is empty`)

	err = s.t.AddRelation("relation-3", &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{},
	})
	c.Assert(err, ErrorMatches, `relation has no services`)

	err = s.t.AddRelation("relation-4", &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RoleProvider, "db"},
		},
	})
	c.Assert(err, ErrorMatches, `relation has provider but no requirer`)

	err = s.t.AddRelation("relation-5", &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RoleProvider, "db"},
			topoEndpoint{"service-r", RolePeer, "db"},
		},
	})
	c.Assert(err, ErrorMatches, `relation has provider but no requirer`)

	err = s.t.AddRelation("relation-6", &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RoleProvider, "db"},
			topoEndpoint{"service-r", RoleRequirer, "db"},
			topoEndpoint{"service-e", RolePeer, "db"},
		},
	})
	c.Assert(err, ErrorMatches, `relation with mixed peer, provider, and requirer roles`)

	err = s.t.AddRelation("relation-7", &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RoleProvider, "db"},
			topoEndpoint{"illegal", RoleRequirer, "db"},
		},
	})
	c.Assert(err, ErrorMatches, `service with key "illegal" not found`)

	err = s.t.AddRelation("relation-1", &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RoleProvider, "db"},
			topoEndpoint{"service-r", RoleRequirer, "db"},
		},
	})
	c.Assert(err, ErrorMatches, `relation key "relation-1" already in use`)
}

func (s *TopologySuite) TestRelationKeys(c *C) {
	// Check that fetching the relation keys works.
	keys := s.t.RelationKeys()
	c.Assert(keys, DeepEquals, []string{})

	s.t.AddService("service-p", "riak")
	s.t.AddRelation("relation-1", &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RolePeer, "cache"},
		},
	})
	keys = s.t.RelationKeys()
	c.Assert(keys, DeepEquals, []string{"relation-1"})

	s.t.AddRelation("relation-2", &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RolePeer, "cache"},
		},
	})
	keys = s.t.RelationKeys()
	c.Assert(keys, DeepEquals, []string{"relation-1", "relation-2"})
}

func (s *TopologySuite) TestRelationsForService(c *C) {
	// Check that fetching the relations for a service works.
	s.t.AddService("service-p", "riak")
	relations, err := s.t.RelationsForService("service-p")
	c.Assert(err, IsNil)
	c.Assert(relations, HasLen, 0)

	s.t.AddRelation("relation-0", &topoRelation{
		Interface: "ifce0",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RolePeer, "cache"},
		},
	})
	s.t.AddRelation("relation-1", &topoRelation{
		Interface: "ifce1",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RolePeer, "cache"},
		},
	})
	relations, err = s.t.RelationsForService("service-p")
	c.Assert(err, IsNil)
	c.Assert(relations, HasLen, 2)
	c.Assert(relations["relation-0"].Interface, Equals, "ifce0")
	c.Assert(relations["relation-1"].Interface, Equals, "ifce1")

	s.t.RemoveRelation("relation-0")
	relations, err = s.t.RelationsForService("service-p")
	c.Assert(err, IsNil)
	c.Assert(relations, HasLen, 1)
	c.Assert(relations["relation-1"].Interface, Equals, "ifce1")
}

func (s *TopologySuite) TestRemoveRelation(c *C) {
	// Check that removing of a relation works.
	s.t.AddService("service-r", "wordpress")
	s.t.AddService("service-p", "mysql")

	err := s.t.AddRelation("relation-1", &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RoleProvider, "db"},
			topoEndpoint{"service-r", RoleRequirer, "db"},
		},
	})
	c.Assert(err, IsNil)
	err = s.t.RemoveRelation("relation-1")
	c.Assert(err, IsNil)
	relation, err := s.t.Relation("relation-1")
	c.Assert(relation, IsNil)
	c.Assert(err, ErrorMatches, `relation "relation-1" does not exist`)
}

func (s *TopologySuite) TestRemoveServiceWithRelations(c *C) {
	// Check that the removing of a service with
	// associated relations leads to an error.
	s.t.AddService("service-p", "riak")
	s.t.AddRelation("relation-1", &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RolePeer, "cache"},
		},
	})

	err := s.t.RemoveService("service-p")
	c.Assert(err, ErrorMatches, `cannot remove service "service-p" with active relations`)
}

func (s *TopologySuite) TestRelationKeyEndpoints(c *C) {
	mysqlep1 := RelationEndpoint{"mysql", "ifce1", "db", RoleProvider, ScopeGlobal}
	blogep1 := RelationEndpoint{"wordpress", "ifce1", "db", RoleRequirer, ScopeGlobal}
	mysqlep2 := RelationEndpoint{"mysql", "ifce2", "db", RoleProvider, ScopeGlobal}
	blogep2 := RelationEndpoint{"wordpress", "ifce2", "db", RoleRequirer, ScopeGlobal}
	mysqlep3 := RelationEndpoint{"mysql", "ifce3", "db", RoleProvider, ScopeGlobal}
	blogep3 := RelationEndpoint{"wordpress", "ifce3", "db", RoleRequirer, ScopeGlobal}
	s.t.AddService("service-r", "wordpress")
	s.t.AddService("service-p", "mysql")
	s.t.AddRelation("relation-0", &topoRelation{
		Interface: "ifce1",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RoleProvider, "db"},
			topoEndpoint{"service-r", RoleRequirer, "db"},
		},
	})
	s.t.AddRelation("relation-1", &topoRelation{
		Interface: "ifce2",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RoleProvider, "db"},
			topoEndpoint{"service-r", RoleRequirer, "db"},
		},
	})

	// Valid relations.
	key, err := s.t.RelationKey(mysqlep1, blogep1)
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "relation-0")
	key, err = s.t.RelationKey(blogep1, mysqlep1)
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "relation-0")
	key, err = s.t.RelationKey(mysqlep2, blogep2)
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "relation-1")
	key, err = s.t.RelationKey(blogep2, mysqlep2)
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "relation-1")

	// Endpoints without relation.
	_, err = s.t.RelationKey(mysqlep3, blogep3)
	c.Assert(err, Equals, noRelationFound)

	// Mix of endpoints of two relations.
	_, err = s.t.RelationKey(mysqlep1, blogep2)
	c.Assert(err, Equals, noRelationFound)

	// Illegal number of endpoints.
	_, err = s.t.RelationKey()
	c.Assert(err, ErrorMatches, `illegal number of relation endpoints provided`)
	_, err = s.t.RelationKey(mysqlep1, mysqlep2, blogep1)
	c.Assert(err, ErrorMatches, `illegal number of relation endpoints provided`)
}

func (s *TopologySuite) TestRelationKeyIllegalEndpoints(c *C) {
	mysqlep1 := RelationEndpoint{"mysql", "ifce", "db", RoleProvider, ScopeGlobal}
	blogep1 := RelationEndpoint{"wordpress", "ifce", "db", RoleRequirer, ScopeGlobal}
	mysqlep2 := RelationEndpoint{"illegal-mysql", "ifce", "db", RoleProvider, ScopeGlobal}
	blogep2 := RelationEndpoint{"illegal-wordpress", "ifce", "db", RoleRequirer, ScopeGlobal}
	riakep3 := RelationEndpoint{"riak", "ifce", "ring", RolePeer, ScopeGlobal}
	s.t.AddService("service-r", "wordpress")
	s.t.AddService("service-p1", "mysql")
	s.t.AddService("service-p2", "riak")
	s.t.AddRelation("relation-0", &topoRelation{
		Interface: "ifce1",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RoleProvider, "db"},
			topoEndpoint{"service-r", RoleRequirer, "db"},
		},
	})

	key, err := s.t.RelationKey(mysqlep1, blogep2)
	c.Assert(key, Equals, "")
	c.Assert(err, Equals, noRelationFound)
	key, err = s.t.RelationKey(mysqlep2, blogep1)
	c.Assert(key, Equals, "")
	c.Assert(err, Equals, noRelationFound)
	key, err = s.t.RelationKey(mysqlep1, riakep3)
	c.Assert(key, Equals, "")
	c.Assert(err, Equals, noRelationFound)
}

func (s *TopologySuite) TestPeerRelationKeyEndpoints(c *C) {
	riakep1 := RelationEndpoint{"riak", "ifce1", "ring", RolePeer, ScopeGlobal}
	riakep2 := RelationEndpoint{"riak", "ifce2", "ring", RolePeer, ScopeGlobal}
	riakep3 := RelationEndpoint{"riak", "ifce3", "ring", RolePeer, ScopeGlobal}
	s.t.AddService("service-p", "riak")
	s.t.AddRelation("relation-0", &topoRelation{
		Interface: "ifce1",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RolePeer, "ring"},
		},
	})
	s.t.AddRelation("relation-1", &topoRelation{
		Interface: "ifce2",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RolePeer, "ring"},
		},
	})

	// Valid relations.
	key, err := s.t.RelationKey(riakep1)
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "relation-0")
	key, err = s.t.RelationKey(riakep2)
	c.Assert(err, IsNil)
	c.Assert(key, Equals, "relation-1")

	// Endpoint without relation.
	key, err = s.t.RelationKey(riakep3)
	c.Assert(err, Equals, noRelationFound)
}

func (s *TopologySuite) TestPeerRelationKeyIllegalEndpoints(c *C) {
	riakep1 := RelationEndpoint{"riak", "ifce", "illegal-ring", RolePeer, ScopeGlobal}
	s.t.AddService("service-p", "riak")
	s.t.AddRelation("relation-0", &topoRelation{
		Interface: "ifce",
		Scope:     ScopeGlobal,
		Endpoints: []topoEndpoint{
			topoEndpoint{"service-p", RolePeer, "ring"},
		},
	})

	key, err := s.t.RelationKey(riakep1)
	c.Assert(key, Equals, "")
	c.Assert(err, Equals, noRelationFound)
}

type ConfigNodeSuite struct {
	testing.ZkConnSuite
	path string
}

var _ = Suite(&ConfigNodeSuite{})

func (s *ConfigNodeSuite) SetUpSuite(c *C) {
	s.ZkConnSuite.SetUpSuite(c)
	s.path = "/config"
}

func (s *ConfigNodeSuite) TestCreateEmptyConfigNode(c *C) {
	// Check that creating an empty node works correctly.
	node, err := createConfigNode(s.ZkConn, s.path, nil)
	c.Assert(err, IsNil)
	c.Assert(node.Keys(), DeepEquals, []string{})
}

func (s *ConfigNodeSuite) TestReadWithoutWrite(c *C) {
	// Check reading without writing.
	node, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	c.Assert(node, Not(IsNil))
}

func (s *ConfigNodeSuite) TestSetWithoutWrite(c *C) {
	// Check that config values can be set.
	_, err := s.ZkConn.Create(s.path, "", 0, zkPermAll)
	c.Assert(err, IsNil)
	node, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Update(options)
	c.Assert(node.Map(), DeepEquals, options)
	// Node data has to be empty.
	yaml, _, err := s.ZkConn.Get("/config")
	c.Assert(err, IsNil)
	c.Assert(yaml, Equals, "")
}

func (s *ConfigNodeSuite) TestSetWithWrite(c *C) {
	// Check that write updates the local and the ZooKeeper state.
	node, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Update(options)
	changes, err := node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "one", nil, 1},
	})
	// Check local state.
	c.Assert(node.Map(), DeepEquals, options)
	// Check ZooKeeper state.
	yaml, _, err := s.ZkConn.Get(s.path)
	c.Assert(err, IsNil)
	zkData := make(map[string]interface{})
	err = goyaml.Unmarshal([]byte(yaml), zkData)
	c.Assert(zkData, DeepEquals, options)
}

func (s *ConfigNodeSuite) TestConflictOnSet(c *C) {
	// Check version conflict errors.
	nodeOne, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	nodeTwo, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)

	optionsOld := map[string]interface{}{"alpha": "beta", "one": 1}
	nodeOne.Update(optionsOld)
	nodeOne.Write()

	nodeTwo.Update(optionsOld)
	changes, err := nodeTwo.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "one", nil, 1},
	})

	// First test node one.
	c.Assert(nodeOne.Map(), DeepEquals, optionsOld)

	// Write on node one.
	optionsNew := map[string]interface{}{"alpha": "gamma", "one": "two"}
	nodeOne.Update(optionsNew)
	changes, err = nodeOne.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemModified, "alpha", "beta", "gamma"},
		{ItemModified, "one", 1, "two"},
	})

	// Verify that node one reports as expected.
	c.Assert(nodeOne.Map(), DeepEquals, optionsNew)

	// Verify that node two has still the old data.
	c.Assert(nodeTwo.Map(), DeepEquals, optionsOld)

	// Now issue a Set/Write from node two. This will
	// merge the data deleting 'one' and updating
	// other values.
	optionsMerge := map[string]interface{}{"alpha": "cappa", "new": "next"}
	nodeTwo.Update(optionsMerge)
	nodeTwo.Delete("one")

	expected := map[string]interface{}{"alpha": "cappa", "new": "next"}
	changes, err = nodeTwo.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemModified, "alpha", "beta", "cappa"},
		{ItemAdded, "new", nil, "next"},
		{ItemDeleted, "one", 1, nil},
	})
	c.Assert(expected, DeepEquals, nodeTwo.Map())

	// But node one still reflects the former data.
	c.Assert(nodeOne.Map(), DeepEquals, optionsNew)
}

func (s *ConfigNodeSuite) TestSetItem(c *C) {
	// Check that Set works as expected.
	node, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	options := map[string]interface{}{"alpha": "beta", "one": 1}
	node.Set("alpha", "beta")
	node.Set("one", 1)
	changes, err := node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "one", nil, 1},
	})
	// Check local state.
	c.Assert(node.Map(), DeepEquals, options)
	// Check ZooKeeper state.
	yaml, _, err := s.ZkConn.Get(s.path)
	c.Assert(err, IsNil)
	zkData := make(map[string]interface{})
	err = goyaml.Unmarshal([]byte(yaml), zkData)
	c.Assert(zkData, DeepEquals, options)
}

func (s *ConfigNodeSuite) TestMultipleReads(c *C) {
	// Check that reads without writes always resets the data.
	nodeOne, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	nodeOne.Update(map[string]interface{}{"alpha": "beta", "foo": "bar"})
	value, ok := nodeOne.Get("alpha")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "bar")
	value, ok = nodeOne.Get("baz")
	c.Assert(ok, Equals, false)

	// A read resets the data to the empty state.
	err = nodeOne.Read()
	c.Assert(err, IsNil)
	c.Assert(nodeOne.Map(), DeepEquals, map[string]interface{}{})
	nodeOne.Update(map[string]interface{}{"alpha": "beta", "foo": "bar"})
	changes, err := nodeOne.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "alpha", nil, "beta"},
		{ItemAdded, "foo", nil, "bar"},
	})

	// A write retains the newly set values.
	value, ok = nodeOne.Get("alpha")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "bar")

	// Now get another state instance and change ZooKeeper state.
	nodeTwo, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	nodeTwo.Update(map[string]interface{}{"foo": "different"})
	changes, err = nodeTwo.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemModified, "foo", "bar", "different"},
	})

	// This should pull in the new state into node one.
	err = nodeOne.Read()
	c.Assert(err, IsNil)
	value, ok = nodeOne.Get("alpha")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "beta")
	value, ok = nodeOne.Get("foo")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "different")
}

func (s *ConfigNodeSuite) TestDeleteEmptiesState(c *C) {
	// Check that delete creates an empty state.
	node, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	node.Set("a", "foo")
	changes, err := node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})
	node.Delete("a")
	changes, err = node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemDeleted, "a", "foo", nil},
	})
	c.Assert(node.Map(), DeepEquals, map[string]interface{}{})
}

func (s *ConfigNodeSuite) TestReadResync(c *C) {
	// Check that read pulls the data into the node.
	nodeOne, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})
	nodeTwo, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	nodeTwo.Delete("a")
	changes, err = nodeTwo.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemDeleted, "a", "foo", nil},
	})
	nodeTwo.Set("a", "bar")
	changes, err = nodeTwo.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "bar"},
	})
	// Read of node one should pick up the new value.
	err = nodeOne.Read()
	c.Assert(err, IsNil)
	value, ok := nodeOne.Get("a")
	c.Assert(ok, Equals, true)
	c.Assert(value, Equals, "bar")
}

func (s *ConfigNodeSuite) TestMultipleWrites(c *C) {
	// Check that multiple writes only do the right changes.
	node, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	node.Update(map[string]interface{}{"foo": "bar", "this": "that"})
	changes, err := node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "foo", nil, "bar"},
		{ItemAdded, "this", nil, "that"},
	})
	node.Delete("this")
	node.Set("another", "value")
	changes, err = node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "another", nil, "value"},
		{ItemDeleted, "this", "that", nil},
	})

	expected := map[string]interface{}{"foo": "bar", "another": "value"}
	c.Assert(expected, DeepEquals, node.Map())

	changes, err = node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{})

	err = node.Read()
	c.Assert(err, IsNil)
	c.Assert(expected, DeepEquals, node.Map())

	changes, err = node.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{})
}

func (s *ConfigNodeSuite) TestWriteTwice(c *C) {
	// Check the correct writing into a node by two config nodes.
	nodeOne, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	nodeOne.Set("a", "foo")
	changes, err := nodeOne.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemAdded, "a", nil, "foo"},
	})

	nodeTwo, err := readConfigNode(s.ZkConn, s.path)
	c.Assert(err, IsNil)
	nodeTwo.Set("a", "bar")
	changes, err = nodeTwo.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{
		{ItemModified, "a", "foo", "bar"},
	})

	// Shouldn't write again. Changes were already
	// flushed and acted upon by other parties.
	changes, err = nodeOne.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []ItemChange{})

	err = nodeOne.Read()
	c.Assert(err, IsNil)
	c.Assert(nodeOne, DeepEquals, nodeTwo)
}
