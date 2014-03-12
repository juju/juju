// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"fmt"
	"time"

	gc "launchpad.net/gocheck"

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
}

var _ instance.Instance = (*testInstance)(nil)

func (t testInstance) Addresses() ([]instance.Address, error) {
	return t.addresses, nil
}

func (t testInstance) Status() string {
	return t.status
}

type testInstanceGetter struct {
	ids     []instance.Id
	results []testInstance
	err     bool
}

func (i *testInstanceGetter) Instances(ids []instance.Id) (result []instance.Instance, err error) {
	i.ids = ids
	if i.err {
		return nil, fmt.Errorf("Some error")
	}
	for _, inst := range i.results {
		result = append(result, inst)
	}
	return
}

func newTestInstance(status string, addresses []string) *testInstance {
	thisInstance := &testInstance{status: status}
	for _, address := range addresses {
		thisInstance.addresses = append(thisInstance.addresses, instance.NewAddress(address))
	}
	return thisInstance
}

func (s *aggregateSuite) TestSingleRequest(c *gc.C) {
	testGetter := new(testInstanceGetter)
	instance1 := newTestInstance("foobar", []string{"127.0.0.1", "192.168.1.1"})
	testGetter.results = []testInstance{*instance1}
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
	s.PatchValue(&GatherTime, 30*time.Millisecond)
	testGetter := new(testInstanceGetter)

	instance1 := newTestInstance("foobar", []string{"127.0.0.1", "192.168.1.1"})
	testGetter.results = []testInstance{*instance1}
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

	testGetter.results = []testInstance{*instance2, *instance3}
	reply2 := <-replyChan2
	reply3 := <-replyChan3
	c.Assert(reply2.err, gc.IsNil)
	c.Assert(reply3.err, gc.IsNil)

	c.Assert(testGetter.ids, gc.DeepEquals, []instance.Id{instance.Id("foo2"), instance.Id("foo3")})
}
