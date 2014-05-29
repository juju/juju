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
	publish := func(apiServers [][]instance.HostPort, instanceIds []instance.Id) error {
		err := statePublish.publishAPIServers(apiServers, instanceIds)
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
	assertAPIHostPorts(c, hps, expectedAPIHostPorts(3))
}

type workerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
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
		m.setInstanceId(instance.Id("id-" + id))
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
			Address: instance.NewAddress(fmt.Sprintf("0.1.2.%d", i+10), instance.NetworkUnknown),
			Port:    apiPort,
		}}
	}
	return servers
}

func addressesWithPort(port int, addrs ...string) []instance.HostPort {
	return instance.AddressesWithPort(instance.NewAddresses(addrs...), port)
}

func (s *workerSuite) TestSetsAndUpdatesMembers(c *gc.C) {
	s.PatchValue(&pollInterval, 5*time.Millisecond)

	st := newFakeState()
	initState(c, st, 3)

	memberWatcher := st.session.members.Watch()
	mustNext(c, memberWatcher)
	assertMembers(c, memberWatcher.Value(), mkMembers("0v"))

	logger.Infof("starting worker")
	w := newWorker(st, noPublisher{})
	defer func() {
		c.Check(worker.Stop(w), gc.IsNil)
	}()

	// Wait for the worker to set the initial members.
	mustNext(c, memberWatcher)
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 1 2"))

	// Update the status of the new members
	// and check that they become voting.
	c.Logf("updating new member status")
	st.session.setStatus(mkStatuses("0p 1s 2s"))
	mustNext(c, memberWatcher)
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v"))

	c.Logf("adding another machine")
	// Add another machine.
	m13 := st.addMachine("13", false)
	m13.setStateHostPort(fmt.Sprintf("0.1.2.%d:%d", 13, mongoPort))
	st.setStateServers("10", "11", "12", "13")

	c.Logf("waiting for new member to be added")
	mustNext(c, memberWatcher)
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v 3"))

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
	assertMembers(c, memberWatcher.Value(), mkMembers("0 1v 2v 3v"))

	c.Logf("removing old machine")
	// Remove the old machine.
	st.removeMachine("10")
	st.setStateServers("11", "12", "13")

	// Check that it's removed from the members.
	c.Logf("waiting for removal")
	mustNext(c, memberWatcher)
	assertMembers(c, memberWatcher.Value(), mkMembers("1v 2v 3v"))
}

func (s *workerSuite) TestHasVoteMaintainedEvenWhenReplicaSetFails(c *gc.C) {
	st := newFakeState()

	// Simulate a state where we have four state servers,
	// one has gone down, and we're replacing it:
	// 0 - hasvote true, wantsvote false, down
	// 1 - hasvote true, wantsvote true
	// 2 - hasvote true, wantsvote true
	// 3 - hasvote false, wantsvote true
	//
	// When it starts, the worker should move the vote from
	// 0 to 3. We'll arrange things so that it will succeed in
	// setting the membership but fail setting the HasVote
	// to false.
	initState(c, st, 4)
	st.machine("10").SetHasVote(true)
	st.machine("11").SetHasVote(true)
	st.machine("12").SetHasVote(true)
	st.machine("13").SetHasVote(false)

	st.machine("10").setWantsVote(false)
	st.machine("11").setWantsVote(true)
	st.machine("12").setWantsVote(true)
	st.machine("13").setWantsVote(true)

	st.session.Set(mkMembers("0v 1v 2v 3"))
	st.session.setStatus(mkStatuses("0H 1p 2s 3s"))

	// Make the worker fail to set HasVote to false
	// after changing the replica set membership.
	setErrorFor("Machine.SetHasVote * false", errors.New("frood"))

	memberWatcher := st.session.members.Watch()
	mustNext(c, memberWatcher)
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v 3"))

	w := newWorker(st, noPublisher{})
	done := make(chan error)
	go func() {
		done <- w.Wait()
	}()

	// Wait for the worker to set the initial members.
	mustNext(c, memberWatcher)
	assertMembers(c, memberWatcher.Value(), mkMembers("0 1v 2v 3v"))

	// The worker should encounter an error setting the
	// has-vote status to false and exit.
	select {
	case err := <-done:
		c.Assert(err, gc.ErrorMatches, `cannot set voting status of "[0-9]+" to false: frood`)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for worker to exit")
	}

	// Start the worker again - although the membership should
	// not change, the HasVote status should be updated correctly.
	resetErrors()
	w = newWorker(st, noPublisher{})

	// Watch all the machines for changes, so we can check
	// their has-vote status without polling.
	changed := make(chan struct{}, 1)
	for i := 10; i < 14; i++ {
		watcher := st.machine(fmt.Sprint(i)).val.Watch()
		defer watcher.Close()
		go func() {
			for watcher.Next() {
				select {
				case changed <- struct{}{}:
				default:
				}
			}
		}()
	}
	timeout := time.After(coretesting.LongWait)
loop:
	for {
		select {
		case <-changed:
			correct := true
			for i := 10; i < 14; i++ {
				hasVote := st.machine(fmt.Sprint(i)).HasVote()
				expectHasVote := i != 10
				if hasVote != expectHasVote {
					correct = false
				}
			}
			if correct {
				break loop
			}
		case <-timeout:
			c.Fatalf("timed out waiting for vote to be set")
		}
	}
}

