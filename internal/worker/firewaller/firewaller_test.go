// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	coremachinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/firewaller"
	"github.com/juju/juju/internal/worker/firewaller/mocks"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

const allEndpoints = ""

// firewallerBaseSuite implements common functionality for embedding
// into each of the other per-mode suites.
type firewallerBaseSuite struct {
	testhelpers.IsolationSuite

	firewaller           *mocks.MockFirewallerAPI
	remoteRelations      *mocks.MockRemoteRelationsAPI
	portService          *mocks.MockPortService
	machineService       *mocks.MockMachineService
	applicationService   *mocks.MockApplicationService
	crossmodelFirewaller *mocks.MockCrossModelFirewallerFacadeCloser
	envFirewaller        *mocks.MockEnvironFirewaller
	envModelFirewaller   *mocks.MockEnvironModelFirewaller
	envInstances         *mocks.MockEnvironInstances

	machinesCh     chan []string
	applicationsCh chan struct{}
	openedPortsCh  chan []string
	remoteRelCh    chan []string
	subnetsCh      chan []string
	modelFwRulesCh chan struct{}

	clock testclock.AdvanceableClock

	firewallerStarted bool
	modelFlushed      chan bool
	modelFlushSkipped chan bool
	machineFlushed    chan machine.Name
	watchingMachine   chan machine.Name

	mode                string
	withIpv6            bool
	withModelFirewaller bool

	modelIngressRules firewall.IngressRules
	envModelPorts     firewall.IngressRules

	nextMachineId int
	nextUnitId    map[string]int

	deadMachines  set.Strings
	instancePorts map[string]firewall.IngressRules
	envPorts      firewall.IngressRules

	mu             sync.Mutex
	unitPortRanges *unitPortRanges
}

func (s *firewallerBaseSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.withIpv6 = true
	s.withModelFirewaller = true
	s.firewaller = nil
	s.firewallerStarted = false

	s.nextMachineId = 0
	s.nextUnitId = make(map[string]int)
	s.deadMachines = set.NewStrings()

	s.unitPortRanges = newUnitPortRanges()
	s.instancePorts = make(map[string]firewall.IngressRules)
	s.envPorts = firewall.IngressRules{}

	s.modelIngressRules = firewall.IngressRules{}
	s.envModelPorts = firewall.IngressRules{}
}

var _ worker.Worker = (*firewaller.Firewaller)(nil)

func (s *firewallerBaseSuite) ensureMocks(c *tc.C, ctrl *gomock.Controller) {
	if s.firewaller != nil {
		panic("firewaller already created")
	}
	if s.clock == nil {
		s.clock = testclock.NewDilatedWallClock(coretesting.ShortWait)
	}

	s.firewaller = mocks.NewMockFirewallerAPI(ctrl)
	s.portService = mocks.NewMockPortService(ctrl)
	s.machineService = mocks.NewMockMachineService(ctrl)
	s.applicationService = mocks.NewMockApplicationService(ctrl)
	s.envFirewaller = mocks.NewMockEnvironFirewaller(ctrl)
	s.envModelFirewaller = mocks.NewMockEnvironModelFirewaller(ctrl)
	s.envInstances = mocks.NewMockEnvironInstances(ctrl)
	s.remoteRelations = mocks.NewMockRemoteRelationsAPI(ctrl)
	s.crossmodelFirewaller = mocks.NewMockCrossModelFirewallerFacadeCloser(ctrl)

	s.machinesCh = make(chan []string, 5)
	s.applicationsCh = make(chan struct{}, 5)
	s.openedPortsCh = make(chan []string, 10)
	s.remoteRelCh = make(chan []string, 5)
	s.subnetsCh = make(chan []string, 5)
	s.modelFwRulesCh = make(chan struct{}, 5)

	// This is the controller machine.
	m, _ := s.addMachine(ctrl)
	inst := s.startInstance(c, ctrl, m)
	inst.EXPECT().IngressRules(gomock.Any(), m.Tag().Id()).Return(nil, nil).AnyTimes()

	s.envFirewaller.EXPECT().IngressRules(gomock.Any()).DoAndReturn(func(ctx context.Context) (firewall.IngressRules, error) {
		return s.envPorts, nil
	}).AnyTimes()

	// Initial event.
	if s.withModelFirewaller {
		s.firewaller.EXPECT().ModelFirewallRules(gomock.Any()).AnyTimes().DoAndReturn(func(context.Context) (firewall.IngressRules, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.modelIngressRules, nil
		})

		s.envModelFirewaller.EXPECT().ModelIngressRules(gomock.Any()).AnyTimes().DoAndReturn(func(arg0 context.Context) (firewall.IngressRules, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.envModelPorts, nil
		})

		s.envModelFirewaller.EXPECT().OpenModelPorts(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, rules firewall.IngressRules) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			add, _ := s.envModelPorts.Diff(rules)
			s.envModelPorts = append(s.envModelPorts, add...)
			return nil
		})

		s.modelFwRulesCh <- struct{}{}
	}

	c.Cleanup(func() {
		s.firewaller = nil
		s.portService = nil
		s.machineService = nil
		s.applicationService = nil
		s.envFirewaller = nil
		s.envModelFirewaller = nil
		s.envInstances = nil
		s.crossmodelFirewaller = nil
		s.remoteRelations = nil

		s.machinesCh = nil
		s.applicationsCh = nil
		s.openedPortsCh = nil
		s.remoteRelCh = nil
		s.subnetsCh = nil
		s.modelFwRulesCh = nil
	})
}

func (s *firewallerBaseSuite) ensureMocksWithoutMachine(ctrl *gomock.Controller) {
	if s.firewaller != nil {
		return
	}
	if s.clock == nil {
		s.clock = testclock.NewDilatedWallClock(coretesting.ShortWait)
	}

	s.firewaller = mocks.NewMockFirewallerAPI(ctrl)
	s.portService = mocks.NewMockPortService(ctrl)
	s.machineService = mocks.NewMockMachineService(ctrl)
	s.applicationService = mocks.NewMockApplicationService(ctrl)
	s.envFirewaller = mocks.NewMockEnvironFirewaller(ctrl)
	s.envModelFirewaller = mocks.NewMockEnvironModelFirewaller(ctrl)
	s.envInstances = mocks.NewMockEnvironInstances(ctrl)
	s.remoteRelations = mocks.NewMockRemoteRelationsAPI(ctrl)
	s.crossmodelFirewaller = mocks.NewMockCrossModelFirewallerFacadeCloser(ctrl)

	s.machinesCh = make(chan []string, 5)
	s.applicationsCh = make(chan struct{}, 5)
	s.openedPortsCh = make(chan []string, 10)
	s.remoteRelCh = make(chan []string, 5)
	s.subnetsCh = make(chan []string, 5)
	s.modelFwRulesCh = make(chan struct{}, 5)

	// Initial event.
	if s.withModelFirewaller {
		s.modelFwRulesCh <- struct{}{}
	}

	s.AddCleanup(func(_ *tc.C) {
		s.firewaller = nil
		s.portService = nil
		s.machineService = nil
		s.applicationService = nil
		s.envFirewaller = nil
		s.envModelFirewaller = nil
		s.envInstances = nil
		s.crossmodelFirewaller = nil
		s.remoteRelations = nil

		s.machinesCh = nil
		s.applicationsCh = nil
		s.openedPortsCh = nil
		s.remoteRelCh = nil
		s.subnetsCh = nil
		s.modelFwRulesCh = nil
	})
}

