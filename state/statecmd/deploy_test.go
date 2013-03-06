package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/statecmd"
)

type DeployLocalSuite struct {
	testing.JujuConnSuite
	conn *juju.Conn
	repo *charm.LocalRepository
}

// Run-time check to ensure DeployLocalSuite implements the Suite
// interface.
var _ = Suite(&DeployLocalSuite{})

func (s *DeployLocalSuite) SetUpTest(c *C) {
	environ, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(environ, false, panicWrite)
	c.Assert(err, IsNil)
	s.conn, err = juju.NewConn(environ)
	c.Assert(err, IsNil)
	s.repo = &charm.LocalRepository{Path: c.MkDir()}
}

func (s *DeployLocalSuite) TearDownTest(c *C) {
	if s.conn == nil {
		return
	}
	err = s.conn.Environ.Destroy(nil)
	c.Check(err, IsNil)
	s.conn.Close()
	s.conn = nil
}

func (s *DeployLocalSuite) TestBadCharmUrl(c *C) {
	charmurl := "notarealcharm"
	err := statecmd.ServiceDeploy(s.conn, charmurl, s.repo, false, "", 1)
	c.Assert(err, ErrorMatches, "Bad charm url")
}

func (s *DeployLocalSuite) TestBadRepo(c *C) {
	charmurl = "mysql"
	repo = nil
	err := statecmd.ServiceDeploy(s.conn, charmurl, repo, false, "", 1)
	c.Assert(err, ErrorMatches, "Bad repo")
}

func (s *DeployLocalSuite) TestDeployDefaultName(c *C) {
	charmurl = "mysql"
	err := statecmd.ServiceDeploy(s.conn, charmurl, s.repo, false, "", 1)
	c.Assert(err, IsNil)
	c.Assert.
}
