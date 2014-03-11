// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
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

func (i *testInstanceGetter) Instances(ids []instance.Id) ([]instance.Instance, error) {
    var results []instance.Instance
    return results, nil
}

func (s *aggregateSuite) TestLoop(c *gc.C) {
    testGetter := new(testInstanceGetter)
    aggregator := newAggregator(testGetter)

    req := &instanceInfoReq{
        reply: make(chan instanceInfoReply),
    }
    aggregator.reqc <- req
    reply :=  <-req.reply
    c.Assert(reply.err, gc.IsNil)
}
