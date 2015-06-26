// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"errors"
	"fmt"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/voyeur"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
)

type TestIPVersion struct {
	version           string
	formatHostPort    string
	formatHost        string
	machineFormatHost string
	extraHostPort     string
	extraHost         string
	extraAddress      string
	addressType       network.AddressType
}

var (
	testIPv4 = TestIPVersion{
		version:           "IPv4",
		formatHostPort:    "0.1.2.%d:%d",
		formatHost:        "0.1.2.%d",
		machineFormatHost: "0.1.2.%d",
		extraHostPort:     "0.1.99.99:9876",
		extraHost:         "0.1.99.13",
		extraAddress:      "0.1.99.13:1234",
		addressType:       network.IPv4Address,
	}
	testIPv6 = TestIPVersion{
		version:           "IPv6",
		formatHostPort:    "[2001:DB8::%d]:%d",
		formatHost:        "[2001:DB8::%d]",
		machineFormatHost: "2001:DB8::%d",
		extraHostPort:     "[2001:DB8::99:99]:9876",
		extraHost:         "2001:DB8::99:13",
		extraAddress:      "[2001:DB8::99:13]:1234",
		addressType:       network.IPv6Address,
	}
)

// DoTestForIPv4AndIPv6 runs the passed test for IPv4 and IPv6.
func DoTestForIPv4AndIPv6(t func(ipVersion TestIPVersion)) {
	t(testIPv4)
	t(testIPv6)
}

type workerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	resetErrors()
}

// InitState initializes the fake state with a single
// replicaset member and numMachines machines
// primed to vote.
func InitState(c *gc.C, st *fakeState, numMachines int, ipVersion TestIPVersion) {
	var ids []string
	for i := 10; i < 10+numMachines; i++ {
		id := fmt.Sprint(i)
		m := st.addMachine(id, true)
		m.setInstanceId(instance.Id("id-" + id))
		m.setStateHostPort(fmt.Sprintf(ipVersion.formatHostPort, i, mongoPort))
		ids = append(ids, id)
		c.Assert(m.MongoHostPorts(), gc.HasLen, 1)

		m.setAPIHostPorts(network.NewHostPorts(
			apiPort, fmt.Sprintf(ipVersion.formatHost, i),
		))
	}
	st.machine("10").SetHasVote(true)
	st.setStateServers(ids...)
	st.session.Set(mkMembers("0v", ipVersion))
	st.session.setStatus(mkStatuses("0p", ipVersion))
	st.check = checkInvariants
}

// ExpectedAPIHostPorts returns the expected addresses
// of the machines as created by InitState.
func ExpectedAPIHostPorts(n int, ipVersion TestIPVersion) [][]network.HostPort {
	servers := make([][]network.HostPort, n)
	for i := range servers {
		servers[i] = network.NewHostPorts(
			apiPort,
			fmt.Sprintf(ipVersion.formatHost, i+10),
		)
	}
	return servers
}

func (s *workerSuite) TestSetsAndUpdatesMembers(c *gc.C) {
	DoTestForIPv4AndIPv6(func(ipVersion TestIPVersion) {
		s.PatchValue(&pollInterval, 5*time.Millisecond)

		st := NewFakeState()
		InitState(c, st, 3, ipVersion)

		memberWatcher := st.session.members.Watch()
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v", ipVersion))

		logger.Infof("starting worker")
		w := newWorker(st, noPublisher{})
		defer func() {
			c.Check(worker.Stop(w), gc.IsNil)
		}()

		// Wait for the worker to set the initial members.
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v 1 2", ipVersion))

		// Update the status of the new members
		// and check that they become voting.
		c.Logf("updating new member status")
		st.session.setStatus(mkStatuses("0p 1s 2s", ipVersion))
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v", ipVersion))

		c.Logf("adding another machine")
		// Add another machine.
		m13 := st.addMachine("13", false)
		m13.setStateHostPort(fmt.Sprintf(ipVersion.formatHostPort, 13, mongoPort))
		st.setStateServers("10", "11", "12", "13")

		c.Logf("waiting for new member to be added")
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v 3", ipVersion))

		// Remove vote from an existing member;
		// and give it to the new machine.
		// Also set the status of the new machine to
		// healthy.
		c.Logf("removing vote from machine 10 and adding it to machine 13")
		st.machine("10").setWantsVote(false)
		st.machine("13").setWantsVote(true)

		st.session.setStatus(mkStatuses("0p 1s 2s 3s", ipVersion))

		// Check that the new machine gets the vote and the
		// old machine loses it.
		c.Logf("waiting for vote switch")
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0 1v 2v 3v", ipVersion))

		c.Logf("removing old machine")
		// Remove the old machine.
		st.removeMachine("10")
		st.setStateServers("11", "12", "13")

		// Check that it's removed from the members.
		c.Logf("waiting for removal")
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("1v 2v 3v", ipVersion))
	})
}