// assertIngressRules retrieves the ingress rules from the provided instance
// and compares them to the expected value.
func (s *firewallerBaseSuite) assertIngressRules(c *tc.C, machineId string,
	expected firewall.IngressRules) {
	start := time.Now()
	for {
		s.mu.Lock()
		got := s.instancePorts[machineId]
		if expected.EqualTo(got) {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		if time.Since(start) > coretesting.LongWait {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
		}
		time.Sleep(coretesting.ShortWait)
	}
}

// assertEnvironPorts retrieves the open ports of environment and compares them
// to the expected.
func (s *firewallerBaseSuite) assertEnvironPorts(c *tc.C, expected firewall.IngressRules) {
	start := time.Now()
	for {
		s.mu.Lock()
		got := s.envPorts
		if got.EqualTo(expected) {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		if time.Since(start) > 2*coretesting.LongWait {
			c.Fatalf("timed out: expected %q; got %q", expected, s.envPorts)
		}
		time.Sleep(coretesting.ShortWait)
	}
}

// assertModelIngressRules retrieves the ingress rules from the model firewall
// and compares them to the expected value
func (s *firewallerBaseSuite) assertModelIngressRules(c *tc.C, expected firewall.IngressRules) {
	start := time.Now()
	for {
		s.mu.Lock()
		got := s.envModelPorts
		if got.EqualTo(expected) {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		if time.Since(start) > coretesting.LongWait {
			c.Fatalf("timed out: expected %q; got %q", expected, got)
		}
		time.Sleep(coretesting.ShortWait)
	}
}

func (s *firewallerBaseSuite) waitForMachineFlush(c *tc.C) {
	select {
	case <-s.machineFlushed:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for firewaller worker machine flush")
	}
}

func (s *firewallerBaseSuite) waitForModelFlush(c *tc.C) {
	select {
	case <-s.modelFlushed:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for firewaller worker model flush")
	}
}

func (s *firewallerBaseSuite) waitForSkipModelFlush(c *tc.C) {
	select {
	case <-s.modelFlushSkipped:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for firewaller worker to skip model flush")
	}
}

func (s *firewallerBaseSuite) waitForMachine(c *tc.C, id machine.Name) {
	select {
	case got := <-s.watchingMachine:
		c.Assert(got, tc.Equals, id)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting to watch machine %v", id)
	}
}

func (s *firewallerBaseSuite) addMachine(ctrl *gomock.Controller) (*mocks.MockMachine, chan []string) {
	return s.addModelMachine(ctrl, false)
}

func (s *firewallerBaseSuite) addModelMachine(ctrl *gomock.Controller, manual bool) (*mocks.MockMachine, chan []string) {
	id := strconv.Itoa(s.nextMachineId)
	s.nextMachineId++

	m := mocks.NewMockMachine(ctrl)
	tag := names.NewMachineTag(id)
	machineName := machine.Name(id)
	s.firewaller.EXPECT().Machine(gomock.Any(), tag).Return(m, nil).MinTimes(1)
	m.EXPECT().Tag().Return(tag).AnyTimes()
	m.EXPECT().Life().DoAndReturn(func() life.Value {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.deadMachines.Contains(id) {
			return life.Dead
		}
		return life.Alive
	}).AnyTimes()
	m.EXPECT().IsManual(gomock.Any()).Return(manual, nil).MinTimes(1)

	var unitsCh chan []string
	if !manual {
		// Added machine watches units.
		unitsCh = make(chan []string, 5)
		unitWatch := watchertest.NewMockStringsWatcher(unitsCh)
		s.applicationService.EXPECT().WatchUnitAddRemoveOnMachine(gomock.Any(), machineName).Return(unitWatch, nil).AnyTimes()
		// Initial event.
		unitsCh <- nil
	}

	return m, unitsCh
}

func (s *firewallerBaseSuite) addApplication(ctrl *gomock.Controller, appName string, exposed bool) *mocks.MockApplication {
	app := mocks.NewMockApplication(ctrl)
	appWatch := watchertest.NewMockNotifyWatcher(s.applicationsCh)
	s.applicationService.EXPECT().WatchApplicationExposed(gomock.Any(), appName).Return(appWatch, nil).AnyTimes()
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), appName).Return(exposed, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), appName).Return(map[string]application.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: set.NewStrings(firewall.AllNetworksIPV4CIDR)}}, nil)
	app.EXPECT().Name().Return(appName).AnyTimes()
	app.EXPECT().Tag().Return(names.NewApplicationTag(appName)).AnyTimes()
	return app
}

func (s *firewallerBaseSuite) addUnit(c *tc.C, ctrl *gomock.Controller, app *mocks.MockApplication) (*mocks.MockUnit, *mocks.MockMachine, chan []string) {
	unitId := s.nextUnitId[app.Name()]
	s.nextUnitId[app.Name()] = unitId + 1
	unitName, err := coreunit.NewNameFromParts(app.Name(), unitId)
	c.Assert(err, tc.ErrorIsNil)
	m, unitsCh := s.addMachine(ctrl)
	u := mocks.NewMockUnit(ctrl)
	s.firewaller.EXPECT().Unit(gomock.Any(), names.NewUnitTag(unitName.String())).Return(u, nil).AnyTimes()
	u.EXPECT().Life().Return(life.Alive)
	u.EXPECT().Name().Return(unitName.String()).AnyTimes()
	u.EXPECT().Application().Return(app, nil).AnyTimes()
	s.applicationService.EXPECT().GetUnitMachineName(gomock.Any(), unitName).Return(machine.Name(m.Tag().Id()), nil).AnyTimes()

	machineUUID := coremachinetesting.GenUUID(c)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(m.Tag().Id())).Return(machineUUID, nil).AnyTimes()
	s.portService.EXPECT().GetMachineOpenedPorts(gomock.Any(), machineUUID.String()).DoAndReturn(
		func(ctx context.Context, machineUUID string) (map[coreunit.Name]network.GroupedPortRanges, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			c.Logf("GetMachineOpenedPorts for %q: %v", m.Tag().Id(), s.unitPortRanges.ByUnitEndpoint())
			opened := map[coreunit.Name]network.GroupedPortRanges{}
			// NOTE: unitPortRanges contains all units across machines. Filtering
			// to this unit only is a naive way to filter to the provided machine
			if r, ok := s.unitPortRanges.ByUnitEndpoint()[unitName]; ok {
				opened[unitName] = r
			}
			return opened, nil
		},
	).AnyTimes()

	unitsCh <- []string{unitName.String()}

	return u, m, unitsCh
}

func (s *firewallerBaseSuite) newFirewaller(c *tc.C, ctrl *gomock.Controller) worker.Worker {
	s.modelFlushed = make(chan bool, 1)
	s.modelFlushSkipped = make(chan bool, 1)
	s.machineFlushed = make(chan machine.Name, 1)
	s.watchingMachine = make(chan machine.Name, 1)

	flushMachineNotify := func(id machine.Name) {
		select {
		case s.machineFlushed <- id:
		default:
		}
	}
	flushModelNotify := func() {
		select {
		case s.modelFlushed <- true:
		default:
		}
	}
	skipFlushModelNotify := func() {
		select {
		case s.modelFlushSkipped <- true:
		default:
		}
	}
	watchMachineNotify := func(name machine.Name) {
		select {
		case s.watchingMachine <- name:
		default:
		}
	}

	cfg := firewaller.Config{
		ModelUUID:              coretesting.ModelTag.Id(),
		Mode:                   s.mode,
		EnvironFirewaller:      s.envFirewaller,
		EnvironInstances:       s.envInstances,
		EnvironIPV6CIDRSupport: s.withIpv6,
		FirewallerAPI:          s.firewaller,
		PortsService:           s.portService,
		MachineService:         s.machineService,
		ApplicationService:     s.applicationService,
		RemoteRelationsApi:     s.remoteRelations,
		NewCrossModelFacadeFunc: func(context.Context, *api.Info) (firewaller.CrossModelFirewallerFacadeCloser, error) {
			return s.crossmodelFirewaller, nil
		},
		Clock:                s.clock,
		Logger:               loggertesting.WrapCheckLog(c),
		WatchMachineNotify:   watchMachineNotify,
		FlushModelNotify:     flushModelNotify,
		FlushMachineNotify:   flushMachineNotify,
		SkipFlushModelNotify: skipFlushModelNotify,
	}
	if s.withModelFirewaller {
		cfg.EnvironModelFirewaller = s.envModelFirewaller
	}

	mWatcher := watchertest.NewMockStringsWatcher(s.machinesCh)
	s.firewaller.EXPECT().WatchModelMachines(gomock.Any()).Return(mWatcher, nil)

	opWatcher := watchertest.NewMockStringsWatcher(s.openedPortsCh)
	s.portService.EXPECT().WatchMachineOpenedPorts(gomock.Any()).Return(opWatcher, nil)

	remoteRelWatcher := watchertest.NewMockStringsWatcher(s.remoteRelCh)
	s.remoteRelations.EXPECT().WatchRemoteRelations(gomock.Any()).Return(remoteRelWatcher, nil)

	subnetsWatcher := watchertest.NewMockStringsWatcher(s.subnetsCh)
	s.firewaller.EXPECT().WatchSubnets(gomock.Any()).Return(subnetsWatcher, nil)

	if s.withModelFirewaller {
		fwRulesWatcher := watchertest.NewMockNotifyWatcher(s.modelFwRulesCh)
		s.firewaller.EXPECT().WatchModelFirewallRules(gomock.Any()).Return(fwRulesWatcher, nil)
	}

	initialised := make(chan bool)
	s.firewaller.EXPECT().AllSpaceInfos(gomock.Any()).DoAndReturn(func(context.Context) (network.SpaceInfos, error) {
		defer close(initialised)
		return nil, nil
	})

	s.envFirewaller.EXPECT().OpenPorts(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, rules firewall.IngressRules) error {
		s.mu.Lock()
		defer s.mu.Unlock()

		c.Logf("open global ports: %v\n", rules)
		s.envPorts = openPorts(s.envPorts, rules)
		c.Logf("global ports are now: %v\n", s.envPorts)
		return nil
	}).AnyTimes()

	s.envFirewaller.EXPECT().ClosePorts(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, rules firewall.IngressRules) error {
		s.mu.Lock()
		defer s.mu.Unlock()

		c.Logf("close global ports: %v\n", rules)
		s.envPorts = closePorts(s.envPorts, rules)
		c.Logf("global ports are now: %v\n", s.envPorts)
		return nil
	}).AnyTimes()

	fw, err := firewaller.NewFirewaller(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Log("firewaller worker started")

	select {
	case <-initialised:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for firewaller worker to start")
	}
	s.firewallerStarted = true

	return fw
}

