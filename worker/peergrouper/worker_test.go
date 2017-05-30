// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"errors"
	"fmt"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/replicaset"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/voyeur"
	gc "gopkg.in/check.v1"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/workertest"
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

type workerSuite struct {
	coretesting.BaseSuite
	clock *testing.Clock
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.clock = testing.NewClock(time.Now())
}

type testSuite interface {
	SetUpTest(c *gc.C)
	TearDownTest(c *gc.C)
}

// DoTestForIPv4AndIPv6 runs the passed test for IPv4 and IPv6.
//
// TODO(axw) the type of address has little to do with the
// behaviour of this worker. so we should not need to run the
// tests for each address type. We can introduce a limited
// number (probably one) of feature tests to check that we
// handle both address types as expected.
func DoTestForIPv4AndIPv6(c *gc.C, s testSuite, t func(ipVersion TestIPVersion)) {
	t(testIPv4)
	s.TearDownTest(c)
	s.SetUpTest(c)
	t(testIPv6)
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
	st.setControllers(ids...)
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

func (s *workerSuite) waitAdvance(c *gc.C, d time.Duration) {
	select {
	case <-s.clock.Alarms():
	case <-time.After(coretesting.ShortWait):
		c.Fatal("worker didn't call clock.After")
	}
	s.clock.Advance(d)
}

func (s *workerSuite) TestSetsAndUpdatesMembers(c *gc.C) {
	logger.SetLogLevel(loggo.TRACE)
	loggo.GetLogger("juju.mongo").SetLogLevel(loggo.INFO)
	loggo.GetLogger("juju.network").SetLogLevel(loggo.INFO)
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		c.Logf("\n\nTestSetsAndUpdatesMembers: %s", ipVersion.version)
		st := NewFakeState()
		InitState(c, st, 3, ipVersion)
		memberWatcher := st.session.members.Watch()
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v", ipVersion))

		logger.Infof("starting worker")
		s.newNoPublishWorker(c, st)
		// Due to the inherit complexity of the multiple goroutines running
		// and listen do different watchers, there is no way to manually
		// advance the testing clock in a controlled manner as the clock.After
		// calls can be replaced in response to other watcher events. Hence
		// using the standard testing clock wait / advance method does not
		// work. So we use the real clock to advance the test clock for this
		// test.
		done := make(chan struct{})
		defer close(done)
		go func() {
			for {
				select {
				case <-time.After(5 * time.Millisecond):
					s.clock.Advance(pollInterval)
				case <-done:
					return
				}
			}
		}()

		// Wait for the worker to set the initial members.
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v 1 2", ipVersion))

		// Update the status of the new members
		// and check that they become voting.
		c.Logf("\nupdating new member status")
		st.session.setStatus(mkStatuses("0p 1s 2s", ipVersion))
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v", ipVersion))

		c.Logf("\nadding another machine")
		m13 := st.addMachine("13", false)
		m13.setStateHostPort(fmt.Sprintf(ipVersion.formatHostPort, 13, mongoPort))
		st.setControllers("10", "11", "12", "13")

		c.Logf("\nwaiting for new member to be added")
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v 3", ipVersion))

		// Remove vote from an existing member; and give it to the new
		// machine. Also set the status of the new machine to healthy.
		c.Logf("\nremoving vote from machine 10 and adding it to machine 13")
		st.machine("10").setWantsVote(false)
		st.machine("13").setWantsVote(true)

		st.session.setStatus(mkStatuses("0p 1s 2s 3s", ipVersion))

		// Check that the new machine gets the vote and the
		// old machine loses it.
		c.Logf("\nwaiting for vote switch")
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0 1v 2v 3v", ipVersion))

		c.Logf("\nremoving old machine")
		// Remove the old machine.
		st.removeMachine("10")
		st.setControllers("11", "12", "13")

		// Check that it's removed from the members.
		c.Logf("\nwaiting for removal")
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("1v 2v 3v", ipVersion))
	})
}

func (s *workerSuite) TestHasVoteMaintainedEvenWhenReplicaSetFails(c *gc.C) {
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		st := NewFakeState()

		// Simulate a state where we have four controllers,
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
		st.errors.setErrorFor("Machine.SetHasVote * false", errors.New("frood"))

		memberWatcher := st.session.members.Watch()
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v 3", ipVersion))

		w, err := newNoPublishWorker(st, s.clock, &noOpHub{})
		c.Assert(err, jc.ErrorIsNil)
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
			c.Assert(err, gc.ErrorMatches, `cannot set HasVote removed: cannot set voting status of "[0-9]+" to false: frood`)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for worker to exit")
		}

		// Start the worker again - although the membership should
		// not change, the HasVote status should be updated correctly.
		st.errors.resetErrors()
		w = s.newNoPublishWorker(c, st)

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
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		st := NewFakeState()
		InitState(c, st, 3, ipVersion)

		memberWatcher := st.session.members.Watch()
		mustNext(c, memberWatcher)
		assertMembers(c, memberWatcher.Value(), mkMembers("0v", ipVersion))

		logger.Infof("starting worker")
		s.newNoPublishWorker(c, st)

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
	})
}

