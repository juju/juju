// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"
)

var _ = gc.Suite(&sequenceMinSuite{})

type sequenceMinSuite struct {
	testing.IsolationSuite
}

func (s *sequenceMinSuite) TestNew(c *gc.C) {
	updater := &fakeSequenceUpdater{
		readResults:   []interface{}{0},
		createResults: []interface{}{true},
	}

	value, err := updateSeqWithMin(updater, 42)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(value, gc.Equals, 42)
	updater.stub.CheckCalls(c, []testing.StubCall{
		{"read", nil},
		{"create", []interface{}{43}},
	})
}

func (s *sequenceMinSuite) TestReadError(c *gc.C) {
	updater := &fakeSequenceUpdater{
		readResults: []interface{}{errors.New("boom")},
	}
	value, err := updateSeqWithMin(updater, 42)
	c.Check(value, gc.Equals, -1)
	c.Check(err, gc.ErrorMatches, "could not read sequence: boom")
}

func (s *sequenceMinSuite) TestCreateError(c *gc.C) {
	updater := &fakeSequenceUpdater{
		readResults:   []interface{}{0},
		createResults: []interface{}{errors.New("boom")},
	}
	value, err := updateSeqWithMin(updater, 42)
	c.Check(value, gc.Equals, -1)
	c.Check(err, gc.ErrorMatches, "could not create sequence: boom")
}

func (s *sequenceMinSuite) TestIncrement(c *gc.C) {
	updater := &fakeSequenceUpdater{
		readResults: []interface{}{3},
		setResults:  []interface{}{true},
	}

	value, err := updateSeqWithMin(updater, 1)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(value, gc.Equals, 3)
	updater.stub.CheckCalls(c, []testing.StubCall{
		{"read", nil},
		{"set", []interface{}{3, 4}},
	})
}

func (s *sequenceMinSuite) TestSetError(c *gc.C) {
	updater := &fakeSequenceUpdater{
		readResults: []interface{}{3},
		setResults:  []interface{}{errors.New("boom")},
	}
	value, err := updateSeqWithMin(updater, 1)
	c.Check(value, gc.Equals, -1)
	c.Check(err, gc.ErrorMatches, "could not set sequence: boom")
}

func (s *sequenceMinSuite) TestJumpDueToMin(c *gc.C) {
	updater := &fakeSequenceUpdater{
		readResults: []interface{}{3},
		setResults:  []interface{}{true},
	}

	value, err := updateSeqWithMin(updater, 99)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(value, gc.Equals, 99)
	updater.stub.CheckCalls(c, []testing.StubCall{
		{"read", nil},
		{"set", []interface{}{3, 100}},
	})
}

func (s *sequenceMinSuite) TestCreateContention(c *gc.C) {
	updater := &fakeSequenceUpdater{
		readResults:   []interface{}{0, 3},
		createResults: []interface{}{false},
		setResults:    []interface{}{true},
	}

	value, err := updateSeqWithMin(updater, 2)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(value, gc.Equals, 3)
	updater.stub.CheckCalls(c, []testing.StubCall{
		{"read", nil},
		{"create", []interface{}{3}},
		{"read", nil},
		{"set", []interface{}{3, 4}},
	})
}

func (s *sequenceMinSuite) TestSetContention(c *gc.C) {
	updater := &fakeSequenceUpdater{
		readResults: []interface{}{3, 5},
		setResults:  []interface{}{false, true},
	}

	value, err := updateSeqWithMin(updater, 1)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(value, gc.Equals, 5)
	updater.stub.CheckCalls(c, []testing.StubCall{
		{"read", nil},
		{"set", []interface{}{3, 4}},
		{"read", nil},
		{"set", []interface{}{5, 6}},
	})
}

func (s *sequenceMinSuite) TestTooMuchContention(c *gc.C) {
	updater := new(fakeSequenceUpdater)
	for i := 0; i < maxSeqRetries; i++ {
		updater.readResults = append(updater.readResults, i+3)
		updater.setResults = append(updater.setResults, false)
	}

	value, err := updateSeqWithMin(updater, 1)
	c.Check(value, gc.Equals, -1)
	c.Check(err, gc.ErrorMatches, "too much contention while updating sequence")
}

type fakeSequenceUpdater struct {
	stub          testing.Stub
	readResults   []interface{}
	createResults []interface{}
	setResults    []interface{}
}

func (su *fakeSequenceUpdater) read() (int, error) {
	su.stub.AddCall("read")
	out, err := su.pop(&su.readResults)
	if err != nil {
		return -1, err
	}
	return out.(int), nil
}

func (su *fakeSequenceUpdater) create(value int) (bool, error) {
	su.stub.AddCall("create", value)
	out, err := su.pop(&su.createResults)
	if err != nil {
		return false, err
	}
	return out.(bool), nil
}

func (su *fakeSequenceUpdater) set(expected, next int) (bool, error) {
	su.stub.AddCall("set", expected, next)
	out, err := su.pop(&su.setResults)
	if err != nil {
		return false, err
	}
	return out.(bool), nil
}

func (su *fakeSequenceUpdater) pop(resultsp *[]interface{}) (interface{}, error) {
	results := *resultsp
	if len(results) == 0 {
		panic("no more results left")
	}
	out := results[0]
	*resultsp = results[1:]
	if err, ok := out.(error); ok {
		return nil, err
	}
	return out, nil
}
