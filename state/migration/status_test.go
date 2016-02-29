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
	statusFields map[string]interface{}
}

var _ = gc.Suite(&StatusSerializationSuite{})

func minimalStatus() *status {
	return newStatus(minimalStatusArgs())
}

func minimalStatusMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"version": 1,
		"status": map[interface{}]interface{}{
			"value":   "running",
			"updated": "2016-01-28T11:50:00Z",
		},
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
	s.statusFields = map[string]interface{}{}
	s.testFields = func(m map[string]interface{}) {
		m["status"] = s.statusFields
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
	s.statusFields["updated"] = "2016-01-28T11:50:00Z"
	_, err := importStatus(testMap)
	c.Check(err.Error(), gc.Equals, "status v1 schema check failed: value: expected string, got nothing")
}

func (s *StatusSerializationSuite) TestMissingUpdated(c *gc.C) {
	testMap := s.makeMap(1)
	s.statusFields["value"] = "running"
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

func (s *StatusSerializationSuite) exportImport(c *gc.C, status_ *status) *status {
	bytes, err := yaml.Marshal(status_)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	status, err := importStatus(source)
	c.Assert(err, jc.ErrorIsNil)
	return status
}

func (s *StatusSerializationSuite) TestParsing(c *gc.C) {
	initial := newStatus(StatusArgs{
		Value:   "started",
		Message: "a message",
		Data: map[string]interface{}{
			"extra": "anther",
		},
		Updated: time.Date(2016, 1, 28, 11, 50, 0, 0, time.UTC),
	})
	status := s.exportImport(c, initial)
	c.Assert(status, jc.DeepEquals, initial)
}

func (s *StatusSerializationSuite) TestOptionalValues(c *gc.C) {
	initial := newStatus(StatusArgs{
		Value:   "started",
		Updated: time.Date(2016, 1, 28, 11, 50, 0, 0, time.UTC),
	})
	status := s.exportImport(c, initial)
	c.Assert(status, jc.DeepEquals, initial)
}

type StatusHistorySerializationSuite struct {
	SerializationSuite
}

var _ = gc.Suite(&StatusHistorySerializationSuite{})

func emptyStatusHistoryMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"version": 1,
		"history": []interface{}{},
	}
}

func testStatusHistoryArgs() []StatusArgs {
	return []StatusArgs{{
		Value:   "running",
		Updated: time.Date(2016, 1, 28, 11, 50, 0, 0, time.UTC),
	}, {
		Value:   "stopped",
		Updated: time.Date(2016, 1, 28, 12, 50, 0, 0, time.UTC),
	}, {
		Value:   "running",
		Updated: time.Date(2016, 1, 28, 13, 50, 0, 0, time.UTC),
	}}
}

func (s *StatusHistorySerializationSuite) SetUpTest(c *gc.C) {
	s.SerializationSuite.SetUpTest(c)
	s.importName = "status"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		history := newStatusHistory()
		if err := importStatusHistory(&history, m); err != nil {
			return nil, err
		}
		return &history, nil
	}
	s.testFields = func(m map[string]interface{}) {
		m["history"] = []interface{}{}
	}
}

func (s *StatusHistorySerializationSuite) TestSetStatusHistory(c *gc.C) {
	// Make sure all the arg values are set.
	args := []StatusArgs{{
		Value:   "running",
		Message: "all good",
		Data: map[string]interface{}{
			"key": "value",
		},
		Updated: time.Date(2016, 1, 28, 11, 50, 0, 0, time.UTC),
	}, {
		Value:   "stopped",
		Updated: time.Date(2016, 1, 28, 12, 50, 0, 0, time.UTC),
	}}
	history := newStatusHistory()
	history.SetStatusHistory(args)

	for i, point := range history.StatusHistory() {
		c.Check(point.Value(), gc.Equals, args[i].Value)
		c.Check(point.Message(), gc.Equals, args[i].Message)
		c.Check(point.Data(), jc.DeepEquals, args[i].Data)
		c.Check(point.Updated(), gc.Equals, args[i].Updated)
	}
}

func (s *StatusHistorySerializationSuite) exportImport(c *gc.C, status_ statusHistory) statusHistory {
	bytes, err := yaml.Marshal(status_)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	history := newStatusHistory()
	err = importStatusHistory(&history, source)
	c.Assert(err, jc.ErrorIsNil)
	return history
}

func (s *StatusHistorySerializationSuite) TestParsing(c *gc.C) {
	initial := newStatusHistory()
	initial.SetStatusHistory(testStatusHistoryArgs())
	history := s.exportImport(c, initial)
	c.Assert(history, jc.DeepEquals, initial)
}

type StatusHistoryMixinSuite struct {
	creator    func() HasStatusHistory
	serializer func(*gc.C, interface{}) HasStatusHistory
}

func (s *StatusHistoryMixinSuite) TestStatusHistory(c *gc.C) {
	initial := s.creator()
	args := testStatusHistoryArgs()
	initial.SetStatusHistory(args)

	entity := s.serializer(c, initial)
	for i, point := range entity.StatusHistory() {
		c.Check(point.Value(), gc.Equals, args[i].Value)
		c.Check(point.Message(), gc.Equals, args[i].Message)
		c.Check(point.Data(), jc.DeepEquals, args[i].Data)
		c.Check(point.Updated(), gc.Equals, args[i].Updated)
	}
}
