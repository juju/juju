package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/state"
)

type CommonSuite struct {
	zkServer   *zookeeper.Server
	zkTestRoot string
	zkTestPort int
	zkAddr     string
	zkConn     *zookeeper.Conn
	st         *state.State
}

func (s *CommonSuite) SetUpSuite(c *C) {
	var err error
	s.zkTestRoot = c.MkDir() + "/zookeeper"
	s.zkTestPort = 21812
	s.zkAddr = fmt.Sprint("localhost:", s.zkTestPort)

	s.zkServer, err = zookeeper.CreateServer(s.zkTestPort, s.zkTestRoot, "")
	if err != nil {
		c.Fatal("Cannot set up ZooKeeper server environment: ", err)
	}
	err = s.zkServer.Start()
	if err != nil {
		c.Fatal("Cannot start ZooKeeper server: ", err)
	}
}

func (s *CommonSuite) TearDownSuite(c *C) {
	if s.zkServer != nil {
		s.zkServer.Destroy()
	}
}

func (s *CommonSuite) SetUpTest(c *C) {
	s.st, s.zkConn = state.OpenAddr(c, s.zkAddr)
	err := s.st.Initialize()
	c.Assert(err, IsNil)
}

func (s *CommonSuite) TearDownTest(c *C) {
	state.ZkRemoveAll(s.zkConn)
	s.zkConn.Close()
}