func openPorts(existing, rules firewall.IngressRules) firewall.IngressRules {
	for _, o := range rules {
		found := false
		for i, up := range existing {
			if up.PortRange.String() == o.PortRange.String() {
				up.SourceCIDRs = up.SourceCIDRs.Union(o.SourceCIDRs)
				existing[i] = up
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, o)
		}
	}
	return existing
}

func closePorts(existing, rules firewall.IngressRules) firewall.IngressRules {
	for _, cl := range rules {
		for i, up := range existing {
			if up.EqualTo(cl) {
				existing = append(existing[:i], existing[i+1:]...)
				break
			}
			if up.PortRange.String() == cl.PortRange.String() {
				up.SourceCIDRs = up.SourceCIDRs.Difference(cl.SourceCIDRs)
				existing[i] = up
				if len(up.SourceCIDRs) == 0 {
					existing = append(existing[:i], existing[i+1:]...)
				}
				break
			}
		}
	}
	return existing
}

// startInstance starts a new instance for the given machine.
func (s *firewallerBaseSuite) startInstance(c *tc.C, ctrl *gomock.Controller, m *mocks.MockMachine) *mocks.MockEnvironInstance {
	instId := instance.Id("inst-" + m.Tag().Id())
	m.EXPECT().InstanceId(gomock.Any()).Return(instId, nil).AnyTimes()
	inst := mocks.NewMockEnvironInstance(ctrl)
	s.envInstances.EXPECT().Instances(gomock.Any(), []instance.Id{instId}).Return([]instances.Instance{inst}, nil).AnyTimes()

	inst.EXPECT().OpenPorts(gomock.Any(), m.Tag().Id(), gomock.Any()).DoAndReturn(func(_ context.Context, machineId string, rules firewall.IngressRules) error {
		s.mu.Lock()
		defer s.mu.Unlock()

		c.Logf("open ports for %q: %v\n", instId, rules)
		unitPorts := openPorts(s.instancePorts[machineId], rules)
		c.Logf("ports for %q are now: %v\n", instId, unitPorts)
		s.instancePorts[machineId] = unitPorts
		return nil
	}).AnyTimes()

	inst.EXPECT().ClosePorts(gomock.Any(), m.Tag().Id(), gomock.Any()).DoAndReturn(func(_ context.Context, machineId string, rules firewall.IngressRules) error {
		s.mu.Lock()
		defer s.mu.Unlock()

		c.Logf("close ports for %q: %v\n", instId, rules)
		unitPorts := closePorts(s.instancePorts[machineId], rules)
		c.Logf("ports for %q are now: %v\n", instId, unitPorts)
		s.instancePorts[machineId] = unitPorts
		return nil
	}).AnyTimes()

	// Start the machine.
	s.machinesCh <- []string{m.Tag().Id()}
	if s.firewallerStarted {
		s.waitForMachineFlush(c)
	}

	return inst
}

type InstanceModeSuite struct {
	firewallerBaseSuite
}

func TestInstanceModeSuite(t *testing.T) {
	tc.Run(t, &InstanceModeSuite{})
}

func (s *InstanceModeSuite) SetUpTest(c *tc.C) {
	s.mode = config.FwInstance
	s.firewallerBaseSuite.SetUpTest(c)
}

func (s *InstanceModeSuite) TestStartStop(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	s.waitForMachine(c, "0")
	// Initial event.
	s.waitForModelFlush(c)
}

func (s *InstanceModeSuite) TestStartStopWithoutModelFirewaller(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	s.withModelFirewaller = false
	fw := s.newFirewaller(c, ctrl)

	defer workertest.CleanKill(c, fw)
	s.waitForMachine(c, "0")
}

func (s *InstanceModeSuite) TestNotExposedApplication(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", true)
	_, m, _ := s.addUnit(c, ctrl, app)
	s.machinesCh <- []string{m.Tag().Id()}
	s.waitForMachineFlush(c)
}

func (s *InstanceModeSuite) TestShouldFlushModelWhenFlushingMachine(c *tc.C) {
	c.Skip(c, "This test is flaky, and needs to be fixed. The mock composition is insane.")

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.modelIngressRules = firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("17070"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	}
	s.ensureMocksWithoutMachine(ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	s.waitForSkipModelFlush(c)

	m := s.addMachineUnitAndEnsureMocks(c, ctrl)

	s.waitForMachine(c, machine.Name(m.Tag().Id()))
	s.waitForMachineFlush(c)
	s.waitForModelFlush(c)
}

func (s *InstanceModeSuite) TestNotExposedApplicationWithoutModelFirewaller(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	s.withModelFirewaller = false
	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", false)
	_, m, _ := s.addUnit(c, ctrl, app)
	s.machinesCh <- []string{m.Tag().Id()}
	s.waitForMachineFlush(c)
}

func (s *InstanceModeSuite) TestExposedApplication(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	s.mustClosePortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
	})

	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *InstanceModeSuite) TestMultipleExposedApplications(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app1 := s.addApplication(ctrl, "wordpress", true)
	u1, m1, _ := s.addUnit(c, ctrl, app1)
	s.startInstance(c, ctrl, m1)

	app2 := s.addApplication(ctrl, "mysql", true)
	u2, m2, _ := s.addUnit(c, ctrl, app2)
	s.startInstance(c, ctrl, m2)

	s.mustOpenPortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	s.mustOpenPortRanges(c, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("3306/tcp"),
	})

	s.assertIngressRules(c, m1.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	s.assertIngressRules(c, m2.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("3306/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	s.mustClosePortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	s.mustClosePortRanges(c, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("3306/tcp"),
	})

	s.assertIngressRules(c, m1.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})
	s.assertIngressRules(c, m2.Tag().Id(), nil)
}

func (s *InstanceModeSuite) TestMachineWithoutInstanceId(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", true)
	// add a unit but don't start its instance yet.
	u1, m1, _ := s.addUnit(c, ctrl, app)

	// add another unit and start its instance, so that
	// we're sure the firewaller has seen the first instance.
	u2, m2, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m2)
	s.mustOpenPortRanges(c, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	s.assertIngressRules(c, m2.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	s.startInstance(c, ctrl, m1)
	s.mustOpenPortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("8080/tcp"),
	})
	s.assertIngressRules(c, m1.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *InstanceModeSuite) TestMultipleUnits(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", true)
	u1, m1, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m1)
	s.mustOpenPortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	u2, m2, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m2)
	s.mustOpenPortRanges(c, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.assertIngressRules(c, m1.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
	s.assertIngressRules(c, m1.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	s.mustClosePortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	s.mustClosePortRanges(c, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.assertIngressRules(c, m1.Tag().Id(), nil)
	s.assertIngressRules(c, m1.Tag().Id(), nil)
}

func (s *InstanceModeSuite) TestStartWithState(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	// Nothing open without firewaller.
	s.assertIngressRules(c, m.Tag().Id(), nil)

	// Starting the firewaller opens the ports.
	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *InstanceModeSuite) TestStartWithPartialState(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	app := s.addApplication(ctrl, "wordpress", true)

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	s.assertIngressRules(c, "1", nil)

	// Complete steps to open port.
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)
	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *InstanceModeSuite) TestStartWithUnexposedApplication(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	app := s.addApplication(ctrl, "wordpress", false)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	s.assertIngressRules(c, m.Tag().Id(), nil)

	// Expose service.
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(true, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: set.NewStrings(firewall.AllNetworksIPV4CIDR)}}, nil)
	s.applicationsCh <- struct{}{}

	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *InstanceModeSuite) TestStartMachineWithManualMachine(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	// Wait for controller (started by setUpTest)
	s.waitForMachine(c, "0")

	m, _ := s.addModelMachine(ctrl, true)
	s.machinesCh <- []string{m.Tag().Id()}

	select {
	case tag := <-s.watchingMachine:
		c.Fatalf("shouldn't be watching manual machine %v", tag)
	case <-time.After(coretesting.ShortWait):
	}

	m, _ = s.addMachine(ctrl)
	s.machinesCh <- []string{m.Tag().Id()}
	s.waitForMachine(c, machine.Name(m.Tag().Id()))
}

func (s *InstanceModeSuite) addMachineUnitAndEnsureMocks(c *tc.C, ctrl *gomock.Controller) *mocks.MockMachine {
	// Create a new machine.
	id := strconv.Itoa(s.nextMachineId)
	s.nextMachineId++

	m := mocks.NewMockMachine(ctrl)
	tag := names.NewMachineTag(id)
	s.firewaller.EXPECT().Machine(gomock.Any(), machine.Name(m.Tag().Id())).Return(m, nil).MinTimes(1)
	m.EXPECT().Tag().Return(tag).MinTimes(1)
	m.EXPECT().Life().DoAndReturn(func() life.Value {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.deadMachines.Contains(id) {
			return life.Dead
		}
		return life.Alive
	}).MinTimes(1)
	m.EXPECT().IsManual(gomock.Any()).Return(false, nil).MinTimes(1)

	s.firewaller.EXPECT().ModelFirewallRules(gomock.Any()).MinTimes(1).MaxTimes(2).DoAndReturn(func(_ context.Context) (firewall.IngressRules, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.modelIngressRules, nil
	})
	s.envModelFirewaller.EXPECT().ModelIngressRules(gomock.Any()).MinTimes(1).MaxTimes(2).DoAndReturn(func(_ context.Context) (firewall.IngressRules, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.envModelPorts, nil
	})
	s.envModelFirewaller.EXPECT().OpenModelPorts(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, rules firewall.IngressRules) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		add, _ := s.envModelPorts.Diff(rules)
		s.envModelPorts = append(s.envModelPorts, add...)
		return nil
	}).Times(1)

	instId := instance.Id("inst-" + m.Tag().Id())
	m.EXPECT().InstanceId(gomock.Any()).Return(instId, nil).AnyTimes()
	inst := mocks.NewMockEnvironInstance(ctrl)
	s.envInstances.EXPECT().Instances(gomock.Any(), []instance.Id{instId}).Return([]instances.Instance{inst}, nil).Times(1)
	inst.EXPECT().IngressRules(gomock.Any(), m.Tag().Id()).Return(nil, nil).Times(1)

	s.machinesCh <- []string{tag.Id()}

	return m
}

