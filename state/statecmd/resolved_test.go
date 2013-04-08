package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

type ResolvedSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&ResolvedSuite{})

func (s *ResolvedSuite) TestMarkResolved(c *C) {
	sch := s.AddTestingCharm(c, "riak")
	svc, err := s.Conn.State.AddService("testriak", sch)
	c.Assert(err, IsNil)
	us, err := s.Conn.AddUnits(svc, 1, "")
	c.Assert(err, IsNil)
	u := us[0]

	err = statecmd.MarkResolved(u, false)
	c.Assert(err, ErrorMatches, `unit "testriak/0" is not in an error state`)
	err = statecmd.MarkResolved(u, true)
	c.Assert(err, ErrorMatches, `unit "testriak/0" is not in an error state`)

	err = u.SetStatus(params.UnitError, "gaaah")
	c.Assert(err, IsNil)
	err = statecmd.MarkResolved(u, false)
	c.Assert(err, IsNil)
	err = statecmd.MarkResolved(u, true)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "testriak/0": already resolved`)
	c.Assert(u.Resolved(), Equals, params.ResolvedNoHooks)

	err = u.ClearResolved()
	c.Assert(err, IsNil)
	err = statecmd.MarkResolved(u, true)
	c.Assert(err, IsNil)
	err = statecmd.MarkResolved(u, false)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "testriak/0": already resolved`)
	c.Assert(u.Resolved(), Equals, params.ResolvedRetryHooks)
}
