package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/statecmd"
	coretesting "launchpad.net/juju-core/testing"
	"os"
	"path/filepath"
)

type DeployLocalSuite struct {
	testing.JujuConnSuite
	repo          *charm.LocalRepository
	defaultSeries string
	seriesPath    string
	charmUrl      *charm.URL
}

// Run-time check to ensure DeployLocalSuite implements the Suite
// interface.
var _ = Suite(&DeployLocalSuite{})

func (s *DeployLocalSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	repoPath := c.MkDir()
	s.defaultSeries = "precise"
	s.repo = &charm.LocalRepository{Path: repoPath}
	s.seriesPath = filepath.Join(repoPath, s.defaultSeries)
	err := os.Mkdir(s.seriesPath, 0777)
	c.Assert(err, IsNil)
	coretesting.Charms.BundlePath(s.seriesPath, "mysql")
	s.charmUrl, err = charm.InferURL("local:mysql", s.defaultSeries)
	c.Assert(err, IsNil)
}

func (s *DeployLocalSuite) TearDownTest(c *C) {
	s.JujuConnSuite.TearDownTest(c)
}

func (s *DeployLocalSuite) TestBadCharmUrl(c *C) {
	charmurl, err := charm.InferURL("local:notarealcharm", s.defaultSeries)
	_, err = statecmd.ServiceDeploy(s.Conn, charmurl, s.repo, false, "", 1, nil, "")
	c.Assert(err, ErrorMatches, "cannot get latest charm revision: no charms found matching \"local:precise/notarealcharm\"")
}

func (s *DeployLocalSuite) TestDefaultName(c *C) {
	svc, err := statecmd.ServiceDeploy(s.Conn, s.charmUrl, s.repo, false, "", 1, nil, "")
	c.Assert(err, IsNil)
	c.Assert(svc.Name(), Equals, "mysql")
}

func (s *DeployLocalSuite) TestCustomName(c *C) {
	svc, err := statecmd.ServiceDeploy(s.Conn, s.charmUrl, s.repo, false, "yoursql", 1, nil, "")
	c.Assert(err, IsNil)
	c.Assert(svc.Name(), Equals, "yoursql")
	units, err := svc.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(len(units), Equals, 1)
}

func (s *DeployLocalSuite) TestSetNumUnits(c *C) {
	svc, err := statecmd.ServiceDeploy(s.Conn, s.charmUrl, s.repo, false, "", 3, nil, "")
	c.Assert(err, IsNil)
	units, err := svc.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(len(units), Equals, 3)
}

func (s *DeployLocalSuite) TestBumpRevision(c *C) {
	curl := coretesting.Charms.ClonedURL(s.repo.Path, s.defaultSeries, "mysql")
	svc, err := statecmd.ServiceDeploy(s.Conn, curl, s.repo, true, "", 1, nil, "")
	c.Assert(err, IsNil)
	c.Assert(svc.Name(), Equals, "mysql")
}
