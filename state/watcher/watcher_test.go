package watcher_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/state/watcher"
	"launchpad.net/juju/go/testing"
	stdtesting "testing"
	"time"
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

type WatcherSuite struct {
	zkConn *zookeeper.Conn
	path   string
}

var _ = Suite(&WatcherSuite{})

func (s *WatcherSuite) SetUpTest(c *C) {
	zk, session, err := zookeeper.Dial(zkAddr, 15e9)
	c.Assert(err, IsNil)
	event := <-session
	c.Assert(event.Ok(), Equals, true)
	c.Assert(event.Type, Equals, zookeeper.EVENT_SESSION)
	c.Assert(event.State, Equals, zookeeper.STATE_CONNECTED)

	s.zkConn = zk
	s.path = "/watcher"

	_, err = s.zkConn.Create(s.path, "", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)
}

func (s *WatcherSuite) TearDownTest(c *C) {
	testing.ZkRemoveTree(s.zkConn, s.path)
	s.zkConn.Close()
}

func (s *WatcherSuite) TestContentWatcher(c *C) {
	watcher := watcher.NewContentWatcher(s.zkConn, s.path)

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.changeContent(c, "foo")

		time.Sleep(50 * time.Millisecond)
		s.changeContent(c, "foo")

		time.Sleep(50 * time.Millisecond)
		s.changeContent(c, "bar")
	}()

	// Receive the two changes.
	change := <-watcher.Changes()
	c.Assert(change, Equals, "foo")

	change = <-watcher.Changes()
	c.Assert(change, Equals, "bar")

	// No more changes.
	select {
	case <-watcher.Changes():
		c.Fail()
	case <-time.After(200 * time.Millisecond):
		// The timeout is expected.
	}

	err := watcher.Stop()
	c.Assert(err, IsNil)

	// Changes() has to be closed
	select {
	case _, ok := <-watcher.Changes():
		c.Assert(ok, Equals, false)
	case <-time.After(200 * time.Millisecond):
		// Timeout should not bee needed.
		c.Fail()
	}
}

func (s *WatcherSuite) TestChildrenWatcher(c *C) {
	watcher := watcher.NewChildrenWatcher(s.zkConn, s.path)

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.changeChildren(c, true, "foo")

		time.Sleep(50 * time.Millisecond)
		s.changeChildren(c, true, "bar")

		time.Sleep(50 * time.Millisecond)
		s.changeChildren(c, false, "foo")
	}()

	// Receive the three changes.
	change := <-watcher.Changes()
	c.Assert(change.Added, DeepEquals, []string{"foo"})

	change = <-watcher.Changes()
	c.Assert(change.Added, DeepEquals, []string{"bar"})

	change = <-watcher.Changes()
	c.Assert(change.Deleted, DeepEquals, []string{"foo"})

	// No more changes.
	select {
	case <-watcher.Changes():
		c.Fail()
	case <-time.After(time.Second):
		// The timeout is expected.
	}

	err := watcher.Stop()
	c.Assert(err, IsNil)

	// Changes() has to be closed
	select {
	case _, ok := <-watcher.Changes():
		c.Assert(ok, Equals, false)
	case <-time.After(200 * time.Millisecond):
		// Timeout should not bee needed.
		c.Fail()
	}
}

func (s *WatcherSuite) TestDeletedNode(c *C) {
	watcher := watcher.NewContentWatcher(s.zkConn, s.path)

	go func() {
		time.Sleep(50 * time.Millisecond)
		testing.ZkRemoveTree(s.zkConn, s.path)
	}()

	// Changes() has to be closed
	select {
	case _, ok := <-watcher.Changes():
		c.Assert(ok, Equals, false)
	case <-time.After(200 * time.Millisecond):
		// Timeout should not bee needed.
		c.Fail()
	}

	err := watcher.Stop()
	c.Assert(err, ErrorMatches, `watcher: node "/watcher" has been deleted`)
}

func (s *WatcherSuite) changeContent(c *C, content string) {
	_, err := s.zkConn.Set(s.path, content, -1)
	c.Assert(err, IsNil)
}

func (s *WatcherSuite) changeChildren(c *C, add bool, child string) {
	var err error
	path := s.path + "/" + child
	if add {
		_, err = s.zkConn.Create(path, "", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	} else {
		err = s.zkConn.Delete(path, -1)
	}
	c.Assert(err, IsNil)
}
