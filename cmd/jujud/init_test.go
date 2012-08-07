package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
)

type InitSuite struct {
	coretesting.LoggingSuite
	testing.StateSuite
	path string
}

var _ = Suite(&InitSuite{})

func (s *InitSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.path = "/watcher"
	s.StateSuite.SetUpTest(c)
}

func (s *InitSuite) TearDownTest(c *C) {
	s.StateSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func initInitCommand(args []string) (*InitCommand, error) {
	c := &InitCommand{}
	return c, initCmd(c, args)
}

func (s *InitSuite) TestParse(c *C) {
	args := []string{}
	_, err := initInitCommand(args)
	c.Assert(err, ErrorMatches, "--instance-id option must be set")

	args = append(args, "--instance-id", "iWhatever")
	_, err = initInitCommand(args)
	c.Assert(err, ErrorMatches, "--env-type option must be set")

	args = append(args, "--env-type", "dummy")
	cmd, err := initInitCommand(args)
	c.Assert(err, IsNil)
	c.Assert(cmd.StateInfo.Addrs, DeepEquals, []string{"127.0.0.1:2181"})
	c.Assert(cmd.InstanceId, Equals, "iWhatever")
	c.Assert(cmd.EnvType, Equals, "dummy")

	args = append(args, "--zookeeper-servers", "zk1:2181,zk2:2181")
	cmd, err = initInitCommand(args)
	c.Assert(err, IsNil)
	c.Assert(cmd.StateInfo.Addrs, DeepEquals, []string{"zk1:2181", "zk2:2181"})

	args = append(args, "haha disregard that")
	_, err = initInitCommand(args)
	c.Assert(err, ErrorMatches, `unrecognized args: \["haha disregard that"\]`)
}

func (s *InitSuite) TestSetMachineId(c *C) {
	args := []string{"--zookeeper-servers"}
	args = append(args, s.StateInfo(c).Addrs...)
	args = append(args, "--instance-id", "over9000", "--env-type", "dummy")
	cmd, err := initInitCommand(args)
	c.Assert(err, IsNil)
	err = cmd.Run(nil)
	c.Assert(err, IsNil)

	machines, err := s.State.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(len(machines), Equals, 1)

	instid, err := machines[0].InstanceId()
	c.Assert(err, IsNil)
	c.Assert(instid, Equals, "over9000")
}