func (s *workerSuite) TestHasVoteMaintainedEvenWhenReplicaSetFails(c *gc.C) {
	DoTestForIPv4AndIPv6(func(ipVersion TestIPVersion) {
		st := NewFakeState()

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
		InitState(c, st, 4, ipVersion)
		st.machine("10").SetHasVote(true)
		st.machine("11").SetHasVote(true)
		st.machine("12").SetHasVote(true)
		st.machine("13").SetHasVote(false)

		st.machine("10").setWantsVote(false)
		st.machine("11").setWantsVote(true)
		st.machine("12").setWantsVote(true)
		st.machine("13").setWantsVote(true)

		st.session.Set(mkMembers("0v 1v 2v 3", ipVersion))
		st.session.setStatus(mkStatuses("0H 1p 2s 3s", ipVersion))

		// Make the worker fail to set HasVote to false
		// after changing the replica set membership.
		setErrorFor("Machine.SetHasVote * false", errors.New("frood"))

		memberWatcher := st.session.members.Watch()
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v 3", ipVersion))

		w := newWorker(st, noPublisher{})
		done := make(chan error)
		go func() {
			done <- w.Wait()
		}()

		// Wait for the worker to set the initial members.
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0 1v 2v 3v", ipVersion))

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
	})
}

func (s *workerSuite) TestAddressChange(c *gc.C) {
	DoTestForIPv4AndIPv6(func(ipVersion TestIPVersion) {
		st := NewFakeState()
		InitState(c, st, 3, ipVersion)

		memberWatcher := st.session.members.Watch()
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v", ipVersion))

		logger.Infof("starting worker")
		w := newWorker(st, noPublisher{})
		defer func() {
			c.Check(worker.Stop(w), gc.IsNil)
		}()

		// Wait for the worker to set the initial members.
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v 1 2", ipVersion))

		// Change an address and wait for it to be changed in the
		// members.
		st.machine("11").setStateHostPort(ipVersion.extraHostPort)

		mustNext(c, memberWatcher)
		expectMembers := mkMembers("0v 1 2", ipVersion)
		expectMembers[1].Address = ipVersion.extraHostPort
		assertMembers(c, memberWatcher.Value(), expectMembers)
		resetErrors()
	})
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
	DoTestForIPv4AndIPv6(func(ipVersion TestIPVersion) {
		s.PatchValue(&pollInterval, 5*time.Millisecond)
		for i, testCase := range fatalErrorsTests {
			c.Logf("test %d: %s -> %s", i, testCase.errPattern, testCase.expectErr)
			resetErrors()
			st := NewFakeState()
			st.session.InstantlyReady = true
			InitState(c, st, 3, ipVersion)
			setErrorFor(testCase.errPattern, errors.New("sample"))
			w := newWorker(st, noPublisher{})
			done := make(chan error)
			go func() {
				done <- w.Wait()
			}()
			select {
			case err := <-done:
				c.Assert(err, gc.ErrorMatches, testCase.expectErr)
			case <-time.After(coretesting.LongWait):
				c.Fatalf("timed out waiting for error")
			}
		}
	})
}

func (s *workerSuite) TestSetMembersErrorIsNotFatal(c *gc.C) {
	coretesting.SkipIfI386(c, "lp:1425569")

	DoTestForIPv4AndIPv6(func(ipVersion TestIPVersion) {
		st := NewFakeState()
		InitState(c, st, 3, ipVersion)
		st.session.setStatus(mkStatuses("0p 1s 2s", ipVersion))
		var setCount voyeur.Value
		setErrorFuncFor("Session.Set", func() error {
			setCount.Set(true)
			return errors.New("sample")
		})
		s.PatchValue(&initialRetryInterval, 10*time.Microsecond)
		s.PatchValue(&maxRetryInterval, coretesting.ShortWait/4)

		w := newWorker(st, noPublisher{})
		defer func() {
			c.Check(worker.Stop(w), gc.IsNil)
		}()

		// See that the worker is retrying.
		setCountW := setCount.Watch()
		mustNext(c, setCountW)
		mustNext(c, setCountW)
		mustNext(c, setCountW)

		resetErrors()
	})
}

