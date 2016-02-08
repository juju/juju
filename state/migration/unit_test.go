// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type UnitSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&UnitSerializationSuite{})

func (s *UnitSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "units"
	s.sliceName = "units"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importUnits(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["units"] = []interface{}{}
	}
}

func (*UnitSerializationSuite) TestUnitsIsMap(c *gc.C) {
	_, err := importUnits(map[string]interface{}{
		"version": 42,
		"units":   []interface{}{"hello"},
	})
	c.Check(err.Error(), gc.Equals, `units version schema check failed: units[0]: expected map, got string("hello")`)
}

func minimalUnitMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"name":            "ubuntu/0",
		"machine":         "0",
		"agent-status":    minimalStatusMap(),
		"workload-status": minimalStatusMap(),
		"password-hash":   "secure-hash",
		"tools":           minimalAgentToolsMap(),
	}
}

func minimalUnit() *unit {
	u := newUnit(minimalUnitArgs())
	u.SetAgentStatus(minimalStatusArgs())
	u.SetWorkloadStatus(minimalStatusArgs())
	u.SetTools(minimalAgentToolsArgs())
	return u
}

func minimalUnitArgs() UnitArgs {
	return UnitArgs{
		Tag:          names.NewUnitTag("ubuntu/0"),
		Machine:      names.NewMachineTag("0"),
		PasswordHash: "secure-hash",
	}
}

func (s *UnitSerializationSuite) completeUnit() *unit {
	// This unit is about completeness, not reasonableness. That is why the
	// unit has a principle (normally only for subordinates), and also a list
	// of subordinates.
	args := UnitArgs{
		Tag:          names.NewUnitTag("ubuntu/0"),
		Machine:      names.NewMachineTag("0"),
		PasswordHash: "secure-hash",
		Principal:    names.NewUnitTag("principal/0"),
		Subordinates: []names.UnitTag{
			names.NewUnitTag("sub1/0"),
			names.NewUnitTag("sub2/0"),
		},
	}
	unit := newUnit(args)
	unit.SetAgentStatus(minimalStatusArgs())
	unit.SetWorkloadStatus(minimalStatusArgs())
	unit.SetAddresses(AddressArgs{
		Value: "8.8.8.8",
		Type:  "public",
	}, AddressArgs{
		Value: "10.10.10.10",
		Type:  "private",
	})
	unit.SetTools(minimalAgentToolsArgs())
	return unit
}

func (s *UnitSerializationSuite) TestNewUnit(c *gc.C) {
	unit := s.completeUnit()

	c.Assert(unit.Tag(), gc.Equals, names.NewUnitTag("ubuntu/0"))
	c.Assert(unit.Name(), gc.Equals, "ubuntu/0")
	c.Assert(unit.Machine(), gc.Equals, names.NewMachineTag("0"))
	c.Assert(unit.PasswordHash(), gc.Equals, "secure-hash")
	c.Assert(unit.Principal(), gc.Equals, names.NewUnitTag("principal/0"))
	c.Assert(unit.Subordinates(), jc.DeepEquals, []names.UnitTag{
		names.NewUnitTag("sub1/0"),
		names.NewUnitTag("sub2/0"),
	})

	publicAddress := unit.PublicAddress()
	c.Assert(publicAddress.Value(), gc.Equals, "8.8.8.8")
	c.Assert(publicAddress.Type(), gc.Equals, "public")

	privateAddress := unit.PrivateAddress()
	c.Assert(privateAddress.Value(), gc.Equals, "10.10.10.10")
	c.Assert(privateAddress.Type(), gc.Equals, "private")

	c.Assert(unit.Tools(), gc.NotNil)
	c.Assert(unit.WorkloadStatus(), gc.NotNil)
	c.Assert(unit.AgentStatus(), gc.NotNil)
}

func (s *UnitSerializationSuite) TestMinimalUnitValid(c *gc.C) {
	unit := minimalUnit()
	c.Assert(unit.Validate(), jc.ErrorIsNil)
}

func (s *UnitSerializationSuite) TestMinimalMatches(c *gc.C) {
	bytes, err := yaml.Marshal(minimalUnit())
	c.Assert(err, jc.ErrorIsNil)

	var source map[interface{}]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(source, jc.DeepEquals, minimalUnitMap())
}

func (s *UnitSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := units{
		Version: 1,
		Units_:  []*unit{s.completeUnit()},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	units, err := importUnits(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(units, jc.DeepEquals, initial.Units_)
}
