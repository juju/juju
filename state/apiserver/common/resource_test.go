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

func (resourceSuite) TestRegisterNamedGetCount(c *gc.C) {
	rs := common.NewResources()
	defer rs.StopAll()
	r1 := &fakeResource{}
	err := rs.RegisterNamed("fake1", r1)
	c.Assert(err, gc.IsNil)
	c.Check(rs.Count(), gc.Equals, 1)
	c.Check(rs.Get("fake1"), gc.Equals, r1)
}

func (resourceSuite) TestRegisterNamedRepeatedName(c *gc.C) {
	rs := common.NewResources()
	defer rs.StopAll()
	r1 := &fakeResource{}
	r2 := &fakeResource{}
	err := rs.RegisterNamed("fake1", r1)
	c.Assert(err, gc.IsNil)
	c.Check(rs.Count(), gc.Equals, 1)
	err = rs.RegisterNamed("fake1", r2)
	c.Check(err, gc.ErrorMatches, `resource "fake1" already registered`)
	c.Check(rs.Count(), gc.Equals, 1)
	c.Check(rs.Get("fake1"), gc.Equals, r1)
}

func (resourceSuite) TestRegisterNamedIntegerName(c *gc.C) {
	rs := common.NewResources()
	defer rs.StopAll()
	r1 := &fakeResource{}
	err := rs.RegisterNamed("1", r1)
	c.Check(err, gc.ErrorMatches, `RegisterNamed does not allow integer names: "1"`)
	c.Check(rs.Count(), gc.Equals, 0)
	c.Check(rs.Get("fake1"), gc.IsNil)
}

func (resourceSuite) TestRegisterNamedIntegerStart(c *gc.C) {
	rs := common.NewResources()
	defer rs.StopAll()
	r1 := &fakeResource{}
	err := rs.RegisterNamed("1fake", r1)
	c.Assert(err, gc.IsNil)
	c.Check(rs.Count(), gc.Equals, 1)
	c.Check(rs.Get("1fake"), gc.Equals, r1)
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
		rs.RegisterNamed("named", &fakeResource{})
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
	start(func() {
		rs.Get("named")
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

func (resourceSuite) TestStringResource(c *gc.C) {
	rs := common.NewResources()
	r1 := common.StringResource("foobar")
	id := rs.Register(r1)
	c.Check(rs.Get(id), gc.Equals, r1)
	asStr := rs.Get(id).(common.StringResource).String()
	c.Check(asStr, gc.Equals, "foobar")
}
