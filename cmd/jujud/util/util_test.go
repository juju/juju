// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

import (
	stderrors "errors"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/upgrader"
)

var (
	_ = gc.Suite(&toolSuite{})
)

type toolSuite struct {
	coretesting.BaseSuite
}

func (*toolSuite) TestErrorImportance(c *gc.C) {

	errorImportanceTests := []error{
		nil,
		stderrors.New("foo"),
		&upgrader.UpgradeReadyError{},
		worker.ErrTerminateAgent,
	}

	for i, err0 := range errorImportanceTests {
		for j, err1 := range errorImportanceTests {
			c.Assert(MoreImportant(err0, err1), gc.Equals, i > j)

			// Should also work if errors are wrapped.
			c.Assert(MoreImportant(errors.Trace(err0), errors.Trace(err1)), gc.Equals, i > j)
		}
	}
}

var isFatalTests = []struct {
	err     error
	isFatal bool
}{
	{
		err:     worker.ErrTerminateAgent,
		isFatal: true,
	}, {
		err:     errors.Trace(worker.ErrTerminateAgent),
		isFatal: true,
	}, {
		err:     &upgrader.UpgradeReadyError{},
		isFatal: true,
	}, {
		err: &params.Error{
			Message: "blah",
			Code:    params.CodeNotProvisioned,
		},
		isFatal: false,
	}, {
		err:     &FatalError{"some fatal error"},
		isFatal: true,
	}, {
		err:     stderrors.New("foo"),
		isFatal: false,
	}, {
		err: &params.Error{
			Message: "blah",
			Code:    params.CodeNotFound,
		},
		isFatal: false,
	},
}

func (s *toolSuite) TestConnectionIsFatal(c *gc.C) {
	var (
		errPinger testPinger = func() error {
			return stderrors.New("ping error")
		}
		okPinger testPinger = func() error {
			return nil
		}
	)
	for i, pinger := range []testPinger{errPinger, okPinger} {
		for j, test := range isFatalTests {
			c.Logf("test %d.%d: %s", i, j, test.err)
			fatal := ConnectionIsFatal(logger, pinger)(test.err)
			if test.isFatal {
				c.Check(fatal, jc.IsTrue)
			} else {
				c.Check(fatal, gc.Equals, i == 0)
			}
		}
	}
}

func (s *toolSuite) TestConnectionIsFatalWithMultipleConns(c *gc.C) {
	var (
		errPinger testPinger = func() error {
			return stderrors.New("ping error")
		}
		okPinger testPinger = func() error {
			return nil
		}
	)

	someErr := stderrors.New("foo")

	c.Assert(ConnectionIsFatal(logger, okPinger, okPinger)(someErr),
		jc.IsFalse)
	c.Assert(ConnectionIsFatal(logger, okPinger, okPinger, okPinger)(someErr),
		jc.IsFalse)
	c.Assert(ConnectionIsFatal(logger, okPinger, errPinger)(someErr),
		jc.IsTrue)
	c.Assert(ConnectionIsFatal(logger, okPinger, okPinger, errPinger)(someErr),
		jc.IsTrue)
	c.Assert(ConnectionIsFatal(logger, errPinger, okPinger, okPinger)(someErr),
		jc.IsTrue)
}

func (*toolSuite) TestIsFatal(c *gc.C) {

	for i, test := range isFatalTests {
		c.Logf("test %d: %s", i, test.err)
		c.Assert(IsFatal(test.err), gc.Equals, test.isFatal)
	}
}

type testPinger func() error

func (f testPinger) Ping() error {
	return f()
}
