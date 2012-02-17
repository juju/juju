package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/jujud"
)

type InitzkSuite struct{}

var _ = Suite(&InitzkSuite{})

func parseInitzkCommand(args []string) (*main.InitzkCommand, error) {
	c := &main.InitzkCommand{}
	err := cmd.Parse(c, args)
	return c, err
}

func (s *InitzkSuite) TestParse(c *C) {
	args := []string{}
	_, err := parseInitzkCommand(args)
	c.Assert(err, ErrorMatches, "--instance-id option must be set")

	args = append(args, "--instance-id", "iWhatever")
	_, err = parseInitzkCommand(args)
	c.Assert(err, ErrorMatches, "--env-type option must be set")

	args = append(args, "--env-type", "dummy")
	izk, err := parseInitzkCommand(args)
	c.Assert(err, IsNil)
	c.Assert(izk.ZookeeperAddrs, Equals, []string{"127.0.0.1:2181"})
	c.Assert(izk.InstanceId, Equals, "iWhatever")
	c.Assert(izk.EnvType, Equals, "dummy")

	args = append(args, "--zookeeper-servers", "zk1:2181,zk2:2181")
	izk, err = parseInitzkCommand(args)
	c.Assert(err, IsNil)
	c.Assert(izk.ZookeeperAddrs, Equals, []string{"zk1:2181", "zk2:2181"})

	args = append(args, "haha disregard that")
	_, err = parseInitzkCommand(args)
	c.Assert(err, ErrorMatches, `unrecognised args: \[haha disregard that\]`)
}
