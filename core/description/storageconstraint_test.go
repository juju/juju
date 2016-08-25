// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type StorageConstraintSerializationSuite struct {
	SerializationSuite
}

var _ = gc.Suite(&StorageConstraintSerializationSuite{})

func (s *StorageConstraintSerializationSuite) SetUpTest(c *gc.C) {
	s.SerializationSuite.SetUpTest(c)
	s.importName = "storageconstraint"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importStorageConstraint(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["pool"] = ""
		m["size"] = 0
		m["count"] = 0
	}
}

func (s *StorageConstraintSerializationSuite) TestMissingValue(c *gc.C) {
	testMap := s.makeMap(1)
	delete(testMap, "pool")
	_, err := importStorageConstraint(testMap)
	c.Check(err.Error(), gc.Equals, "storageconstraint v1 schema check failed: pool: expected string, got nothing")
}

func (*StorageConstraintSerializationSuite) TestParsing(c *gc.C) {
	addr, err := importStorageConstraint(map[string]interface{}{
		"version": 1,
		"pool":    "olympic",
		"size":    50,
		"count":   2,
	})
	c.Assert(err, jc.ErrorIsNil)
	expected := &storageconstraint{
		Version: 1,
		Pool_:   "olympic",
		Size_:   50,
		Count_:  2,
	}
	c.Assert(addr, jc.DeepEquals, expected)
}

func (*StorageConstraintSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := &storageconstraint{
		Version: 1,
		Pool_:   "olympic",
		Size_:   50,
		Count_:  2,
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	storageconstraints, err := importStorageConstraint(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(storageconstraints, jc.DeepEquals, initial)
}
