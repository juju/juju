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
	contentWatcher := watcher.NewContentWatcher(s.zkConn, s.path)

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

	var expectedChanges = []watcher.ContentChange{
		{true, "init"},
		{true, "foo"},
		{false, ""},
		{true, "done"},
	}
	for _, want := range expectedChanges {
		select {
		case got, ok := <-contentWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, Equals, want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", want)
		}
	}

	select {
	case got, _ := <-contentWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	err := contentWatcher.Stop()
	c.Assert(err, IsNil)

	select {
	case _, ok := <-contentWatcher.Changes():
		c.Assert(ok, Equals, false)
	case <-time.After(200 * time.Millisecond):
		c.Fatalf("unexpected timeout")
	}
}

func (s *WatcherSuite) TestChildrenWatcher(c *C) {
	s.createPath(c, "init")
	childrenWatcher := watcher.NewChildrenWatcher(s.zkConn, s.path)

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.changeChildren(c, true, "foo")
		time.Sleep(50 * time.Millisecond)
		s.changeChildren(c, true, "bar")
		time.Sleep(50 * time.Millisecond)
		s.changeChildren(c, false, "foo")
	}()

	var expectedChanges = []watcher.ChildrenChange{
		{[]string{"foo"}, nil},
		{[]string{"bar"}, nil},
		{nil, []string{"foo"}},
	}
	for _, want := range expectedChanges {
		select {
		case got, ok := <-childrenWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, DeepEquals, want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", want)
		}
	}

	select {
	case got, _ := <-childrenWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	err := childrenWatcher.Stop()
	c.Assert(err, IsNil)

	select {
	case _, ok := <-childrenWatcher.Changes():
		c.Assert(ok, Equals, false)
	case <-time.After(200 * time.Millisecond):
		c.Fatalf("unexpected timeout")
	}
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