var fatalErrorsTests = []struct {
	errPattern   string
	err          error
	expectErr    string
	advanceCount int
}{{
	errPattern: "State.ControllerInfo",
	expectErr:  "cannot get controller info: sample",
}, {
	errPattern:   "Machine.SetHasVote 11 true",
	expectErr:    `cannot set HasVote added: cannot set voting status of "11" to true: sample`,
	advanceCount: 2,
}, {
	errPattern: "Session.CurrentStatus",
	expectErr:  "cannot get peergrouper info: cannot get replica set status: sample",
}, {
	errPattern: "Session.CurrentMembers",
	expectErr:  "cannot get peergrouper info: cannot get replica set members: sample",
}, {
	errPattern: "State.Machine *",
	expectErr:  `cannot get machine "10": sample`,
}, {
	errPattern: "Machine.InstanceId *",
	expectErr:  `cannot get API server info: sample`,
}}

func (s *workerSuite) TestFatalErrors(c *gc.C) {
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		s.PatchValue(&pollInterval, 5*time.Millisecond)
		for i, testCase := range fatalErrorsTests {
			c.Logf("\n(%s) test %d: %s -> %s", ipVersion.version, i, testCase.errPattern, testCase.expectErr)
			st := NewFakeState()
			st.session.InstantlyReady = true
			InitState(c, st, 3, ipVersion)
			st.errors.setErrorFor(testCase.errPattern, errors.New("sample"))
			s.clock = testing.NewClock(time.Now())
			w, err := newNoPublishWorker(st, s.clock, &noOpHub{})
			c.Assert(err, jc.ErrorIsNil)
			for j := 0; j < testCase.advanceCount; j++ {
				s.waitAdvance(c, pollInterval)
			}
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

	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		st := NewFakeState()
		InitState(c, st, 3, ipVersion)
		st.session.setStatus(mkStatuses("0p 1s 2s", ipVersion))
		called := make(chan error)
		setErr := errors.New("sample")
		st.errors.setErrorFuncFor("Session.Set", func() error {
			called <- setErr
			return setErr
		})

		s.newNoPublishWorker(c, st)

		// Just watch three error retries
		retryInterval := initialRetryInterval
		for i := 0; i < 3; i++ {
			s.waitAdvance(c, retryInterval)
			retryInterval = scaleRetry(retryInterval)
			select {
			case err := <-called:
				c.Check(err, gc.Equals, setErr)
			case <-time.After(coretesting.LongWait):
				c.Fatalf("timed out waiting for loop #%d", i)
			}
		}
	})
}

type PublisherFunc func(apiServers [][]network.HostPort, instanceIds []instance.Id) error

func (f PublisherFunc) publishAPIServers(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
	return f(apiServers, instanceIds)
}

func (s *workerSuite) TestControllersArePublished(c *gc.C) {
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		publishCh := make(chan [][]network.HostPort)
		publish := func(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
			publishCh <- apiServers
			return nil
		}

		st := NewFakeState()
		InitState(c, st, 3, ipVersion)
		s.newPublishWorker(c, st, PublisherFunc(publish))

		select {
		case servers := <-publishCh:
			AssertAPIHostPorts(c, servers, ExpectedAPIHostPorts(3, ipVersion))
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for publish")
		}

		// Change one of the servers' API addresses and check that it's published.
		var newMachine10APIHostPorts []network.HostPort
		newMachine10APIHostPorts = network.NewHostPorts(apiPort, ipVersion.extraHost)
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

func (s *workerSuite) TestControllersArePublishedOverHub(c *gc.C) {
	st := NewFakeState()
	InitState(c, st, 3, testIPv4)
	hub := pubsub.NewStructuredHub(nil)
	event := make(chan apiserver.Details)
	_, err := hub.Subscribe(apiserver.DetailsTopic, func(topic string, data apiserver.Details, err error) {
		c.Check(err, jc.ErrorIsNil)
		event <- data
	})
	c.Assert(err, jc.ErrorIsNil)

	w, err := newNoPublishWorker(st, s.clock, hub)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	expected := apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"10": apiserver.APIServer{ID: "10", Addresses: []string{"0.1.2.10:5678"}},
			"11": apiserver.APIServer{ID: "11", Addresses: []string{"0.1.2.11:5678"}},
			"12": apiserver.APIServer{ID: "12", Addresses: []string{"0.1.2.12:5678"}},
		},
		LocalOnly: true,
	}

	select {
	case obtained := <-event:
		c.Assert(obtained, jc.DeepEquals, expected)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for event")
	}

}