func (s *InstanceModeSuite) TestSetClearExposedApplication(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", false)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)
	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	// Not exposed application, so no open port.
	s.assertIngressRules(c, m.Tag().Id(), nil)

	// Expose service.
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(true, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: set.NewStrings(firewall.AllNetworksIPV4CIDR)}}, nil)
	s.applicationsCh <- struct{}{}

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	}
	s.assertIngressRules(c, m.Tag().Id(), rules)

	// ClearExposed closes the ports again.
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(false, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: set.NewStrings(firewall.AllNetworksIPV4CIDR)}}, nil)
	s.applicationsCh <- struct{}{}

	s.assertIngressRules(c, m.Tag().Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveUnit(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", true)
	u1, m1, unitsCh := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m1)

	s.mustOpenPortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	u2, m2, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m2)
	s.mustOpenPortRanges(c, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.assertIngressRules(c, m1.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
	s.assertIngressRules(c, m2.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Remove unit.
	u1.EXPECT().Life().Return(life.Dead)
	unitsCh <- []string{u1.Name()}

	s.assertIngressRules(c, m1.Tag().Id(), nil)
	s.assertIngressRules(c, m2.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *InstanceModeSuite) TestRemoveApplication(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, unitsCh := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	rules1 := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	}
	s.assertIngressRules(c, m.Tag().Id(), rules1)

	// Remove application.
	u.EXPECT().Life().Return(life.Dead)
	unitsCh <- []string{u.Name()}

	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").
		Return(false, errors.NotFoundf("wordpress")).MaxTimes(1)
	s.applicationsCh <- struct{}{}

	s.waitForMachineFlush(c)
	s.assertIngressRules(c, m.Tag().Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveMultipleApplications(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app1 := s.addApplication(ctrl, "wordpress", true)
	u1, m1, unitsCh1 := s.addUnit(c, ctrl, app1)
	s.startInstance(c, ctrl, m1)
	s.mustOpenPortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	app2 := s.addApplication(ctrl, "mysql", true)
	u2, m2, unitsCh2 := s.addUnit(c, ctrl, app2)
	s.startInstance(c, ctrl, m2)
	s.mustOpenPortRanges(c, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("3306/tcp"),
	})

	rules1 := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	}
	s.assertIngressRules(c, m1.Tag().Id(), rules1)
	rules2 := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("3306/tcp"), firewall.AllNetworksIPV4CIDR),
	}
	s.assertIngressRules(c, m2.Tag().Id(), rules2)

	// Remove applications.
	u1.EXPECT().Life().Return(life.Dead)
	unitsCh1 <- []string{u1.Name()}

	removed1 := make(chan bool)
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").
		DoAndReturn(func(context.Context, string) (bool, error) {
			defer close(removed1)
			return false, errors.NotFoundf(app1.Name())
		})
	s.applicationsCh <- struct{}{}

	u2.EXPECT().Life().Return(life.Dead)
	unitsCh2 <- []string{u2.Name()}

	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "mysql").
		Return(false, errors.NotFoundf(app2.Name())).MaxTimes(1)
	s.applicationsCh <- struct{}{}

	s.assertIngressRules(c, m1.Tag().Id(), nil)
	s.assertIngressRules(c, m2.Tag().Id(), nil)

	select {
	case <-removed1:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for app1 removal")
	}
	s.waitForMachineFlush(c)
}

func (s *InstanceModeSuite) TestDeadMachine(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, unitsCh := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	rules1 := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	}
	s.assertIngressRules(c, m.Tag().Id(), rules1)

	// Remove unit and application, also tested without. Has no effect.
	u.EXPECT().Life().Return(life.Dead).AnyTimes()
	unitsCh <- []string{u.Name()}

	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").
		Return(false, errors.NotFoundf("wordpress")).MaxTimes(1)
	s.applicationsCh <- struct{}{}
	s.waitForMachineFlush(c)

	// Kill machine.
	s.mu.Lock()
	s.deadMachines.Add(m.Tag().Id())
	s.mu.Unlock()
	s.machinesCh <- []string{m.Tag().Id()}
	s.waitForMachineFlush(c)

	s.assertIngressRules(c, m.Tag().Id(), nil)
}

func (s *InstanceModeSuite) TestRemoveMachine(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.DirtyKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)
	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	rules1 := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	}
	s.assertIngressRules(c, m.Tag().Id(), rules1)

	// Kill machine.
	s.deadMachines.Add(m.Tag().Id())
	s.machinesCh <- []string{m.Tag().Id()}
	s.waitForMachineFlush(c)

	// TODO (manadart 2019-02-01): This fails intermittently with a "not found"
	// error for the machine. This is not a huge problem in production, as the
	// worker will restart and proceed happily thereafter.
	// That error is detected here for expediency, but the ideal mitigation is
	// a refactoring of the worker logic as per LP:1814277.
	fw.Kill()
	err := fw.Wait()
	c.Assert(err == nil || params.IsCodeNotFound(err), tc.IsTrue)
}

func (s *InstanceModeSuite) TestStartWithStateOpenPortsBroken(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, _ := s.addUnit(c, ctrl, app)

	instId := instance.Id("inst-" + m.Tag().Id())
	m.EXPECT().InstanceId(gomock.Any()).Return(instId, nil).AnyTimes()
	inst := mocks.NewMockEnvironInstance(ctrl)
	s.envInstances.EXPECT().Instances(gomock.Any(), []instance.Id{instId}).Return([]instances.Instance{inst}, nil).AnyTimes()
	s.machinesCh <- []string{m.Tag().Id()}

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	called := make(chan bool)
	inst.EXPECT().OpenPorts(gomock.Any(), m.Tag().Id(), gomock.Any()).DoAndReturn(func(_ context.Context, machineId string, rules firewall.IngressRules) error {
		defer close(called)
		return errors.New("open ports is broken")
	})
	s.openedPortsCh <- []string{m.Tag().Id()}

	// Nothing open without firewaller.
	s.assertIngressRules(c, m.Tag().Id(), nil)

	// Starting the firewaller should attempt to open the ports,
	// and fail due to the method being broken.
	// Starting the firewaller opens the ports.
	fw := s.newFirewaller(c, ctrl)
	defer workertest.DirtyKill(c, fw)

	errc := make(chan error, 1)
	go func() { errc <- fw.Wait() }()
	select {
	case err := <-errc:
		c.Assert(err, tc.ErrorMatches, "open ports is broken")
	case <-time.After(coretesting.LongWait):
		fw.Kill()
		fw.Wait()
		c.Fatal("timed out waiting for firewaller to stop")
	}
}

func (s *InstanceModeSuite) TestDefaultModelFirewall(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.modelIngressRules = firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("17070"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	}

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	s.waitForModelFlush(c)
	s.assertModelIngressRules(c, s.modelIngressRules)
}

func (s *InstanceModeSuite) TestShouldSkipFlushModelWhenNoMachines(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocksWithoutMachine(ctrl)

	fw := s.newFirewaller(c, ctrl)
	workertest.CleanKill(c, fw)
}

func (s *InstanceModeSuite) TestConfigureModelFirewall(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.modelIngressRules = firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("17070"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	}

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	s.assertModelIngressRules(c, s.modelIngressRules)

	s.modelIngressRules = append(s.modelIngressRules,
		firewall.NewIngressRule(network.MustParsePortRange("666"), "192.168.0.0/24"))

	s.modelFwRulesCh <- struct{}{}

	s.assertModelIngressRules(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("666"), "192.168.0.0/24"),
		firewall.NewIngressRule(network.MustParsePortRange("22"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("17070"), firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR),
	})
}

