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

// TODO MAYBE: move this test into the slice serialization base.
func (*UnitSerializationSuite) TestUnitsIsMap(c *gc.C) {
	_, err := importUnits(map[string]interface{}{
		"version": 42,
		"units":   []interface{}{"hello"},
	})
	c.Check(err.Error(), gc.Equals, `units version schema check failed: units[0]: expected map, got string("hello")`)
}

func minimalUnitMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"name":      "ubuntu",
		"series":    "trusty",
		"charm-url": "cs:trusty/ubuntu",
		"status":    minimalStatusMap(),
		"settings": map[interface{}]interface{}{
			"key": "value",
		},
		"settings-refcount": 1,
		"leadership-settings": map[interface{}]interface{}{
			"leader": true,
		},
	}
}

func minimalUnit() *unit {
	s := newUnit(minimalUnitArgs())
	s.SetStatus(minimalStatusArgs())
	return s
}

func minimalUnitArgs() UnitArgs {
	return UnitArgs{
		Tag:      names.NewUnitTag("ubuntu"),
		Series:   "trusty",
		CharmURL: "cs:trusty/ubuntu",
		Settings: map[string]interface{}{
			"key": "value",
		},
		SettingsRefCount: 1,
		LeadershipSettings: map[string]interface{}{
			"leader": true,
		},
	}
}

func (s *UnitSerializationSuite) TestNewUnit(c *gc.C) {
	args := UnitArgs{
		Tag:         names.NewUnitTag("magic"),
		Series:      "zesty",
		Subordinate: true,
		CharmURL:    "cs:zesty/magic",
		ForceCharm:  true,
		Exposed:     true,
		MinUnits:    42, // no judgement is made by the migration code
		Settings: map[string]interface{}{
			"key": "value",
		},
		SettingsRefCount: 1,
		LeadershipSettings: map[string]interface{}{
			"leader": true,
		},
	}
	unit := newUnit(args)

	c.Assert(unit.Name(), gc.Equals, "magic")
	c.Assert(unit.Tag(), gc.Equals, names.NewUnitTag("magic"))
	c.Assert(unit.Series(), gc.Equals, "zesty")
	c.Assert(unit.Subordinate(), jc.IsTrue)
	c.Assert(unit.CharmURL(), gc.Equals, "cs:zesty/magic")
	c.Assert(unit.ForceCharm(), jc.IsTrue)
	c.Assert(unit.Exposed(), jc.IsTrue)
	c.Assert(unit.MinUnits(), gc.Equals, 42)
	c.Assert(unit.Settings(), jc.DeepEquals, args.Settings)
	c.Assert(unit.SettingsRefCount(), gc.Equals, 1)
	c.Assert(unit.LeadershipSettings(), jc.DeepEquals, args.LeadershipSettings)
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
	svc := minimalUnit()
	initial := units{
		Version: 1,
		Units_:  []*unit{svc},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("-- bytes --\n%s\n", bytes)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	c.Logf("-- map --")
	unitMap := source["units"].([]interface{})[0].(map[interface{}]interface{})
	for key, value := range unitMap {
		c.Logf("%s: %v", key, value)
	}

	units, err := importUnits(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(units, jc.DeepEquals, initial.Units_)
}
