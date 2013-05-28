package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
	coretesting "launchpad.net/juju-core/testing"
	"os"
)

type DeploySuite struct {
	testing.RepoSuite
}

var _ = Suite(&DeploySuite{})

func (s *DeploySuite) runDeploy(c *C, args params.ServiceDeploy) error {
	c.Logf("Running deploy: %+v", args)
	conf, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	curl, err := charm.InferURL(args.CharmUrl, conf.DefaultSeries())
	c.Assert(err, IsNil)
	repo, err := charm.InferRepository(curl, os.Getenv("JUJU_REPOSITORY"))
	c.Assert(err, IsNil)
	return statecmd.ServiceDeploy(s.State, args, s.Conn, curl, repo)
}

func (s *DeploySuite) TestCharmDir(c *C) {
	coretesting.Charms.ClonedDirPath(s.SeriesPath, "dummy")
	args := params.ServiceDeploy{
		CharmUrl: "local:dummy",
		NumUnits: 1,
	}
	err := s.runDeploy(c, args)
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	// Note that this tests the automatic creation of a service name (dummy)
	// from the charm URL.  This functionality will be going away soon as
	// ServiceName becomes a required argument.
	s.AssertService(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestUpgradeCharmDir(c *C) {
	dirPath := coretesting.Charms.ClonedDirPath(s.SeriesPath, "dummy")
	args := params.ServiceDeploy{
		CharmUrl:     "local:dummy",
		BumpRevision: true,
		NumUnits:     1,
	}
	err := s.runDeploy(c, args)
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-2")
	s.AssertService(c, "dummy", curl, 1, 0)
	// Check the charm really was upgraded.
	ch, err := charm.ReadDir(dirPath)
	c.Assert(err, IsNil)
	c.Assert(ch.Revision(), Equals, 2)
}

func (s *DeploySuite) TestCharmBundle(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	args := params.ServiceDeploy{
		CharmUrl:    "local:dummy",
		ServiceName: "some-service-name",
		NumUnits:    1,
	}
	err := s.runDeploy(c, args)
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)
}

func (s *DeploySuite) TestForceMachine(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, IsNil)
	args := params.ServiceDeploy{
		CharmUrl:       "local:dummy",
		ServiceName:    "portlandia",
		ForceMachineId: machine.Id(),
		NumUnits:       1,
	}
	err = s.runDeploy(c, args)
	c.Assert(err, IsNil)
	svc, err := s.State.Service("portlandia")
	c.Assert(err, IsNil)
	units, err := svc.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 1)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, machine.Id())
}

func (s *DeploySuite) TestForceMachineInvalid(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	args := params.ServiceDeploy{
		CharmUrl:       "local:dummy",
		ServiceName:    "portlandia",
		ForceMachineId: "42",
		NumUnits:       1,
	}
	err := s.runDeploy(c, args)
	c.Assert(err, ErrorMatches, `cannot assign unit "portlandia/0" to machine: machine 42 not found`)

	args = params.ServiceDeploy{
		CharmUrl:       "local:dummy",
		ServiceName:    "portlandia",
		ForceMachineId: "abc",
		NumUnits:       1,
	}
	err = s.runDeploy(c, args)
	c.Assert(err, ErrorMatches, `invalid machine id "abc"`)

	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	args = params.ServiceDeploy{
		CharmUrl:       "local:dummy",
		ServiceName:    "portlandia",
		ForceMachineId: machine.Id(),
		NumUnits:       5,
	}
	err = s.runDeploy(c, args)
	c.Assert(err, ErrorMatches, `force-machine cannot be used for multiple units`)

	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	args = params.ServiceDeploy{
		CharmUrl:       "local:logging",
		ForceMachineId: machine.Id(),
	}
	err = s.runDeploy(c, args)
	c.Assert(err, ErrorMatches, `subordinate service cannot specify force-machine`)
}

func (s *DeploySuite) TestCannotUpgradeCharmBundle(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	args := params.ServiceDeploy{
		CharmUrl:     "local:dummy",
		BumpRevision: true,
	}
	err := s.runDeploy(c, args)
	c.Assert(err, ErrorMatches, `cannot increment revision of charm "local:precise/dummy-1": not a directory`)
	// Verify state not touched...
	curl := charm.MustParseURL("local:precise/dummy-1")
	_, err = s.State.Charm(curl)
	c.Assert(err, ErrorMatches, `charm "local:precise/dummy-1" not found`)
	_, err = s.State.Service("dummy")
	c.Assert(err, ErrorMatches, `service "dummy" not found`)
}

func (s *DeploySuite) TestAddsPeerRelations(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "riak")
	args := params.ServiceDeploy{
		CharmUrl: "local:riak",
		NumUnits: 1,
	}
	err := s.runDeploy(c, args)
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/riak-7")
	_, rels := s.AssertService(c, "riak", curl, 1, 1)
	rel := rels[0]
	ep, err := rel.Endpoint("riak")
	c.Assert(err, IsNil)
	c.Assert(ep.Name, Equals, "ring")
	c.Assert(ep.Role, Equals, charm.RolePeer)
	c.Assert(ep.Scope, Equals, charm.ScopeGlobal)
}

func (s *DeploySuite) TestNumUnits(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	args := params.ServiceDeploy{
		CharmUrl: "local:dummy",
		NumUnits: 13,
	}
	err := s.runDeploy(c, args)
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "dummy", curl, 13, 0)
}

func (s *DeploySuite) TestNumUnitsZero(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	args := params.ServiceDeploy{
		CharmUrl: "local:dummy",
		NumUnits: 0,
	}
	err := s.runDeploy(c, args)
	c.Assert(err, ErrorMatches, "must deploy at least one unit")
}

func (s *DeploySuite) TestSubordinateCharm(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	args := params.ServiceDeploy{
		CharmUrl: "local:logging",
	}
	err := s.runDeploy(c, args)
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/logging-1")
	s.AssertService(c, "logging", curl, 0, 0)
}

func (s *DeploySuite) TestConfigMap(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	args := params.ServiceDeploy{
		CharmUrl: "local:dummy",
		Config: map[string]string{
			"skill-level": "1",
		},
		NumUnits: 1,
	}
	err := s.runDeploy(c, args)
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "dummy", curl, 1, 0)
	svc, err := s.State.Service("dummy")
	c.Assert(err, IsNil)
	cfg, err := svc.Config()
	c.Assert(err, IsNil)
	skill, _ := cfg.Get("skill-level")
	c.Assert(skill, Equals, int64(1))
}

func (s *DeploySuite) TestConstraints(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	args := params.ServiceDeploy{
		CharmUrl:    "local:dummy",
		Constraints: constraints.MustParse("mem=2G cpu-cores=2"),
		NumUnits:    1,
	}
	err := s.runDeploy(c, args)
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	service, _ := s.AssertService(c, "dummy", curl, 1, 0)
	cons, err := service.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons, DeepEquals, constraints.MustParse("mem=2G cpu-cores=2"))
}

func (s *DeploySuite) TestSubordinateConstraints(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	args := params.ServiceDeploy{
		CharmUrl:    "local:logging",
		Constraints: constraints.MustParse("mem=1G"),
	}
	err := s.runDeploy(c, args)
	c.Assert(err, Equals, state.ErrSubordinateConstraints)
}
