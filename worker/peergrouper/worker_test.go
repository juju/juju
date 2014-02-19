package peergrouper

import (
	"fmt"
	"time"

	gc "launchpad.net/gocheck"

	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils/voyeur"
	"launchpad.net/juju-core/worker"
)

//type workerJujuConnSuite struct {
//	testing.JujuConnSuite
//}
//
//var _ = gc.Suite(&workerJujuConnSuite{})
//
//func (s *workerJujuConnSuite) TestStartStop(c *gc.C) {
//	w, err := New(s.State)
//	c.Assert(err, gc.IsNil)
//	err = worker.Stop(w)
//	c.Assert(err, gc.IsNil)
//}

type workerSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestSetsMembersInitially(c *gc.C) {
	st := newFakeState()
	var ids []string
	for i := 10; i < 13; i++ {
		id := fmt.Sprint(i)
		m := st.addMachine(id, true)
		m.setStateHostPort(fmt.Sprintf("0.1.2.%d", i))
		ids = append(ids, id)
	}
	st.setStateServers(ids...)
	st.session.Set(mkMembers("0v"))
	st.session.setStatus(mkStatuses("0p"))

	memberWatcher := st.session.members.Watch()
	mustNext(c, memberWatcher)
	c.Assert(memberWatcher.Value(), gc.HasLen, 1)

	logger.Infof("starting worker")
	w := newWorker(st)
	defer func() {
		c.Check(worker.Stop(w), gc.IsNil)
	}()

	mustNext(c, memberWatcher)
	c.Assert(memberWatcher.Value(), gc.HasLen, 3)
}

func mustNext(c *gc.C, w *voyeur.Watcher) (ok bool) {
	done := make(chan struct{})
	go func() {
		ok = w.Next()
		done <- struct{}{}
	}()
	select {
	case <-done:
		return
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for value to be set")
	}
	panic("unreachable")
}
