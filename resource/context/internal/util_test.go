// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/resource/context/internal"
)

var _ = gc.Suite(&UtilSuite{})

type UtilSuite struct {
	testing.IsolationSuite

	stub *internalStub
}

func (s *UtilSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = newInternalStub()
}

func (s *UtilSuite) TestCloseAndLogNoError(c *gc.C) {
	closer := &filetesting.StubCloser{Stub: s.stub.Stub}
	logger := &stubLogger{Stub: s.stub.Stub}

	internal.CloseAndLog(closer, "a thing", logger)

	s.stub.CheckCallNames(c, "Close")
}

func (s *UtilSuite) TestCloseAndLog(c *gc.C) {
	closer := &filetesting.StubCloser{Stub: s.stub.Stub}
	logger := &stubLogger{Stub: s.stub.Stub}
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)

	internal.CloseAndLog(closer, "a thing", logger)

	s.stub.CheckCallNames(c, "Close", "Errorf")
	c.Check(logger.logged, gc.Equals, "while closing a thing: <failure>")
}

func (s *UtilSuite) TestReplaceDirectory(c *gc.C) {
	deps := s.stub

	err := internal.ReplaceDirectory("target_dir", "source_dir", deps)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "RemoveDir", "Move")
}

type stubLogger struct {
	*testing.Stub

	logged string
}

func (s *stubLogger) Errorf(msg string, args ...interface{}) {
	s.AddCall("Errorf", msg, args)
	s.NextErr() // Pop one off.

	s.logged = fmt.Sprintf(msg, args...)
}
