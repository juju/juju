// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/instancepoller/mocks"
	"github.com/juju/juju/rpc/params"
)

var (
	_ = tc.Suite(&configSuite{})
	_ = tc.Suite(&pollGroupEntrySuite{})
	_ = tc.Suite(&workerSuite{})

	testAddrs = network.ProviderAddresses{
		network.NewMachineAddress(
			"10.0.0.1", network.WithCIDR("10.0.0.0/24"), network.WithScope(network.ScopeCloudLocal),
		).AsProviderAddress(),
		network.NewMachineAddress(
			"1.1.1.42", network.WithCIDR("1.1.1.0/24"), network.WithScope(network.ScopePublic),
		).AsProviderAddress(),
	}

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
)

type configSuite struct{}

func (s *configSuite) TestConfigValidation(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	origCfg := Config{
		Clock:   testclock.NewClock(time.Now()),
		Facade:  newMockFacadeAPI(ctrl, nil),
		Environ: mocks.NewMockEnviron(ctrl),
		Logger:  loggertesting.WrapCheckLog(c),
	}
	c.Assert(origCfg.Validate(), jc.ErrorIsNil)

	testCfg := origCfg
	testCfg.Clock = nil
	c.Assert(testCfg.Validate(), tc.ErrorMatches, "nil clock.Clock.*")

	testCfg = origCfg
	testCfg.Facade = nil
	c.Assert(testCfg.Validate(), tc.ErrorMatches, "nil Facade.*")

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

type workerSuite struct{}

func (s *workerSuite) TestQueueingNewMachineAddsItToShortPollGroup(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	// Instance poller will look up machine with id "0" and get back a
	// non-manual machine.
	machineTag := names.NewMachineTag("0")
	nonManualMachine := mocks.NewMockMachine(ctrl)
	nonManualMachine.EXPECT().IsManual(gomock.Any()).Return(false, nil)
	mocked.facadeAPI.addMachine(machineTag, nonManualMachine)

	// Queue machine.
	err := updWorker.queueMachineForPolling(context.Background(), machineTag)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 1, tc.Commentf("machine didn't end up in short poll group"))
}

func (s *workerSuite) TestQueueingExistingMachineAlwaysMovesItToShortPollGroup(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, _ := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	machineTag := names.NewMachineTag("0")
	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().Refresh(gomock.Any()).Return(nil)
	machine.EXPECT().Life().Return(life.Alive)
	machine.EXPECT().String().Return("machine-0").AnyTimes()
	updWorker.appendToShortPollGroup(machineTag, machine)

	// Manually move entry to long poll group.
	entry, _ := updWorker.lookupPolledMachine(machineTag)
	entry.shortPollInterval = LongPoll
	updWorker.pollGroup[longPollGroup][machineTag] = entry
	delete(updWorker.pollGroup[shortPollGroup], machineTag)

	// Queue machine.
	err := updWorker.queueMachineForPolling(context.Background(), machineTag)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 1, tc.Commentf("machine didn't end up in short poll group"))
	c.Assert(entry.shortPollInterval, tc.Equals, ShortPoll, tc.Commentf("poll interval was not reset"))
}

func (s *workerSuite) TestUpdateOfStatusAndAddressDetails(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, _ := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	// Start with an entry for machine "0"
	machineTag := names.NewMachineTag("0")
	machine := mocks.NewMockMachine(ctrl)
	entry := &pollGroupEntry{
		tag:        machineTag,
		m:          machine,
		instanceID: "b4dc0ffee",
	}

	// The machine is alive, has an instance status of "provisioning" and
	// is aware of its network addresses.
	machine.EXPECT().Id().Return("0").AnyTimes()
	machine.EXPECT().Life().Return(life.Alive)
	machine.EXPECT().InstanceStatus(gomock.Any()).Return(params.StatusResult{Status: string(status.Provisioning)}, nil)

	// The provider reports the instance status as running and also indicates
	// that network addresses have been *changed*.
	instInfo := mocks.NewMockInstance(ctrl)
	instInfo.EXPECT().Status(gomock.Any()).Return(instance.Status{Status: status.Running, Message: "Running wild"})

	// When we process the instance info we expect the machine instance
	// status and list of network addresses to be updated so they match
	// the values reported by the provider.
	machine.EXPECT().SetInstanceStatus(gomock.Any(), status.Running, "Running wild", nil).Return(nil)
	machine.EXPECT().SetProviderNetworkConfig(gomock.Any(), testNetIfs).Return(testAddrs, true, nil)

	providerStatus, addrCount, err := updWorker.processProviderInfo(context.Background(), entry, instInfo, testNetIfs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerStatus, tc.Equals, status.Running)
	c.Assert(addrCount, tc.Equals, len(testAddrs))
}

