// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type AddressSerializationSuite struct {
	SerializationSuite
}

var _ = gc.Suite(&AddressSerializationSuite{})

func (s *AddressSerializationSuite) SetUpTest(c *gc.C) {
	s.SerializationSuite.SetUpTest(c)
	s.importName = "address"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importAddress(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["value"] = ""
		m["type"] = ""
	}
}

func (s *AddressSerializationSuite) TestMissingValue(c *gc.C) {
	testMap := s.makeMap(1)
	delete(testMap, "value")
	_, err := importAddress(testMap)
	c.Check(err.Error(), gc.Equals, "address v1 schema check failed: value: expected string, got nothing")
}

func (s *AddressSerializationSuite) TestMissingType(c *gc.C) {
	testMap := s.makeMap(1)
	delete(testMap, "type")
	_, err := importAddress(testMap)
	c.Check(err.Error(), gc.Equals, "address v1 schema check failed: type: expected string, got nothing")
}

func (*AddressSerializationSuite) TestParsing(c *gc.C) {
	addr, err := importAddress(map[string]interface{}{
		"version":      1,
		"value":        "no",
		"type":         "content",
		"network-name": "validation",
		"scope":        "done",
		"origin":       "here",
	})
	c.Assert(err, jc.ErrorIsNil)
	expected := &address{
		Version:      1,
		Value_:       "no",
		Type_:        "content",
		NetworkName_: "validation",
		Scope_:       "done",
		Origin_:      "here",
	}
	c.Assert(addr, jc.DeepEquals, expected)
}

func (*AddressSerializationSuite) TestOptionalValues(c *gc.C) {
	addr, err := importAddress(map[string]interface{}{
		"version": 1,
		"value":   "no",
		"type":    "content",
	})
	c.Assert(err, jc.ErrorIsNil)
	expected := &address{
		Version: 1,
		Value_:  "no",
		Type_:   "content",
	}
	c.Assert(addr, jc.DeepEquals, expected)
}

func (*AddressSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := &address{
		Version:      1,
		Value_:       "no",
		Type_:        "content",
		NetworkName_: "validation",
		Scope_:       "done",
		Origin_:      "here",
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	addresss, err := importAddress(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(addresss, jc.DeepEquals, initial)
}
