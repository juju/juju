package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
	coretesting "launchpad.net/juju-core/testing"
	"os"
)

type UpgradeCharmSuite struct {
	testing.RepoSuite
	riak *state.Service
	path string
}

var _ = Suite(&UpgradeCharmSuite{})

func (s *UpgradeCharmSuite) runDeploy(c *C, args params.ServiceDeploy) error {
	c.Logf("Running deploy: %+v", args)
	conf, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	curl, err := charm.InferURL(args.CharmUrl, conf.DefaultSeries())
	c.Assert(err, IsNil)
	repo, err := charm.InferRepository(curl, os.Getenv("JUJU_REPOSITORY"))
	c.Assert(err, IsNil)
	return statecmd.ServiceDeploy(s.State, args, s.Conn, curl, repo)
}

func (s *UpgradeCharmSuite) runUpgradeCharm(c *C, args params.ServiceUpgradeCharm) error {
	service, err := s.State.Service(args.ServiceName)
	if err != nil {
		return err
	}
	conf, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	curl, err := charm.InferURL(args.CharmUrl, conf.DefaultSeries())
	if err != nil {
		return err
	}
	repo, err := charm.InferRepository(curl, os.Getenv("JUJU_REPOSITORY"))
	c.Assert(err, IsNil)
	return statecmd.ServiceUpgradeCharm(s.State, service, s.Conn, curl, repo, args.Force, true)
}

func (s *UpgradeCharmSuite) SetUpTest(c *C) {
	s.RepoSuite.SetUpTest(c)
	s.path = coretesting.Charms.ClonedDirPath(s.SeriesPath, "riak")
	deployArgs := params.ServiceDeploy{
		CharmUrl: "local:riak",
		NumUnits: 1,
	}
	err := s.runDeploy(c, deployArgs)
	c.Assert(err, IsNil)
	s.riak, err = s.State.Service("riak")
	c.Assert(err, IsNil)
	ch, forced, err := s.riak.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.Revision(), Equals, 7)
	c.Assert(forced, Equals, false)
}

func (s *UpgradeCharmSuite) TestUpgradeCharm(c *C) {
	args := params.ServiceUpgradeCharm{
		ServiceName: "riak",
		CharmUrl:    "local:riak",
		Force:       false,
	}
	err := s.runUpgradeCharm(c, args)
	c.Assert(err, IsNil)
	err = s.riak.Refresh()
	c.Assert(err, IsNil)
	ch, force, err := s.riak.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.Revision(), Equals, 8)
	c.Assert(force, Equals, false)
	s.AssertCharmUploaded(c, ch.URL())
}

func (s *UpgradeCharmSuite) TestServiceDoesNotExist(c *C) {
	args := params.ServiceUpgradeCharm{
		ServiceName: "not-real",
		CharmUrl:    "local:riak-8",
		Force:       false,
	}
	err := s.runUpgradeCharm(c, args)
	c.Assert(err, ErrorMatches, `service \"not-real\" not found`)
}

func (s *UpgradeCharmSuite) TestCharmAlreadyMostRecent(c *C) {
	args := params.ServiceUpgradeCharm{
		ServiceName: "riak",
		CharmUrl:    "local:riak-7",
		Force:       false,
	}
	err := s.runUpgradeCharm(c, args)
	c.Assert(err, ErrorMatches, `already running specified charm "local:precise/riak-7"`)
}
