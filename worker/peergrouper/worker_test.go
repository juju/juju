// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/replicaset"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/voyeur"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	"github.com/kr/pretty"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type TestIPVersion struct {
	version     string
	formatHost  string
	extraHost   string
	addressType network.AddressType
}

var (
	testIPv4 = TestIPVersion{
		version:     "IPv4",
		formatHost:  "0.1.2.%d",
		extraHost:   "0.1.99.13",
		addressType: network.IPv4Address,
	}
	testIPv6 = TestIPVersion{
		version:     "IPv6",
		formatHost:  "2001:DB8::%d",
		extraHost:   "2001:DB8::99:13",
		addressType: network.IPv6Address,
	}
)

type workerSuite struct {
	coretesting.BaseSuite
	clock *testclock.Clock
	hub   Hub
	idle  chan struct{}
	mu    sync.Mutex
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.clock = testclock.NewClock(time.Now())
	s.hub = nopHub{}
	logger.SetLogLevel(loggo.TRACE)
	s.PatchValue(&IdleFunc, s.idleNotify)
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

// InitState initializes the fake state with a single replica-set member and
// numNodes nodes primed to vote.
func InitState(c *gc.C, st *fakeState, numNodes int, ipVersion TestIPVersion) {
	var ids []string
	for i := 10; i < 10+numNodes; i++ {
		id := fmt.Sprint(i)
		m := st.addController(id, true)
		m.setAddresses(network.NewSpaceAddress(fmt.Sprintf(ipVersion.formatHost, i)))
		ids = append(ids, id)
		c.Assert(m.Addresses(), gc.HasLen, 1)
	}
	st.setControllers(ids...)
	err := st.session.Set(mkMembers("0v", ipVersion))
	c.Assert(err, jc.ErrorIsNil)
	st.session.setStatus(mkStatuses("0p", ipVersion))
	err = st.controller("10").SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	st.setCheck(checkInvariants)
}

// ExpectedAPIHostPorts returns the expected addresses
// of the nodes as created by InitState.
func ExpectedAPIHostPorts(n int, ipVersion TestIPVersion) []network.SpaceHostPorts {
	servers := make([]network.SpaceHostPorts, n)
	for i := range servers {
		servers[i] = network.NewSpaceHostPorts(
			apiPort,
			fmt.Sprintf(ipVersion.formatHost, i+10),
		)
	}
	return servers
}

func (s *workerSuite) TestSetsAndUpdatesMembersIPv4(c *gc.C) {
	s.doTestSetAndUpdateMembers(c, testIPv4)
}

func (s *workerSuite) TestSetsAndUpdatesMembersIPv6(c *gc.C) {
	s.doTestSetAndUpdateMembers(c, testIPv6)
}

func (s *workerSuite) doTestSetAndUpdateMembers(c *gc.C, ipVersion TestIPVersion) {
	c.Logf("\n\nTestSetsAndUpdatesMembers: %s", ipVersion.version)
	st := NewFakeState()
	InitState(c, st, 3, ipVersion)
	memberWatcher := st.session.members.Watch()
	mustNext(c, memberWatcher, "init")
	assertMembers(c, memberWatcher.Value(), mkMembers("0v", ipVersion))

	logger.Infof("starting worker")
	w := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true)
	defer workertest.CleanKill(c, w)

	// Due to the inherit complexity of the multiple goroutines running
	// and listen do different watchers, there is no way to manually
	// advance the testing clock in a controlled manner as the clock.After
	// calls can be replaced in response to other watcher events. Hence
	// using the standard testing clock wait / advance method does not
	// work. So we use the real clock to advance the test clock for this
	// test.
	// Every 5ms we advance the testing clock by pollInterval (1min)
	done := make(chan struct{})
	clockAdvancerFinished := make(chan struct{})
	defer func() {
		close(done)
		select {
		case <-clockAdvancerFinished:
			return
		case <-time.After(coretesting.LongWait):
			c.Error("advancing goroutine didn't finish")
		}
	}()
	go func() {
		defer close(clockAdvancerFinished)
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
	mustNext(c, memberWatcher, "initial members")
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 1 2", ipVersion))

	// Update the status of the new members
	// and check that they become voting.
	c.Logf("\nupdating new member status")
	st.session.setStatus(mkStatuses("0s 1p 2s", ipVersion))
	mustNext(c, memberWatcher, "new member status")
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v", ipVersion))

	c.Logf("\nadding another controller")
	m13 := st.addController("13", false)
	m13.setAddresses(network.NewSpaceAddress(fmt.Sprintf(ipVersion.formatHost, 13)))
	st.setControllers("10", "11", "12", "13")

	mustNext(c, memberWatcher, "waiting for new member to be added")
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v 3", ipVersion))

	// Remove vote from an existing member; and give it to the new
	// controller. Also set the status of the new controller to healthy.
	c.Logf("\nremoving vote from controller 10 and adding it to controller 13")
	st.controller("10").setWantsVote(false)
	mustNext(c, memberWatcher, "waiting for vote switch")
	assertMembers(c, memberWatcher.Value(), mkMembers("0 1v 2 3", ipVersion))

	st.controller("13").setWantsVote(true)

	st.session.setStatus(mkStatuses("0s 1p 2s 3s", ipVersion))

	// Check that the new controller gets the vote and the
	// old controller loses it.
	mustNext(c, memberWatcher, "waiting for vote switch")
	assertMembers(c, memberWatcher.Value(), mkMembers("0 1v 2v 3v", ipVersion))

	c.Logf("\nremoving old controller")
	// Remove the old controller.
	st.removeController("10")
	st.setControllers("11", "12", "13")

	// Check that it's removed from the members.
	mustNext(c, memberWatcher, "waiting for removal")
	assertMembers(c, memberWatcher.Value(), mkMembers("1v 2v 3v", ipVersion))
}

func (s *workerSuite) TestHasVoteMaintainedEvenWhenReplicaSetFailsIPv4(c *gc.C) {
	s.doTestHasVoteMaintainsEvenWhenReplicaSetFails(c, testIPv4)
}

func (s *workerSuite) TestHasVoteMaintainedEvenWhenReplicaSetFailsIPv6(c *gc.C) {
	s.doTestHasVoteMaintainsEvenWhenReplicaSetFails(c, testIPv6)
}

func (s *workerSuite) doTestHasVoteMaintainsEvenWhenReplicaSetFails(c *gc.C, ipVersion TestIPVersion) {
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
	err := st.controller("10").SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	err = st.controller("11").SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	err = st.controller("12").SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	err = st.controller("13").SetHasVote(false)
	c.Assert(err, jc.ErrorIsNil)

	st.controller("10").setWantsVote(false)
	st.controller("11").setWantsVote(true)
	st.controller("12").setWantsVote(true)
	st.controller("13").setWantsVote(true)

	err = st.session.Set(mkMembers("0v 1v 2v 3", ipVersion))
	c.Assert(err, jc.ErrorIsNil)
	st.session.setStatus(mkStatuses("0H 1p 2s 3s", ipVersion))

	// Make the worker fail to set HasVote to false
	// after changing the replica set membership.
	st.errors.setErrorFor("Controller.SetHasVote * false", errors.New("frood"))

	memberWatcher := st.session.members.Watch()
	mustNext(c, memberWatcher, "waiting for SetHasVote failure")
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v 3", ipVersion))

	w := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true)
	defer workertest.DirtyKill(c, w)

	// Wait for the worker to set the initial members.
	mustNext(c, memberWatcher, "initial members")
	assertMembers(c, memberWatcher.Value(), mkMembers("0 1v 2v 3v", ipVersion))

	// The worker should encounter an error setting the
	// has-vote status to false and exit.
	err = workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, `removing non-voters: cannot set voting status of "[0-9]+" to false: frood`)

	// Start the worker again - although the membership should
	// not change, the HasVote status should be updated correctly.
	st.errors.resetErrors()
	w = s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true)
	defer workertest.CleanKill(c, w)

	// Watch all the controllers for changes, so we can check
	// their has-vote status without polling.
	changed := make(chan struct{}, 1)
	for i := 10; i < 14; i++ {
		watcher := st.controller(fmt.Sprint(i)).val.Watch()
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
				hasVote := st.controller(fmt.Sprint(i)).HasVote()
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
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		st := NewFakeState()
		InitState(c, st, 3, ipVersion)

		memberWatcher := st.session.members.Watch()
		mustNext(c, memberWatcher, "init")
		assertMembers(c, memberWatcher.Value(), mkMembers("0v", ipVersion))

		logger.Infof("starting worker")
		w := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true)
		defer workertest.CleanKill(c, w)

		// Wait for the worker to set the initial members.
		mustNext(c, memberWatcher, "initial members")
		assertMembers(c, memberWatcher.Value(), mkMembers("0v 1 2", ipVersion))

		// Change an address and wait for it to be changed in the
		// members.
		st.controller("11").setAddresses(network.NewSpaceAddress(ipVersion.extraHost))

		mustNext(c, memberWatcher, "waiting for new address")
		expectMembers := mkMembers("0v 1 2", ipVersion)
		expectMembers[1].Address = net.JoinHostPort(ipVersion.extraHost, fmt.Sprint(mongoPort))
		assertMembers(c, memberWatcher.Value(), expectMembers)
	})
}

