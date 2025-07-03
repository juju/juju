// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"fmt"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	corewatcher "github.com/juju/juju/core/watcher"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/instancepoller/mocks"
)

func TestConfigSuite(t *testing.T) { tc.Run(t, &configSuite{}) }
func TestPollGroupEntrySuite(t *testing.T) {
	tc.Run(t, &pollGroupEntrySuite{})
}

func TestWorkerSuite(t *testing.T) { tc.Run(t, &workerSuite{}) }

var (
	testNetIfs = network.InterfaceInfos{
		{
			DeviceIndex:   0,
			InterfaceName: "eth0",
			MACAddress:    "de:ad:be:ef:00:00",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress(
					"10.0.0.1", network.WithCIDR("10.0.0.0/24"), network.WithScope(network.ScopeCloudLocal),
				).AsProviderAddress(),
			},
			ShadowAddresses: network.ProviderAddresses{
				network.NewMachineAddress(
					"1.1.1.42", network.WithCIDR("1.1.1.0/24"), network.WithScope(network.ScopePublic),
				).AsProviderAddress(),
			},
		},
	}

	testDevices = []domainnetwork.NetInterface{
		{
			Name:        "eth0",
			MACAddress:  ptr("de:ad:be:ef:00:00"),
			IsAutoStart: true,
			IsEnabled:   true,
			Addrs: []domainnetwork.NetAddr{
				{
					InterfaceName: "eth0",
					AddressValue:  "10.0.0.1",
					AddressType:   network.IPv4Address,
					Origin:        network.OriginProvider,
					Scope:         network.ScopeCloudLocal,
				},
				{
					InterfaceName: "eth0",
					AddressValue:  "1.1.1.42",
					AddressType:   network.IPv4Address,
					Origin:        network.OriginProvider,
					Scope:         network.ScopePublic,
					IsShadow:      true,
				},
			},
		},
	}
)

type configSuite struct{}

func (s *configSuite) TestConfigValidation(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	origCfg := Config{
		Clock:          testclock.NewClock(time.Now()),
		MachineService: mocks.NewMockMachineService(ctrl),
		StatusService:  mocks.NewMockStatusService(ctrl),
		NetworkService: mocks.NewMockNetworkService(ctrl),
		Environ:        mocks.NewMockEnviron(ctrl),
		Logger:         loggertesting.WrapCheckLog(c),
	}
	c.Assert(origCfg.Validate(), tc.ErrorIsNil)

	testCfg := origCfg
	testCfg.Clock = nil
	c.Assert(testCfg.Validate(), tc.ErrorMatches, "nil clock.Clock.*")

	testCfg = origCfg
	testCfg.MachineService = nil
	c.Assert(testCfg.Validate(), tc.ErrorMatches, "nil MachineService.*")

	testCfg = origCfg
	testCfg.StatusService = nil
	c.Assert(testCfg.Validate(), tc.ErrorMatches, "nil StatusService.*")

	testCfg = origCfg
	testCfg.NetworkService = nil
	c.Assert(testCfg.Validate(), tc.ErrorMatches, "nil NetworkService.*")

	testCfg = origCfg
	testCfg.Environ = nil
	c.Assert(testCfg.Validate(), tc.ErrorMatches, "nil Environ.*")

	testCfg = origCfg
	testCfg.Logger = nil
	c.Assert(testCfg.Validate(), tc.ErrorMatches, "nil Logger.*")
}

type pollGroupEntrySuite struct{}

