package common_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state/apiserver/common"
	"sync"
)

type resourceSuite struct{}

var _ = Suite(resourceSuite{})

type fakeResource struct {
	stopped bool
}

func (r *fakeResource) Stop() error {
	r.stopped = true
	return nil
}

func (resourceSuite) TestRegisterGetCount(c *C) {
	rs := common.NewResources()
	r1 := &fakeResource{}
	id := rs.Register(r1)
	c.Assert(id, Equals, "1")
	c.Assert(rs.Get("1"), Equals, r1)
	c.Assert(rs.Count(), Equals, 1)

	r2 := &fakeResource{}
	id = rs.Register(r2)
	c.Assert(id, Equals, "2")
	c.Assert(rs.Get("2"), Equals, r2)
	c.Assert(rs.Count(), Equals, 2)
}

func (resourceSuite) TestConcurrency(c *C) {
	// This test is designed to cause the race detector
	// to fail if the locking is not done correctly.
	var wg sync.WaitGroup
	rs := common.NewResources()

	start := func(f func()) {
		wg.Add(1)
		go func() {
			f()
			wg.Done()
		}()
	}
	rs.Register(&fakeResource{})
	start(func() {
		rs.Register(&fakeResource{})
	})
	start(func() {
		rs.Stop("1")
	})
	start(func() {
		rs.Count()
	})
	start(func() {
		rs.StopAll()
	})
	start(func() {
		rs.Get("2")
	})
	wg.Wait()
}

func (resourceSuite) TestStop(c *C) {
	rs := common.NewResources()
	r1 := &fakeResource{}
	rs.Register(r1)
	r2 := &fakeResource{}
	rs.Register(r2)
	rs.Stop("1")
	c.Assert(r1.stopped, Equals, true)
	c.Assert(rs.Get("1"), IsNil)
	c.Assert(r2.stopped, Equals, false)
	c.Assert(rs.Get("2"), Equals, r2)
	c.Assert(rs.Count(), Equals, 1)
}

func (resourceSuite) TestStopAll(c *C) {
	rs := common.NewResources()
	r1 := &fakeResource{}
	rs.Register(r1)
	r2 := &fakeResource{}
	rs.Register(r2)
	rs.StopAll()

	c.Assert(r1.stopped, Equals, true)
	c.Assert(rs.Get("1"), IsNil)
	c.Assert(r2.stopped, Equals, true)
	c.Assert(rs.Get("2"), IsNil)

	c.Assert(rs.Count(), Equals, 0)
}
