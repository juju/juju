package util

import (
	gc "gopkg.in/check.v1"
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
		}
	}
}

func (*toolSuite) TestIsFatal(c *gc.C) {

	isFatalTests := []struct {
		err     error
		isFatal bool
	}{{
		err:     worker.ErrTerminateAgent,
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
		err:     &cmdutil.FatalError{"some fatal error"},
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
	}}

	for i, test := range isFatalTests {
		c.Logf("test %d: %s", i, test.err)
		c.Assert(cmdutil.IsFatal(test.err), gc.Equals, test.isFatal)
	}
}

type testPinger func() error

func (f testPinger) Ping() error {
	return f()
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
			fatal := cmdutil.ConnectionIsFatal(logger, pinger)(test.err)
			if test.isFatal {
				c.Check(fatal, jc.IsTrue)
			} else {
				c.Check(fatal, gc.Equals, i == 0)
			}
		}
	}
}