func (s *workerSuite) TestAddressChangeNoHA(c *gc.C) {
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		st := NewFakeState()
		InitState(c, st, 3, ipVersion)

		memberWatcher := st.session.members.Watch()
		mustNext(c, memberWatcher, "init")
		assertMembers(c, memberWatcher.Value(), mkMembers("0v", ipVersion))

		logger.Infof("starting worker")
		w := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, false)
		defer workertest.CleanKill(c, w)

		// There must be no replicaset updates.
		type voyeurResult struct {
			ok  bool
			val interface{}
		}
		done := make(chan voyeurResult)
		go func() {
			ok := memberWatcher.Next()
			val := memberWatcher.Value()
			if ok {
				members := val.([]replicaset.Member)
				val = "\n" + prettyReplicaSetMembersSlice(members)
			}
			done <- voyeurResult{ok, val}
		}()
		select {
		case <-done:
			c.Fatalf("unexpected event")
		case <-time.After(coretesting.ShortWait):
		}
	})
}

var fatalErrorsTests = []struct {
	errPattern   string
	err          error
	expectErr    string
	advanceCount int
}{{
	errPattern: "State.ControllerIds",
	expectErr:  "cannot get controller ids: sample",
}, {
	errPattern:   "Controller.SetHasVote 11 true",
	expectErr:    `adding new voters: cannot set voting status of "11" to true: sample`,
	advanceCount: 2,
}, {
	errPattern: "Session.CurrentStatus",
	expectErr:  "creating peer group info: cannot get replica set status: sample",
}, {
	errPattern: "Session.CurrentMembers",
	expectErr:  "creating peer group info: cannot get replica set members: sample",
}, {
	errPattern: "State.ControllerNode *",
	expectErr:  `cannot get controller "10": sample`,
}, {
	errPattern: "State.ControllerHost *",
	expectErr:  `cannot get controller "10": sample`,
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

			w := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true)
			defer workertest.DirtyKill(c, w)

			for j := 0; j < testCase.advanceCount; j++ {
				_ = s.clock.WaitAdvance(pollInterval, coretesting.ShortWait, 1)
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

		w := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true)
		defer workertest.CleanKill(c, w)

		// Just watch three error retries
		retryInterval := initialRetryInterval
		for i := 0; i < 3; i++ {
			_ = s.clock.WaitAdvance(retryInterval, coretesting.ShortWait, 1)
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

type SetAPIHostPortsFunc func(apiServers []network.SpaceHostPorts) error

func (f SetAPIHostPortsFunc) SetAPIHostPorts(apiServers []network.SpaceHostPorts) error {
	return f(apiServers)
}

func (s *workerSuite) TestControllersArePublished(c *gc.C) {
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		publishCh := make(chan []network.SpaceHostPorts)
		publish := func(apiServers []network.SpaceHostPorts) error {
			publishCh <- apiServers
			return nil
		}

		st := NewFakeState()
		InitState(c, st, 3, ipVersion)
		w := s.newWorker(c, st, st.session, SetAPIHostPortsFunc(publish), true)
		defer workertest.CleanKill(c, w)

		select {
		case servers := <-publishCh:
			AssertAPIHostPorts(c, servers, ExpectedAPIHostPorts(3, ipVersion))
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for publish")
		}

		// If a config change wakes up the loop *after* the controller topology
		// is published, then we will get another call to setAPIHostPorts.
		select {
		case <-publishCh:
		case <-time.After(coretesting.ShortWait):
		}

		// Change one of the server API addresses and check that it is
		// published.
		newMachine10Addresses := network.NewSpaceAddresses(ipVersion.extraHost)
		st.controller("10").setAddresses(newMachine10Addresses...)
		select {
		case servers := <-publishCh:
			expected := ExpectedAPIHostPorts(3, ipVersion)
			expected[0] = network.SpaceAddressesWithPort(newMachine10Addresses, apiPort)
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
	s.hub = hub

	w := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true)
	defer workertest.CleanKill(c, w)

	expected := apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"10": {ID: "10", Addresses: []string{"0.1.2.10:5678"}, InternalAddress: "0.1.2.10:5678"},
			"11": {ID: "11", Addresses: []string{"0.1.2.11:5678"}, InternalAddress: "0.1.2.11:5678"},
			"12": {ID: "12", Addresses: []string{"0.1.2.12:5678"}, InternalAddress: "0.1.2.12:5678"},
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

func (s *workerSuite) TestControllersPublishedWithControllerAPIPort(c *gc.C) {
	st := NewFakeState()
	InitState(c, st, 3, testIPv4)

	hub := pubsub.NewStructuredHub(nil)
	event := make(chan apiserver.Details)
	_, err := hub.Subscribe(apiserver.DetailsTopic, func(topic string, data apiserver.Details, err error) {
		c.Check(err, jc.ErrorIsNil)
		event <- data
	})
	c.Assert(err, jc.ErrorIsNil)
	s.hub = hub

	w := s.newWorkerWithConfig(c, Config{
		Clock:                s.clock,
		State:                st,
		MongoSession:         st.session,
		APIHostPortsSetter:   nopAPIHostPortsSetter{},
		MongoPort:            mongoPort,
		APIPort:              apiPort,
		ControllerAPIPort:    controllerAPIPort,
		Hub:                  s.hub,
		SupportsHA:           true,
		PrometheusRegisterer: noopRegisterer{},
	})
	defer workertest.CleanKill(c, w)

	expected := apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"10": {ID: "10", Addresses: []string{"0.1.2.10:5678"}, InternalAddress: "0.1.2.10:9876"},
			"11": {ID: "11", Addresses: []string{"0.1.2.11:5678"}, InternalAddress: "0.1.2.11:9876"},
			"12": {ID: "12", Addresses: []string{"0.1.2.12:5678"}, InternalAddress: "0.1.2.12:9876"},
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

func (s *workerSuite) TestControllersArePublishedOverHubWithNewVoters(c *gc.C) {
	st := NewFakeState()
	var ids []string
	for i := 10; i < 13; i++ {
		id := fmt.Sprint(i)
		m := st.addController(id, true)
		err := m.SetHasVote(true)
		c.Assert(err, jc.ErrorIsNil)
		m.setAddresses(network.NewSpaceAddress(fmt.Sprintf(testIPv4.formatHost, i)))
		ids = append(ids, id)
		c.Assert(m.Addresses(), gc.HasLen, 1)
	}
	st.setControllers(ids...)
	err := st.session.Set(mkMembers("0v 1 2", testIPv4))
	c.Assert(err, jc.ErrorIsNil)
	st.session.setStatus(mkStatuses("0p 1s 2s", testIPv4))
	st.setCheck(checkInvariants)

	hub := pubsub.NewStructuredHub(nil)
	event := make(chan apiserver.Details)
	_, err = hub.Subscribe(apiserver.DetailsTopic, func(topic string, data apiserver.Details, err error) {
		c.Check(err, jc.ErrorIsNil)
		event <- data
	})
	c.Assert(err, jc.ErrorIsNil)
	s.hub = hub

	w := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true)
	defer workertest.CleanKill(c, w)

	expected := apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"10": {ID: "10", Addresses: []string{"0.1.2.10:5678"}, InternalAddress: "0.1.2.10:5678"},
			"11": {ID: "11", Addresses: []string{"0.1.2.11:5678"}, InternalAddress: "0.1.2.11:5678"},
			"12": {ID: "12", Addresses: []string{"0.1.2.12:5678"}, InternalAddress: "0.1.2.12:5678"},
		},
		LocalOnly: true,
	}

	select {
	case obtained := <-event:
		c.Assert(obtained, jc.DeepEquals, expected)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for event")
	}

	// And check that they can be republished on request.
	_, err = hub.Publish(apiserver.DetailsRequestTopic, apiserver.DetailsRequest{
		Requester: "dad",
		LocalOnly: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	select {
	case obtained := <-event:
		c.Assert(obtained, jc.DeepEquals, expected)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for event")
	}
}

func haSpaceTestCommonSetup(c *gc.C, ipVersion TestIPVersion, members string) *fakeState {
	st := NewFakeState()
	InitState(c, st, 3, ipVersion)

	addrs := network.NewSpaceAddresses(
		fmt.Sprintf(ipVersion.formatHost, 1),
		fmt.Sprintf(ipVersion.formatHost, 2),
		fmt.Sprintf(ipVersion.formatHost, 3),
	)
	for i := range addrs {
		addrs[i].Scope = network.ScopeCloudLocal
	}

	spaces := []string{"one", "two", "three"}
	controllers := []int{10, 11, 12}
	for _, id := range controllers {
		controller := st.controller(strconv.Itoa(id))
		err := controller.SetHasVote(true)
		c.Assert(err, jc.ErrorIsNil)
		controller.setWantsVote(true)

		// Each controller gets 3 addresses in 3 different spaces.
		// Space "one" address on controller 10 ends with "10"
		// Space "two" address ends with "11"
		// Space "three" address ends with "12"
		// Space "one" address on controller 20 ends with "20"
		// Space "two" address ends with "21"
		// ...
		addrs := make(network.SpaceAddresses, 3)
		for i, name := range spaces {
			addr := network.NewScopedSpaceAddress(fmt.Sprintf(ipVersion.formatHost, i*10+id), network.ScopeCloudLocal)
			addr.SpaceID = name
			addrs[i] = addr
		}
		controller.setAddresses(addrs...)
	}

	err := st.session.Set(mkMembers(members, ipVersion))
	c.Assert(err, jc.ErrorIsNil)
	return st
}

func (s *workerSuite) TestUsesConfiguredHASpaceIPv4(c *gc.C) {
	s.doTestUsesConfiguredHASpace(c, testIPv4)
}

func (s *workerSuite) TestUsesConfiguredHASpaceIPv6(c *gc.C) {
	s.doTestUsesConfiguredHASpace(c, testIPv6)
}

func (s *workerSuite) doTestUsesConfiguredHASpace(c *gc.C, ipVersion TestIPVersion) {
	st := haSpaceTestCommonSetup(c, ipVersion, "0v 1v 2v")

	// Set one of the statuses to ensure it is cleared upon determination
	// of a new peer group.
	now := time.Now()
	err := st.controller("11").SetStatus(status.StatusInfo{
		Status:  status.Started,
		Message: "You said that would be bad, Egon",
		Since:   &now,
	})
	c.Assert(err, gc.IsNil)

	st.setHASpace("two")
	s.runUntilPublish(c, st, "")
	assertMemberAddresses(c, st, ipVersion.formatHost, 2)

	sInfo, err := st.controller("11").Status()
	c.Assert(err, gc.IsNil)
	c.Check(sInfo.Status, gc.Equals, status.Started)
	c.Check(sInfo.Message, gc.Equals, "")
}

// runUntilPublish runs a worker until addresses are published over the pub/sub
// hub. Note that the replica-set is updated earlier than the publish,
// so this sync can be used to check for those changes.
// If errMsg is not empty, it is used to check for a matching error.
func (s *workerSuite) runUntilPublish(c *gc.C, st *fakeState, errMsg string) {
	hub := pubsub.NewStructuredHub(nil)
	event := make(chan apiserver.Details)
	_, err := hub.Subscribe(apiserver.DetailsTopic, func(topic string, data apiserver.Details, err error) {
		c.Check(err, jc.ErrorIsNil)
		event <- data
	})
	c.Assert(err, jc.ErrorIsNil)
	s.hub = hub

	w := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true)
	defer func() {
		if errMsg == "" {
			workertest.CleanKill(c, w)
		} else {
			err := workertest.CheckKill(c, w)
			c.Assert(err, gc.ErrorMatches, errMsg)
		}
	}()

	select {
	case <-event:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for event")
	}
}

func (s *workerSuite) TestDetectsAndUsesHASpaceChangeIPv4(c *gc.C) {
	s.doTestDetectsAndUsesHASpaceChange(c, testIPv4)
}

func (s *workerSuite) TestDetectsAndUsesHASpaceChangeIPv6(c *gc.C) {
	s.doTestDetectsAndUsesHASpaceChange(c, testIPv6)
}

func (s *workerSuite) doTestDetectsAndUsesHASpaceChange(c *gc.C, ipVersion TestIPVersion) {
	st := haSpaceTestCommonSetup(c, ipVersion, "0v 1v 2v")
	st.setHASpace("one")

	// Set up a hub and channel on which to receive notifications.
	hub := pubsub.NewStructuredHub(nil)
	event := make(chan apiserver.Details)
	_, err := hub.Subscribe(apiserver.DetailsTopic, func(topic string, data apiserver.Details, err error) {
		c.Check(err, jc.ErrorIsNil)
		event <- data
	})
	c.Assert(err, jc.ErrorIsNil)
	s.hub = hub

	w := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true)
	defer workertest.CleanKill(c, w)

	select {
	case <-event:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for event")
	}
	assertMemberAddresses(c, st, ipVersion.formatHost, 1)

	// Changing the space does not change the API server details, so the
	// change will not be broadcast via the hub.
	// We watch the members collection, which *will* change.
	memberWatcher := st.session.members.Watch()
	mustNext(c, memberWatcher, "initial watch")

	// HA space config change should invoke the worker.
	// Replica set addresses should change to the new space.
	st.setHASpace("three")
	mustNext(c, memberWatcher, "waiting for members to be updated for space change")
	assertMemberAddresses(c, st, ipVersion.formatHost, 3)
}

func assertMemberAddresses(c *gc.C, st *fakeState, addrTemplate string, addrDesignator int) {
	members, _ := st.session.CurrentMembers()
	obtained := make([]string, 3)
	for i, m := range members {
		obtained[i] = m.Address
	}
	sort.Strings(obtained)

	expected := make([]string, 3)
	for i := 0; i < 3; i++ {
		expected[i] = net.JoinHostPort(fmt.Sprintf(addrTemplate, 10*addrDesignator+i), fmt.Sprint(mongoPort))
	}

	c.Check(obtained, gc.DeepEquals, expected)
}

func (s *workerSuite) TestErrorAndStatusForNewPeersAndNoHASpaceAndMachinesWithMultiAddrIPv4(c *gc.C) {
	s.doTestErrorAndStatusForNewPeersAndNoHASpaceAndMachinesWithMultiAddr(c, testIPv4)
}

func (s *workerSuite) TestErrorAndStatusForNewPeersAndNoHASpaceAndMachinesWithMultiAddrIPv6(c *gc.C) {
	s.doTestErrorAndStatusForNewPeersAndNoHASpaceAndMachinesWithMultiAddr(c, testIPv6)
}

func (s *workerSuite) doTestErrorAndStatusForNewPeersAndNoHASpaceAndMachinesWithMultiAddr(
	c *gc.C, ipVersion TestIPVersion,
) {
	st := haSpaceTestCommonSetup(c, ipVersion, "0v")
	err := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true).Wait()
	errMsg := `computing desired peer group: updating member addresses: ` +
		`juju-ha-space is not set and these nodes have more than one usable address: 1[12], 1[12]` +
		"\nrun \"juju controller-config juju-ha-space=<name>\" to set a space for Mongo peer communication"
	c.Check(err, gc.ErrorMatches, errMsg)

	for _, id := range []string{"11", "12"} {
		sInfo, err := st.controller(id).Status()
		c.Assert(err, gc.IsNil)
		c.Check(sInfo.Status, gc.Equals, status.Started)
		c.Check(sInfo.Message, gc.Not(gc.Equals), "")
	}
}

