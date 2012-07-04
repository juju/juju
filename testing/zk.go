package testing

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/log"
	"net"
	"os"
	pathpkg "path"
	stdtesting "testing"
)

// ZkTestPackage should be called to register the tests for any package that
// requires a ZooKeeper server.
func ZkTestPackage(t *stdtesting.T) {
	srv := StartZkServer()
	defer srv.Destroy()
	var err error
	ZkAddr, err = srv.Addr()
	if err != nil {
		t.Fatalf("could not get ZooKeeper server address: %v", err)
	}
	a, err := net.ResolveTCPAddr("tcp", ZkAddr)
	if err != nil {
		// realy quite impossible
		t.Fatalf("could not convert resolve ZkAddr: %v", err)
	}
	ZkPort = a.Port
	TestingT(t)
}

var (
	// ZkAddr holds the address of the shared Zookeeper server set up by
	// ZkTestPackage.
	ZkAddr string

	// ZKPort holds the port portion of ZkAddr above
	ZkPort int
)

// StartZkServer starts a ZooKeeper server in a temporary directory.
// It panics if it encounters an error.
func StartZkServer() *zookeeper.Server {
	// In normal use, dir will be deleted by srv.Destroy, and does not need to
	// be tracked separately.
	dir, err := ioutil.TempDir("", "test-zk")
	if err != nil {
		panic(fmt.Errorf("cannot create temporary directory: %v", err))
	}
	srv, err := zookeeper.CreateServer(FindTCPPort(), dir, "")
	if err != nil {
		os.RemoveAll(dir)
		panic(fmt.Errorf("cannot create ZooKeeper server: %v", err))
	}
	err = srv.Start()
	if err != nil {
		srv.Destroy()
		panic(fmt.Errorf("cannot start ZooKeeper server: %v", err))
	}
	addr, err := srv.Addr()
	if err != nil {
		srv.Destroy()
		panic(fmt.Errorf("cannot get address of ZooKeeper server: %v", err))
	}
	log.Printf("testing: started zk server on %v", addr)
	return srv
}

// ZkSuite is a suite that deletes all content from the shared ZooKeeper server
// at the end of every test.
type ZkSuite struct{}

func (s *ZkSuite) SetUpSuite(c *C) {
	if ZkAddr == "" {
		panic("ZkSuite tests must be run with ZkTestPackage")
	}
}

func (s *ZkSuite) TearDownTest(c *C) {
	ZkReset()
}

// ZkConnSuite is a suite that supplies a connection to the shared
// ZooKeeper server.
type ZkConnSuite struct {
	ZkSuite
	ZkConn *zookeeper.Conn
}

func (s *ZkConnSuite) SetUpSuite(c *C) {
	s.ZkSuite.SetUpSuite(c)
	s.ZkConn = ZkConnect()
}

func (s *ZkConnSuite) TearDownSuite(c *C) {
	c.Assert(s.ZkConn.Close(), IsNil)
}

// ZkConnect returns a new connection to the shared Zookeeper server.
func ZkConnect() *zookeeper.Conn {
	conn, session, err := zookeeper.Dial(ZkAddr, 15e9)
	if err != nil {
		panic(err)
	}
	event := <-session
	assertf(event.Ok(), "initial event not OK")
	assertf(event.Type == zookeeper.EVENT_SESSION, "bad initial event type %#v", event.Type)
	assertf(event.State == zookeeper.STATE_CONNECTED, "bad initial event state %#v", event.State)
	return conn
}

// ZkReset deletes all content from the shared ZooKeeper server.
func ZkReset() {
	zk := ZkConnect()
	defer zk.Close()
	ZkRemoveTree(zk, "/")
	log.Printf("testing: reset zk server at %v", ZkAddr)
}

// ZkRemoveTree recursively removes a zookeeper node
// and all its children; it panics if it encounters an error.
// It does not delete "/zookeeper" or the root node itself and it does not
// consider deleting a nonexistent node to be an error.
func ZkRemoveTree(zk *zookeeper.Conn, path string) {
	// If we try to delete the zookeeper node (for example when
	// calling ZkRemoveTree(zk, "/")) we silently ignore it.
	if path == "/zookeeper" {
		return
	}
	// First recursively delete the children.
	children, _, err := zk.Children(path)
	if err != nil {
		if zookeeper.IsError(err, zookeeper.ZNONODE) {
			return
		}
		panic(err)
	}
	for _, child := range children {
		ZkRemoveTree(zk, pathpkg.Join(path, child))
	}
	// Now delete the path itself unless it's the root node.
	if path == "/" {
		return
	}
	err = zk.Delete(path, -1)
	// Technically we can't get a ZNONODE error unless something
	// else is deleting nodes concurrently, because otherwise the
	// call to Children above would have failed, but check anyway
	// for completeness.
	if err != nil && !zookeeper.IsError(err, zookeeper.ZNONODE) {
		panic(err)
	}
}

// FindTCPPort finds an unused TCP port and returns it.
// Use of this function has an inherent race condition - another
// process may claim the port before we try to use it.
// We hope that the probability is small enough during
// testing to be negligible.
func FindTCPPort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func assertf(b bool, msg string, args ...interface{}) {
	if !b {
		panic(fmt.Errorf(msg, args...))
	}
}