func (s *pollGroupEntrySuite) TestShortPollIntervalLogic(c *tc.C) {
	clock := testclock.NewClock(time.Now())
	entry := new(pollGroupEntry)

	// Test reset logic.
	entry.resetShortPollInterval(clock)
	c.Assert(entry.shortPollInterval, tc.Equals, ShortPoll)
	c.Assert(entry.shortPollAt, tc.Equals, clock.Now().Add(ShortPoll))

	// Ensure that bumping the short poll duration caps when we reach the
	// LongPoll interval.
	for i := 0; entry.shortPollInterval < LongPoll && i < 100; i++ {
		entry.bumpShortPollInterval(clock)
	}
	c.Assert(entry.shortPollInterval, tc.Equals, ShortPollCap, tc.Commentf(
		"short poll interval did not reach short poll cap interval after 100 interval bumps"))

	// Check that once we reach the short poll cap interval we stay capped at it.
	entry.bumpShortPollInterval(clock)
	c.Assert(entry.shortPollInterval, tc.Equals, ShortPollCap, tc.Commentf(
		"short poll should have been capped at the short poll cap interval"))
	c.Assert(entry.shortPollAt, tc.Equals, clock.Now().Add(ShortPollCap))
}

type workerSuite struct {
	watcherChangeCh chan []string
}

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.watcherChangeCh = make(chan []string)

	c.Cleanup(func() {
		close(s.watcherChangeCh)
		s.watcherChangeCh = nil
	})
}

func (s *workerSuite) TestQueueingNewMachineAddsItToShortPollGroup(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	// Instance poller will look up machine with id "0" and get back a
	// non-manual machine.
	machineName := machine.Name("0")
	mocked.machineService.EXPECT().IsMachineManuallyProvisioned(gomock.Any(), machineName).Return(false, nil)

	// Queue machine.
	err := updWorker.queueMachineForPolling(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 1, tc.Commentf("machine didn't end up in short poll group"))
}

func (s *workerSuite) TestQueueingExistingMachineAlwaysMovesItToShortPollGroup(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	machineName := machine.Name("0")
	mocked.machineService.EXPECT().GetMachineLife(gomock.Any(), machineName).Return(life.Alive, nil)
	updWorker.appendToShortPollGroup(machineName)

	// Manually move entry to long poll group.
	entry, _ := updWorker.lookupPolledMachine(machineName)
	entry.shortPollInterval = LongPoll
	updWorker.pollGroup[longPollGroup][machineName] = entry
	delete(updWorker.pollGroup[shortPollGroup], machineName)

	// Queue machine.
	err := updWorker.queueMachineForPolling(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 1, tc.Commentf("machine didn't end up in short poll group"))
	c.Assert(entry.shortPollInterval, tc.Equals, ShortPoll, tc.Commentf("poll interval was not reset"))
}

func (s *workerSuite) TestUpdateOfStatusAndAddressDetails(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	// Start with an entry for machine "0"
	machineUUID := machinetesting.GenUUID(c)
	machineName := machine.Name("0")
	entry := &pollGroupEntry{
		machineName: machineName,
		instanceID:  "b4dc0ffee",
	}

	// The machine is alive, has an instance status of "provisioning" and
	// is aware of its network addresses.
	mocked.machineService.EXPECT().GetMachineLife(gomock.Any(), machineName).Return(life.Alive, nil)
	mocked.statusService.EXPECT().GetInstanceStatus(gomock.Any(), machineName).Return(status.StatusInfo{Status: status.Provisioning}, nil)

	// The provider reports the instance status as running and also indicates
	// that network addresses have been *changed*.
	instInfo := mocks.NewMockInstance(ctrl)
	instInfo.EXPECT().Status(gomock.Any()).Return(instance.Status{Status: status.Running, Message: "Running wild"})

	// When we process the instance info we expect the machine instance
	// status and list of network addresses to be updated so they match
	// the values reported by the provider.
	mocked.statusService.EXPECT().SetInstanceStatus(gomock.Any(), machineName, status.StatusInfo{
		Status:  status.Running,
		Message: "Running wild",
	}).Return(nil)

	mocked.machineService.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, nil)
	mocked.networkService.EXPECT().SetProviderNetConfig(gomock.Any(), machineUUID, testDevices).Return(nil)

	providerStatus, err := updWorker.processProviderInfo(c.Context(), entry, instInfo, testNetIfs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(providerStatus, tc.Equals, status.Running)
}