func (s *workerSuite) TestErrorAndStatusForHASpaceWithNoAddressesAddrIPv4(c *gc.C) {
	s.doTestErrorAndStatusForHASpaceWithNoAddresses(c, testIPv4)
}

func (s *workerSuite) TestErrorAndStatusForHASpaceWithNoAddressesAddrIPv6(c *gc.C) {
	s.doTestErrorAndStatusForHASpaceWithNoAddresses(c, testIPv6)
}

func (s *workerSuite) doTestErrorAndStatusForHASpaceWithNoAddresses(
	c *gc.C, ipVersion TestIPVersion,
) {
	st := haSpaceTestCommonSetup(c, ipVersion, "0v")
	st.setHASpace("nope")

	err := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true).Wait()
	errMsg := `computing desired peer group: updating member addresses: ` +
		`no usable Mongo addresses found in configured juju-ha-space "nope" for nodes: 1[012], 1[012], 1[012]`
	c.Check(err, gc.ErrorMatches, errMsg)

	for _, id := range []string{"10", "11", "12"} {
		sInfo, err := st.controller(id).Status()
		c.Assert(err, gc.IsNil)
		c.Check(sInfo.Status, gc.Equals, status.Started)
		c.Check(sInfo.Message, gc.Not(gc.Equals), "")
	}
}