func (s *workerSuite) TestStartedMachineWithNetAddressesMovesToLongPollGroup(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, _ := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	// Start with machine "0" in the short poll group.
	machineTag := names.NewMachineTag("0")
	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().String().Return("machine-0").AnyTimes()

	updWorker.appendToShortPollGroup(machineTag, machine)
	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 1)

	// The provider reports an instance status of "running"; the machine
	// reports it's machine status as "started".
	entry, _ := updWorker.lookupPolledMachine(machineTag)
	updWorker.maybeSwitchPollGroup(context.Background(), shortPollGroup, entry, status.Running, status.Started, 1)

	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 0)
	c.Assert(updWorker.pollGroup[longPollGroup], tc.HasLen, 1)
}

func (s *workerSuite) TestNonStartedMachinesGetBumpedPollInterval(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, _ := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	machine := mocks.NewMockMachine(ctrl)

	specs := []status.Status{status.Allocating, status.Pending}
	for specIndex, spec := range specs {
		c.Logf("provider reports instance status as: %q", spec)
		machineTag := names.NewMachineTag(fmt.Sprint(specIndex))
		updWorker.appendToShortPollGroup(machineTag, machine)
		entry, _ := updWorker.lookupPolledMachine(machineTag)

		updWorker.maybeSwitchPollGroup(context.Background(), shortPollGroup, entry, spec, status.Pending, 0)
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
	machineTag := names.NewMachineTag("0")
	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().String().Return("machine-0").AnyTimes()

	// Move the machine to the long poll group.
	updWorker.appendToShortPollGroup(machineTag, machine)
	entry, _ := updWorker.lookupPolledMachine(machineTag)
	updWorker.maybeSwitchPollGroup(context.Background(), shortPollGroup, entry, status.Running, status.Started, 1)
	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 0)
	c.Assert(updWorker.pollGroup[longPollGroup], tc.HasLen, 1)

	// If we get unknown status from the provider we expect the machine to
	// be moved back to the short poll group.
	updWorker.maybeSwitchPollGroup(context.Background(), longPollGroup, entry, status.Unknown, status.Started, 1)
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

	machineTag := names.NewMachineTag("0")
	machine := mocks.NewMockMachine(ctrl)

	// Add machine to short poll group and bump its poll interval
	updWorker.appendToShortPollGroup(machineTag, machine)
	entry, _ := updWorker.lookupPolledMachine(machineTag)
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

	machineTag := names.NewMachineTag("0")
	machine := mocks.NewMockMachine(ctrl)

	// Add machine to short poll group
	updWorker.appendToShortPollGroup(machineTag, machine)
	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 1)

	// On next refresh, the machine reports as dead
	machine.EXPECT().Refresh(gomock.Any()).Return(nil)
	machine.EXPECT().Life().Return(life.Dead)
	machine.EXPECT().String().Return("machine-0").AnyTimes()

	// Emit a change for the machine so the queueing code detects the
	// dead machine and removes it.
	s.assertWorkerCompletesLoop(c, updWorker, func() {
		mocked.facadeAPI.assertEnqueueChange(c, []string{"0"})
	})

	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 0, tc.Commentf("dead machine has not been removed"))
}

func (s *workerSuite) TestReapedMachineIsTreatedAsDeadAndRemoved(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	machineTag := names.NewMachineTag("0")
	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().String().Return("machine-0").AnyTimes()

	// Add machine to short poll group
	updWorker.appendToShortPollGroup(machineTag, machine)
	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 1)

	// On next refresh, the machine refresh fails with NotFoudn
	machine.EXPECT().Refresh(gomock.Any()).Return(
		errors.NotFoundf("this is not the machine you are looking for"),
	)

	// Emit a change for the machine so the queueing code detects the
	// dead machine and removes it.
	s.assertWorkerCompletesLoop(c, updWorker, func() {
		mocked.facadeAPI.assertEnqueueChange(c, []string{"0"})
	})

	c.Assert(updWorker.pollGroup[shortPollGroup], tc.HasLen, 0, tc.Commentf("dead machine has not been removed"))
}

