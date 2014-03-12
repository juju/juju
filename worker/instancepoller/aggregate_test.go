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
    id instance.Id
    address instance.Address
}

func (t *testInstance) Addresses() ([]instance.Address, error) {
    return t.addresses, nil
}

func (t *testInstance) Id() (instance.Id) {
    return t.id
}

type testInstanceGetter struct {
    ids []instance.Id
    results []instance.Instance
    err bool
}

func (i *testInstanceGetter) Instances(ids []instance.Id) ([]instance.Instance, error) {
    i.ids = ids
    if i.err {
        return nil, fmt.Errorf("Some error")
    }
    return i.results, nil
}

func (s *aggregateSuite) TestLoop(c *gc.C) {
    testGetter := new(testInstanceGetter)
    testGetter.err = true
    aggregator := newAggregator(testGetter)

    replyChan := make(chan instanceInfoReply)
    req := instanceInfoReq{
        reply: replyChan,
        instId: instance.Id("foo"),
    }
    aggregator.reqc <- req
    reply :=  <-replyChan
    c.Assert(reply.info, gc.DeepEquals, instanceInfo{})
    c.Assert(testGetter.ids, gc.DeepEquals, []instance.Id{instance.Id("foo")})
}
