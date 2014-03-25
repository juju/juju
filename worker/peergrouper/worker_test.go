// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"errors"
	"fmt"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils/voyeur"
	"launchpad.net/juju-core/worker"
)

type workerJujuConnSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&workerJujuConnSuite{})

func (s *workerJujuConnSuite) TestStartStop(c *gc.C) {
	w, err := New(s.State)
	c.Assert(err, gc.IsNil)
	err = worker.Stop(w)
	c.Assert(err, gc.IsNil)
}

func (s *workerJujuConnSuite) TestPublisherSetsAPIHostPorts(c *gc.C) {
	st := newFakeState()
	initState(c, st, 3)

	watcher := s.State.WatchAPIHostPorts()
	cwatch := statetesting.NewNotifyWatcherC(c, s.State, watcher)
	cwatch.AssertOneChange()

	statePublish := newPublisher(s.State)

	// Wrap the publisher so that we can call StartSync immediately
	// after the publishAPIServers method is called.
	publish := func(apiServers [][]instance.HostPort) error {
		err := statePublish.publishAPIServers(apiServers)
		s.State.StartSync()
		return err
	}

	w := newWorker(st, publisherFunc(publish))
	defer func() {
		c.Check(worker.Stop(w), gc.IsNil)
	}()

	cwatch.AssertOneChange()
	hps, err := s.State.APIHostPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(hps, jc.DeepEquals, expectedAPIHostPorts(3))
}

type workerSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	resetErrors()
}

// initState initializes the fake state with a single
// replicaset member and numMachines machines
// primed to vote.
func initState(c *gc.C, st *fakeState, numMachines int) {
	var ids []string
	for i := 10; i < 10+numMachines; i++ {
		id := fmt.Sprint(i)
		m := st.addMachine(id, true)
		m.setStateHostPort(fmt.Sprintf("0.1.2.%d:%d", i, mongoPort))
		ids = append(ids, id)
		c.Assert(m.MongoHostPorts(), gc.HasLen, 1)

		m.setAPIHostPorts(addressesWithPort(apiPort, fmt.Sprintf("0.1.2.%d", i)))
	}
	st.machine("10").SetHasVote(true)
	st.setStateServers(ids...)
	st.session.Set(mkMembers("0v"))
	st.session.setStatus(mkStatuses("0p"))
	st.check = checkInvariants
}

// expectedAPIHostPorts returns the expected addresses
// of the machines as created by initState.
func expectedAPIHostPorts(n int) [][]instance.HostPort {
	servers := make([][]instance.HostPort, n)
	for i := range servers {
		servers[i] = []instance.HostPort{{
			Address: instance.Address{
				Value:        fmt.Sprintf("0.1.2.%d", i+10),
				NetworkScope: instance.NetworkUnknown,
				Type:         instance.Ipv4Address,
			},
			Port: apiPort,
		}}
	}
	return servers
}

func addressesWithPort(port int, addrs ...string) []instance.HostPort {
	return instance.AddressesWithPort(instance.NewAddresses(addrs), port)
}