func (s *workerSuite) TestSamePeersAndNoHASpaceAndMachinesWithMultiAddrIPv4(c *gc.C) {
	s.doTestSamePeersAndNoHASpaceAndMachinesWithMultiAddr(c, testIPv4)
}

func (s *workerSuite) TestSamePeersAndNoHASpaceAndMachinesWithMultiAddrIPv6(c *gc.C) {
	s.doTestSamePeersAndNoHASpaceAndMachinesWithMultiAddr(c, testIPv6)
}

func (s *workerSuite) doTestSamePeersAndNoHASpaceAndMachinesWithMultiAddr(c *gc.C, ipVersion TestIPVersion) {
	st := haSpaceTestCommonSetup(c, ipVersion, "0v 1v 2v")
	s.runUntilPublish(c, st, "")
	assertMemberAddresses(c, st, ipVersion.formatHost, 1)
}

func (s *workerSuite) TestWorkerRetriesOnSetAPIHostPortsErrorIPv4(c *gc.C) {
	s.doTestWorkerRetriesOnSetAPIHostPortsError(c, testIPv4)
}

func (s *workerSuite) TestWorkerRetriesOnSetAPIHostPortsErrorIPv6(c *gc.C) {
	s.doTestWorkerRetriesOnSetAPIHostPortsError(c, testIPv6)
}

