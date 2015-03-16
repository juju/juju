// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/utils/syslog"
)

type serviceSuite struct {
	syslog.BaseSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestRestartRoot(c *gc.C) {
	s.Stub.Euid = 0

	err := syslog.Restart()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "Geteuid", "Restart")
}

func (s *serviceSuite) TestRestartNotRoot(c *gc.C) {
	s.Stub.Euid = 1000

	err := syslog.Restart()

	c.Check(err, gc.ErrorMatches, `.*must be root.*`)
	s.Stub.CheckCallNames(c, "Geteuid")
}

func (s *serviceSuite) TestRestartError(c *gc.C) {
	s.Stub.Euid = 0
	failure := errors.New("<failed>")
	s.Stub.SetErrors(nil, failure) // Geteuid, Restart

	err := syslog.Restart()

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "Geteuid", "Restart")
}
