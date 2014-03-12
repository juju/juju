// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"fmt"

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

func (s *aggregateSuite) TestSingleRequest(c *gc.C) {
	testGetter := new(testInstanceGetter)
	address1 := instance.NewAddress("127.0.0.1")
	address2 := instance.NewAddress("192.168.1.1")
	instance1 := testInstance{
			status:    "foobar",
			addresses: []instance.Address{address1, address2},
		}
	testGetter.results = []testInstance{instance1}
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
		addresses: []instance.Address{address1, address2},
	})
	c.Assert(testGetter.ids, gc.DeepEquals, []instance.Id{instance.Id("foo")})
}