func (s *workerSuite) doTestWorkerRetriesOnSetAPIHostPortsError(c *gc.C, ipVersion TestIPVersion) {
	logger.SetLogLevel(loggo.TRACE)

	publishCh := make(chan []network.SpaceHostPorts, 10)
	failedOnce := false
	publish := func(apiServers []network.SpaceHostPorts) error {
		if !failedOnce {
			failedOnce = true
			return fmt.Errorf("publish error")
		}
		publishCh <- apiServers
		return nil
	}
	st := NewFakeState()
	InitState(c, st, 3, ipVersion)

	w := s.newWorker(c, st, st.session, SetAPIHostPortsFunc(publish), true)
	defer workertest.CleanKill(c, w)

	retryInterval := initialRetryInterval
	_ = s.clock.WaitAdvance(retryInterval, coretesting.ShortWait, 1)
	select {
	case servers := <-publishCh:
		AssertAPIHostPorts(c, servers, ExpectedAPIHostPorts(3, ipVersion))
		break
	case <-time.After(coretesting.ShortWait):
		c.Fatal("APIHostPorts were not published")
	}
	// There isn't any point checking for additional publish
	// calls as we are also racing against config changed, which
	// will also call SetAPIHostPorts. But we may not get this.
}

func (s *workerSuite) initialize3Voters(c *gc.C) (*fakeState, worker.Worker, *voyeur.Watcher) {
	st := NewFakeState()
	InitState(c, st, 1, testIPv4)
	err := st.controller("10").SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	st.session.setStatus(mkStatuses("0p", testIPv4))

	w := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true)
	defer func() {
		if r := recover(); r != nil {
			// we aren't exiting cleanly, so kill the worker
			workertest.CleanKill(c, w)
			// but let the stack trace continue
			panic(r)
		}
	}()

	memberWatcher := st.session.members.Watch()
	mustNext(c, memberWatcher, "init")
	assertMembers(c, memberWatcher.Value(), mkMembers("0v", testIPv4))
	// Now that 1 has come up successfully, bring in the next 2
	for i := 11; i < 13; i++ {
		id := fmt.Sprint(i)
		m := st.addController(id, true)
		m.setAddresses(network.NewSpaceAddress(fmt.Sprintf(testIPv4.formatHost, i)))
		c.Check(m.Addresses(), gc.HasLen, 1)
	}
	// Now that we've added 2 more, flag them as started and mark them as participating
	st.session.setStatus(mkStatuses("0p 1 2", testIPv4))
	st.setControllers("10", "11", "12")
	mustNext(c, memberWatcher, "nonvoting members")
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 1 2", testIPv4))
	st.session.setStatus(mkStatuses("0p 1s 2s", testIPv4))
	s.waitUntilIdle(c)
	s.clock.Advance(pollInterval)
	mustNext(c, memberWatcher, "status ok")
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 1v 2v", testIPv4))
	err = st.controller("11").SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	err = st.controller("12").SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	return st, w, memberWatcher
}

