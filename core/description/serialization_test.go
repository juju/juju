// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type SerializationSuite struct {
	testing.BaseSuite
	importName string
	importFunc func(map[string]interface{}) (interface{}, error)
	testFields func(map[string]interface{})
}

func (s *SerializationSuite) makeMap(version interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	if s.testFields != nil {
		s.testFields(result)
	}
	if version != nil {
		result["version"] = version
	}
	return result
}

func (s *SerializationSuite) TestNil(c *gc.C) {
	_, err := s.importFunc(nil)
	c.Check(err, gc.ErrorMatches, s.importName+" version schema check failed: .*")
}

func (s *SerializationSuite) TestMissingVersion(c *gc.C) {
	_, err := s.importFunc(s.makeMap(nil))
	c.Check(err.Error(), gc.Equals, s.importName+" version schema check failed: version: expected int, got nothing")
}

func (s *SerializationSuite) TestNonIntVersion(c *gc.C) {
	_, err := s.importFunc(s.makeMap("hello"))
	c.Check(err.Error(), gc.Equals, s.importName+` version schema check failed: version: expected int, got string("hello")`)
}

func (s *SerializationSuite) TestUnknownVersion(c *gc.C) {
	_, err := s.importFunc(s.makeMap("42"))
	c.Check(err.Error(), gc.Equals, `version 42 not valid`)
}

type SliceSerializationSuite struct {
	SerializationSuite
	sliceName string
}

func (s *SliceSerializationSuite) TestMissingSlice(c *gc.C) {
	_, err := s.importFunc(map[string]interface{}{
		"version": 1,
	})
	c.Check(err.Error(), gc.Equals, s.importName+" version schema check failed: "+s.sliceName+": expected list, got nothing")
}

func (s *SliceSerializationSuite) TestSliceNameIsMap(c *gc.C) {
	_, err := s.importFunc(map[string]interface{}{
		"version":   1,
		s.sliceName: []interface{}{"hello"},
	})
	c.Check(err.Error(), gc.Equals, s.importName+" version schema check failed: "+s.sliceName+`[0]: expected map, got string("hello")`)
}