func (s *InstanceModeSuite) setupRemoteRelationRequirerRoleConsumingSide(c *tc.C) (chan []string, *macaroon.Macaroon) {
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	s.remoteRelations.EXPECT().Relations(gomock.Any(), []string{"wordpress:db remote-mysql:server"}).Return(
		[]params.RemoteRelationResult{{
			Result: &params.RemoteRelation{
				Life:            "alive",
				Suspended:       false,
				Id:              666,
				Key:             "wordpress:db remote-mysql:server",
				ApplicationName: "wordpress",
				Endpoint: params.RemoteEndpoint{
					Role: "requirer",
				},
				UnitCount:             2,
				RemoteApplicationName: "remote-mysql",
				RemoteEndpointName:    "server",
				SourceModelUUID:       coretesting.ModelTag.Id(),
			},
		}}, nil).MinTimes(1)
	s.remoteRelations.EXPECT().RemoteApplications(gomock.Any(), []string{"remote-mysql"}).Return(
		[]params.RemoteApplicationResult{{
			Result: &params.RemoteApplication{
				Name:            "remote-mysql",
				OfferUUID:       "offer-uuid",
				Life:            "alive",
				Status:          "active",
				ModelUUID:       coretesting.ModelTag.Id(),
				IsConsumerProxy: false,
				ConsumeVersion:  66,
				Macaroon:        mac,
			},
		}}, nil).MinTimes(1)
	relTag := names.NewRelationTag("wordpress:db remote-mysql:server")
	s.remoteRelations.EXPECT().GetToken(gomock.Any(), relTag).Return("rel-token", nil).MinTimes(1)

	s.firewaller.EXPECT().ControllerAPIInfoForModel(gomock.Any(), coretesting.ModelTag.Id()).Return(
		&api.Info{
			Addrs:  []string{"1.2.3.4:1234"},
			CACert: coretesting.CACert,
		}, nil).AnyTimes()
	s.firewaller.EXPECT().MacaroonForRelation(gomock.Any(), relTag.Id()).Return(mac, nil).MinTimes(1)

	localEgressCh := make(chan []string, 1)
	remoteEgressWatch := watchertest.NewMockStringsWatcher(localEgressCh)
	s.firewaller.EXPECT().WatchEgressAddressesForRelation(gomock.Any(), relTag).Return(remoteEgressWatch, nil).MinTimes(1)
	s.crossmodelFirewaller.EXPECT().Close().Return(nil).MinTimes(1)

	s.remoteRelCh <- []string{"wordpress:db remote-mysql:server"}

	return localEgressCh, mac
}

func (s *InstanceModeSuite) TestRemoteRelationRequirerRoleConsumingSide(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	// Create the firewaller facade on the consuming model.
	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	published := make(chan bool)
	app := s.addApplication(ctrl, "wordpress", true)
	_, m, _ := s.addUnit(c, ctrl, app)
	s.machinesCh <- []string{m.Tag().Id()}
	relSubnetCh, mac := s.setupRemoteRelationRequirerRoleConsumingSide(c)

	// Have a unit on the consuming app enter the relation scope.
	// This will trigger the firewaller to publish the changes.
	event := params.IngressNetworksChangeEvent{
		RelationToken:   "rel-token",
		Networks:        []string{"10.0.0.0/24"},
		IngressRequired: true,
		Macaroons:       macaroon.Slice{mac},
		BakeryVersion:   bakery.LatestVersion,
	}
	s.crossmodelFirewaller.EXPECT().PublishIngressNetworkChange(gomock.Any(), event).DoAndReturn(func(_ context.Context, _ params.IngressNetworksChangeEvent) error {
		published <- true
		return nil
	})

	relSubnetCh <- []string{"10.0.0.0/24"}

	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on enter scope")
	case <-published:
	}

	// Trigger watcher for unit on the consuming app (leave the relation scope).
	event.IngressRequired = false
	event.Networks = []string{}
	s.crossmodelFirewaller.EXPECT().PublishIngressNetworkChange(gomock.Any(), event).DoAndReturn(func(_ context.Context, _ params.IngressNetworksChangeEvent) error {
		published <- true
		return nil
	})

	relSubnetCh <- []string{}

	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on leave scope")
	case <-published:
	}
}

func (s *InstanceModeSuite) TestRemoteRelationWorkerError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	// Create the firewaller facade on the consuming model.
	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	published := make(chan bool)
	app := s.addApplication(ctrl, "wordpress", true)
	_, m, _ := s.addUnit(c, ctrl, app)
	s.machinesCh <- []string{m.Tag().Id()}
	relSubnetCh, mac := s.setupRemoteRelationRequirerRoleConsumingSide(c)

	// Have a unit on the consuming app enter the relation scope.
	event := params.IngressNetworksChangeEvent{
		RelationToken:   "rel-token",
		Networks:        []string{"10.0.0.0/24"},
		IngressRequired: true,
		Macaroons:       macaroon.Slice{mac},
		BakeryVersion:   bakery.LatestVersion,
	}
	s.crossmodelFirewaller.EXPECT().PublishIngressNetworkChange(gomock.Any(), event).DoAndReturn(func(_ context.Context, _ params.IngressNetworksChangeEvent) error {
		published <- true
		return errors.New("fail")
	})

	relSubnetCh <- []string{"10.0.0.0/24"}

	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on enter scope")
	case <-published:
	}

	s.crossmodelFirewaller.EXPECT().PublishIngressNetworkChange(gomock.Any(), event).DoAndReturn(func(_ context.Context, _ params.IngressNetworksChangeEvent) error {
		published <- true
		return nil
	})

	relSubnetCh <- []string{"10.0.0.0/24"}

	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for ingress change to be published on enter scope")
	case <-published:
	}
}

func (s *InstanceModeSuite) TestRemoteRelationProviderRoleConsumingSide(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	// Create the firewaller facade on the consuming model.
	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "mysql", true)
	_, m, _ := s.addUnit(c, ctrl, app)
	s.machinesCh <- []string{m.Tag().Id()}

	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	s.remoteRelations.EXPECT().Relations(gomock.Any(), []string{"remote-wordpress:db mysql:server"}).Return(
		[]params.RemoteRelationResult{{
			Result: &params.RemoteRelation{
				Life:            "alive",
				Suspended:       false,
				Id:              666,
				Key:             "remote-wordpress:db mysql:server",
				ApplicationName: "mysql",
				Endpoint: params.RemoteEndpoint{
					Role: "provider",
				},
				UnitCount:             2,
				RemoteApplicationName: "remote-wordpress",
				RemoteEndpointName:    "db",
				SourceModelUUID:       coretesting.ModelTag.Id(),
			},
		}}, nil)
	s.remoteRelations.EXPECT().RemoteApplications(gomock.Any(), []string{"remote-wordpress"}).Return(
		[]params.RemoteApplicationResult{{
			Result: &params.RemoteApplication{
				Name:            "remote-wordpress",
				OfferUUID:       "offer-uuid",
				Life:            "alive",
				Status:          "active",
				ModelUUID:       coretesting.ModelTag.Id(),
				IsConsumerProxy: false,
				ConsumeVersion:  66,
				Macaroon:        mac,
			},
		}}, nil)
	relTag := names.NewRelationTag("remote-wordpress:db mysql:server")
	s.remoteRelations.EXPECT().GetToken(gomock.Any(), relTag).Return("rel-token", nil)

	s.firewaller.EXPECT().ControllerAPIInfoForModel(gomock.Any(), coretesting.ModelTag.Id()).Return(
		&api.Info{
			Addrs:  []string{"1.2.3.4:1234"},
			CACert: coretesting.CACert,
		}, nil).AnyTimes()
	s.firewaller.EXPECT().MacaroonForRelation(gomock.Any(), relTag.Id()).Return(mac, nil)

	watched := make(chan bool, 2)

	localEgressCh := make(chan []string, 1)
	remoteEgressWatch := watchertest.NewMockStringsWatcher(localEgressCh)
	arg := params.RemoteEntityArg{
		Token:     "rel-token",
		Macaroons: macaroon.Slice{mac},
	}
	s.crossmodelFirewaller.EXPECT().WatchEgressAddressesForRelation(gomock.Any(), arg).DoAndReturn(func(_ context.Context, _ params.RemoteEntityArg) (watcher.StringsWatcher, error) {
		watched <- true
		return remoteEgressWatch, nil
	})
	s.crossmodelFirewaller.EXPECT().Close().AnyTimes()

	s.remoteRelCh <- []string{"remote-wordpress:db mysql:server"}
	localEgressCh <- []string{"10.0.0.0/24"}

	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for watcher call")
	case <-watched:
	}
}