func (s *workerSuite) TestSetsAndUpdatesMembers(c *gc.C) {
	s.PatchValue(&pollInterval, 5*time.Millisecond)

	st := newFakeState()
	initState(c, st, 3)

	memberWatcher := st.session.members.Watch()
	mustNext(c, memberWatcher)
	c.Assert(memberWatcher.Value(), jc.DeepEquals, mkMembers("0v"))

	logger.Infof("starting worker")
	w := newWorker(st, noPublisher{})
	defer func() {
		c.Check(worker.Stop(w), gc.IsNil)
	}()

	// Wait for the worker to set the initial members.
	mustNext(c, memberWatcher)
	c.Assert(memberWatcher.Value(), jc.DeepEquals, mkMembers("0v 1 2"))

	// Update the status of the new members
	// and check that they become voting.
	c.Logf("updating new member status")
	st.session.setStatus(mkStatuses("0p 1s 2s"))
	mustNext(c, memberWatcher)
	c.Assert(memberWatcher.Value(), jc.DeepEquals, mkMembers("0v 1v 2v"))

	c.Logf("adding another machine")
	// Add another machine.
	m13 := st.addMachine("13", false)
	m13.setStateHostPort(fmt.Sprintf("0.1.2.%d:%d", 13, mongoPort))
	st.setStateServers("10", "11", "12", "13")

	c.Logf("waiting for new member to be added")
	mustNext(c, memberWatcher)
	c.Assert(memberWatcher.Value(), jc.DeepEquals, mkMembers("0v 1v 2v 3"))

	// Remove vote from an existing member;
	// and give it to the new machine.
	// Also set the status of the new machine to
	// healthy.
	c.Logf("removing vote from machine 10 and adding it to machine 13")
	st.machine("10").setWantsVote(false)
	st.machine("13").setWantsVote(true)

	st.session.setStatus(mkStatuses("0p 1s 2s 3s"))

	// Check that the new machine gets the vote and the
	// old machine loses it.
	c.Logf("waiting for vote switch")
	mustNext(c, memberWatcher)
	c.Assert(memberWatcher.Value(), jc.DeepEquals, mkMembers("0 1v 2v 3v"))

	c.Logf("removing old machine")
	// Remove the old machine.
	st.removeMachine("10")
	st.setStateServers("11", "12", "13")

	// Check that it's removed from the members.
	c.Logf("waiting for removal")
	mustNext(c, memberWatcher)
	c.Assert(memberWatcher.Value(), jc.DeepEquals, mkMembers("1v 2v 3v"))
}

func (s *workerSuite) TestAddressChange(c *gc.C) {
	st := newFakeState()
	initState(c, st, 3)

	memberWatcher := st.session.members.Watch()
	mustNext(c, memberWatcher)
	c.Assert(memberWatcher.Value(), jc.DeepEquals, mkMembers("0v"))

	logger.Infof("starting worker")
	w := newWorker(st, noPublisher{})
	defer func() {
		c.Check(worker.Stop(w), gc.IsNil)
	}()

	// Wait for the worker to set the initial members.
	mustNext(c, memberWatcher)
	c.Assert(memberWatcher.Value(), jc.DeepEquals, mkMembers("0v 1 2"))

	// Change an address and wait for it to be changed in the
	// members.
	st.machine("11").setStateHostPort("0.1.99.99:9876")

	mustNext(c, memberWatcher)
	expectMembers := mkMembers("0v 1 2")
	expectMembers[1].Address = "0.1.99.99:9876"
	c.Assert(memberWatcher.Value(), jc.DeepEquals, expectMembers)
}

var fatalErrorsTests = []struct {
	errPattern string
	err        error
	expectErr  string
}{{
	errPattern: "State.StateServerInfo",
	expectErr:  "cannot get state server info: sample",
}, {
	errPattern: "Machine.SetHasVote 11 true",
	expectErr:  `cannot set voting status of "11" to true: sample`,
}, {
	errPattern: "Session.CurrentStatus",
	expectErr:  "cannot get replica set status: sample",
}, {
	errPattern: "Session.CurrentMembers",
	expectErr:  "cannot get replica set members: sample",
}, {
	errPattern: "State.Machine *",
	expectErr:  `cannot get machine "10": sample`,
}}

func (s *workerSuite) TestFatalErrors(c *gc.C) {
	s.PatchValue(&pollInterval, 5*time.Millisecond)
	for i, test := range fatalErrorsTests {
		c.Logf("test %d: %s -> %s", i, test.errPattern, test.expectErr)
		resetErrors()
		st := newFakeState()
		st.session.InstantlyReady = true
		initState(c, st, 3)
		setErrorFor(test.errPattern, errors.New("sample"))
		w := newWorker(st, noPublisher{})
		done := make(chan error)
		go func() {
			done <- w.Wait()
		}()
		select {
		case err := <-done:
			c.Assert(err, gc.ErrorMatches, test.expectErr)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for error")
		}
	}
}

