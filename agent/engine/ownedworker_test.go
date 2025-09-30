// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine_test

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/internal/testhelpers"
)

type OwnedWorkerSuite struct {
	testhelpers.IsolationSuite
}

func TestOwnedWorkerSuite(t *testing.T) {
	tc.Run(t, &OwnedWorkerSuite{})
}

func (s *OwnedWorkerSuite) TestNewOwnedWorker_Success(c *tc.C) {
	w, err := engine.NewOwnedWorker(newErrWorker(nil))
	c.Assert(err, tc.ErrorIsNil)

	err = worker.Stop(w)
	c.Check(err, tc.ErrorIsNil)
}

func (s *OwnedWorkerSuite) TestNewOwnedWorker_NilValue(c *tc.C) {
	w, err := engine.NewOwnedWorker(nil)
	c.Check(err, tc.ErrorMatches, "NewOwnedWorker expects a value")
	c.Check(w, tc.IsNil)
}

func (s *OwnedWorkerSuite) TestValueWorkerOutput_Success(c *tc.C) {
	value := &testType{}
	w, err := engine.NewOwnedWorker(value)
	c.Assert(err, tc.ErrorIsNil)

	var outVal testInterface
	err = engine.ValueWorkerOutput(w, &outVal)
	c.Check(err, tc.ErrorIsNil)
	c.Check(outVal, tc.DeepEquals, value)
}

func (s *OwnedWorkerSuite) TestValueWorkerOutput_BadInput(c *tc.C) {
	var outVal testInterface
	err := engine.ValueWorkerOutput(&testType{}, &outVal)
	c.Check(err, tc.ErrorMatches, "in should be a \\*valueWorker; is .*")
	c.Check(outVal, tc.IsNil)
}

func (s *OwnedWorkerSuite) TestValueWorkerOutput_BadOutputIndirection(c *tc.C) {
	value := &testType{}
	w, err := engine.NewOwnedWorker(value)
	c.Assert(err, tc.ErrorIsNil)

	var outVal string
	err = engine.ValueWorkerOutput(w, outVal)
	c.Check(err, tc.ErrorMatches, "out should be a pointer; is .*")
	c.Check(outVal, tc.Equals, "")
}

func (s *OwnedWorkerSuite) TestValueWorkerOutput_BadOutputType(c *tc.C) {
	value := &testType{}
	w, err := engine.NewOwnedWorker(value)
	c.Assert(err, tc.ErrorIsNil)

	var outVal string
	err = engine.ValueWorkerOutput(w, &outVal)
	c.Check(err, tc.ErrorMatches, "cannot output into \\*string")
	c.Check(outVal, tc.Equals, "")
}

type errWorker struct {
	tomb tomb.Tomb
}

func newErrWorker(err error) worker.Worker {
	w := &errWorker{}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return err
	})
	return w
}

func (e *errWorker) Kill() {
	e.tomb.Kill(nil)
}

func (e *errWorker) Wait() error {
	return e.tomb.Wait()
}

type testOwnedInterface interface {
	errWorker
	Foobar()
}

type testOwnedType struct {
	testInterface
}