type PublisherFunc func(apiServers [][]network.HostPort, instanceIds []instance.Id) error

func (f PublisherFunc) publishAPIServers(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
	return f(apiServers, instanceIds)
}

func (s *workerSuite) TestStateServersArePublished(c *gc.C) {
	DoTestForIPv4AndIPv6(func(ipVersion TestIPVersion) {
		publishCh := make(chan [][]network.HostPort)
		publish := func(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
			publishCh <- apiServers
			return nil
		}

		st := NewFakeState()
		InitState(c, st, 3, ipVersion)
		w := newWorker(st, PublisherFunc(publish))
		defer func() {
			c.Check(worker.Stop(w), gc.IsNil)
		}()
		select {
		case servers := <-publishCh:
			AssertAPIHostPorts(c, servers, ExpectedAPIHostPorts(3, ipVersion))
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for publish")
		}

		// Change one of the servers' API addresses and check that it's published.
		var newMachine10APIHostPorts []network.HostPort
		newMachine10APIHostPorts = network.NewHostPorts(apiPort, ipVersion.extraHostPort)
		st.machine("10").setAPIHostPorts(newMachine10APIHostPorts)
		select {
		case servers := <-publishCh:
			expected := ExpectedAPIHostPorts(3, ipVersion)
			expected[0] = newMachine10APIHostPorts
			AssertAPIHostPorts(c, servers, expected)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for publish")
		}
	})
}

func (s *workerSuite) TestWorkerRetriesOnPublishError(c *gc.C) {
	DoTestForIPv4AndIPv6(func(ipVersion TestIPVersion) {
		s.PatchValue(&pollInterval, coretesting.LongWait+time.Second)
		s.PatchValue(&initialRetryInterval, 5*time.Millisecond)
		s.PatchValue(&maxRetryInterval, initialRetryInterval)

		publishCh := make(chan [][]network.HostPort, 100)

		count := 0
		publish := func(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
			publishCh <- apiServers
			count++
			if count <= 3 {
				return fmt.Errorf("publish error")
			}
			return nil
		}
		st := NewFakeState()
		InitState(c, st, 3, ipVersion)

		w := newWorker(st, PublisherFunc(publish))
		defer func() {
			c.Check(worker.Stop(w), gc.IsNil)
		}()

		for i := 0; i < 4; i++ {
			select {
			case servers := <-publishCh:
				AssertAPIHostPorts(c, servers, ExpectedAPIHostPorts(3, ipVersion))
			case <-time.After(coretesting.LongWait):
				c.Fatalf("timed out waiting for publish #%d", i)
			}
		}
		select {
		case <-publishCh:
			c.Errorf("unexpected publish event")
		case <-time.After(coretesting.ShortWait):
		}
	})
}

func (s *workerSuite) TestWorkerPublishesInstanceIds(c *gc.C) {
	DoTestForIPv4AndIPv6(func(ipVersion TestIPVersion) {
		s.PatchValue(&pollInterval, coretesting.LongWait+time.Second)
		s.PatchValue(&initialRetryInterval, 5*time.Millisecond)
		s.PatchValue(&maxRetryInterval, initialRetryInterval)

		publishCh := make(chan []instance.Id, 100)

		publish := func(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
			publishCh <- instanceIds
			return nil
		}
		st := NewFakeState()
		InitState(c, st, 3, ipVersion)

		w := newWorker(st, PublisherFunc(publish))
		defer func() {
			c.Check(worker.Stop(w), gc.IsNil)
		}()

		select {
		case instanceIds := <-publishCh:
			c.Assert(instanceIds, jc.SameContents, []instance.Id{"id-10", "id-11", "id-12"})
		case <-time.After(coretesting.LongWait):
			c.Errorf("timed out waiting for publish")
		}
	})
}

// mustNext waits for w's value to be set and returns it.
func mustNext(c *gc.C, w *voyeur.Watcher) (val interface{}) {
	type voyeurResult struct {
		ok  bool
		val interface{}
	}
	done := make(chan voyeurResult)
	go func() {
		c.Logf("mustNext %p", w)
		ok := w.Next()
		val = w.Value()
		c.Logf("mustNext done %p, ok: %v, val: %#v", w, ok, val)
		done <- voyeurResult{ok, val}
	}()
	select {
	case result := <-done:
		c.Assert(result.ok, jc.IsTrue)
		return result.val
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for value to be set")
	}
	panic("unreachable")
}

type noPublisher struct{}

func (noPublisher) publishAPIServers(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
	return nil
}
