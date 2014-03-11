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

type testInstanceGetter struct {
    ids []instance.Id
    results []instanceInfoReply
}

type testInstance struct {
    instance.Instance
}

func (i *testInstanceGetter) Instances(ids []instance.Id) ([]instance.Instance, error) {
//    var results []instance.Instance
//    results[0] = testInstance{}
    return nil, fmt.Errorf("Some error")
}

func (s *aggregateSuite) TestLoop(c *gc.C) {
    testGetter := new(testInstanceGetter)
    aggregator := newAggregator(testGetter)

    replyChan := make(chan instanceInfoReply)
    req := instanceInfoReq{
        reply: replyChan,
        instId: instance.Id("foo"),
    }
    aggregator.reqc <- req
    reply :=  <-replyChan
    c.Assert(reply.err, gc.IsNil)
}