func (s *workerSuite) TestStartedMachineWithNetAddressesMovesToLongPollGroup(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, _ := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	// Start with machine "0" in the short poll group.
	machineName := machine.Name("0")

	updWorker.appendToShortPollGroup(machineName)
	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 1)

	// The provider reports an instance status of "running"; the machine
	// reports it's machine status as "started".
	entry, _ := updWorker.lookupPolledMachine(machineName)
	updWorker.maybeSwitchPollGroup(c.Context(), shortPollGroup, entry, status.Running, status.Started, 1)

	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 0)
	c.Assert(updWorker.pollGroup[longPollGroup], tc.HasLen, 1)
}

func (s *workerSuite) TestNonStartedMachinesGetBumpedPollInterval(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, _ := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	specs := []status.Status{status.Allocating, status.Pending}
	for specIndex, spec := range specs {
		c.Logf("provider reports instance status as: %q", spec)
		machineName := machine.Name(fmt.Sprint(specIndex))
		updWorker.appendToShortPollGroup(machineName)
		entry, _ := updWorker.lookupPolledMachine(machineName)

		updWorker.maybeSwitchPollGroup(c.Context(), shortPollGroup, entry, spec, status.Pending, 0)
		c.Assert(entry.shortPollInterval, tc.Equals, time.Duration(float64(ShortPoll)*ShortPollBackoff))
	}
}

func (s *workerSuite) TestMoveMachineWithUnknownStatusBackToShortPollGroup(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, _ := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	// The machine is assigned a network address.
	machineName := machine.Name("0")

	// Move the machine to the long poll group.
	updWorker.appendToShortPollGroup(machineName)
	entry, _ := updWorker.lookupPolledMachine(machineName)
	updWorker.maybeSwitchPollGroup(c.Context(), shortPollGroup, entry, status.Running, status.Started, 1)
	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 0)
	c.Assert(updWorker.pollGroup[longPollGroup], tc.HasLen, 1)

	// If we get unknown status from the provider we expect the machine to
	// be moved back to the short poll group.
	updWorker.maybeSwitchPollGroup(c.Context(), longPollGroup, entry, status.Unknown, status.Started, 1)
	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 1)
	c.Assert(updWorker.pollGroup[longPollGroup], tc.HasLen, 0)
	c.Assert(entry.shortPollInterval, tc.Equals, ShortPoll)
}

func (s *workerSuite) TestSkipMachineIfShortPollTargetTimeNotElapsed(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	machineName := machine.Name("0")

	// Add machine to short poll group and bump its poll interval
	updWorker.appendToShortPollGroup(machineName)
	entry, _ := updWorker.lookupPolledMachine(machineName)
	entry.bumpShortPollInterval(mocked.clock)
	pollAt := entry.shortPollAt

	// Advance the clock to trigger processing of the short poll groups
	// but not far enough to process the entry with the bumped interval.
	s.assertWorkerCompletesLoop(c, updWorker, func() {
		mocked.clock.Advance(ShortPoll)
	})

	c.Assert(pollAt, tc.Equals, entry.shortPollAt, tc.Commentf("machine shouldn't have been polled"))
}

func (s *workerSuite) TestDeadMachineGetsRemoved(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	machineName := machine.Name("0")

	// Add machine to short poll group
	updWorker.appendToShortPollGroup(machineName)
	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 1)

	mocked.machineService.EXPECT().GetMachineLife(gomock.Any(), machineName).Return(life.Dead, nil)

	// Emit a change for the machine so the queueing code detects the
	// dead machine and removes it.
	s.assertWorkerCompletesLoop(c, updWorker, func() {
		s.assertEnqueueChange(c, []string{"0"})
	})

	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 0, tc.Commentf("dead machine has not been removed"))
}

