package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/juju/state"
	"launchpad.net/juju-core/juju/testing"
	stdtesting "testing"
)

var zkAddr string

func TestPackage(t *stdtesting.T) {
	srv := testing.StartZkServer()
	defer srv.Destroy()
	var err error
	zkAddr, err = srv.Addr()
	if err != nil {
		t.Fatalf("could not get ZooKeeper server address: %v", err)
	}
	TestingT(t)
}

type zkSuite struct {
	zkConn *zookeeper.Conn
	zkInfo *state.Info
}

func (f *zkSuite) SetUpTest(c *C) {
	zk, session, err := zookeeper.Dial(zkAddr, 15e9)
	c.Assert(err, IsNil)
	event := <-session
	c.Assert(event.Ok(), Equals, true)
	c.Assert(event.Type, Equals, zookeeper.EVENT_SESSION)
	c.Assert(event.State, Equals, zookeeper.STATE_CONNECTED)

	f.zkConn = zk
	f.zkInfo = &state.Info{
		Addrs: []string{zkAddr},
	}
}

func (f *zkSuite) TearDownTest() {
	testing.ZkRemoveTree(f.zkConn, "/")
	f.zkConn.Close()
}