func (s *workerSuite) TestDyingMachinesAreRemoved(c *gc.C) {
	st, w, memberWatcher := s.initialize3Voters(c)
	defer workertest.CleanKill(c, w)
	// Now we have gotten to a prepared replicaset.

	// When we advance the lifecycle (aka controller.Destroy()), we should notice that the controller no longer wants a vote
	// controller.Destroy() advances to both Dying and SetWantsVote(false)
	st.controller("11").advanceLifecycle(state.Dying, false)
	// we should notice that we want to remove the vote first
	mustNext(c, memberWatcher, "removing vote")
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 1 2", testIPv4))
	// And once we don't have the vote, and we see the controller is Dying we should remove it
	mustNext(c, memberWatcher, "remove dying controller")
	assertMembers(c, memberWatcher.Value(), mkMembers("0v 2", testIPv4))

	// Now, controller 2 no longer has the vote, but if we now flag it as dying,
	// then it should also get progressed to dead as well.
	st.controller("12").advanceLifecycle(state.Dying, false)
	mustNext(c, memberWatcher, "removing dying controller")
	assertMembers(c, memberWatcher.Value(), mkMembers("0v", testIPv4))
}

func (s *workerSuite) TestRemovePrimaryValidSecondaries(c *gc.C) {
	st, w, memberWatcher := s.initialize3Voters(c)
	defer workertest.CleanKill(c, w)
	statusWatcher := st.session.status.Watch()
	testStatus := mustNextStatus(c, statusWatcher, "init")
	c.Check(testStatus.Members, gc.DeepEquals, mkStatuses("0p 1s 2s", testIPv4))
	primaryMemberIndex := 0

	st.controller("10").setWantsVote(false)
	// we should notice that the primary has failed, and have called StepDownPrimary which should ultimately cause
	// a change in the Status.
	testStatus = mustNextStatus(c, statusWatcher, "stepping down primary")
	// find out which one is primary, should only be one of 1 or 2
	c.Assert(testStatus.Members, gc.HasLen, 3)
	c.Check(testStatus.Members[0].State, gc.Equals, replicaset.MemberState(replicaset.SecondaryState))
	if testStatus.Members[1].State == replicaset.PrimaryState {
		primaryMemberIndex = 1
		c.Check(testStatus.Members[2].State, gc.Equals, replicaset.MemberState(replicaset.SecondaryState))
	} else {
		primaryMemberIndex = 2
		c.Check(testStatus.Members[2].State, gc.Equals, replicaset.MemberState(replicaset.PrimaryState))
	}
	// Now we have to wait for time to advance for us to reevaluate the system
	s.waitUntilIdle(c)
	s.clock.Advance(2 * pollInterval)
	mustNext(c, memberWatcher, "reevaluting member post-step-down")
	// we should now have switch the vote over to whoever became the primary
	if primaryMemberIndex == 1 {
		assertMembers(c, memberWatcher.Value(), mkMembers("0 1v 2", testIPv4))
	} else {
		assertMembers(c, memberWatcher.Value(), mkMembers("0 1 2v", testIPv4))
	}
	// Now we ask the primary to step down again, and we should first reconfigure the group to include
	// the other secondary. We first unset the invariant checker, because we are intentionally going to an even number
	// of voters, but this is not the normal condition
	st.setCheck(nil)
	if primaryMemberIndex == 1 {
		st.controller("11").setWantsVote(false)
	} else {
		st.controller("12").setWantsVote(false)
	}
	// member watcher must fire first
	mustNext(c, memberWatcher, "observing member step down")
	assertMembers(c, memberWatcher.Value(), mkMembers("0 1v 2v", testIPv4))
	// as part of stepping down the only primary, we re-enable the vote for the other secondary, and then can call
	// StepDownPrimary and then can remove its vote.
	// now we timeout so that the system will notice we really do still want to step down the primary, and ask
	// for it to revote.
	s.waitUntilIdle(c)
	s.clock.Advance(2 * pollInterval)
	testStatus = mustNextStatus(c, statusWatcher, "stepping down new primary")
	if primaryMemberIndex == 1 {
		// 11 was the primary, now 12 is
		c.Check(testStatus.Members[1].State, gc.Equals, replicaset.MemberState(replicaset.SecondaryState))
		c.Check(testStatus.Members[2].State, gc.Equals, replicaset.MemberState(replicaset.PrimaryState))
	} else {
		c.Check(testStatus.Members[1].State, gc.Equals, replicaset.MemberState(replicaset.PrimaryState))
		c.Check(testStatus.Members[2].State, gc.Equals, replicaset.MemberState(replicaset.SecondaryState))
	}
	// and then we again notice that the primary has been rescheduled and changed the member votes again
	s.waitUntilIdle(c)
	s.clock.Advance(pollInterval)
	mustNext(c, memberWatcher, "reevaluting member post-step-down")
	if primaryMemberIndex == 1 {
		// primary was 11, now it is 12 as the only voter
		assertMembers(c, memberWatcher.Value(), mkMembers("0 1 2v", testIPv4))
	} else {
		// primary was 12, now it is 11 as the only voter
		assertMembers(c, memberWatcher.Value(), mkMembers("0 1v 2", testIPv4))
	}
}

