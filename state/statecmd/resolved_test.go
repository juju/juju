package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/statecmd"
	coretesting "launchpad.net/juju-core/testing"
	stdtesting "testing"
)

type ResolvedSuite struct {
	testing.JujuConnSuite
}

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ = Suite(&ResolvedSuite{})

func (s *ResolvedSuite) TestResolved(c *C) {
	curl := coretesting.Charms.ClonedURL(s.repo.Path, "series", "riak")
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, IsNil)
	svc, err := s.conn.State.AddService("testriak", sch)
	c.Assert(err, IsNil)
	us, err := s.conn.AddUnits(svc, 1)
	c.Assert(err, IsNil)
	u := us[0]

	err = statecmd.Resolved(u, false)
	c.Assert(err, ErrorMatches, `unit "testriak/0" is not in an error state`)
	err = statecmd.Resolved(u, true)
	c.Assert(err, ErrorMatches, `unit "testriak/0" is not in an error state`)

	err = u.SetStatus(state.UnitError, "gaaah")
	c.Assert(err, IsNil)
	err = statecmd.Resolved(u, false)
	c.Assert(err, IsNil)
	err = statecmd.Resolved(u, true)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "testriak/0": already resolved`)
	c.Assert(u.Resolved(), Equals, state.ResolvedNoHooks)

	err = u.ClearResolved()
	c.Assert(err, IsNil)
	err = statecmd.Resolved(u, true)
	c.Assert(err, IsNil)
	err = jujstatecmd.Resolved(u, false)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "testriak/0": already resolved`)
	c.Assert(u.Resolved(), Equals, state.ResolvedRetryHooks)
}
