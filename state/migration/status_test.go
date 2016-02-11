// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type StatusSerializationSuite struct {
	SerializationSuite
}

var _ = gc.Suite(&StatusSerializationSuite{})

func minimalStatus() *status {
	return newStatus(minimalStatusArgs())
}

func minimalStatusMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"version": 1,
		"value":   "running",
		"updated": "2016-01-28T11:50:00Z",
	}
}

func minimalStatusArgs() StatusArgs {
	return StatusArgs{
		Value:   "running",
		Updated: time.Date(2016, 1, 28, 11, 50, 0, 0, time.UTC),
	}
}

func (s *StatusSerializationSuite) SetUpTest(c *gc.C) {
	s.SerializationSuite.SetUpTest(c)
	s.importName = "status"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importStatus(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["value"] = "value"
		m["updated"] = time.Now()
	}
}

func (s *StatusSerializationSuite) TestMinimalMatches(c *gc.C) {
	bytes, err := yaml.Marshal(minimalStatus())
	c.Assert(err, jc.ErrorIsNil)

	var source map[interface{}]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(source, jc.DeepEquals, minimalStatusMap())
}

func (s *StatusSerializationSuite) TestMissingValue(c *gc.C) {
	testMap := s.makeMap(1)
	delete(testMap, "value")
	_, err := importStatus(testMap)
	c.Check(err.Error(), gc.Equals, "status v1 schema check failed: value: expected string, got nothing")
}

func (s *StatusSerializationSuite) TestMissingUpdated(c *gc.C) {
	testMap := s.makeMap(1)
	delete(testMap, "updated")
	_, err := importStatus(testMap)
	c.Check(err.Error(), gc.Equals, "status v1 schema check failed: updated: expected string or time.Time, got nothing")
}

func (s *StatusSerializationSuite) TestNewStatus(c *gc.C) {
	args := StatusArgs{
		Value:   "value",
		Message: "message",
		Data: map[string]interface{}{
			"extra": "anther",
		},
		Updated: time.Date(2016, 1, 28, 11, 50, 0, 0, time.UTC),
	}
	status := newStatus(args)
	c.Assert(status.Value(), gc.Equals, args.Value)
	c.Assert(status.Message(), gc.Equals, args.Message)
	c.Assert(status.Data(), jc.DeepEquals, args.Data)
	c.Assert(status.Updated(), gc.Equals, args.Updated)
}

func (*StatusSerializationSuite) TestParsing(c *gc.C) {
	updated := time.Date(2016, 1, 28, 11, 50, 0, 0, time.UTC)
	addr, err := importStatus(map[string]interface{}{
		"version": 1,
		"value":   "started",
		"message": "a message",
		"data": map[string]interface{}{
			"extra": "anther",
		},
		"updated": updated.Format(time.RFC3339Nano),
	})
	c.Assert(err, jc.ErrorIsNil)
	expected := &status{
		Version:  1,
		Value_:   "started",
		Message_: "a message",
		Data_: map[string]interface{}{
			"extra": "anther",
		},
		Updated_: updated,
	}
	c.Assert(addr, jc.DeepEquals, expected)
}

func (*StatusSerializationSuite) TestOptionalValues(c *gc.C) {
	updated := time.Date(2016, 1, 28, 11, 50, 0, 0, time.UTC)
	addr, err := importStatus(map[string]interface{}{
		"version": 1,
		"value":   "started",
		"updated": updated.Format(time.RFC3339Nano),
	})
	c.Assert(err, jc.ErrorIsNil)
	expected := &status{
		Version:  1,
		Value_:   "started",
		Updated_: updated,
	}
	c.Assert(addr, jc.DeepEquals, expected)
}

func (*StatusSerializationSuite) TestParsingSerializedData(c *gc.C) {

	args := StatusArgs{
		Value:   "value",
		Message: "message",
		Data: map[string]interface{}{
			"extra": "anther",
		},
		Updated: time.Date(2016, 1, 28, 11, 50, 0, 0, time.UTC),
	}
	initial := newStatus(args)

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	statuss, err := importStatus(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(statuss, jc.DeepEquals, initial)
}
