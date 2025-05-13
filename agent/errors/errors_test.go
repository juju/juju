// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors_test

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/tc"

	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker"
	"github.com/juju/juju/rpc/params"
)

var (
	_ = tc.Suite(&toolSuite{})
)

type toolSuite struct {
	testing.BaseSuite
	logger logger.Logger
}

func (s *toolSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.logger = loggertesting.WrapCheckLog(c)
}

func (*toolSuite) TestErrorImportance(c *tc.C) {

	errorImportanceTests := []error{
		nil,
		stderrors.New("foo"),
		&agenterrors.UpgradeReadyError{},
		worker.ErrTerminateAgent,
	}

	for i, err0 := range errorImportanceTests {
		for j, err1 := range errorImportanceTests {
			c.Assert(agenterrors.MoreImportant(err0, err1), tc.Equals, i > j)

			// Should also work if errors are wrapped.
			c.Assert(agenterrors.MoreImportant(errors.Trace(err0), errors.Trace(err1)), tc.Equals, i > j)
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
		err:     worker.ErrRestartAgent,
		isFatal: true,
	}, {
		err:     errors.Trace(worker.ErrRestartAgent),
		isFatal: true,
	}, {
		err:     &agenterrors.UpgradeReadyError{},
		isFatal: true,
	}, {
		err: &params.Error{
			Message: "blah",
			Code:    params.CodeNotProvisioned,
		},
		isFatal: false,
	}, {
		err:     fmt.Errorf("some %w error", agenterrors.FatalError),
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

func (s *toolSuite) TestConnectionIsFatal(c *tc.C) {
	okConn := &testConn{broken: false}
	errConn := &testConn{broken: true}

	for i, conn := range []*testConn{errConn, okConn} {
		for j, test := range isFatalTests {
			c.Logf("test %d.%d: %s", i, j, test.err)
			fatal := agenterrors.ConnectionIsFatal(context.Background(), s.logger, conn)(test.err)
			if test.isFatal {
				c.Check(fatal, tc.IsTrue)
			} else {
				c.Check(fatal, tc.Equals, i == 0)
			}
		}
	}
}

func (s *toolSuite) TestConnectionIsFatalWithMultipleConns(c *tc.C) {
	okConn := &testConn{broken: false}
	errConn := &testConn{broken: true}

	someErr := stderrors.New("foo")

	ctx := context.Background()
	c.Assert(agenterrors.ConnectionIsFatal(ctx, s.logger, okConn, okConn)(someErr),
		tc.IsFalse)
	c.Assert(agenterrors.ConnectionIsFatal(ctx, s.logger, okConn, okConn, okConn)(someErr),
		tc.IsFalse)
	c.Assert(agenterrors.ConnectionIsFatal(ctx, s.logger, okConn, errConn)(someErr),
		tc.IsTrue)
	c.Assert(agenterrors.ConnectionIsFatal(ctx, s.logger, okConn, okConn, errConn)(someErr),
		tc.IsTrue)
	c.Assert(agenterrors.ConnectionIsFatal(ctx, s.logger, errConn, okConn, okConn)(someErr),
		tc.IsTrue)
}

func (s *toolSuite) TestPingerIsFatal(c *tc.C) {
	var errPinger testPinger = func() error {
		return stderrors.New("ping error")
	}
	var okPinger testPinger = func() error {
		return nil
	}
	for i, pinger := range []testPinger{errPinger, okPinger} {
		for j, test := range isFatalTests {
			c.Logf("test %d.%d: %s", i, j, test.err)
			fatal := agenterrors.PingerIsFatal(s.logger, pinger)(test.err)
			if test.isFatal {
				c.Check(fatal, tc.IsTrue)
			} else {
				c.Check(fatal, tc.Equals, i == 0)
			}
		}
	}
}

func (s *toolSuite) TestPingerIsFatalWithMultipleConns(c *tc.C) {
	var errPinger testPinger = func() error {
		return stderrors.New("ping error")
	}
	var okPinger testPinger = func() error {
		return nil
	}
	someErr := stderrors.New("foo")
	c.Assert(agenterrors.PingerIsFatal(s.logger, okPinger, okPinger)(someErr),
		tc.IsFalse)
	c.Assert(agenterrors.PingerIsFatal(s.logger, okPinger, okPinger, okPinger)(someErr),
		tc.IsFalse)
	c.Assert(agenterrors.PingerIsFatal(s.logger, okPinger, errPinger)(someErr),
		tc.IsTrue)
	c.Assert(agenterrors.PingerIsFatal(s.logger, okPinger, okPinger, errPinger)(someErr),
		tc.IsTrue)
	c.Assert(agenterrors.PingerIsFatal(s.logger, errPinger, okPinger, okPinger)(someErr),
		tc.IsTrue)
}

func (*toolSuite) TestIsFatal(c *tc.C) {

	for i, test := range isFatalTests {
		c.Logf("test %d: %s", i, test.err)
		c.Assert(agenterrors.IsFatal(test.err), tc.Equals, test.isFatal)
	}
}

type testConn struct {
	broken bool
}

func (c *testConn) IsBroken(_ context.Context) bool {
	return c.broken
}

type testPinger func() error

func (f testPinger) Ping() error {
	return f()
}
