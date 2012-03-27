package watcher_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/state/watcher"
	"launchpad.net/juju/go/testing"
	stdtesting "testing"
	"time"
)

var zkAddr string

// wrapper shows the returning of user defined types instead of
// pure sting slices by the users of a watcher.
type wrapper struct {
	FieldA string
	FieldB string
}

func TestPackage(t *stdtesting.T) {
	srv := testing.StartZkServer(t)
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
	testing.ZkRemoveTree(c, s.zkConn, s.path)
	s.zkConn.Close()
}

func (s *WatcherSuite) TestContentWatcher(c *C) {
	watcher, err := watcher.NewContentWatcher(s.zkConn, s.path)
	c.Assert(err, IsNil)

	go func() {
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
	case <-time.After(time.Second):
		// The timeout is expected.
	}

	err = watcher.Stop()
	c.Assert(err, IsNil)
}

func (s *WatcherSuite) TestWrappedContentWatcher(c *C) {
	watcher, err := watcher.NewContentWatcher(s.zkConn, s.path)
	c.Assert(err, IsNil)

	wrapperChan := make(chan *wrapper, 1)

	go func() {
		// Receive raw changes and send as wrapper instance.
		for change := range watcher.Changes() {
			w := &wrapper{}
			err := goyaml.Unmarshal([]byte(change), w)
			c.Assert(err, IsNil)
			wrapperChan <- w
		}
	}()

	go func() {
		time.Sleep(50 * time.Millisecond)
		w := &wrapper{"foo", "bar"}
		yaml, err := goyaml.Marshal(w)
		c.Assert(err, IsNil)
		s.changeContent(c, string(yaml))

		time.Sleep(50 * time.Millisecond)
		w = &wrapper{"bar", "foo"}
		yaml, err = goyaml.Marshal(w)
		c.Assert(err, IsNil)
		s.changeContent(c, string(yaml))
	}()

	// Receive the two changes.
	change := <-wrapperChan
	c.Assert(change.FieldA, Equals, "foo")
	c.Assert(change.FieldB, Equals, "bar")

	change = <-wrapperChan
	c.Assert(change.FieldA, Equals, "bar")
	c.Assert(change.FieldB, Equals, "foo")

	// No more changes.
	select {
	case <-watcher.Changes():
		c.Fail()
	case <-time.After(time.Second):
		// The timeout is expected.
	}

	err = watcher.Stop()
	c.Assert(err, IsNil)
}

func (s *WatcherSuite) TestChildrenWatcher(c *C) {
	watcher, err := watcher.NewChildrenWatcher(s.zkConn, s.path)
	c.Assert(err, IsNil)

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
	c.Assert(change.New, DeepEquals, []string{"foo"})

	change = <-watcher.Changes()
	c.Assert(change.New, DeepEquals, []string{"bar"})

	change = <-watcher.Changes()
	c.Assert(change.Del, DeepEquals, []string{"foo"})

	// No more changes.
	select {
	case <-watcher.Changes():
		c.Fail()
	case <-time.After(time.Second):
		// The timeout is expected.
	}

	err = watcher.Stop()
	c.Assert(err, IsNil)
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
		c.Assert(err, IsNil)
	} else {
		err = s.zkConn.Delete(path, -1)
	}
	c.Assert(err, IsNil)
}