func hostPortInSpace(address, spaceName string) network.HostPort {
	netAddress := network.Address{
		Value:     address,
		Type:      network.IPv4Address,
		Scope:     network.ScopeUnknown,
		SpaceName: network.SpaceName(spaceName),
	}
	return network.HostPort{
		Address: netAddress,
		Port:    4711,
	}
}

func mongoSpaceTestCommonSetup(c *gc.C, ipVersion TestIPVersion, noSpaces bool) (*fakeState, []string, []network.HostPort) {
	st := NewFakeState()
	InitState(c, st, 3, ipVersion)
	var hostPorts []network.HostPort

	if noSpaces {
		hostPorts = []network.HostPort{
			hostPortInSpace(fmt.Sprintf(ipVersion.machineFormatHost, 1), ""),
			hostPortInSpace(fmt.Sprintf(ipVersion.machineFormatHost, 2), ""),
			hostPortInSpace(fmt.Sprintf(ipVersion.machineFormatHost, 3), ""),
		}
	} else {
		hostPorts = []network.HostPort{
			hostPortInSpace(fmt.Sprintf(ipVersion.machineFormatHost, 1), "one"),
			hostPortInSpace(fmt.Sprintf(ipVersion.machineFormatHost, 2), "two"),
			hostPortInSpace(fmt.Sprintf(ipVersion.machineFormatHost, 3), "three"),
		}
	}

	machines := []string{"10", "11", "12"}
	for _, machine := range machines {
		st.machine(machine).SetHasVote(true)
		st.machine(machine).setWantsVote(true)
	}

	st.session.Set(mkMembers("0v 1v 2v", ipVersion))

	return st, machines, hostPorts
}

func startWorkerSupportingSpaces(c *gc.C, st *fakeState, ipVersion TestIPVersion) *pgWorker {
	w, err := newWorker(st, clock.WallClock, noPublisher{}, true, &noOpHub{})
	c.Assert(err, jc.ErrorIsNil)
	return w.(*pgWorker)
}

func runWorkerUntilMongoStateIs(c *gc.C, st *fakeState, w *pgWorker, mss state.MongoSpaceStates) {
	changes := st.controllers.Watch()
	changes.Next()
	for st.getMongoSpaceState() != mss {
		changes.Next()
	}
	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestMongoFindAndUseSpace(c *gc.C) {
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		st, machines, hostPorts := mongoSpaceTestCommonSetup(c, ipVersion, false)

		for i, machine := range machines {
			// machine 10 gets a host port in space one
			// machine 11 gets host ports in spaces one and two
			// machine 12 gets host ports in spaces one, two and three
			st.machine(machine).setMongoHostPorts(hostPorts[0 : i+1])
		}

		w := startWorkerSupportingSpaces(c, st, ipVersion)
		runWorkerUntilMongoStateIs(c, st, w, state.MongoSpaceValid)

		// Only space one has all three servers in it
		c.Assert(st.getMongoSpaceName(), gc.Equals, "one")

		// All machines have the same address in this test for simplicity. The
		// space three address is 0.0.0.3 giving us the host port of 0.0.0.3:4711
		members := st.session.members.Get().([]replicaset.Member)
		c.Assert(members, gc.HasLen, 3)
		for i := 0; i < 3; i++ {
			c.Assert(members[i].Address, gc.Equals, fmt.Sprintf(ipVersion.formatHostPort, 1, 4711))
		}
	})
}

func (s *workerSuite) TestMongoErrorNoCommonSpace(c *gc.C) {
	c.Skip("dimitern: test disabled as it needs refactoring")
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		st, machines, hostPorts := mongoSpaceTestCommonSetup(c, ipVersion, false)

		for i, machine := range machines {
			// machine 10 gets a host port in space one
			// machine 11 gets a host port in space two
			// machine 12 gets a host port in space three
			st.machine(machine).setMongoHostPorts(hostPorts[i : i+1])
		}

		w := startWorkerSupportingSpaces(c, st, ipVersion)
		done := make(chan error)
		go func() {
			done <- w.Wait()
		}()
		select {
		case err := <-done:
			c.Assert(err, gc.ErrorMatches, ".*couldn't find a space containing all peer group machines")
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for worker to exit")
		}

		// Each machine is in a unique space, so the Mongo space should be empty
		c.Assert(st.getMongoSpaceName(), gc.Equals, "")
		c.Assert(st.getMongoSpaceState(), gc.Equals, state.MongoSpaceInvalid)
	})
}