func (s *workerSuite) TestAddressChange(c *gc.C) {
	st := newFakeState()
	initState(c, st, 3)

	memberWatcher := st.session.members.Watch()
	mustNext(c, memberWatcher)
	assertMembers(c, memberWatcher.Value(), mkMembers("0v"))

	logger.Infof("starting worker")
	w := newWorker(st, noPublisher{})
	defer func() {
		c.Check(worker.Stop(w), gc.IsNil)
	}()

	// Wait for the worker to set the initial members.
	mustNext(c, memberWatcher)
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 1 2"))

	// Change an address and wait for it to be changed in the
	// members.
	st.machine("11").setStateHostPort("0.1.99.99:9876")

	mustNext(c, memberWatcher)
	expectMembers := mkMembers("0v 1 2")
	expectMembers[1].Address = "0.1.99.99:9876"
	assertMembers(c, memberWatcher.Value(), expectMembers)
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
}, {
	errPattern: "Machine.InstanceId *",
	expectErr:  `cannot get API server info: sample`,
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
	s.PatchValue(&initialRetryInterval, 10*time.Microsecond)
	s.PatchValue(&maxRetryInterval, coretesting.ShortWait/4)

	expectedIterations := 0
	for d := initialRetryInterval; d < maxRetryInterval*2; d *= 2 {
		expectedIterations++
	}

	w := newWorker(st, noPublisher{})
	defer func() {
		c.Check(worker.Stop(w), gc.IsNil)
	}()
	isSetWatcher := isSet.Watch()

	n0 := mustNext(c, isSetWatcher).(int)
	time.Sleep(maxRetryInterval * 2)
	n1 := mustNext(c, isSetWatcher).(int)

	// The worker should have backed off exponentially...
	c.Assert(n1-n0, jc.LessThan, expectedIterations+1)
	c.Logf("actual iterations %d; expected iterations %d", n1-n0, expectedIterations)

	// ... but only up to the maximum retry interval
	n0 = mustNext(c, isSetWatcher).(int)
	time.Sleep(maxRetryInterval * 2)
	n1 = mustNext(c, isSetWatcher).(int)

	c.Assert(n1-n0, jc.LessThan, 3)
}

type publisherFunc func(apiServers [][]instance.HostPort, instanceIds []instance.Id) error

func (f publisherFunc) publishAPIServers(apiServers [][]instance.HostPort, instanceIds []instance.Id) error {
	return f(apiServers, instanceIds)
}

func (s *workerSuite) TestStateServersArePublished(c *gc.C) {
	publishCh := make(chan [][]instance.HostPort)
	publish := func(apiServers [][]instance.HostPort, instanceIds []instance.Id) error {
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
		assertAPIHostPorts(c, servers, expectedAPIHostPorts(3))
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
		assertAPIHostPorts(c, servers, expected)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for publish")
	}
}

func (s *workerSuite) TestWorkerRetriesOnPublishError(c *gc.C) {
	s.PatchValue(&pollInterval, coretesting.LongWait+time.Second)
	s.PatchValue(&initialRetryInterval, 5*time.Millisecond)
	s.PatchValue(&maxRetryInterval, initialRetryInterval)

	publishCh := make(chan [][]instance.HostPort, 100)

	count := 0
	publish := func(apiServers [][]instance.HostPort, instanceIds []instance.Id) error {
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
			assertAPIHostPorts(c, servers, expectedAPIHostPorts(3))
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

func (s *workerSuite) TestWorkerPublishesInstanceIds(c *gc.C) {
	s.PatchValue(&pollInterval, coretesting.LongWait+time.Second)
	s.PatchValue(&initialRetryInterval, 5*time.Millisecond)
	s.PatchValue(&maxRetryInterval, initialRetryInterval)

	publishCh := make(chan []instance.Id, 100)

	publish := func(apiServers [][]instance.HostPort, instanceIds []instance.Id) error {
		publishCh <- instanceIds
		return nil
	}
	st := newFakeState()
	initState(c, st, 3)

	w := newWorker(st, publisherFunc(publish))
	defer func() {
		c.Check(worker.Stop(w), gc.IsNil)
	}()

	select {
	case instanceIds := <-publishCh:
		c.Assert(instanceIds, jc.SameContents, []instance.Id{"id-10", "id-11", "id-12"})
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for publish")
	}
}

// mustNext waits for w's value to be set and returns it.
func mustNext(c *gc.C, w *voyeur.Watcher) (val interface{}) {
	done := make(chan bool)
	go func() {
		c.Logf("mustNext %p", w)
		ok := w.Next()
		val = w.Value()
		c.Logf("mustNext done %p, ok %v", w, ok)
		done <- ok
	}()
	select {
	case ok := <-done:
		c.Assert(ok, jc.IsTrue)
		return
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for value to be set")
	}
	panic("unreachable")
}

type noPublisher struct{}

func (noPublisher) publishAPIServers(apiServers [][]instance.HostPort, instanceIds []instance.Id) error {
	return nil
}