// mustNext waits for w's value to be set and returns it.
func mustNext(c *gc.C, w *voyeur.Watcher, context string) (val interface{}) {
	type voyeurResult struct {
		ok  bool
		val interface{}
	}
	done := make(chan voyeurResult)
	go func() {
		c.Logf("mustNext %v", context)
		ok := w.Next()
		val = w.Value()
		if ok {
			members := val.([]replicaset.Member)
			val = "\n" + prettyReplicaSetMembersSlice(members)
		}
		c.Logf("mustNext %v done, ok: %v, val: %v", context, ok, val)
		done <- voyeurResult{ok, val}
	}()
	select {
	case result := <-done:
		c.Assert(result.ok, jc.IsTrue)
		return result.val
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for value to be set %v", context)
	}
	panic("unreachable")
}

func mustNextStatus(c *gc.C, w *voyeur.Watcher, context string) *replicaset.Status {
	type voyeurResult struct {
		ok  bool
		val *replicaset.Status
	}
	done := make(chan voyeurResult)
	go func() {
		c.Logf("mustNextStatus %v", context)
		var result voyeurResult
		result.ok = w.Next()
		if result.ok {
			val := w.Value()
			result.val = val.(*replicaset.Status)
		}
		c.Logf("mustNextStatus %v done, ok: %v, val: %v", context, result.ok, pretty.Sprint(result.val))
		done <- result
	}()
	select {
	case result := <-done:
		c.Assert(result.ok, jc.IsTrue)
		return result.val
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for value to be set %v", context)
	}
	panic("unreachable")
}