func (s *workerSuite) TestSetMembersErrorIsNotFatal(c *gc.C) {
	st := newFakeState()
	initState(c, st, 3)
	st.session.setStatus(mkStatuses("0p 1s 2s"))
	var isSet voyeur.Value
	count := 0
	setErrorFuncFor("Session.Set", func() error {
		isSet.Set(count)
		count++
		return errors.New("sample")
	})
	s.PatchValue(&retryInterval, 5*time.Millisecond)
	w := newWorker(st, noPublisher{})
	defer func() {
		c.Check(worker.Stop(w), gc.IsNil)
	}()
	isSetWatcher := isSet.Watch()
	n0, _ := mustNext(c, isSetWatcher)
	// The worker should not retry more than every
	// retryInterval.
	time.Sleep(retryInterval * 10)
	n1, _ := mustNext(c, isSetWatcher)
	c.Assert(n0.(int)-n0.(int), jc.LessThan, 11)
	c.Assert(n1, jc.GreaterThan, n0)
}

type publisherFunc func(apiServers [][]instance.HostPort) error

func (f publisherFunc) publishAPIServers(apiServers [][]instance.HostPort) error {
	return f(apiServers)
}

func (s *workerSuite) TestStateServersArePublished(c *gc.C) {
	publishCh := make(chan [][]instance.HostPort)
	publish := func(apiServers [][]instance.HostPort) error {
		publishCh <- apiServers
		return nil
	}

	st := newFakeState()
	initState(c, st, 3)
	w := newWorker(st, publisherFunc(publish))
	defer func() {
		c.Check(worker.Stop(w), gc.IsNil)
	}()
	select {
	case servers := <-publishCh:
		c.Assert(servers, gc.DeepEquals, expectedAPIHostPorts(3))
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for publish")
	}

	// Change one of the servers' API addresses and check that it's published.

	newMachine10APIHostPorts := addressesWithPort(apiPort, "0.2.8.124")
	st.machine("10").setAPIHostPorts(newMachine10APIHostPorts)
	select {
	case servers := <-publishCh:
		expected := expectedAPIHostPorts(3)
		expected[0] = newMachine10APIHostPorts
		c.Assert(servers, jc.DeepEquals, expected)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for publish")
	}
}

func (s *workerSuite) TestWorkerRetriesOnPublishError(c *gc.C) {
	s.PatchValue(&pollInterval, coretesting.LongWait+time.Second)
	s.PatchValue(&retryInterval, 5*time.Millisecond)

	publishCh := make(chan [][]instance.HostPort, 100)

	count := 0
	publish := func(apiServers [][]instance.HostPort) error {
		publishCh <- apiServers
		count++
		if count <= 3 {
			return fmt.Errorf("publish error")
		}
		return nil
	}
	st := newFakeState()
	initState(c, st, 3)

	w := newWorker(st, publisherFunc(publish))
	defer func() {
		c.Check(worker.Stop(w), gc.IsNil)
	}()

	for i := 0; i < 4; i++ {
		select {
		case servers := <-publishCh:
			c.Assert(servers, jc.DeepEquals, expectedAPIHostPorts(3))
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for publish #%d", i)
		}
	}
	select {
	case <-publishCh:
		c.Errorf("unexpected publish event")
	case <-time.After(coretesting.ShortWait):
	}
}

func mustNext(c *gc.C, w *voyeur.Watcher) (val interface{}, ok bool) {
	done := make(chan struct{})
	go func() {
		c.Logf("mustNext %p", w)
		ok = w.Next()
		val = w.Value()
		c.Logf("mustNext done %p, ok %v", w, ok)
		done <- struct{}{}
	}()
	select {
	case <-done:
		return
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for value to be set")
	}
	panic("unreachable")
}

type noPublisher struct{}

func (noPublisher) publishAPIServers(apiServers [][]instance.HostPort) error {
	return nil
}
