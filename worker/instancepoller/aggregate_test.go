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
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type aggregateSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&aggregateSuite{})

type testInstance struct {
	instance.Instance
	id        instance.Id
	addresses []network.Address
	status    string
	err       error
}

var _ instance.Instance = (*testInstance)(nil)

func (t *testInstance) Id() instance.Id {
	return t.id
}

func (t *testInstance) Addresses() ([]network.Address, error) {
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
	results map[instance.Id]instance.Instance
	err     error
	counter int32
}

func (tig *testInstanceGetter) Instances(ids []instance.Id) (result []instance.Instance, err error) {
	tig.ids = ids
	atomic.AddInt32(&tig.counter, 1)
	results := make([]instance.Instance, len(ids))
	for i, id := range ids {
		// We don't check 'ok' here, because we want the Instance{nil}
		// response for those
		results[i] = tig.results[id]
	}
	return results, tig.err
}

func (tig *testInstanceGetter) newTestInstance(id instance.Id, status string, addresses []string) *testInstance {
	if tig.results == nil {
		tig.results = make(map[instance.Id]instance.Instance)
	}
	thisInstance := &testInstance{
		id:        id,
		status:    status,
		addresses: network.NewAddresses(addresses...),
	}
	tig.results[thisInstance.Id()] = thisInstance
	return thisInstance
}

func (s *aggregateSuite) TestSingleRequest(c *gc.C) {
	testGetter := new(testInstanceGetter)
	instance1 := testGetter.newTestInstance("foo", "foobar", []string{"127.0.0.1", "192.168.1.1"})
	aggregator := newAggregator(testGetter)

	info, err := aggregator.instanceInfo("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.DeepEquals, instanceInfo{
		status:    "foobar",
		addresses: instance1.addresses,
	})
	c.Assert(testGetter.ids, gc.DeepEquals, []instance.Id{"foo"})
}

func (s *aggregateSuite) TestMultipleResponseHandling(c *gc.C) {
	s.PatchValue(&gatherTime, 30*time.Millisecond)
	testGetter := new(testInstanceGetter)

	testGetter.newTestInstance("foo", "foobar", []string{"127.0.0.1", "192.168.1.1"})
	aggregator := newAggregator(testGetter)

	replyChan := make(chan instanceInfoReply)
	req := instanceInfoReq{
		reply:  replyChan,
		instId: instance.Id("foo"),
	}
	aggregator.reqc <- req
	reply := <-replyChan
	c.Assert(reply.err, gc.IsNil)

	testGetter.newTestInstance("foo2", "not foobar", []string{"192.168.1.2"})
	testGetter.newTestInstance("foo3", "ok-ish", []string{"192.168.1.3"})

	var wg sync.WaitGroup
	checkInfo := func(id instance.Id, expectStatus string) {
		info, err := aggregator.instanceInfo(id)
		c.Check(err, jc.ErrorIsNil)
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
	totalCount int
	batchSize  int
	started    int
}

func (g *batchingInstanceGetter) Instances(ids []instance.Id) ([]instance.Instance, error) {
	insts, err := g.testInstanceGetter.Instances(ids)
	g.startRequests()
	return insts, err
}

func (g *batchingInstanceGetter) startRequests() {
	n := g.totalCount - g.started
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
	// We only need to inform the system about 1 instance, because all the
	// requests are for the same instance.
	testGetter.newTestInstance("foo", "foobar", []string{"127.0.0.1", "192.168.1.1"})
	testGetter.totalCount = 100
	testGetter.batchSize = 10
	testGetter.wg.Add(testGetter.totalCount)
	// startRequest will trigger one request, which ends up calling
	// Instances, which will turn around and trigger batchSize requests,
	// which should get aggregated into a single call to Instances, which
	// then should trigger another round of batchSize requests.
	testGetter.startRequest()
	testGetter.wg.Wait()
	c.Assert(testGetter.counter, gc.Equals, int32(testGetter.totalCount/testGetter.batchSize)+1)
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

	aggregator := newAggregator(testGetter)
	_, err := aggregator.instanceInfo("foo")

	c.Assert(err, gc.ErrorMatches, "instance foo not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *aggregateSuite) TestAddressesError(c *gc.C) {
	testGetter := new(testInstanceGetter)
	instance1 := testGetter.newTestInstance("foo", "foobar", []string{"127.0.0.1", "192.168.1.1"})
	ourError := fmt.Errorf("gotcha")
	instance1.err = ourError

	aggregator := newAggregator(testGetter)
	_, err := aggregator.instanceInfo("foo")
	c.Assert(err, gc.Equals, ourError)
}

func (s *aggregateSuite) TestKillAndWait(c *gc.C) {
	testGetter := new(testInstanceGetter)
	aggregator := newAggregator(testGetter)
	aggregator.Kill()
	err := aggregator.Wait()
	c.Assert(err, jc.ErrorIsNil)
}