func (s *InstanceModeSuite) TestRemoteRelationIngressRejected(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	// Create the firewaller facade on the consuming model.
	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "mysql", true)
	_, m, _ := s.addUnit(c, ctrl, app)
	s.machinesCh <- []string{m.Tag().Id()}

	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	s.remoteRelations.EXPECT().Relations(gomock.Any(), []string{"wordpress:db remote-mysql:server"}).Return(
		[]params.RemoteRelationResult{{
			Result: &params.RemoteRelation{
				Life:            "alive",
				Suspended:       false,
				Id:              666,
				Key:             "wordpress:db remote-mysql:server",
				ApplicationName: "wordpress",
				Endpoint: params.RemoteEndpoint{
					Role: "requirer",
				},
				UnitCount:             2,
				RemoteApplicationName: "remote-mysql",
				RemoteEndpointName:    "server",
				SourceModelUUID:       coretesting.ModelTag.Id(),
			},
		}}, nil).MinTimes(1)
	s.remoteRelations.EXPECT().RemoteApplications(gomock.Any(), []string{"remote-mysql"}).Return(
		[]params.RemoteApplicationResult{{
			Result: &params.RemoteApplication{
				Name:            "remote-mysql",
				OfferUUID:       "offer-uuid",
				Life:            "alive",
				Status:          "active",
				ModelUUID:       coretesting.ModelTag.Id(),
				IsConsumerProxy: false,
				ConsumeVersion:  66,
				Macaroon:        mac,
			},
		}}, nil).MinTimes(1)
	relTag := names.NewRelationTag("wordpress:db remote-mysql:server")
	s.remoteRelations.EXPECT().GetToken(gomock.Any(), relTag).Return("rel-token", nil).MinTimes(1)

	s.firewaller.EXPECT().ControllerAPIInfoForModel(gomock.Any(), coretesting.ModelTag.Id()).Return(
		&api.Info{
			Addrs:  []string{"1.2.3.4:1234"},
			CACert: coretesting.CACert,
		}, nil).AnyTimes()
	s.firewaller.EXPECT().MacaroonForRelation(gomock.Any(), relTag.Id()).Return(mac, nil).MinTimes(1)

	localEgressCh := make(chan []string, 1)
	remoteEgressWatch := watchertest.NewMockStringsWatcher(localEgressCh)
	s.firewaller.EXPECT().WatchEgressAddressesForRelation(gomock.Any(), relTag).Return(remoteEgressWatch, nil).MinTimes(1)
	s.crossmodelFirewaller.EXPECT().Close().Return(nil).MinTimes(1)

	s.remoteRelCh <- []string{"wordpress:db remote-mysql:server"}

	updated := make(chan bool)

	// Have a unit on the consuming app enter the relation scope.
	// This will trigger the firewaller to publish the changes.
	event := params.IngressNetworksChangeEvent{
		RelationToken:   "rel-token",
		Networks:        []string{"10.0.0.0/24"},
		IngressRequired: true,
		Macaroons:       macaroon.Slice{mac},
		BakeryVersion:   bakery.LatestVersion,
	}
	s.crossmodelFirewaller.EXPECT().PublishIngressNetworkChange(gomock.Any(), event).DoAndReturn(func(_ context.Context, _ params.IngressNetworksChangeEvent) error {
		return &params.Error{Code: params.CodeForbidden, Message: "error"}
	})
	s.firewaller.EXPECT().SetRelationStatus(gomock.Any(), relTag.Id(), relation.Error, "error").DoAndReturn(func(context.Context, string, relation.Status, string) error {
		updated <- true
		return nil
	})

	localEgressCh <- []string{"10.0.0.0/24"}

	select {
	case <-time.After(coretesting.LongWait):
		c.Fatal("time out waiting for relation to be updated")
	case <-updated:
	}
}

func (s *InstanceModeSuite) assertIngressCidrs(c *tc.C, ctrl *gomock.Controller, ingress []string, expected []string) {
	// Create the firewaller facade on the offering model.
	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	// Set up the offering model - create the local app.
	app := s.addApplication(ctrl, "mysql", false)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("3306/tcp"),
	})

	// Set up the offering model - create the remote app.
	mac, err := jujutesting.NewMacaroon("id")
	c.Assert(err, tc.ErrorIsNil)
	remoteRelParams := params.RemoteRelation{
		Life:            "alive",
		Suspended:       false,
		Id:              666,
		Key:             "remote-wordpress:db mysql:server",
		ApplicationName: "mysql",
		Endpoint: params.RemoteEndpoint{
			Role: "provider",
		},
		UnitCount:             2,
		RemoteApplicationName: "remote-wordpress",
		RemoteEndpointName:    "db",
		SourceModelUUID:       coretesting.ModelTag.Id(),
	}
	s.remoteRelations.EXPECT().Relations(gomock.Any(), []string{"remote-wordpress:db mysql:server"}).Return(
		[]params.RemoteRelationResult{{Result: &remoteRelParams}}, nil)
	s.remoteRelations.EXPECT().RemoteApplications(gomock.Any(), []string{"remote-wordpress"}).Return(
		[]params.RemoteApplicationResult{{
			Result: &params.RemoteApplication{
				Name:            "remote-wordpress",
				OfferUUID:       "offer-uuid",
				Life:            "alive",
				Status:          "active",
				ModelUUID:       coretesting.ModelTag.Id(),
				IsConsumerProxy: true,
				ConsumeVersion:  66,
				Macaroon:        mac,
			},
		}}, nil).MinTimes(1)
	relTag := names.NewRelationTag("remote-wordpress:db mysql:server")
	s.remoteRelations.EXPECT().GetToken(gomock.Any(), relTag).Return("rel-token", nil).MinTimes(1)

	localIngressCh := make(chan []string, 1)
	remoteIngressWatch := watchertest.NewMockStringsWatcher(localIngressCh)
	s.firewaller.EXPECT().WatchIngressAddressesForRelation(gomock.Any(), relTag).Return(remoteIngressWatch, nil).MinTimes(1)

	// No port changes yet.
	s.waitForMachineFlush(c)
	s.assertIngressRules(c, m.Tag().Id(), nil)

	// Save a new ingress network against the relation.
	s.remoteRelCh <- []string{"remote-wordpress:db mysql:server"}
	localIngressCh <- ingress

	// Ports opened.
	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("3306/tcp"), expected...),
	})

	// Change should be sent when ingress networks disappear.
	localIngressCh <- nil
	s.assertIngressRules(c, m.Tag().Id(), nil)

	localIngressCh <- ingress
	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("3306/tcp"), expected...),
	})

	// And again when relation is suspended.
	remoteRelParams.Suspended = true
	s.remoteRelations.EXPECT().Relations(gomock.Any(), []string{"remote-wordpress:db mysql:server"}).Return(
		[]params.RemoteRelationResult{{Result: &remoteRelParams}}, nil)
	s.remoteRelCh <- []string{"remote-wordpress:db mysql:server"}
	s.assertIngressRules(c, m.Tag().Id(), nil)

	// And again when relation is resumed.
	remoteRelParams.Suspended = false
	s.remoteRelations.EXPECT().Relations(gomock.Any(), []string{"remote-wordpress:db mysql:server"}).Return(
		[]params.RemoteRelationResult{{Result: &remoteRelParams}}, nil)
	s.remoteRelCh <- []string{"remote-wordpress:db mysql:server"}
	localIngressCh <- ingress
	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("3306/tcp"), expected...),
	})

	// And again when relation is destroyed.
	localIngressCh <- nil
	s.waitForMachineFlush(c)
	s.assertIngressRules(c, m.Tag().Id(), nil)
	s.remoteRelations.EXPECT().Relations(gomock.Any(), []string{"remote-wordpress:db mysql:server"}).Return(
		[]params.RemoteRelationResult{{Error: &params.Error{Code: params.CodeNotFound}}}, nil)
	s.remoteRelCh <- []string{"remote-wordpress:db mysql:server"}
}

func (s *InstanceModeSuite) TestRemoteRelationProviderRoleOffering(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	s.assertIngressCidrs(c, ctrl, []string{"10.0.0.4/16"}, []string{"10.0.0.4/16"})
}

func (s *InstanceModeSuite) TestRemoteRelationIngressFallbackToWhitelist(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	attr := map[string]interface{}{
		"name":               "name",
		"uuid":               coretesting.ModelTag.Id(),
		"type":               "foo",
		"saas-ingress-allow": "192.168.1.0/16",
	}
	cfg, err := config.New(config.UseDefaults, attr)
	c.Assert(err, tc.ErrorIsNil)
	s.firewaller.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil).AnyTimes()
	var ingress []string
	for i := 1; i < 30; i++ {
		ingress = append(ingress, fmt.Sprintf("10.%d.0.1/32", i))
	}
	s.assertIngressCidrs(c, ctrl, ingress, []string{"192.168.1.0/16"})
}

func (s *InstanceModeSuite) TestRemoteRelationIngressMergesCIDRS(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	ingress := []string{
		"192.0.1.254/31",
		"192.0.2.0/28",
		"192.0.2.16/28",
		"192.0.2.32/28",
		"192.0.2.48/28",
		"192.0.2.64/28",
		"192.0.2.80/28",
		"192.0.2.96/28",
		"192.0.2.112/28",
		"192.0.2.128/28",
		"192.0.2.144/28",
		"192.0.2.160/28",
		"192.0.2.176/28",
		"192.0.2.192/28",
		"192.0.2.208/28",
		"192.0.2.224/28",
		"192.0.2.240/28",
		"192.0.3.0/28",
		"192.0.4.0/28",
		"192.0.5.0/28",
		"192.0.6.0/28",
	}
	expected := []string{
		"192.0.1.254/31",
		"192.0.2.0/24",
		"192.0.3.0/28",
		"192.0.4.0/28",
		"192.0.5.0/28",
		"192.0.6.0/28",
	}
	s.assertIngressCidrs(c, ctrl, ingress, expected)
}