func (s *workerSuite) TestQueuingOfManualMachines(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	// Add two manual machines, one with "provisioning" status and one with
	// "started" status. We expect the former to have its instance status
	// changed to "running".
	machineTag0 := names.NewMachineTag("0")
	machine0 := mocks.NewMockMachine(ctrl)
	machine0.EXPECT().IsManual(gomock.Any()).Return(true, nil)
	machine0.EXPECT().InstanceStatus(gomock.Any()).Return(params.StatusResult{Status: string(status.Provisioning)}, nil)
	machine0.EXPECT().SetInstanceStatus(gomock.Any(), status.Running, "Manually provisioned machine", nil).Return(nil)
	mocked.facadeAPI.addMachine(machineTag0, machine0)

	machineTag1 := names.NewMachineTag("1")
	machine1 := mocks.NewMockMachine(ctrl)
	machine1.EXPECT().IsManual(gomock.Any()).Return(true, nil)
	machine1.EXPECT().InstanceStatus(gomock.Any()).Return(params.StatusResult{Status: string(status.Running)}, nil)
	mocked.facadeAPI.addMachine(machineTag1, machine1)

	// Emit change for both machines.
	s.assertWorkerCompletesLoop(c, updWorker, func() {
		mocked.facadeAPI.assertEnqueueChange(c, []string{"0", "1"})
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
	machineTag0 := names.NewMachineTag("0")
	machine0 := mocks.NewMockMachine(ctrl)
	machine0.EXPECT().InstanceId(gomock.Any()).Return(instance.Id(""), apiservererrors.ServerError(errors.NotProvisionedf("not there")))
	machine0.EXPECT().Id().Return("0")
	machine0.EXPECT().String().Return("machine-0").AnyTimes()
	updWorker.appendToShortPollGroup(machineTag0, machine0)

	machineTag1 := names.NewMachineTag("1")
	machine1 := mocks.NewMockMachine(ctrl)
	machine1.EXPECT().Life().Return(life.Alive)
	machine1.EXPECT().InstanceId(gomock.Any()).Return(instance.Id("b4dc0ffee"), nil)
	machine1.EXPECT().InstanceStatus(gomock.Any()).Return(params.StatusResult{Status: string(status.Running)}, nil)
	machine1.EXPECT().Status(gomock.Any()).Return(params.StatusResult{Status: string(status.Started)}, nil)
	machine1.EXPECT().SetProviderNetworkConfig(gomock.Any(), testNetIfs).Return(testAddrs, false, nil)
	machine1.EXPECT().String().Return("machine-1").AnyTimes()
	updWorker.appendToShortPollGroup(machineTag1, machine1)

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

	machineTag := names.NewMachineTag("0")
	machine := mocks.NewMockMachine(ctrl)

	// Add machine to short poll group and manually move it to long poll group.
	updWorker.appendToShortPollGroup(machineTag, machine)
	entry, _ := updWorker.lookupPolledMachine(machineTag)
	updWorker.pollGroup[longPollGroup][machineTag] = entry
	delete(updWorker.pollGroup[shortPollGroup], machineTag)

	// Allow instance ID to be resolved but have the provider's Instances
	// call fail with a partial instance list.
	instID := instance.Id("d3adc0de")
	machine.EXPECT().InstanceId(gomock.Any()).Return(instID, nil)
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

	machineTag := names.NewMachineTag("0")
	machine := mocks.NewMockMachine(ctrl)

	updWorker.appendToShortPollGroup(machineTag, machine)

	// Allow instance ID to be resolved but have the provider's Instances
	// call fail with a partial instance list.
	instID := instance.Id("d3adc0de")
	machine.EXPECT().InstanceId(gomock.Any()).Return(instID, nil)
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
	entry, _ := updWorker.lookupPolledMachine(machineTag)
	c.Assert(entry.shortPollInterval, tc.Equals, time.Duration(float64(ShortPoll)*ShortPollBackoff))
}

func (s *workerSuite) TestLongPollNoMachineInGroupKnownByProvider(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	w, mocked := s.startWorker(c, ctrl)
	defer workertest.CleanKill(c, w)
	updWorker := w.(*updaterWorker)

	machineTag := names.NewMachineTag("0")
	machine := mocks.NewMockMachine(ctrl)

	// Add machine to short poll group and manually move it to long poll group.
	updWorker.appendToShortPollGroup(machineTag, machine)
	entry, _ := updWorker.lookupPolledMachine(machineTag)
	updWorker.pollGroup[longPollGroup][machineTag] = entry
	delete(updWorker.pollGroup[shortPollGroup], machineTag)

	// Allow instance ID to be resolved but have the provider's Instances
	// call fail with ErrNoInstances. This is probably rare but can happen
	// and shouldn't cause the worker to exit with an error!
	instID := instance.Id("d3adc0de")
	machine.EXPECT().InstanceId(gomock.Any()).Return(instID, nil)
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

	machineTag := names.NewMachineTag("0")
	machine := mocks.NewMockMachine(ctrl)

	// Add machine to short poll group and manually move it to long poll group.
	updWorker.appendToShortPollGroup(machineTag, machine)

	// Allow instance ID to be resolved but have the provider's Instances
	// call fail with ErrNoInstances. This is probably rare but can happen
	// and shouldn't cause the worker to exit with an error!
	instID := instance.Id("d3adc0de")
	machine.EXPECT().InstanceId(gomock.Any()).Return(instID, nil)
	mocked.environ.EXPECT().Instances(gomock.Any(), []instance.Id{instID}).Return(
		nil, environs.ErrNoInstances,
	)

	// Advance the clock to trigger processing of the short poll group.
	s.assertWorkerCompletesLoops(c, updWorker, 1, func() {
		mocked.clock.Advance(ShortPoll)
	})

	// Check that we have backed off the poll interval.
	entry, _ := updWorker.lookupPolledMachine(machineTag)
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
	clock     *testclock.Clock
	facadeAPI *mockFacadeAPI
	environ   *mocks.MockEnviron
}

func (s *workerSuite) startWorker(c *tc.C, ctrl *gomock.Controller) (worker.Worker, workerMocks) {
	workerMainLoopEnteredCh := make(chan struct{}, 1)
	mocked := workerMocks{
		clock:     testclock.NewClock(time.Now()),
		facadeAPI: newMockFacadeAPI(ctrl, workerMainLoopEnteredCh),
		environ:   mocks.NewMockEnviron(ctrl),
	}

	w, err := NewWorker(Config{
		Clock:   mocked.clock,
		Facade:  mocked.facadeAPI,
		Environ: mocked.environ,
		Logger:  loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Wait for worker to reach main loop before we allow tests to
	// manipulate the clock.
	select {
	case <-workerMainLoopEnteredCh:
	case <-time.After(coretesting.ShortWait):
		c.Fatal("timed out wating for worker to enter main loop")
	}

	return w, mocked
}

// mockFacadeAPI is a workaround for not being able to use gomock for the
// FacadeAPI interface. Because the Machine() method returns a Machine interface,
// gomock will import instancepoller and cause an import cycle.
type mockFacadeAPI struct {
	machineMap map[names.MachineTag]Machine

	sw              *mocks.MockStringsWatcher
	watcherChangeCh chan []string
}

func newMockFacadeAPI(ctrl *gomock.Controller, workerGotWatcherCh chan<- struct{}) *mockFacadeAPI {
	api := &mockFacadeAPI{
		machineMap:      make(map[names.MachineTag]Machine),
		sw:              mocks.NewMockStringsWatcher(ctrl),
		watcherChangeCh: make(chan []string),
	}

	api.sw.EXPECT().Changes().DoAndReturn(func() watcher.StringsChannel {
		select {
		case workerGotWatcherCh <- struct{}{}:
		default:
		}
		return api.watcherChangeCh
	}).AnyTimes()
	api.sw.EXPECT().Kill().AnyTimes()
	api.sw.EXPECT().Wait().AnyTimes()
	return api
}

func (api *mockFacadeAPI) assertEnqueueChange(c *tc.C, values []string) {
	select {
	case api.watcherChangeCh <- values:
	case <-time.After(coretesting.ShortWait):
		c.Fatal("timed out waiting for worker to pick up change")
	}
}
func (api *mockFacadeAPI) addMachine(tag names.MachineTag, m Machine) { api.machineMap[tag] = m }

func (api *mockFacadeAPI) WatchModelMachines(context.Context) (watcher.StringsWatcher, error) {
	return api.sw, nil
}
func (api *mockFacadeAPI) Machine(_ context.Context, tag names.MachineTag) (Machine, error) {
	if found := api.machineMap[tag]; found != nil {
		return found, nil
	}
	return nil, errors.NotFoundf(tag.String())
}
