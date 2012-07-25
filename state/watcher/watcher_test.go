package watcher_test

import (
	"errors"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/testing"
	"launchpad.net/tomb"
	stdtesting "testing"
	"time"
)

func TestPackage(t *stdtesting.T) {
	testing.ZkTestPackage(t)
}

type WatcherSuite struct {
	testing.ZkConnSuite
	path string
}

var _ = Suite(&WatcherSuite{})

func (s *WatcherSuite) SetUpSuite(c *C) {
	s.ZkConnSuite.SetUpSuite(c)
	s.path = "/watcher"
}

type contentWatcherTest struct {
	test func(*C, *WatcherSuite)
	want watcher.ContentChange
}

var contentWatcherTests = []contentWatcherTest{
	{func(c *C, s *WatcherSuite) {}, watcher.ContentChange{}},
	{func(c *C, s *WatcherSuite) { s.createPath(c, "init") }, watcher.ContentChange{true, 0, "init"}},
	{func(c *C, s *WatcherSuite) { s.changeContent(c, "foo") }, watcher.ContentChange{true, 1, "foo"}},
	{func(c *C, s *WatcherSuite) { s.changeContent(c, "foo") }, watcher.ContentChange{true, 2, "foo"}},
	{func(c *C, s *WatcherSuite) { s.removePath(c) }, watcher.ContentChange{}},
	{func(c *C, s *WatcherSuite) { s.createPath(c, "done") }, watcher.ContentChange{true, 0, "done"}},
}

func (s *WatcherSuite) TestContentWatcher(c *C) {
	contentWatcher := watcher.NewContentWatcher(s.ZkConn, s.path)

	for i, test := range contentWatcherTests {
		c.Logf("test %d", i)
		test.test(c, s)
		select {
		case got, ok := <-contentWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, Equals, test.want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("did not get change: %#v", test.want)
		}
	}

	select {
	case got, _ := <-contentWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	c.Assert(contentWatcher.Err(), Equals, tomb.ErrStillAlive)
	err := contentWatcher.Stop()
	c.Assert(err, IsNil)
	c.Assert(contentWatcher.Err(), IsNil)

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
	childrenWatcher := watcher.NewChildrenWatcher(s.ZkConn, s.path)

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
				c.Fatalf("did not get change: %#v", test.want)
			}
		}
	}

	select {
	case got, _ := <-childrenWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	c.Assert(childrenWatcher.Err(), Equals, tomb.ErrStillAlive)
	err := childrenWatcher.Stop()
	c.Assert(err, IsNil)
	c.Assert(childrenWatcher.Err(), IsNil)

	select {
	case _, ok := <-childrenWatcher.Changes():
		c.Assert(ok, Equals, false)
	case <-time.After(200 * time.Millisecond):
		c.Fatalf("unexpected timeout")
	}
}

func (s *WatcherSuite) createPath(c *C, content string) {
	_, err := s.ZkConn.Create(s.path, content, 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)
}

func (s *WatcherSuite) removePath(c *C) {
	testing.ZkRemoveTree(s.ZkConn, s.path)
}

func (s *WatcherSuite) changeContent(c *C, content string) {
	_, err := s.ZkConn.Set(s.path, content, -1)
	c.Assert(err, IsNil)
}

func (s *WatcherSuite) changeChildren(c *C, add bool, child string) {
	var err error
	path := s.path + "/" + child
	if add {
		_, err = s.ZkConn.Create(path, "", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	} else {
		err = s.ZkConn.Delete(path, -1)
	}
	c.Assert(err, IsNil)
}

type dummyWatcher struct {
	err error
}

func (w *dummyWatcher) Stop() error {
	return w.err
}

func (w *dummyWatcher) Err() error {
	return w.err
}

func (s *WatcherSuite) TestStop(c *C) {
	t := &tomb.Tomb{}
	watcher.Stop(&dummyWatcher{nil}, t)
	c.Assert(t.Err(), Equals, tomb.ErrStillAlive)

	watcher.Stop(&dummyWatcher{errors.New("BLAM")}, t)
	c.Assert(t.Err(), ErrorMatches, "BLAM")
}

func (s *WatcherSuite) TestMustErr(c *C) {
	err := watcher.MustErr(&dummyWatcher{errors.New("POW")})
	c.Assert(err, ErrorMatches, "POW")

	stillAlive := func() { watcher.MustErr(&dummyWatcher{tomb.ErrStillAlive}) }
	c.Assert(stillAlive, PanicMatches, "watcher is still running")

	noErr := func() { watcher.MustErr(&dummyWatcher{nil}) }
	c.Assert(noErr, PanicMatches, "watcher was stopped cleanly")
}
