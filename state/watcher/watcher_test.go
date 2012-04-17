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

	c.Assert(err, IsNil)
}

func (s *WatcherSuite) TearDownTest(c *C) {
	testing.ZkRemoveTree(s.zkConn, s.path)
	s.zkConn.Close()
}

func (s *WatcherSuite) TestContentWatcher(c *C) {
	receiveChange := func(w *watcher.ContentWatcher) (*watcher.ContentChange, bool, bool) {
		select {
		case change, ok := <-w.Changes():
			return &change, ok, false
		case <-time.After(200 * time.Millisecond):
			return nil, false, true
		}
		return nil, false, false
	}
	watcher := watcher.NewContentWatcher(s.zkConn, s.path)

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.createPath(c, "init")
		time.Sleep(50 * time.Millisecond)
		s.changeContent(c, "foo")
		time.Sleep(50 * time.Millisecond)
		s.changeContent(c, "foo")
		time.Sleep(50 * time.Millisecond)
		s.removePath(c)
		time.Sleep(50 * time.Millisecond)
		s.createPath(c, "done")
	}()

	// Receive the four changes create, content change, 
	// delete and create again.
	change, ok, timeout := receiveChange(watcher)
	c.Assert(timeout, Equals, false)
	c.Assert(ok, Equals, true)
	c.Assert(change.Exists, Equals, true)
	c.Assert(change.Content, Equals, "init")

	change, ok, timeout = receiveChange(watcher)
	c.Assert(timeout, Equals, false)
	c.Assert(ok, Equals, true)
	c.Assert(change.Exists, Equals, true)
	c.Assert(change.Content, Equals, "foo")

	change, ok, timeout = receiveChange(watcher)
	c.Assert(timeout, Equals, false)
	c.Assert(ok, Equals, true)
	c.Assert(change.Exists, Equals, false)
	c.Assert(change.Content, Equals, "")

	change, ok, timeout = receiveChange(watcher)
	c.Assert(timeout, Equals, false)
	c.Assert(ok, Equals, true)
	c.Assert(change.Exists, Equals, true)
	c.Assert(change.Content, Equals, "done")

	// No more changes.
	_, _, timeout = receiveChange(watcher)
	c.Assert(timeout, Equals, true)

	err := watcher.Stop()
	c.Assert(err, IsNil)

	// Changes() has to be closed.
	_, ok, timeout = receiveChange(watcher)
	c.Assert(ok, Equals, false)
	c.Assert(timeout, Equals, false)
}

func (s *WatcherSuite) TestChildrenWatcher(c *C) {
	receiveChange := func(w *watcher.ChildrenWatcher) (*watcher.ChildrenChange, bool, bool) {
		select {
		case change, ok := <-w.Changes():
			return &change, ok, false
		case <-time.After(200 * time.Millisecond):
			return nil, false, true
		}
		return nil, false, false
	}
	s.createPath(c, "init")
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
	change, ok, timeout := receiveChange(watcher)
	c.Assert(timeout, Equals, false)
	c.Assert(ok, Equals, true)
	c.Assert(change.Added, DeepEquals, []string{"foo"})

	change, ok, timeout = receiveChange(watcher)
	c.Assert(timeout, Equals, false)
	c.Assert(ok, Equals, true)
	c.Assert(change.Added, DeepEquals, []string{"bar"})

	change, ok, timeout = receiveChange(watcher)
	c.Assert(timeout, Equals, false)
	c.Assert(ok, Equals, true)
	c.Assert(change.Deleted, DeepEquals, []string{"foo"})

	// No more changes.
	_, _, timeout = receiveChange(watcher)
	c.Assert(timeout, Equals, true)

	err := watcher.Stop()
	c.Assert(err, IsNil)

	// Changes() has to be closed.
	_, ok, timeout = receiveChange(watcher)
	c.Assert(ok, Equals, false)
	c.Assert(timeout, Equals, false)
}

func (s *WatcherSuite) createPath(c *C, content string) {
	_, err := s.zkConn.Create(s.path, content, 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)
}

func (s *WatcherSuite) removePath(c *C) {
	testing.ZkRemoveTree(s.zkConn, s.path)
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
