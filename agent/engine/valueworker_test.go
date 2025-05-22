// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine_test

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/internal/testhelpers"
)

type ValueWorkerSuite struct {
	testhelpers.IsolationSuite
}

func TestValueWorkerSuite(t *testing.T) {
	tc.Run(t, &ValueWorkerSuite{})
}

func (s *ValueWorkerSuite) TestNewValueWorker_Success(c *tc.C) {
	w, err := engine.NewValueWorker("cheese")
	c.Assert(err, tc.ErrorIsNil)

	err = worker.Stop(w)
	c.Check(err, tc.ErrorIsNil)
}

func (s *ValueWorkerSuite) TestNewValueWorker_NilValue(c *tc.C) {
	w, err := engine.NewValueWorker(nil)
	c.Check(err, tc.ErrorMatches, "NewValueWorker expects a value")
	c.Check(w, tc.IsNil)
}

func (s *ValueWorkerSuite) TestValueWorkerOutput_Success(c *tc.C) {
	value := &testType{}
	w, err := engine.NewValueWorker(value)
	c.Assert(err, tc.ErrorIsNil)

	var outVal testInterface
	err = engine.ValueWorkerOutput(w, &outVal)
	c.Check(err, tc.ErrorIsNil)
	c.Check(outVal, tc.DeepEquals, value)
}

func (s *ValueWorkerSuite) TestValueWorkerOutput_BadInput(c *tc.C) {
	var outVal testInterface
	err := engine.ValueWorkerOutput(&testType{}, &outVal)
	c.Check(err, tc.ErrorMatches, "in should be a \\*valueWorker; is .*")
	c.Check(outVal, tc.IsNil)
}

func (s *ValueWorkerSuite) TestValueWorkerOutput_BadOutputIndirection(c *tc.C) {
	value := &testType{}
	w, err := engine.NewValueWorker(value)
	c.Assert(err, tc.ErrorIsNil)

	var outVal string
	err = engine.ValueWorkerOutput(w, outVal)
	c.Check(err, tc.ErrorMatches, "out should be a pointer; is .*")
	c.Check(outVal, tc.Equals, "")
}

func (s *ValueWorkerSuite) TestValueWorkerOutput_BadOutputType(c *tc.C) {
	value := &testType{}
	w, err := engine.NewValueWorker(value)
	c.Assert(err, tc.ErrorIsNil)

	var outVal string
	err = engine.ValueWorkerOutput(w, &outVal)
	c.Check(err, tc.ErrorMatches, "cannot output into \\*string")
	c.Check(outVal, tc.Equals, "")
}

type testInterface interface {
	worker.Worker
	Foobar()
}

type testType struct {
	testInterface
}
