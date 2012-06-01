package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/testing"
	stdtesting "testing"
)

var zkAddr string

func TestPackage(t *stdtesting.T) {
	srv := testing.StartZkServer()
	defer srv.Destroy()
	var err error
	zkAddr, err = srv.Addr()
	if err != nil {
		t.Fatalf("could not get ZooKeeper server address")
	}
	TestingT(t)
}

type InitzkSuite struct {
	zkConn *zookeeper.Conn
	path   string
}

var _ = Suite(&InitzkSuite{})

func (s *InitzkSuite) SetUpTest(c *C) {
	zk, session, err := zookeeper.Dial(zkAddr, 15e9)
	c.Assert(err, IsNil)
	event := <-session
	c.Assert(event.Ok(), Equals, true)
	c.Assert(event.Type, Equals, zookeeper.EVENT_SESSION)
	c.Assert(event.State, Equals, zookeeper.STATE_CONNECTED)

	s.zkConn = zk
	s.path = "/watcher"

	c.Assert(err, IsNil)
}

func (s *InitzkSuite) TearDownTest(c *C) {
	testing.ZkRemoveTree(s.zkConn, s.path)
	s.zkConn.Close()
}

func initInitzkCommand(args []string) (*InitzkCommand, error) {
	c := &InitzkCommand{}
	return c, initCmd(c, args)
}

func (s *InitzkSuite) TestParse(c *C) {
	args := []string{}
	_, err := initInitzkCommand(args)
	c.Assert(err, ErrorMatches, "--instance-id option must be set")

	args = append(args, "--instance-id", "iWhatever")
	_, err = initInitzkCommand(args)
	c.Assert(err, ErrorMatches, "--env-type option must be set")

	args = append(args, "--env-type", "dummy")
	izk, err := initInitzkCommand(args)
	c.Assert(err, IsNil)
	c.Assert(izk.StateInfo.Addrs, DeepEquals, []string{"127.0.0.1:2181"})
	c.Assert(izk.InstanceId, Equals, "iWhatever")
	c.Assert(izk.EnvType, Equals, "dummy")

	args = append(args, "--zookeeper-servers", "zk1:2181,zk2:2181")
	izk, err = initInitzkCommand(args)
	c.Assert(err, IsNil)
	c.Assert(izk.StateInfo.Addrs, DeepEquals, []string{"zk1:2181", "zk2:2181"})

	args = append(args, "haha disregard that")
	_, err = initInitzkCommand(args)
	c.Assert(err, ErrorMatches, `unrecognized args: \["haha disregard that"\]`)
}
