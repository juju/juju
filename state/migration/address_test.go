// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/testing"
)

type AddressSerializationSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&AddressSerializationSuite{})

func (*AddressSerializationSuite) TestNil(c *gc.C) {
	_, err := importAddress(nil)
	c.Check(err, gc.ErrorMatches, "address version schema check failed: .*")
}

func (*AddressSerializationSuite) TestMissingVersion(c *gc.C) {
	_, err := importAddress(map[string]interface{}{
		"value": "",
		"type":  "",
	})
	c.Check(err.Error(), gc.Equals, "address version schema check failed: version: expected int, got nothing")
}

func (*AddressSerializationSuite) TestMissingValue(c *gc.C) {
	_, err := importAddress(map[string]interface{}{
		"version": 1,
		"type":    "",
	})
	c.Check(err.Error(), gc.Equals, "address v1 schema check failed: value: expected string, got nothing")
}

func (*AddressSerializationSuite) TestMissingType(c *gc.C) {
	_, err := importAddress(map[string]interface{}{
		"version": 1,
		"value":   "",
	})
	c.Check(err.Error(), gc.Equals, "address v1 schema check failed: type: expected string, got nothing")
}

func (*AddressSerializationSuite) TestNonIntVersion(c *gc.C) {
	_, err := importAddress(map[string]interface{}{
		"version": "hello",
		"value":   "",
		"type":    "",
	})
	c.Check(err.Error(), gc.Equals, `address version schema check failed: version: expected int, got string("hello")`)
}

func (*AddressSerializationSuite) TestUnknownVersion(c *gc.C) {
	_, err := importAddress(map[string]interface{}{
		"version": 42,
		"value":   "",
		"type":    "",
	})
	c.Check(err.Error(), gc.Equals, `version 42 not valid`)
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