func (s *workerSuite) TestReapedMachineIsTreatedAsDeadAndRemoved(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	machineName := machine.Name("0")

	// Add machine to short poll group
	updWorker.appendToShortPollGroup(machineName)
	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 1)

	mocked.machineService.EXPECT().GetMachineLife(gomock.Any(), machineName).Return("", machineerrors.MachineNotFound)

	// Emit a change for the machine so the queueing code detects the
	// dead machine and removes it.
	s.assertWorkerCompletesLoop(c, updWorker, func() {
		s.assertEnqueueChange(c, []string{"0"})
	})

	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 0, tc.Commentf("dead machine has not been removed"))
}

func (s *workerSuite) TestQueuingOfManualMachines(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	now := mocked.clock.Now()

	// Add two manual machines, one with "provisioning" status and one with
	// "started" status. We expect the former to have its instance status
	// changed to "running".
	machineName0 := machine.Name("0")
	mocked.machineService.EXPECT().IsMachineManuallyProvisioned(gomock.Any(), machineName0).Return(true, nil)
	mocked.statusService.EXPECT().GetInstanceStatus(gomock.Any(), machineName0).Return(status.StatusInfo{Status: status.Provisioning}, nil)
	mocked.statusService.EXPECT().SetInstanceStatus(gomock.Any(), machineName0, status.StatusInfo{
		Status:  status.Running,
		Message: "Manually provisioned machine",
		Since:   &now,
	}).Return(nil)

	machineName1 := machine.Name("1")
	mocked.machineService.EXPECT().IsMachineManuallyProvisioned(gomock.Any(), machineName1).Return(true, nil)
	mocked.statusService.EXPECT().GetInstanceStatus(gomock.Any(), machineName1).Return(status.StatusInfo{Status: status.Running}, nil)

	// Emit change for both machines.
	s.assertWorkerCompletesLoop(c, updWorker, func() {
		s.assertEnqueueChange(c, []string{"0", "1"})
	})

	// None of the machines should have been added.
	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 0)
	c.Assert(updWorker.pollGroup[longPollGroup], tc.HasLen, 0)
}

func (s *workerSuite) TestBatchPollingOfGroupMembers(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	// Add two machines, one that is not yet provisioned and one that is
	// has a "created" machine status and a "running" instance status.
	machineName0 := machine.Name("0")
	mocked.machineService.EXPECT().GetInstanceIDByMachineName(gomock.Any(), machineName0).Return(instance.Id(""), machineerrors.NotProvisioned)
	updWorker.appendToShortPollGroup(machineName0)

	machineUUID1 := machinetesting.GenUUID(c)
	machineName1 := machine.Name("1")
	mocked.machineService.EXPECT().GetMachineLife(gomock.Any(), machineName1).Return(life.Alive, nil)
	mocked.machineService.EXPECT().GetInstanceIDByMachineName(gomock.Any(), machineName1).Return(instance.Id("b4dc0ffee"), nil)
	mocked.statusService.EXPECT().GetInstanceStatus(gomock.Any(), machineName1).Return(status.StatusInfo{Status: status.Running}, nil)
	mocked.statusService.EXPECT().GetMachineStatus(gomock.Any(), machineName1).Return(status.StatusInfo{Status: status.Started}, nil)
	mocked.machineService.EXPECT().GetMachineUUID(gomock.Any(), machineName1).Return(machineUUID1, nil)
	mocked.networkService.EXPECT().SetProviderNetConfig(gomock.Any(), machineUUID1, testDevices).Return(nil)
	updWorker.appendToShortPollGroup(machineName1)

	machine1Info := mocks.NewMockInstance(ctrl)
	machine1Info.EXPECT().Status(gomock.Any()).Return(instance.Status{Status: status.Running})
	mocked.environ.EXPECT().Instances(gomock.Any(), []instance.Id{"b4dc0ffee"}).Return([]instances.Instance{machine1Info}, nil)
	mocked.environ.EXPECT().NetworkInterfaces(gomock.Any(), []instance.Id{"b4dc0ffee"}).Return(
		[]network.InterfaceInfos{testNetIfs},
		nil,
	)

	// Trigger a poll of the short poll group and wait for the worker loop
	// to complete.
	s.assertWorkerCompletesLoop(c, updWorker, func() {
		mocked.clock.Advance(ShortPoll)
	})
}

