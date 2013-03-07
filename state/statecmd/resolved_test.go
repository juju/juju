package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/statecmd"
	coretesting "launchpad.net/juju-core/testing"
)

type ResolvedSuite struct {
	testing.JujuConnSuite
	conn *juju.Conn
	repo *charm.LocalRepository
}

// Ensure our test suite satisfies Suite
var _ = Suite(&ResolvedSuite{})

func panicWrite(name string, cert, key []byte) error {
	panic("writeCertAndKey called unexpectedly")
}

func (s *ResolvedSuite) SetUpTest(c *C) {
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"admin-secret":    "deploy-test-secret",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	}
	environ, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(environ, false, panicWrite)
	c.Assert(err, IsNil)
	s.conn, err = juju.NewConn(environ)
	c.Assert(err, IsNil)
	s.repo = &charm.LocalRepository{Path: c.MkDir()}
}

func (s *ResolvedSuite) TearDownTest(c *C) {
	c.Assert(s.conn.Close(), IsNil)
	s.Conn = nil
}

func (s *ResolvedSuite) TestMarkResolved(c *C) {
	curl := coretesting.Charms.ClonedURL(s.repo.Path, "series", "riak")
	sch, err := s.conn.PutCharm(curl, s.repo, false)
	c.Assert(err, IsNil)
	svc, err := s.conn.State.AddService("testriak", sch)
	c.Assert(err, IsNil)
	us, err := s.conn.AddUnits(svc, 1)
	c.Assert(err, IsNil)
	u := us[0]

	err = statecmd.MarkResolved(u, false)
	c.Assert(err, ErrorMatches, `unit "testriak/0" is not in an error state`)
	err = statecmd.MarkResolved(u, true)
	c.Assert(err, ErrorMatches, `unit "testriak/0" is not in an error state`)

	err = u.SetStatus(state.UnitError, "gaaah")
	c.Assert(err, IsNil)
	err = statecmd.MarkResolved(u, false)
	c.Assert(err, IsNil)
	err = statecmd.MarkResolved(u, true)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "testriak/0": already resolved`)
	c.Assert(u.Resolved(), Equals, state.ResolvedNoHooks)

	err = u.ClearResolved()
	c.Assert(err, IsNil)
	err = statecmd.MarkResolved(u, true)
	c.Assert(err, IsNil)
	err = statecmd.MarkResolved(u, false)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "testriak/0": already resolved`)
	c.Assert(u.Resolved(), Equals, state.ResolvedRetryHooks)
}
