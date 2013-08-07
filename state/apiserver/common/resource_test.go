package common_test

import (
	"sync"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state/apiserver/common"
)

type resourceSuite struct{}

var _ = gc.Suite(resourceSuite{})

type fakeResource struct {
	stopped bool
}

func (r *fakeResource) Stop() error {
	r.stopped = true
	return nil
}

func (resourceSuite) TestRegisterGetCount(c *gc.C) {
	rs := common.NewResources()
	r1 := &fakeResource{}
	id := rs.Register(r1)
	c.Assert(id, gc.Equals, "1")
	c.Assert(rs.Get("1"), gc.Equals, r1)
	c.Assert(rs.Count(), gc.Equals, 1)

	r2 := &fakeResource{}
	id = rs.Register(r2)
	c.Assert(id, gc.Equals, "2")
	c.Assert(rs.Get("2"), gc.Equals, r2)
	c.Assert(rs.Count(), gc.Equals, 2)
}

func (resourceSuite) TestConcurrency(c *gc.C) {
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

func (resourceSuite) TestStop(c *gc.C) {
	rs := common.NewResources()
	r1 := &fakeResource{}
	rs.Register(r1)
	r2 := &fakeResource{}
	rs.Register(r2)
	rs.Stop("1")
	c.Assert(r1.stopped, gc.Equals, true)
	c.Assert(rs.Get("1"), gc.IsNil)
	c.Assert(r2.stopped, gc.Equals, false)
	c.Assert(rs.Get("2"), gc.Equals, r2)
	c.Assert(rs.Count(), gc.Equals, 1)
}

func (resourceSuite) TestStopAll(c *gc.C) {
	rs := common.NewResources()
	r1 := &fakeResource{}
	rs.Register(r1)
	r2 := &fakeResource{}
	rs.Register(r2)
	rs.StopAll()

	c.Assert(r1.stopped, gc.Equals, true)
	c.Assert(rs.Get("1"), gc.IsNil)
	c.Assert(r2.stopped, gc.Equals, true)
	c.Assert(rs.Get("2"), gc.IsNil)

	c.Assert(rs.Count(), gc.Equals, 0)
}
