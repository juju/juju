// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/replicaset/v3"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
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
	clock                   *testclock.Clock
	hub                     Hub
	controllerConfigService *MockControllerConfigService
	idle                    chan struct{}
	mu                      sync.Mutex

	memberUpdates [][]replicaset.Member
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *workerSuite) SetUpTest(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.BaseSuite.SetUpTest(c)
	s.clock = testclock.NewClock(time.Now())
	s.hub = nopHub{}
	s.controllerConfigService = NewMockControllerConfigService(ctrl)

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

func (s *workerSuite) expectControllerConfigWatcher(c *gc.C) chan []string {
	ch := make(chan []string)
	// Seed the initial event.
	go func() {
		select {
		case ch <- []string{}:
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out seeding initial event")
		}
	}()

	watcher := watchertest.NewMockStringsWatcher(ch)

	s.controllerConfigService.EXPECT().WatchControllerConfig().Return(watcher, nil)

	return ch
}

func (s *workerSuite) TestAddressChange(c *gc.C) {
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		s.expectControllerConfigWatcher(c)
		s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(nil, nil).AnyTimes()

		st := NewFakeState()
		InitState(c, st, 3, ipVersion)

		memberWatcher := st.session.members.Watch()
		defer memberWatcher.Close()

		s.recordMemberChanges(c, memberWatcher)
		update := s.mustNext(c, "init")
		assertMembers(c, update, mkMembers("0v", ipVersion))

		logger.Infof("starting worker")
		w := s.newWorker(c, st, st.session, nopAPIHostPortsSetter{}, true)
		defer workertest.CleanKill(c, w)

		// Wait for the worker to set the initial members.
		update = s.mustNext(c, "initial members")
		assertMembers(c, update, mkMembers("0v 1 2", ipVersion))

		// Change an address and wait for it to be changed in the
		// members.
		st.controller("11").setAddresses(network.NewSpaceAddress(ipVersion.extraHost))

		update = s.mustNext(c, "waiting for new address")
		expectMembers := mkMembers("0v 1 2", ipVersion)
		expectMembers[1].Address = net.JoinHostPort(ipVersion.extraHost, fmt.Sprint(mongoPort))
		assertMembers(c, update, expectMembers)
	})
}

func (s *workerSuite) TestAddressChangeNoHA(c *gc.C) {
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		s.expectControllerConfigWatcher(c)
		s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(nil, nil).AnyTimes()
		st := NewFakeState()
		InitState(c, st, 3, ipVersion)

		memberWatcher := st.session.members.Watch()
		defer memberWatcher.Close()

		s.recordMemberChanges(c, memberWatcher)
		update := s.mustNext(c, "init")
		assertMembers(c, update, mkMembers("0v", ipVersion))

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

func (s *workerSuite) TestSetMembersErrorIsNotFatal(c *gc.C) {
	DoTestForIPv4AndIPv6(c, s, func(ipVersion TestIPVersion) {
		s.expectControllerConfigWatcher(c)
		s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(nil, nil).AnyTimes()
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

func (f SetAPIHostPortsFunc) SetAPIHostPorts(_ controller.Config, apiServers, agentAddresses []network.SpaceHostPorts) error {
	return f(apiServers)
}

func (s *workerSuite) TestWorkerRetriesOnSetAPIHostPortsErrorIPv4(c *gc.C) {
	s.doTestWorkerRetriesOnSetAPIHostPortsError(c, testIPv4)
}

func (s *workerSuite) TestWorkerRetriesOnSetAPIHostPortsErrorIPv6(c *gc.C) {
	s.doTestWorkerRetriesOnSetAPIHostPortsError(c, testIPv6)
}

func (s *workerSuite) doTestWorkerRetriesOnSetAPIHostPortsError(c *gc.C, ipVersion TestIPVersion) {
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

	s.expectControllerConfigWatcher(c)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(nil, nil).AnyTimes()
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

// recordMemberChanges starts a go routine to record member changes.
func (s *workerSuite) recordMemberChanges(c *gc.C, w *voyeur.Watcher) {
	go func() {
		for {
			c.Logf("waiting for next update")
			ok := w.Next()
			if !ok {
				c.Logf("watcher closed")
				return
			}
			val := w.Value()
			members := val.([]replicaset.Member)
			c.Logf("next update, val: %v", "\n"+prettyReplicaSetMembersSlice(members))
			s.mu.Lock()
			s.memberUpdates = append(s.memberUpdates, members)
			s.mu.Unlock()
		}
	}()
}

// mustNext waits for w's value to be set and returns it.
func (s *workerSuite) mustNext(c *gc.C, context string) []replicaset.Member {
	c.Logf("waiting for next update: %v", context)
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		s.mu.Lock()
		if len(s.memberUpdates) == 0 {
			s.mu.Unlock()
			continue
		}
		update := s.memberUpdates[0]
		s.memberUpdates = s.memberUpdates[1:]
		s.mu.Unlock()
		return update
	}
	c.Fatalf("no replicaset update: %v", context)
	return nil
}

type nopAPIHostPortsSetter struct{}

func (nopAPIHostPortsSetter) SetAPIHostPorts(controller.Config, []network.SpaceHostPorts, []network.SpaceHostPorts) error {
	return nil
}

type nopHub struct{}

func (nopHub) Publish(topic string, data interface{}) (func(), error) {
	return func() {}, nil
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
	session *fakeMongoSession,
	apiHostPortsSetter APIHostPortsSetter,
	supportsHA bool,
) worker.Worker {
	return s.newWorkerWithConfig(c, Config{
		Clock:                   s.clock,
		ControllerConfigService: s.controllerConfigService,
		State:                   st,
		MongoSession:            session,
		APIHostPortsSetter:      apiHostPortsSetter,
		ControllerId:            session.currentPrimary,
		MongoPort:               mongoPort,
		APIPort:                 apiPort,
		Hub:                     s.hub,
		SupportsHA:              supportsHA,
		PrometheusRegisterer:    noopRegisterer{},
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