func (s *InstanceModeSuite) TestExposedApplicationWithExposedEndpoints(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	// Create a space with a single subnet.
	s.firewaller.EXPECT().AllSpaceInfos(gomock.Any()).Return(network.SpaceInfos{{
		ID:   "sp-1",
		Name: "myspace",
		Subnets: network.SubnetInfos{{
			ID:        "subnet-1",
			CIDR:      "42.42.0.0/16",
			SpaceID:   "sp-1",
			SpaceName: "myspace",
		}},
	}}, nil)

	s.subnetsCh <- []string{}

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	s.mustOpenPortRanges(c, u, "url", []network.PortRange{
		network.MustParsePortRange("1337/tcp"),
		network.MustParsePortRange("1337/udp"),
	})

	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(true, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {
			ExposeToCIDRs: set.NewStrings("10.0.0.0/24"),
		},
		"url": {
			ExposeToCIDRs:    set.NewStrings("192.168.0.0/24", "192.168.1.0/24"),
			ExposeToSpaceIDs: set.NewStrings("sp-1"),
		},
	}, nil)

	s.applicationsCh <- struct{}{}

	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		// We have opened port 80 for ALL endpoints (including "url"),
		// then exposed ALL endpoints to 10.0.0.0/24 and the "url"
		// endpoint to 192.168.{0,1}.0/24 and 42.42.0.0/16 (subnet
		// of space-1).
		//
		// We expect to see port 80 use all three CIDRs as valid input sources
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24", "192.168.0.0/24", "192.168.1.0/24",
			"42.42.0.0/16"),
		//
		// The 1337/{tcp,udp} ports have only been opened for the "url"
		// endpoint and the "url" endpoint has been exposed to 192.168.{0,1}.0/24
		// and 42.42.0.0/16 (the subnet for space-1).
		//
		// The ports should only be reachable from these CIDRs. Note
		// that the expose for the wildcard ("") endpoint is ignored
		// here as the expose settings for the "url" endpoint must
		// supersede it.
		firewall.NewIngressRule(network.MustParsePortRange("1337/tcp"), "192.168.0.0/24", "192.168.1.0/24",
			"42.42.0.0/16"),
		firewall.NewIngressRule(network.MustParsePortRange("1337/udp"), "192.168.0.0/24", "192.168.1.0/24",
			"42.42.0.0/16"),
	})

	// Change the expose settings and remove the entry for the wildcard endpoint
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(true, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		"url": {
			ExposeToCIDRs:    set.NewStrings("192.168.0.0/24", "192.168.1.0/24"),
			ExposeToSpaceIDs: set.NewStrings("sp-1"),
		},
	}, nil)
	s.applicationsCh <- struct{}{}

	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		// We unexposed the wildcard endpoint so only the "url" endpoint
		// remains exposed. This endpoint has ports 1337/{tcp,udp}
		// explicitly open as well as port 80 which is opened for ALL
		// endpoints. These three ports should be exposed to the
		// CIDRs used when the "url" endpoint was exposed
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24", "192.168.1.0/24",
			"42.42.0.0/16"),
		firewall.NewIngressRule(network.MustParsePortRange("1337/tcp"), "192.168.0.0/24", "192.168.1.0/24",
			"42.42.0.0/16"),
		firewall.NewIngressRule(network.MustParsePortRange("1337/udp"), "192.168.0.0/24", "192.168.1.0/24",
			"42.42.0.0/16"),
	})
}

func (s *InstanceModeSuite) TestExposedApplicationWithExposedEndpointsWhenSpaceTopologyChanges(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	// Create two spaces and add a subnet to each one
	s.firewaller.EXPECT().AllSpaceInfos(gomock.Any()).Return(network.SpaceInfos{{
		ID:   "sp-1",
		Name: "myspace",
		Subnets: network.SubnetInfos{{
			ID:        "subnet-1",
			CIDR:      "192.168.0.0/24",
			SpaceID:   "sp-1",
			SpaceName: "myspace",
		}},
	}, {
		ID:   "sp-2",
		Name: "myspace2",
		Subnets: network.SubnetInfos{{
			ID:        "subnet-2",
			CIDR:      "192.168.1.0/24",
			SpaceID:   "sp-2",
			SpaceName: "myspace2",
		}},
	}}, nil)

	s.subnetsCh <- []string{}

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	// Expose app to space-1
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(true, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {
			ExposeToSpaceIDs: set.NewStrings("sp-1"),
		},
	}, nil)

	s.applicationsCh <- struct{}{}

	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		// We expect to see port 80 use the subnet-1 CIDR
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24"),
	})

	// Trigger a space topology change by moving subnet-2 into space 1
	s.firewaller.EXPECT().AllSpaceInfos(gomock.Any()).Return(network.SpaceInfos{{
		ID:   "sp-1",
		Name: "myspace",
		Subnets: network.SubnetInfos{{
			ID:        "subnet-1",
			CIDR:      "192.168.0.0/24",
			SpaceID:   "sp-1",
			SpaceName: "myspace",
		}, {
			ID:        "subnet-2",
			CIDR:      "192.168.1.0/24",
			SpaceID:   "sp-1",
			SpaceName: "myspace2",
		}},
	}, {
		ID:      "sp-2",
		Name:    "myspace2",
		Subnets: network.SubnetInfos{}},
	}, nil)

	s.subnetsCh <- []string{}

	// Check that worker picked up the change and updated the rules
	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		// We expect to see port 80 use subnet-{1,2} CIDRs
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24", "192.168.1.0/24"),
	})
}

func (s *InstanceModeSuite) TestExposedApplicationWithExposedEndpointsWhenSpaceDeleted(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	// Create two spaces and add a subnet to each one
	s.firewaller.EXPECT().AllSpaceInfos(gomock.Any()).Return(network.SpaceInfos{{
		ID:   "sp-1",
		Name: "myspace",
		Subnets: network.SubnetInfos{{
			ID:        "subnet-1",
			CIDR:      "192.168.0.0/24",
			SpaceID:   "sp-1",
			SpaceName: "myspace",
		}},
	}, {
		ID:   "sp-2",
		Name: "myspace2",
		Subnets: network.SubnetInfos{{
			ID:        "subnet-2",
			CIDR:      "192.168.1.0/24",
			SpaceID:   "sp-2",
			SpaceName: "myspace2",
		}},
	}}, nil)

	s.subnetsCh <- []string{}

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	// Expose app to space-1
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(true, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {
			ExposeToSpaceIDs: set.NewStrings("sp-1"),
		},
	}, nil)

	s.applicationsCh <- struct{}{}

	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		// We expect to see port 80 use the subnet-1 CIDR
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24"),
	})

	// Simulate the deletion of a space, with subnets moving back to alpha.
	s.firewaller.EXPECT().AllSpaceInfos(gomock.Any()).Return(network.SpaceInfos{{
		ID:   "sp-2",
		Name: "myspace2",
		Subnets: network.SubnetInfos{{
			ID:        "subnet-2",
			CIDR:      "192.168.1.0/24",
			SpaceID:   "sp-2",
			SpaceName: "myspace2",
		}},
	}}, nil)

	s.subnetsCh <- []string{}

	// We expect to see NO ingress rules as the referenced space does not exist.
	s.assertIngressRules(c, m.Tag().Id(), nil)
}

func (s *InstanceModeSuite) TestExposedApplicationWithExposedEndpointsWhenSpaceHasNoSubnets(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	// Create a space with a single subnet.
	s.firewaller.EXPECT().AllSpaceInfos(gomock.Any()).Return(network.SpaceInfos{{
		ID:   "sp-1",
		Name: "myspace",
		Subnets: network.SubnetInfos{{
			ID:        "subnet-1",
			CIDR:      "192.168.0.0/24",
			SpaceID:   "sp-1",
			SpaceName: "myspace",
		}},
	}}, nil)

	s.subnetsCh <- []string{}

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	s.mustOpenPortRanges(c, u, "url", []network.PortRange{
		network.MustParsePortRange("1337/tcp"),
	})

	// Expose app to space-1.
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(true, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {ExposeToSpaceIDs: set.NewStrings("sp-1")},
		"url":        {ExposeToSpaceIDs: set.NewStrings("sp-1")},
	}, nil)

	s.applicationsCh <- struct{}{}

	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		// We expect to see port 80 and 1337 use the subnet-1 CIDR
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "192.168.0.0/24"),
		firewall.NewIngressRule(network.MustParsePortRange("1337/tcp"), "192.168.0.0/24"),
	})

	// Move endpoint back to alpha space. This will leave space-1 with no
	// endpoints.
	s.firewaller.EXPECT().AllSpaceInfos(gomock.Any()).Return(network.SpaceInfos{{
		ID:      "sp-1",
		Name:    "myspace",
		Subnets: network.SubnetInfos{},
	}}, nil)

	s.subnetsCh <- []string{}

	// We expect to see NO ingress rules (and warnings in the logs) as
	// there are no CIDRs to access the exposed application.
	s.assertIngressRules(c, m.Tag().Id(), nil)
}

