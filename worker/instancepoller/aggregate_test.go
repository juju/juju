// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing"
)

type aggregateSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&aggregateSuite{})

type testInstance struct {
	instance.Instance
	addresses []instance.Address
	status    string
	err       error
}

var _ instance.Instance = (*testInstance)(nil)

func (t *testInstance) Addresses() ([]instance.Address, error) {
	if t.err != nil {
		return nil, t.err
	}
	return t.addresses, nil
}

func (t *testInstance) Status() string {
	return t.status
}

type testInstanceGetter struct {
	// ids is set when the Instances method is called.
	ids     []instance.Id
	results []instance.Instance
	err     error
	counter int32
}

func (i *testInstanceGetter) Instances(ids []instance.Id) (result []instance.Instance, err error) {
	i.ids = ids
	atomic.AddInt32(&i.counter, 1)
	return i.results, i.err
}

func newTestInstance(status string, addresses []string) *testInstance {
	thisInstance := testInstance{status: status}
	thisInstance.addresses = instance.NewAddresses(addresses...)
	return &thisInstance
}

func (s *aggregateSuite) TestSingleRequest(c *gc.C) {
	testGetter := new(testInstanceGetter)
	instance1 := newTestInstance("foobar", []string{"127.0.0.1", "192.168.1.1"})
	testGetter.results = []instance.Instance{instance1}
	aggregator := newAggregator(testGetter)

	info, err := aggregator.instanceInfo("foo")
	c.Assert(err, gc.IsNil)
	c.Assert(info, gc.DeepEquals, instanceInfo{
		status:    "foobar",
		addresses: instance1.addresses,
	})
	c.Assert(testGetter.ids, gc.DeepEquals, []instance.Id{"foo"})
}

func (s *aggregateSuite) TestMultipleResponseHandling(c *gc.C) {
	s.PatchValue(&gatherTime, 30*time.Millisecond)
	testGetter := new(testInstanceGetter)

	instance1 := newTestInstance("foobar", []string{"127.0.0.1", "192.168.1.1"})
	testGetter.results = []instance.Instance{instance1}
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
	testGetter.results = []instance.Instance{instance2, instance3}

	var wg sync.WaitGroup
	checkInfo := func(id instance.Id, expectStatus string) {
		info, err := aggregator.instanceInfo(id)
		c.Check(err, gc.IsNil)
		c.Check(info.status, gc.Equals, expectStatus)
		wg.Done()
	}

	wg.Add(2)
	go checkInfo("foo2", "not foobar")
	go checkInfo("foo3", "ok-ish")
	wg.Wait()

	c.Assert(len(testGetter.ids), gc.DeepEquals, 2)
}

type batchingInstanceGetter struct {
	testInstanceGetter
	wg         sync.WaitGroup
	aggregator *aggregator
	batchSize  int
	started    int
}

func (g *batchingInstanceGetter) Instances(ids []instance.Id) ([]instance.Instance, error) {
	insts, err := g.testInstanceGetter.Instances(ids)
	g.startRequests()
	return insts, err
}

func (g *batchingInstanceGetter) startRequests() {
	n := len(g.results) - g.started
	if n > g.batchSize {
		n = g.batchSize
	}
	for i := 0; i < n; i++ {
		g.startRequest()
	}
}

func (g *batchingInstanceGetter) startRequest() {
	g.started++
	go func() {
		_, err := g.aggregator.instanceInfo("foo")
		if err != nil {
			panic(err)
		}
		g.wg.Done()
	}()
}

func (s *aggregateSuite) TestBatching(c *gc.C) {
	s.PatchValue(&gatherTime, 10*time.Millisecond)
	var testGetter batchingInstanceGetter
	testGetter.aggregator = newAggregator(&testGetter)
	testGetter.results = make([]instance.Instance, 100)
	for i := range testGetter.results {
		testGetter.results[i] = newTestInstance("foobar", []string{"127.0.0.1", "192.168.1.1"})
	}
	testGetter.batchSize = 10
	testGetter.wg.Add(len(testGetter.results))
	testGetter.startRequest()
	testGetter.wg.Wait()
	c.Assert(testGetter.counter, gc.Equals, int32(len(testGetter.results)/testGetter.batchSize)+1)
}

func (s *aggregateSuite) TestError(c *gc.C) {
	testGetter := new(testInstanceGetter)
	ourError := fmt.Errorf("Some error")
	testGetter.err = ourError

	aggregator := newAggregator(testGetter)

	_, err := aggregator.instanceInfo("foo")
	c.Assert(err, gc.Equals, ourError)
}

func (s *aggregateSuite) TestPartialErrResponse(c *gc.C) {
	testGetter := new(testInstanceGetter)
	testGetter.err = environs.ErrPartialInstances
	testGetter.results = []instance.Instance{nil}

	aggregator := newAggregator(testGetter)
	_, err := aggregator.instanceInfo("foo")

	c.Assert(err, gc.ErrorMatches, "instance foo not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *aggregateSuite) TestAddressesError(c *gc.C) {
	testGetter := new(testInstanceGetter)
	instance1 := newTestInstance("foobar", []string{"127.0.0.1", "192.168.1.1"})
	ourError := fmt.Errorf("gotcha")
	instance1.err = ourError
	testGetter.results = []instance.Instance{instance1}

	aggregator := newAggregator(testGetter)
	_, err := aggregator.instanceInfo("foo")
	c.Assert(err, gc.Equals, ourError)
}

func (s *aggregateSuite) TestKillAndWait(c *gc.C) {
	testGetter := new(testInstanceGetter)
	aggregator := newAggregator(testGetter)
	aggregator.Kill()
	err := aggregator.Wait()
	c.Assert(err, gc.IsNil)
}
