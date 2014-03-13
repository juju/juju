// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"fmt"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing/testbase"
)

type aggregateSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&aggregateSuite{})

type testInstance struct {
	instance.Instance
	addresses []instance.Address
	status    string
	err       bool
}

var _ instance.Instance = (*testInstance)(nil)

func (t *testInstance) Addresses() ([]instance.Address, error) {
	if t.err {
		return nil, fmt.Errorf("gotcha")
	}
	return t.addresses, nil
}

func (t *testInstance) Status() string {
	return t.status
}

type testInstanceGetter struct {
	ids     []instance.Id
	results []*testInstance
	err     error
}

func (i *testInstanceGetter) Instances(ids []instance.Id) (result []instance.Instance, err error) {
	i.ids = ids
	err = i.err
	for _, inst := range i.results {
		if inst == nil {
			result = append(result, nil)
		} else {
			result = append(result, inst)
		}
	}
	return
}

func newTestInstance(status string, addresses []string) *testInstance {
	thisInstance := testInstance{status: status}
	for _, address := range addresses {
		thisInstance.addresses = append(thisInstance.addresses, instance.NewAddress(address))
	}
	return &thisInstance
}

func (s *aggregateSuite) TestSingleRequest(c *gc.C) {
	testGetter := new(testInstanceGetter)
	instance1 := newTestInstance("foobar", []string{"127.0.0.1", "192.168.1.1"})
	testGetter.results = []*testInstance{instance1}
	aggregator := newAggregator(testGetter)

	replyChan := make(chan instanceInfoReply)
	req := instanceInfoReq{
		reply:  replyChan,
		instId: instance.Id("foo"),
	}
	aggregator.reqc <- req
	reply := <-replyChan
	c.Assert(reply.err, gc.IsNil)
	c.Assert(reply.info, gc.DeepEquals, instanceInfo{
		status:    "foobar",
		addresses: instance1.addresses,
	})
	c.Assert(testGetter.ids, gc.DeepEquals, []instance.Id{instance.Id("foo")})
}

func (s *aggregateSuite) TestRequestBatching(c *gc.C) {
	s.PatchValue(&gatherTime, 30*time.Millisecond)
	testGetter := new(testInstanceGetter)

	instance1 := newTestInstance("foobar", []string{"127.0.0.1", "192.168.1.1"})
	testGetter.results = []*testInstance{instance1}
	aggregator := newAggregator(testGetter)

	replyChan := make(chan instanceInfoReply)
	req := instanceInfoReq{
		reply:  replyChan,
		instId: instance.Id("foo"),
	}
	aggregator.reqc <- req
	reply := <-replyChan
	c.Assert(reply.err, gc.IsNil)

	instance2 := newTestInstance("not foobar", []string{"192.168.1.2"})
	instance3 := newTestInstance("ok-ish", []string{"192.168.1.3"})

	replyChan2 := make(chan instanceInfoReply)
	replyChan3 := make(chan instanceInfoReply)

	aggregator.reqc <- instanceInfoReq{reply: replyChan2, instId: instance.Id("foo2")}
	aggregator.reqc <- instanceInfoReq{reply: replyChan3, instId: instance.Id("foo3")}

	testGetter.results = []*testInstance{instance2, instance3}
	reply2 := <-replyChan2
	reply3 := <-replyChan3
	c.Assert(reply2.err, gc.IsNil)
	c.Assert(reply3.err, gc.IsNil)
	c.Assert(reply2.info.status, gc.Equals, "not foobar")
	c.Assert(reply3.info.status, gc.Equals, "ok-ish")

	c.Assert(testGetter.ids, gc.DeepEquals, []instance.Id{instance.Id("foo2"), instance.Id("foo3")})
}

func (s *aggregateSuite) TestError(c *gc.C) {
	testGetter := new(testInstanceGetter)
	ourError := fmt.Errorf("Some error")
	testGetter.err = ourError

	aggregator := newAggregator(testGetter)

	replyChan := make(chan instanceInfoReply)
	req := instanceInfoReq{
		reply:  replyChan,
		instId: instance.Id("foo"),
	}
	aggregator.reqc <- req
	reply := <-replyChan
	c.Assert(reply.err, gc.Equals, ourError)
}

func (s *aggregateSuite) TestPartialErrResponse(c *gc.C) {
	testGetter := new(testInstanceGetter)
	testGetter.err = environs.ErrPartialInstances
	testGetter.results = []*testInstance{nil}

	aggregator := newAggregator(testGetter)

	replyChan := make(chan instanceInfoReply)
	req := instanceInfoReq{
		reply:  replyChan,
		instId: instance.Id("foo"),
	}
	aggregator.reqc <- req
	reply := <-replyChan
	c.Assert(reply.err, gc.DeepEquals, errors.NotFoundf("instance foo"))
}

func (s *aggregateSuite) TestAddressesError(c *gc.C) {
	testGetter := new(testInstanceGetter)
	instance1 := newTestInstance("foobar", []string{"127.0.0.1", "192.168.1.1"})
	instance1.err = true
	testGetter.results = []*testInstance{instance1}
	aggregator := newAggregator(testGetter)

	replyChan := make(chan instanceInfoReply)
	req := instanceInfoReq{
		reply:  replyChan,
		instId: instance.Id("foo"),
	}
	aggregator.reqc <- req
	reply := <-replyChan
	c.Assert(reply.err, gc.DeepEquals, fmt.Errorf("gotcha"))
}

func (s *aggregateSuite) TestKillAndWait(c *gc.C) {
	testGetter := new(testInstanceGetter)
	aggregator := newAggregator(testGetter)
	aggregator.Kill()
	err := aggregator.Wait()
	c.Assert(err, gc.IsNil)
}

func (s *aggregateSuite) TestLoopDying(c *gc.C) {
	testGetter := new(testInstanceGetter)
	aggregator := newAggregator(testGetter)
	close(aggregator.reqc)
	err := aggregator.Wait()
	c.Assert(err, gc.NotNil)
}
