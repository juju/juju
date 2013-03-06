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
	repoPath      string
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
}

func (s *DeployLocalSuite) TearDownTest(c *C) {
	s.JujuConnSuite.TearDownTest(c)
}

func (s *DeployLocalSuite) TestBadCharmUrl(c *C) {
	charmurl, err := charm.InferURL("local:notarealcharm", s.defaultSeries)
	_, err = statecmd.ServiceDeploy(s.Conn, charmurl, s.repo, false, "", 1, nil, "")
	c.Assert(err, ErrorMatches, "cannot get latest charm revision: no charms found matching \"local:precise/notarealcharm\"")
}

func (s *DeployLocalSuite) TestDeployDefaultName(c *C) {
	coretesting.Charms.BundlePath(s.seriesPath, "mysql")
	charmurl, err := charm.InferURL("local:mysql", s.defaultSeries)
	svc, err := statecmd.ServiceDeploy(s.Conn, charmurl, s.repo, false, "", 1, nil, "")
	c.Assert(err, IsNil)
	c.Assert(svc.Name(), Equals, "mysql")
}