func (s *InstanceModeSuite) TestExposeToIPV6CIDRsOnIPV4OnlyProvider(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	s.withIpv6 = false
	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	// Expose app to space-1.
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(true, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "2002::1234:abcd:ffff:c0a8:101/64")},
	}, nil)

	s.applicationsCh <- struct{}{}

	// Since the provider only supports IPV4 CIDRs, the firewall worker
	// will filter the IPV6 CIDRs when opening ports.
	s.assertIngressRules(c, m.Tag().Id(), firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24"),
	})
}

type GlobalModeSuite struct {
	firewallerBaseSuite
}

func TestGlobalModeSuite(t *testing.T) {
	tc.Run(t, &GlobalModeSuite{})
}

func (s *GlobalModeSuite) SetUpTest(c *tc.C) {
	s.mode = config.FwGlobal
	s.firewallerBaseSuite.SetUpTest(c)
}

func (s *GlobalModeSuite) TestStartStop(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	s.waitForMachine(c, "0")
	// Initial event.
	s.waitForModelFlush(c)
}

func (s *GlobalModeSuite) TestGlobalMode(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app1 := s.addApplication(ctrl, "wordpress", true)
	u1, m1, _ := s.addUnit(c, ctrl, app1)
	s.startInstance(c, ctrl, m1)

	s.mustOpenPortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	app2 := s.addApplication(ctrl, "mysql", true)
	u2, m2, _ := s.addUnit(c, ctrl, app2)
	s.startInstance(c, ctrl, m2)

	s.mustOpenPortRanges(c, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
	})

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Closing a port opened by a different unit won't touch the environment.
	s.mustClosePortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
	})
	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Closing a port used just once changes the environment.
	s.mustClosePortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("8080/tcp"),
	})
	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Closing the last port also modifies the environment.
	s.mustClosePortRanges(c, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
	})
	s.assertEnvironPorts(c, nil)
}

func (s *GlobalModeSuite) TestStartWithUnexposedApplication(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	app := s.addApplication(ctrl, "wordpress", false)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	// Starting the firewaller, no open ports.
	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	s.assertEnvironPorts(c, nil)

	// Expose service.
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(true, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: set.NewStrings(firewall.AllNetworksIPV4CIDR)}}, nil)
	s.applicationsCh <- struct{}{}

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *GlobalModeSuite) TestRestart(c *tc.C) {
	// Start firewaller and open ports.
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, unitsCh := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Stop firewaller and close one and open a different port.
	err := worker.Stop(fw)
	c.Assert(err, tc.ErrorIsNil)
	s.firewallerStarted = false

	s.mustClosePortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("8080/tcp"),
	})
	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("8888/tcp"),
	})

	// Start firewaller and check port.
	u.EXPECT().Life().Return(life.Alive)
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(true, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {
			ExposeToCIDRs: set.NewStrings(firewall.AllNetworksIPV4CIDR),
		},
	}, nil)

	fw = s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	s.machinesCh <- []string{m.Tag().Id()}
	unitsCh <- []string{u.Name()}

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8888/tcp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *GlobalModeSuite) TestRestartUnexposedApplication(c *tc.C) {
	// Start firewaller and open ports.
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, unitsCh := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80-90/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-90/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Stop firewaller and clear exposed flag on application.
	err := worker.Stop(fw)
	c.Assert(err, tc.ErrorIsNil)
	s.firewallerStarted = false

	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(false, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: set.NewStrings(firewall.AllNetworksIPV4CIDR)}}, nil)

	// Start firewaller and check port.
	u.EXPECT().Life().Return(life.Alive)

	fw = s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	s.machinesCh <- []string{m.Tag().Id()}
	unitsCh <- []string{u.Name()}

	s.assertEnvironPorts(c, nil)
}

func (s *GlobalModeSuite) TestRestartPortCount(c *tc.C) {
	// Start firewaller and open ports.
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	fw := s.newFirewaller(c, ctrl)
	defer workertest.DirtyKill(c, fw)

	app1 := s.addApplication(ctrl, "wordpress", true)
	u1, m1, unitsCh1 := s.addUnit(c, ctrl, app1)
	s.startInstance(c, ctrl, m1)

	s.mustOpenPortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
		network.MustParsePortRange("8080/tcp"),
	})

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Stop firewaller and add another application using the port.
	err := worker.Stop(fw)
	c.Assert(err, tc.ErrorIsNil)

	app2 := s.addApplication(ctrl, "mysql", true)
	u2, m2, unitsCh2 := s.addUnit(c, ctrl, app2)
	s.startInstance(c, ctrl, m2)
	s.mustOpenPortRanges(c, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(true, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: set.NewStrings(firewall.AllNetworksIPV4CIDR)}}, nil)
	u1.EXPECT().Life().Return(life.Alive)
	u2.EXPECT().Life().Return(life.Alive)

	// Start firewaller and check port.
	fw2 := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw2)

	s.machinesCh <- []string{m1.Tag().Id(), m2.Tag().Id()}
	unitsCh1 <- []string{u1.Name()}
	unitsCh2 <- []string{u2.Name()}

	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Closing a port opened by a different unit won't touch the environment.
	s.mustClosePortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("8080/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Closing a port used just once changes the environment.
	s.mustClosePortRanges(c, u1, allEndpoints, []network.PortRange{
		network.MustParsePortRange("8080/tcp"),
	})
	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
	})

	// Closing the last port also modifies the environment.
	s.mustClosePortRanges(c, u2, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})
	s.assertEnvironPorts(c, nil)
}

func (s *GlobalModeSuite) TestExposeToIPV6CIDRsOnIPV4OnlyProvider(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.ensureMocks(c, ctrl)

	s.withIpv6 = false
	fw := s.newFirewaller(c, ctrl)
	defer workertest.CleanKill(c, fw)

	app := s.addApplication(ctrl, "wordpress", true)
	u, m, _ := s.addUnit(c, ctrl, app)
	s.startInstance(c, ctrl, m)

	s.mustOpenPortRanges(c, u, allEndpoints, []network.PortRange{
		network.MustParsePortRange("80/tcp"),
	})

	// Expose app to space-1.
	s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), "wordpress").Return(true, nil)
	s.applicationService.EXPECT().GetExposedEndpoints(gomock.Any(), "wordpress").Return(map[string]application.ExposedEndpoint{
		allEndpoints: {ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "2002::1234:abcd:ffff:c0a8:101/64")},
	}, nil)

	s.applicationsCh <- struct{}{}

	// Since the provider only supports IPV4 CIDRs, the firewall worker
	// will filter the IPV6 CIDRs when opening ports.
	s.assertEnvironPorts(c, firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp"), "10.0.0.0/24"),
	})
}

type NoneModeSuite struct {
	firewallerBaseSuite
}

func TestNoneModeSuite(t *testing.T) {
	tc.Run(t, &NoneModeSuite{})
}

func (s *NoneModeSuite) TestStopImmediately(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	cfg := firewaller.Config{
		ModelUUID:              coretesting.ModelTag.Id(),
		Mode:                   config.FwNone,
		EnvironFirewaller:      s.envFirewaller,
		EnvironInstances:       s.envInstances,
		EnvironIPV6CIDRSupport: s.withIpv6,
		FirewallerAPI:          s.firewaller,
		PortsService:           s.portService,
		MachineService:         s.machineService,
		ApplicationService:     s.applicationService,
		RemoteRelationsApi:     s.remoteRelations,
		NewCrossModelFacadeFunc: func(context.Context, *api.Info) (firewaller.CrossModelFirewallerFacadeCloser, error) {
			return s.crossmodelFirewaller, nil
		},
		Clock:  s.clock,
		Logger: loggertesting.WrapCheckLog(c),
	}

	fw, err := firewaller.NewFirewaller(cfg)
	defer workertest.CheckNilOrKill(c, fw)
	c.Assert(err, tc.ErrorMatches, `invalid firewall-mode "none"`)
}

func (s *firewallerBaseSuite) mustOpenPortRanges(c *tc.C, u *mocks.MockUnit, endpointName string, portRanges []network.PortRange) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pr := range portRanges {
		s.unitPortRanges.Open(endpointName, pr)
	}
	op := newUnitPortRangesCommit(s.unitPortRanges, coreunit.Name(u.Name()))
	modified, err := op.Commit()
	c.Assert(err, tc.ErrorIsNil)
	if !modified {
		return
	}

	machineName, err := s.applicationService.GetUnitMachineName(c.Context(), coreunit.Name(u.Name()))
	c.Assert(err, tc.ErrorIsNil)

	if s.firewallerStarted {
		s.openedPortsCh <- []string{machineName.String()}
	}
}

func (s *firewallerBaseSuite) mustClosePortRanges(c *tc.C, u *mocks.MockUnit, endpointName string, portRanges []network.PortRange) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pr := range portRanges {
		s.unitPortRanges.Close(endpointName, pr)
	}
	op := newUnitPortRangesCommit(s.unitPortRanges, coreunit.Name(u.Name()))
	modified, err := op.Commit()
	c.Assert(err, tc.ErrorIsNil)
	if !modified {
		return
	}

	machineName, err := s.applicationService.GetUnitMachineName(c.Context(), coreunit.Name(u.Name()))
	c.Assert(err, tc.ErrorIsNil)

	if s.firewallerStarted {
		s.openedPortsCh <- []string{machineName.String()}
	}
}