func (s *workerSuite) TestLongPollMachineNotKnownByProvider(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	machineName := machine.Name("0")

	// Add machine to short poll group and manually move it to long poll group.
	updWorker.appendToShortPollGroup(machineName)
	entry, _ := updWorker.lookupPolledMachine(machineName)
	updWorker.pollGroup[longPollGroup][machineName] = entry
	delete(updWorker.pollGroup[shortPollGroup], machineName)

	// Allow instance ID to be resolved but have the provider's Instances
	// call fail with a partial instance list.
	instID := instance.Id("d3adc0de")
	mocked.machineService.EXPECT().GetInstanceIDByMachineName(gomock.Any(), machineName).Return(instID, nil)
	mocked.environ.EXPECT().Instances(gomock.Any(), []instance.Id{instID}).Return(
		[]instances.Instance{}, environs.ErrPartialInstances,
	)
	mocked.environ.EXPECT().NetworkInterfaces(gomock.Any(), []instance.Id{instID}).Return(
		nil, nil,
	)

	// Advance the clock to trigger processing of both the short AND long
	// poll groups. This should trigger to full loop runs.
	s.assertWorkerCompletesLoops(c, updWorker, 2, func() {
		mocked.clock.Advance(LongPoll)
	})
}

func (s *workerSuite) TestShortPollMachineNotKnownByProviderIntervalBackoff(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	mahcineName := machine.Name("0")

	updWorker.appendToShortPollGroup(mahcineName)

	// Allow instance ID to be resolved but have the provider's Instances
	// call fail with a partial instance list.
	instID := instance.Id("d3adc0de")
	mocked.machineService.EXPECT().GetInstanceIDByMachineName(gomock.Any(), mahcineName).Return(instID, nil)
	mocked.environ.EXPECT().Instances(gomock.Any(), []instance.Id{instID}).Return(
		[]instances.Instance{nil}, environs.ErrPartialInstances,
	)
	mocked.environ.EXPECT().NetworkInterfaces(gomock.Any(), []instance.Id{instID}).Return(
		nil, nil,
	)

	// Advance the clock to trigger processing of the short poll group.
	s.assertWorkerCompletesLoops(c, updWorker, 1, func() {
		mocked.clock.Advance(ShortPoll)
	})

	// Check that we have backed off the poll interval.
	entry, _ := updWorker.lookupPolledMachine(mahcineName)
	c.Assert(entry.shortPollInterval, tc.Equals, time.Duration(float64(ShortPoll)*ShortPollBackoff))
}

func (s *workerSuite) TestLongPollNoMachineInGroupKnownByProvider(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	machineName := machine.Name("0")

	// Add machine to short poll group and manually move it to long poll group.
	updWorker.appendToShortPollGroup(machineName)
	entry, _ := updWorker.lookupPolledMachine(machineName)
	updWorker.pollGroup[longPollGroup][machineName] = entry
	delete(updWorker.pollGroup[shortPollGroup], machineName)

	// Allow instance ID to be resolved but have the provider's Instances
	// call fail with ErrNoInstances. This is probably rare but can happen
	// and shouldn't cause the worker to exit with an error!
	instID := instance.Id("d3adc0de")
	mocked.machineService.EXPECT().GetInstanceIDByMachineName(gomock.Any(), machineName).Return(instID, nil)
	mocked.environ.EXPECT().Instances(gomock.Any(), []instance.Id{instID}).Return(
		nil, environs.ErrNoInstances,
	)

	// Advance the clock to trigger processing of both the short AND long
	// poll groups. This should trigger to full loop runs.
	s.assertWorkerCompletesLoops(c, updWorker, 2, func() {
		mocked.clock.Advance(LongPoll)
	})
}

