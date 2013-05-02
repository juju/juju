package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
)

type DeploySuite struct {
	testing.RepoSuite
}

var _ = Suite(&DeploySuite{})

func runDeploy(c *C, args ...string) error {
	_, err := coretesting.RunCommand(c, &DeployCommand{}, args)
	return err
}

var initErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: nil,
		err:  `no charm specified`,
	}, {
		args: []string{"craz~ness"},
		err:  `invalid charm name "craz~ness"`,
	}, {
		args: []string{"craziness", "burble-1"},
		err:  `invalid service name "burble-1"`,
	}, {
		args: []string{"craziness", "burble1", "-n", "0"},
		err:  `must deploy at least one unit`,
	}, {
		args: []string{"craziness", "burble1", "--constraints", "gibber=plop"},
		err:  `invalid value "gibber=plop" for flag --constraints: unknown constraint "gibber"`,
	},
}

func (s *DeploySuite) TestInitErrors(c *C) {
	for i, t := range initErrorTests {
		c.Logf("test %d", i)
		err := coretesting.InitCommand(&DeployCommand{}, t.args)
		c.Assert(err, ErrorMatches, t.err)
	}
}

func (s *DeploySuite) TestCharmDir(c *C) {
	coretesting.Charms.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "dummy", curl, 1, 0)
}

func (s *DeploySuite) TestUpgradeCharmDir(c *C) {
	dirPath := coretesting.Charms.ClonedDirPath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "-u")
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
	err := runDeploy(c, "local:dummy", "some-service-name")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "some-service-name", curl, 1, 0)
}

func (s *DeploySuite) TestForceMachine(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = runDeploy(c, "--force-machine", machine.Id(), "local:dummy", "portlandia")
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
	err := runDeploy(c, "--force-machine", "42", "local:dummy", "portlandia")
	c.Assert(err, ErrorMatches, `cannot assign unit "portlandia/0" to machine: machine 42 not found`)

	err = runDeploy(c, "--force-machine", "abc", "local:dummy", "portlandia")
	c.Assert(err, ErrorMatches, `invalid machine id "abc"`)

	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	err = runDeploy(c, "--force-machine", machine.Id(), "-n", "5", "local:dummy", "portlandia")
	c.Assert(err, ErrorMatches, `force-machine cannot be used for multiple units`)

	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	err = runDeploy(c, "--force-machine", machine.Id(), "local:logging")
	c.Assert(err, ErrorMatches, `subordinate service cannot specify force-machine`)
}

func (s *DeploySuite) TestCannotUpgradeCharmBundle(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "-u")
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
	err := runDeploy(c, "local:riak")
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
	err := runDeploy(c, "local:dummy", "-n", "13")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	s.AssertService(c, "dummy", curl, 13, 0)
}

func (s *DeploySuite) TestSubordinateCharm(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	err := runDeploy(c, "local:logging")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/logging-1")
	s.AssertService(c, "logging", curl, 0, 0)
}

func (s *DeploySuite) TestConstraints(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "dummy")
	err := runDeploy(c, "local:dummy", "--constraints", "mem=2G cpu-cores=2")
	c.Assert(err, IsNil)
	curl := charm.MustParseURL("local:precise/dummy-1")
	service, _ := s.AssertService(c, "dummy", curl, 1, 0)
	cons, err := service.Constraints()
	c.Assert(err, IsNil)
	c.Assert(cons, DeepEquals, constraints.MustParse("mem=2G cpu-cores=2"))
}

func (s *DeploySuite) TestSubordinateConstraints(c *C) {
	coretesting.Charms.BundlePath(s.SeriesPath, "logging")
	err := runDeploy(c, "local:logging", "--constraints", "mem=1G")
	c.Assert(err, Equals, state.ErrSubordinateConstraints)
}
