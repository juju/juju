package watcher_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/juju/state/watcher"
	"launchpad.net/juju-core/juju/testing"
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

type contentWatcherTest struct {
	test    func(*C, *WatcherSuite)
	want    watcher.ContentChange
	timeout bool
}

var contentWatcherTests = []contentWatcherTest{
	{func(c *C, s *WatcherSuite) {}, watcher.ContentChange{false, ""}, false},
	{func(c *C, s *WatcherSuite) { s.createPath(c, "init") }, watcher.ContentChange{true, "init"}, false},
	{func(c *C, s *WatcherSuite) { s.changeContent(c, "foo") }, watcher.ContentChange{true, "foo"}, false},
	{func(c *C, s *WatcherSuite) { s.changeContent(c, "foo") }, watcher.ContentChange{}, true},
	{func(c *C, s *WatcherSuite) { s.removePath(c) }, watcher.ContentChange{false, ""}, false},
	{func(c *C, s *WatcherSuite) { s.createPath(c, "done") }, watcher.ContentChange{true, "done"}, false},
}

func (s *WatcherSuite) TestContentWatcher(c *C) {
	contentWatcher := watcher.NewContentWatcher(s.zkConn, s.path)

	for i, test := range contentWatcherTests {
		c.Logf("test %d", i)
		test.test(c, s)
		select {
		case got, ok := <-contentWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, Equals, test.want)
		case <-time.After(200 * time.Millisecond):
			if !test.timeout {
				c.Fatalf("didn't get change: %#v", test.want)
			}
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

type childrenWatcherTest struct {
	test func(*C, *WatcherSuite)
	want *watcher.ChildrenChange
}

var childrenWatcherTests = []childrenWatcherTest{
	{func(c *C, s *WatcherSuite) {}, &watcher.ChildrenChange{}},
	{func(c *C, s *WatcherSuite) { s.changeChildren(c, true, "foo") }, &watcher.ChildrenChange{[]string{"foo"}, nil}},
	{func(c *C, s *WatcherSuite) { s.changeChildren(c, true, "bar") }, &watcher.ChildrenChange{[]string{"bar"}, nil}},
	{func(c *C, s *WatcherSuite) { s.changeChildren(c, false, "foo") }, &watcher.ChildrenChange{nil, []string{"foo"}}},
	{func(c *C, s *WatcherSuite) { s.removePath(c) }, &watcher.ChildrenChange{nil, []string{"bar"}}},
	{func(c *C, s *WatcherSuite) { s.createPath(c, "") }, nil},
	{func(c *C, s *WatcherSuite) { s.changeChildren(c, true, "bar") }, &watcher.ChildrenChange{[]string{"bar"}, nil}},
}

func (s *WatcherSuite) TestChildrenWatcher(c *C) {
	s.createPath(c, "init")
	childrenWatcher := watcher.NewChildrenWatcher(s.zkConn, s.path)

	for i, test := range childrenWatcherTests {
		c.Logf("test %d", i)
		test.test(c, s)
		select {
		case got, ok := <-childrenWatcher.Changes():
			if test.want != nil {
				c.Assert(ok, Equals, true)
				c.Assert(got, DeepEquals, *test.want)
			} else if ok {
				c.Fatalf("got unwanted change: %#v", got)
			}
		case <-time.After(200 * time.Millisecond):
			if test.want != nil {
				c.Fatalf("didn't get change: %#v", test.want)
			}
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