type nopAPIHostPortsSetter struct{}

func (nopAPIHostPortsSetter) SetAPIHostPorts(apiServers []network.SpaceHostPorts) error {
	return nil
}

type nopHub struct{}

func (nopHub) Publish(topic string, data interface{}) (<-chan struct{}, error) {
	return nil, nil
}

func (nopHub) Subscribe(topic string, handler interface{}) (func(), error) {
	return func() {}, nil
}

type noopRegisterer struct {
	prometheus.Registerer
}

func (noopRegisterer) Register(prometheus.Collector) error {
	return nil
}

func (noopRegisterer) Unregister(prometheus.Collector) bool {
	return true
}

func (s *workerSuite) newWorkerWithConfig(
	c *gc.C,
	config Config,
) worker.Worker {
	// We create a new clock for the worker so we can wait on alarms even when
	// a single test tests both ipv4 and 6 so is creating two workers.
	s.clock = testclock.NewClock(time.Now())
	config.Clock = s.clock
	w, err := New(config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, w) })
	return w
}

func (s *workerSuite) newWorker(
	c *gc.C,
	st State,
	session MongoSession,
	apiHostPortsSetter APIHostPortsSetter,
	supportsHA bool,
) worker.Worker {
	return s.newWorkerWithConfig(c, Config{
		State:                st,
		MongoSession:         session,
		APIHostPortsSetter:   apiHostPortsSetter,
		MongoPort:            mongoPort,
		APIPort:              apiPort,
		Hub:                  s.hub,
		SupportsHA:           supportsHA,
		PrometheusRegisterer: noopRegisterer{},
	})
}

func (s *workerSuite) idleNotify() {
	logger.Infof("idleNotify signalled")
	s.mu.Lock()
	idle := s.idle
	s.mu.Unlock()
	if idle == nil {
		return
	}
	// Send down the idle channel if it is set.
	select {
	case idle <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		// no-op
		logger.Infof("... no one watching")
	}
}

func (s *workerSuite) waitUntilIdle(c *gc.C) {
	logger.Infof("wait for idle")
	s.mu.Lock()
	s.idle = make(chan struct{})
	s.mu.Unlock()

	select {
	case <-s.idle:
		// All good.
	case <-time.After(coretesting.LongWait):
		c.Fatalf("idle channel not signalled in worker")
	}

	s.mu.Lock()
	s.idle = nil
	s.mu.Unlock()
}