func (s *workerSuite) TestMongoNoSpaces(c *gc.C) {
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		st, machines, hostPorts := mongoSpaceTestCommonSetup(c, ipVersion, true)

		for i, machine := range machines {
			st.machine(machine).setMongoHostPorts(hostPorts[i : i+1])
		}

		w := startWorkerSupportingSpaces(c, st, ipVersion)
		runWorkerUntilMongoStateIs(c, st, w, state.MongoSpaceValid)

		// Only space one has all three servers in it
		c.Assert(st.getMongoSpaceName(), gc.Equals, "")
	})
}

func (s *workerSuite) TestMongoSpaceNotOverwritten(c *gc.C) {
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		st, machines, hostPorts := mongoSpaceTestCommonSetup(c, ipVersion, false)

		for i, machine := range machines {
			// machine 10 gets a host port in space one
			// machine 11 gets host ports in spaces one and two
			// machine 12 gets host ports in spaces one, two and three
			st.machine(machine).setMongoHostPorts(hostPorts[0 : i+1])
		}

		w := startWorkerSupportingSpaces(c, st, ipVersion)
		runWorkerUntilMongoStateIs(c, st, w, state.MongoSpaceValid)

		// Only space one has all three servers in it
		c.Assert(st.getMongoSpaceName(), gc.Equals, "one")

		// Set st.mongoSpaceName to something different

		st.SetMongoSpaceState(state.MongoSpaceUnknown)
		st.SetOrGetMongoSpaceName("testing")

		// Only space one has all three servers in it
		c.Assert(st.getMongoSpaceName(), gc.Equals, "testing")
		c.Assert(st.getMongoSpaceState(), gc.Equals, state.MongoSpaceValid)
	})
}

func (s *workerSuite) TestMongoSpaceNotCalculatedWhenSpacesNotSupported(c *gc.C) {
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		st, machines, hostPorts := mongoSpaceTestCommonSetup(c, ipVersion, false)

		for i, machine := range machines {
			// machine 10 gets a host port in space one
			// machine 11 gets host ports in spaces one and two
			// machine 12 gets host ports in spaces one, two and three
			st.machine(machine).setMongoHostPorts(hostPorts[0 : i+1])
		}

		// Set some garbage up to check that it isn't overwritten
		st.SetOrGetMongoSpaceName("garbage")
		st.SetMongoSpaceState(state.MongoSpaceUnknown)

		// Start a worker that doesn't support spaces
		w := s.newNoPublishWorker(c, st)
		runWorkerUntilMongoStateIs(c, st, w.(*pgWorker), state.MongoSpaceUnsupported)

		// Only space one has all three servers in it
		c.Assert(st.getMongoSpaceName(), gc.Equals, "garbage")
		c.Assert(st.getMongoSpaceState(), gc.Equals, state.MongoSpaceUnsupported)
	})
}

func (s *workerSuite) TestWorkerRetriesOnPublishError(c *gc.C) {
	logger.SetLogLevel(loggo.TRACE)
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		publishCh := make(chan [][]network.HostPort, 100)

		count := 0
		publish := func(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
			c.Logf("** publish call: count=%d", count)
			publishCh <- apiServers
			count++
			if count <= 3 {
				return fmt.Errorf("publish error")
			}
			return nil
		}
		st := NewFakeState()
		InitState(c, st, 3, ipVersion)

		s.newPublishWorker(c, st, PublisherFunc(publish))
		retryInterval := initialRetryInterval
		for i := 0; i < 4; i++ {
			s.waitAdvance(c, retryInterval)
			retryInterval = scaleRetry(retryInterval)
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
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
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

		s.newPublishWorker(c, st, PublisherFunc(publish))

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
		if ok {
			members := val.([]replicaset.Member)
			val = "\n" + prettyReplicaSetMembers(members)
		}
		c.Logf("mustNext done %p, ok: %v, val: %v", w, ok, val)
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

type noOpHub struct{}

func (h *noOpHub) Publish(topic string, data interface{}) (<-chan struct{}, error) {
	return nil, nil
}

func (s *workerSuite) newNoPublishWorker(c *gc.C, st stateInterface) worker.Worker {
	// We create a new clock for the worker so we can wait on alarms even when
	// a single test tests both ipv4 and 6 so is creating two workers.
	s.clock = testing.NewClock(time.Now())
	w, err := newNoPublishWorker(st, s.clock, &noOpHub{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, w) })
	return w
}

func (s *workerSuite) newPublishWorker(c *gc.C, st stateInterface, pub publisherInterface) worker.Worker {
	// We create a new clock for the worker so we can wait on alarms even when
	// a single test tests both ipv4 and 6 so is creating two workers.
	s.clock = testing.NewClock(time.Now())
	w, err := newWorker(st, s.clock, pub, false, &noOpHub{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, w) })
	return w
}

func newNoPublishWorker(st stateInterface, clock clock.Clock, hub Hub) (worker.Worker, error) {
	return newWorker(st, clock, noPublisher{}, false, hub)
}