func (s *workerSuite) TestShortPollNoMachineInGroupKnownByProviderIntervalBackoff(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	machineName := machine.Name("0")

	// Add machine to short poll group and manually move it to long poll group.
	updWorker.appendToShortPollGroup(machineName)

	// Allow instance ID to be resolved but have the provider's Instances
	// call fail with ErrNoInstances. This is probably rare but can happen
	// and shouldn't cause the worker to exit with an error!
	instID := instance.Id("d3adc0de")
	mocked.machineService.EXPECT().GetInstanceIDByMachineName(gomock.Any(), machineName).Return(instID, nil)
	mocked.environ.EXPECT().Instances(gomock.Any(), []instance.Id{instID}).Return(
		nil, environs.ErrNoInstances,
	)

	// Advance the clock to trigger processing of the short poll group.
	s.assertWorkerCompletesLoops(c, updWorker, 1, func() {
		mocked.clock.Advance(ShortPoll)
	})

	// Check that we have backed off the poll interval.
	entry, _ := updWorker.lookupPolledMachine(machineName)
	c.Assert(entry.shortPollInterval, tc.Equals, time.Duration(float64(ShortPoll)*ShortPollBackoff))
}

func (s *workerSuite) assertWorkerCompletesLoop(c *tc.C, w *updaterWorker, triggerFn func()) {
	s.assertWorkerCompletesLoops(c, w, 1, triggerFn)
}

func (s *workerSuite) assertWorkerCompletesLoops(c *tc.C, w *updaterWorker, numLoops int, triggerFn func()) {
	ch := make(chan struct{})
	defer func() { w.loopCompletedHook = nil }()

	w.loopCompletedHook = func() { ch <- struct{}{} }
	triggerFn()

	for loop := 0; loop < numLoops; loop++ {
		select {
		case <-ch: // loop completed
		case <-time.After(coretesting.ShortWait):
			c.Fatal("timed out waiting for instance poller to complete a full loop")
		}
	}
}

type workerMocks struct {
	clock          *testclock.Clock
	machineService *mocks.MockMachineService
	statusService  *mocks.MockStatusService
	networkService *mocks.MockNetworkService
	environ        *mocks.MockEnviron
}

func (s *workerSuite) startWorker(c *tc.C, ctrl *gomock.Controller) (worker.Worker, workerMocks) {
	mocked := workerMocks{
		clock:          testclock.NewClock(time.Now()),
		machineService: mocks.NewMockMachineService(ctrl),
		statusService:  mocks.NewMockStatusService(ctrl),
		networkService: mocks.NewMockNetworkService(ctrl),
		environ:        mocks.NewMockEnviron(ctrl),
	}

	workerMainLoopEnteredCh := make(chan struct{}, 1)
	watcher := mocks.NewMockStringsWatcher(ctrl)
	mocked.machineService.EXPECT().WatchModelMachineLifeAndStartTimes(gomock.Any()).Return(watcher, nil)
	watcher.EXPECT().Changes().DoAndReturn(func() corewatcher.StringsChannel {
		select {
		case workerMainLoopEnteredCh <- struct{}{}:
		default:
		}
		return s.watcherChangeCh
	}).AnyTimes()
	watcher.EXPECT().Kill().AnyTimes()
	watcher.EXPECT().Wait().AnyTimes()

	w, err := NewWorker(Config{
		Clock:          mocked.clock,
		MachineService: mocked.machineService,
		StatusService:  mocked.statusService,
		NetworkService: mocked.networkService,
		Environ:        mocked.environ,
		Logger:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)

	// Wait for worker to reach main loop before we allow tests to
	// manipulate the clock.
	select {
	case <-workerMainLoopEnteredCh:
	case <-time.After(coretesting.ShortWait):
		c.Fatal("timed out wating for worker to enter main loop")
	}

	return w, mocked
}

func (s *workerSuite) assertEnqueueChange(c *tc.C, values []string) {
	select {
	case s.watcherChangeCh <- values:
	case <-time.After(coretesting.ShortWait):
		c.Fatal("timed out waiting for worker to pick up change")
	}
}
