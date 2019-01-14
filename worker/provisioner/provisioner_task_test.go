// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/api"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	apiprovisionermock "github.com/juju/juju/api/provisioner/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller/authentication"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	jujuversion "github.com/juju/juju/juju/version"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/common/mocks"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/provisioner"
	provisionermocks "github.com/juju/juju/worker/provisioner/mocks"
)

type ProvisionerTaskSuite struct {
	testing.IsolationSuite

	modelMachinesChanges chan []string
	modelMachinesWatcher watcher.StringsWatcher

	machineErrorRetryChanges chan struct{}
	machineErrorRetryWatcher watcher.NotifyWatcher

	modelMachinesProfileChanges chan []string
	modelMachinesProfileWatcher watcher.StringsWatcher

	machinesResults      []apiprovisioner.MachineResult
	machineStatusResults []apiprovisioner.MachineStatusResult
	machineGetter        *testMachineGetter

	instances      []instances.Instance
	instanceBroker *testInstanceBroker

	callCtx           *context.CloudCallContext
	invalidCredential bool

	auth *testAuthenticationProvider
}

var _ = gc.Suite(&ProvisionerTaskSuite{})

func (s *ProvisionerTaskSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.modelMachinesChanges = make(chan []string)
	s.modelMachinesWatcher = watchertest.NewMockStringsWatcher(s.modelMachinesChanges)

	s.machineErrorRetryChanges = make(chan struct{})
	s.machineErrorRetryWatcher = watchertest.NewMockNotifyWatcher(s.machineErrorRetryChanges)

	s.modelMachinesProfileChanges = make(chan []string)
	s.modelMachinesProfileWatcher = watchertest.NewMockStringsWatcher(s.modelMachinesProfileChanges)

	s.machinesResults = []apiprovisioner.MachineResult{}
	s.machineStatusResults = []apiprovisioner.MachineStatusResult{}
	s.machineGetter = &testMachineGetter{
		Stub: &testing.Stub{},
		machinesFunc: func(machines ...names.MachineTag) ([]apiprovisioner.MachineResult, error) {
			return s.machinesResults, nil
		},
		machinesWithTransientErrorsFunc: func() ([]apiprovisioner.MachineStatusResult, error) {
			return s.machineStatusResults, nil
		},
	}

	s.instances = []instances.Instance{}
	s.instanceBroker = &testInstanceBroker{
		Stub:      &testing.Stub{},
		callsChan: make(chan string, 2),
		allInstancesFunc: func(ctx context.ProviderCallContext) ([]instances.Instance, error) {
			return s.instances, nil
		},
	}

	s.callCtx = &context.CloudCallContext{
		InvalidateCredentialFunc: func(string) error {
			s.invalidCredential = true
			return nil
		},
	}
	s.auth = &testAuthenticationProvider{&testing.Stub{}}
}

func (s *ProvisionerTaskSuite) TestStartStop(c *gc.C) {
	task := s.newProvisionerTask(c,
		config.HarvestAll,
		&mockDistributionGroupFinder{},
		mockToolsFinder{},
	)
	workertest.CheckAlive(c, task)
	workertest.CleanKill(c, task)

	err := workertest.CheckKilled(c, task)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.modelMachinesWatcher)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.machineErrorRetryWatcher)
	c.Assert(err, jc.ErrorIsNil)
	s.machineGetter.CheckNoCalls(c)
	s.instanceBroker.CheckNoCalls(c)
}

func (s *ProvisionerTaskSuite) TestStopInstancesIgnoresMachinesWithKeep(c *gc.C) {
	task := s.newProvisionerTask(c,
		config.HarvestAll,
		&mockDistributionGroupFinder{},
		mockToolsFinder{},
	)
	defer workertest.CleanKill(c, task)

	i0 := &testInstance{id: "zero"}
	i1 := &testInstance{id: "one"}
	s.instances = []instances.Instance{
		i0,
		i1,
	}

	m0 := &testMachine{
		id:       "0",
		life:     params.Dead,
		instance: i0,
	}
	m1 := &testMachine{
		id:           "1",
		life:         params.Dead,
		instance:     i1,
		keepInstance: true,
	}
	c.Assert(m0.markForRemoval, jc.IsFalse)
	c.Assert(m1.markForRemoval, jc.IsFalse)

	s.machinesResults = []apiprovisioner.MachineResult{
		{Machine: m0},
		{Machine: m1},
	}

	s.sendModelMachinesChange(c, "0", "1")

	s.waitForTask(c, []string{"AllInstances", "StopInstances"})

	workertest.CleanKill(c, task)
	close(s.instanceBroker.callsChan)
	s.machineGetter.CheckCallNames(c, "Machines")
	s.instanceBroker.CheckCalls(c, []testing.StubCall{
		{"AllInstances", []interface{}{s.callCtx}},
		{"StopInstances", []interface{}{s.callCtx, []instance.Id{"zero"}}},
	})
	c.Assert(m0.markForRemoval, jc.IsTrue)
	c.Assert(m1.markForRemoval, jc.IsTrue)
}

func (s *ProvisionerTaskSuite) TestProvisionerRetries(c *gc.C) {
	s.instanceBroker.SetErrors(
		errors.New("errors 1"),
		errors.New("errors 2"),
	)

	task := s.newProvisionerTaskWithRetry(c,
		config.HarvestAll,
		&mockDistributionGroupFinder{},
		mockToolsFinder{},
		provisioner.NewRetryStrategy(0*time.Second, 1),
	)

	m0 := &testMachine{
		id: "0",
	}
	s.machineStatusResults = []apiprovisioner.MachineStatusResult{
		{Machine: m0, Status: params.StatusResult{}},
	}
	s.sendMachineErrorRetryChange(c)

	s.waitForTask(c, []string{"StartInstance", "StartInstance"})

	workertest.CleanKill(c, task)
	close(s.instanceBroker.callsChan)
	s.machineGetter.CheckCallNames(c, "MachinesWithTransientErrors")
	s.auth.CheckCallNames(c, "SetupAuthentication")
	s.instanceBroker.CheckCallNames(c, "StartInstance", "StartInstance")
}

func (s *ProvisionerTaskSuite) TestProcessProfileChanges(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Setup mockMachine0 to successfully change from an
	// old profile to a new profile.
	mockMachine0, info0 := setUpSuccessfulMockProfileMachine(ctrl, "0", "juju-default-lxd-profile-0", false)
	mockMachine0.EXPECT().SetCharmProfiles([]string{info0.NewProfileName, "juju-default-different-0"}).Return(nil)

	// Setup mockMachine1 to successfully change from an
	// old profile to a new profile.
	mockMachine1, info1 := setUpSuccessfulMockProfileMachine(ctrl, "1", "juju-default-lxd-profile-0", true)
	mockMachine1.EXPECT().SetCharmProfiles([]string{info1.NewProfileName, "juju-default-different-0"}).Return(nil)

	// Setup mockMachine2 to have a failure from CharmProfileChangeInfo()
	mockMachine2 := setUpFailureMockProfileMachine(ctrl, "2")

	// Setup mockMachine3 to be a new subordinate unit adding
	// an lxd profile.
	mockMachine3, info3 := setUpSuccessfulMockProfileMachine(ctrl, "3", "", true)
	mockMachine3.EXPECT().SetCharmProfiles([]string{"juju-default-different-0", info3.NewProfileName}).Return(nil)

	s.machinesResults = []apiprovisioner.MachineResult{
		{Machine: mockMachine0, Err: nil},
		{Machine: mockMachine1, Err: nil},
		{Machine: mockMachine2, Err: nil},
		{Machine: mockMachine3, Err: nil},
	}

	mockBroker := provisionermocks.NewMockLXDProfileInstanceBroker(ctrl)
	lExp := mockBroker.EXPECT()
	machineCharmProfiles := []string{"default", "juju-default", info0.NewProfileName, "juju-default-different-0"}
	lExp.ReplaceOrAddInstanceProfile(
		"0", info0.OldProfileName, info0.NewProfileName, info0.LXDProfile,
	).Return(machineCharmProfiles, nil)
	lExp.ReplaceOrAddInstanceProfile(
		"1", info1.OldProfileName, info1.NewProfileName, info1.LXDProfile,
	).Return(machineCharmProfiles, nil)
	lExp.ReplaceOrAddInstanceProfile(
		"3", info3.OldProfileName, info3.NewProfileName, info3.LXDProfile,
	).Return([]string{"default", "juju-default", "juju-default-different-0", info3.NewProfileName}, nil)

	task := s.newProvisionerTaskWithBroker(c, mockBroker, nil)
	c.Assert(provisioner.ProcessProfileChanges(task, []string{"0", "1", "2", "3"}), jc.ErrorIsNil)
}

func setUpSuccessfulMockProfileMachine(ctrl *gomock.Controller, num, old string, sub bool) (*apiprovisionermock.MockMachineProvisioner, apiprovisioner.CharmProfileChangeInfo) {
	mockMachine := apiprovisionermock.NewMockMachineProvisioner(ctrl)
	mExp := mockMachine.EXPECT()
	newProfileName := "juju-default-lxd-profile-1"
	info := apiprovisioner.CharmProfileChangeInfo{
		OldProfileName: old,
		NewProfileName: newProfileName,
		LXDProfile:     nil,
		Subordinate:    sub,
	}
	mExp.CharmProfileChangeInfo().Return(info, nil)
	mExp.Id().Return(num)
	mExp.InstanceId().Return(instance.Id(num), nil)
	if old == "" && sub {
		mExp.RemoveUpgradeCharmProfileData().Return(nil)
	} else {
		mExp.SetInstanceStatus(status.Running, "Running", nil).Return(nil)
		mExp.SetStatus(status.Started, "", nil).Return(nil)
		mExp.SetUpgradeCharmProfileComplete(lxdprofile.SuccessStatus).Return(nil)
	}

	return mockMachine, info
}

func setUpFailureMockProfileMachine(ctrl *gomock.Controller, num string) *apiprovisionermock.MockMachineProvisioner {
	mockMachine := apiprovisionermock.NewMockMachineProvisioner(ctrl)
	mExp := mockMachine.EXPECT()
	mExp.CharmProfileChangeInfo().Return(apiprovisioner.CharmProfileChangeInfo{}, errors.New("fail me"))
	mExp.Id().Return(num)
	mExp.SetInstanceStatus(status.Error, gomock.Any(), nil).Return(nil)
	mExp.SetUpgradeCharmProfileComplete(gomock.Any()).Return(nil)

	return mockMachine
}

func (s *ProvisionerTaskSuite) TestProcessProfileChangesNoLXDBroker(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockMachine := apiprovisionermock.NewMockMachineProvisioner(ctrl)
	mExp := mockMachine.EXPECT()
	mExp.SetUpgradeCharmProfileComplete(lxdprofile.NotSupportedStatus).Return(nil)

	s.machinesResults = []apiprovisioner.MachineResult{
		{Machine: mockMachine, Err: nil},
	}

	task := s.newProvisionerTask(c,
		config.HarvestAll,
		&mockDistributionGroupFinder{},
		mockToolsFinder{},
	)
	defer workertest.CleanKill(c, task)

	c.Assert(provisioner.ProcessProfileChanges(task, []string{"0"}), jc.ErrorIsNil)
}

func (s *ProvisionerTaskSuite) testProcessOneMachineProfileChangeAddProfile(c *gc.C, sub bool) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockMachineProvisioner := apiprovisionermock.NewMockMachineProvisioner(ctrl)
	mExp := mockMachineProvisioner.EXPECT()
	newProfileName := "juju-default-lxd-profile-0"
	info := apiprovisioner.CharmProfileChangeInfo{
		OldProfileName: "",
		NewProfileName: newProfileName,
		LXDProfile:     nil,
		Subordinate:    sub,
	}
	mExp.CharmProfileChangeInfo().Return(info, nil)
	mExp.Id().Return("0")
	mExp.InstanceId().Return(instance.Id("0"), nil)
	differentProfileName := "juju-default-different-0"
	mExp.SetCharmProfiles([]string{differentProfileName, newProfileName}).Return(nil)

	mockLXDProfiler := provisionermocks.NewMockLXDProfileInstanceBroker(ctrl)
	lExp := mockLXDProfiler.EXPECT()
	machineCharmProfiles := []string{"default", "juju-default", differentProfileName, newProfileName}
	lExp.ReplaceOrAddInstanceProfile(
		"0", info.OldProfileName, info.NewProfileName, info.LXDProfile,
	).Return(machineCharmProfiles, nil)

	remove, err := provisioner.ProcessOneMachineProfileChanges(mockMachineProvisioner, mockLXDProfiler)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remove, gc.Equals, sub)
}

func (s *ProvisionerTaskSuite) TestProcessOneMachineProfileChangeAddProfile(c *gc.C) {
	s.testProcessOneMachineProfileChangeAddProfile(c, false)
}

func (s *ProvisionerTaskSuite) TestProcessOneMachineProfileChangeAddProfileSubordinate(c *gc.C) {
	s.testProcessOneMachineProfileChangeAddProfile(c, true)
}

func (s *ProvisionerTaskSuite) TestProcessOneMachineProfileChangeRemoveProfileSubordinate(c *gc.C) {
	info := apiprovisioner.CharmProfileChangeInfo{
		OldProfileName: "juju-default-lxd-profile-0",
		NewProfileName: "",
		LXDProfile:     nil,
		Subordinate:    true,
	}

	ctrl, mockMachineProvisioner, mockLXDProfiler := setUpMocksProcessOneMachineProfileChange(c, info)
	defer ctrl.Finish()

	remove, err := provisioner.ProcessOneMachineProfileChanges(mockMachineProvisioner, mockLXDProfiler)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remove, gc.Equals, false)
}

func (s *ProvisionerTaskSuite) TestProcessOneMachineProfileChangeChangeProfile(c *gc.C) {
	info := apiprovisioner.CharmProfileChangeInfo{
		OldProfileName: "juju-default-lxd-profile-0",
		NewProfileName: "juju-default-lxd-profile-1",
		LXDProfile:     nil,
		Subordinate:    true,
	}

	ctrl, mockMachineProvisioner, mockLXDProfiler := setUpMocksProcessOneMachineProfileChange(c, info)
	defer ctrl.Finish()

	remove, err := provisioner.ProcessOneMachineProfileChanges(mockMachineProvisioner, mockLXDProfiler)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remove, gc.Equals, false)
}

func setUpMocksProcessOneMachineProfileChange(c *gc.C, info apiprovisioner.CharmProfileChangeInfo) (*gomock.Controller, *apiprovisionermock.MockMachineProvisioner, *provisionermocks.MockLXDProfileInstanceBroker) {
	ctrl := gomock.NewController(c)

	mockMachineProvisioner := apiprovisionermock.NewMockMachineProvisioner(ctrl)
	mExp := mockMachineProvisioner.EXPECT()
	mExp.CharmProfileChangeInfo().Return(info, nil)
	mExp.Id().Return("0")
	mExp.InstanceId().Return(instance.Id("0"), nil)

	differentProfileName := "juju-default-different-0"
	machineCharmProfiles := []string{"default", "juju-default"}
	if info.NewProfileName != "" {
		mExp.SetCharmProfiles([]string{info.NewProfileName, differentProfileName}).Return(nil)
		machineCharmProfiles = append(machineCharmProfiles, info.NewProfileName)
	} else {
		mExp.SetCharmProfiles([]string{differentProfileName}).Return(nil)
	}
	machineCharmProfiles = append(machineCharmProfiles, differentProfileName)

	mockLXDProfiler := provisionermocks.NewMockLXDProfileInstanceBroker(ctrl)
	mockLXDProfiler.EXPECT().ReplaceOrAddInstanceProfile(
		"0", info.OldProfileName, info.NewProfileName, info.LXDProfile,
	).Return(machineCharmProfiles, nil)

	return ctrl, mockMachineProvisioner, mockLXDProfiler
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsNoZoneAvailable(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := s.setUpZonedEnviron(ctrl)

	// Constraint for availability zone az9 can not be satisfied;
	// this broker only knows of az1, az2, az3.
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az9")
	broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil)

	task := s.newProvisionerTaskWithBroker(c, broker, nil)

	m0 := &testMachine{
		id:          "0",
		constraints: "zones=az9",
	}
	s.machineStatusResults = []apiprovisioner.MachineStatusResult{{Machine: m0, Status: params.StatusResult{}}}
	s.sendMachineErrorRetryChange(c)

	// Wait for instance status to be set.
	timeout := time.After(coretesting.LongWait)
	for msg := ""; msg == ""; {
		select {
		case <-time.After(coretesting.ShortWait):
			_, msg, _ = m0.InstanceStatus()
		case <-timeout:
			c.Fatalf("machine InstanceStatus was not set")
		}
	}

	_, msg, err := m0.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(msg, gc.Equals, "suitable availability zone for machine 0 not found")

	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsNoDistributionGroup(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := s.setUpZonedEnviron(ctrl)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az1")
	broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil)

	// For the call to start instance, we expect the same zone constraint to
	// be present, but we also expect that the zone in start instance params
	// matches the constraint, based on being available in this environ.
	azConstraintsAndDerivedZone := newAZConstraintStartInstanceParamsMatcher("az1")
	azConstraintsAndDerivedZone.addMatch("availability zone: az1", func(p environs.StartInstanceParams) bool {
		return p.AvailabilityZone == "az1"
	})

	// Use satisfaction of this call as the synchronisation point.
	started := make(chan struct{})
	broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(&environs.StartInstanceResult{
		Instance: &testInstance{id: "instance-1"},
	}, nil).Do(func(_ ...interface{}) {
		go func() { started <- struct{}{} }()
	})

	task := s.newProvisionerTaskWithBroker(c, broker, nil)

	m0 := &testMachine{
		id:          "0",
		constraints: "zones=az1",
	}
	s.machineStatusResults = []apiprovisioner.MachineStatusResult{{Machine: m0, Status: params.StatusResult{}}}
	s.sendMachineErrorRetryChange(c)

	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no matching call to StartInstance")
	}

	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsWithDistributionGroup(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := s.setUpZonedEnviron(ctrl)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az1", "az2")
	broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil)

	// For the call to start instance, we expect the same zone constraints to
	// be present, but we also expect that the zone in start instance params
	// was selected from the constraints, based on a machine from the same
	// distribution group already being in one of the zones.
	azConstraintsAndDerivedZone := newAZConstraintStartInstanceParamsMatcher("az1", "az2")
	azConstraintsAndDerivedZone.addMatch("availability zone: az2", func(p environs.StartInstanceParams) bool {
		return p.AvailabilityZone == "az2"
	})

	// Use satisfaction of this call as the synchronisation point.
	started := make(chan struct{})
	broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(&environs.StartInstanceResult{
		Instance: &testInstance{id: "instance-1"},
	}, nil).Do(func(_ ...interface{}) {
		go func() { started <- struct{}{} }()
	})

	// Another machine from the same distribution group is already in az1,
	// so we expect the machine to be created in az2.
	task := s.newProvisionerTaskWithBroker(c, broker, map[names.MachineTag][]string{
		names.NewMachineTag("0"): {"az1"},
	})

	m0 := &testMachine{
		id:          "0",
		constraints: "zones=az1,az2",
	}
	s.machineStatusResults = []apiprovisioner.MachineStatusResult{{Machine: m0, Status: params.StatusResult{}}}
	s.sendMachineErrorRetryChange(c)

	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no matching call to StartInstance")
	}

	workertest.CleanKill(c, task)
}

// setUpZonedEnviron creates a mock environ with instances based on those set
// on the test suite, and 3 availability zones.
func (s *ProvisionerTaskSuite) setUpZonedEnviron(ctrl *gomock.Controller) *mocks.MockZonedEnviron {
	instanceIds := make([]instance.Id, len(s.instances))
	for i, inst := range s.instances {
		instanceIds[i] = inst.Id()
	}

	// Environ has 3 availability zones: az1, az2, az3.
	zones := make([]common.AvailabilityZone, 3)
	for i := 0; i < 3; i++ {
		az := mocks.NewMockAvailabilityZone(ctrl)
		az.EXPECT().Name().Return(fmt.Sprintf("az%d", i+1))
		az.EXPECT().Available().Return(true)
		zones[i] = az
	}

	broker := mocks.NewMockZonedEnviron(ctrl)
	exp := broker.EXPECT()
	exp.AllInstances(s.callCtx).Return(s.instances, nil)
	exp.InstanceAvailabilityZoneNames(s.callCtx, instanceIds).Return([]string{}, nil)
	exp.AvailabilityZones(s.callCtx).Return(zones, nil)
	return broker
}

func (s *ProvisionerTaskSuite) waitForTask(c *gc.C, expectedCalls []string) {
	var calls []string
	for {
		select {
		case call := <-s.instanceBroker.callsChan:
			calls = append(calls, call)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("stopping worker chan didn't stop")
		}
		if reflect.DeepEqual(expectedCalls, calls) {
			// we are done
			break
		}
	}
}

func (s *ProvisionerTaskSuite) sendModelMachinesChange(c *gc.C, ids ...string) {
	select {
	case s.modelMachinesChanges <- ids:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending model machines change")
	}
}

func (s *ProvisionerTaskSuite) sendMachineErrorRetryChange(c *gc.C) {
	select {
	case s.machineErrorRetryChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending machine error retry change")
	}
}

func (s *ProvisionerTaskSuite) newProvisionerTask(
	c *gc.C,
	harvestingMethod config.HarvestMode,
	distributionGroupFinder provisioner.DistributionGroupFinder,
	toolsFinder provisioner.ToolsFinder,
) provisioner.ProvisionerTask {
	return s.newProvisionerTaskWithRetry(c,
		harvestingMethod,
		distributionGroupFinder,
		toolsFinder,
		provisioner.NewRetryStrategy(0*time.Second, 0),
	)
}

func (s *ProvisionerTaskSuite) newProvisionerTaskWithRetry(
	c *gc.C,
	harvestingMethod config.HarvestMode,
	distributionGroupFinder provisioner.DistributionGroupFinder,
	toolsFinder provisioner.ToolsFinder,
	retryStrategy provisioner.RetryStrategy,
) provisioner.ProvisionerTask {
	w, err := provisioner.NewProvisionerTask(
		coretesting.ControllerTag.Id(),
		names.NewMachineTag("0"),
		harvestingMethod,
		s.machineGetter,
		distributionGroupFinder,
		toolsFinder,
		s.modelMachinesWatcher,
		s.machineErrorRetryWatcher,
		s.modelMachinesProfileWatcher,
		s.instanceBroker,
		s.auth,
		imagemetadata.ReleasedStream,
		retryStrategy,
		s.callCtx,
	)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *ProvisionerTaskSuite) newProvisionerTaskWithBroker(
	c *gc.C, broker environs.InstanceBroker, distributionGroups map[names.MachineTag][]string,
) provisioner.ProvisionerTask {
	task, err := provisioner.NewProvisionerTask(
		coretesting.ControllerTag.Id(),
		names.NewMachineTag("0"),
		config.HarvestAll,
		s.machineGetter,
		&mockDistributionGroupFinder{groups: distributionGroups},
		mockToolsFinder{},
		s.modelMachinesWatcher,
		s.machineErrorRetryWatcher,
		s.modelMachinesProfileWatcher,
		broker,
		s.auth,
		imagemetadata.ReleasedStream,
		provisioner.NewRetryStrategy(0*time.Second, 0),
		s.callCtx,
	)
	c.Assert(err, jc.ErrorIsNil)
	return task
}

type testMachineGetter struct {
	*testing.Stub

	machinesFunc                    func(machines ...names.MachineTag) ([]apiprovisioner.MachineResult, error)
	machinesWithTransientErrorsFunc func() ([]apiprovisioner.MachineStatusResult, error)
}

func (m *testMachineGetter) Machines(machines ...names.MachineTag) ([]apiprovisioner.MachineResult, error) {
	m.AddCall("Machines", machines)
	return m.machinesFunc(machines...)
}

func (m *testMachineGetter) MachinesWithTransientErrors() ([]apiprovisioner.MachineStatusResult, error) {
	m.AddCall("MachinesWithTransientErrors")
	return m.machinesWithTransientErrorsFunc()
}

type testInstanceBroker struct {
	*testing.Stub

	callsChan chan string

	allInstancesFunc func(ctx context.ProviderCallContext) ([]instances.Instance, error)
}

func (t *testInstanceBroker) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	t.AddCall("StartInstance", ctx, args)
	t.callsChan <- "StartInstance"
	return nil, t.NextErr()
}

func (t *testInstanceBroker) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	t.AddCall("StopInstances", ctx, ids)
	t.callsChan <- "StopInstances"
	return t.NextErr()
}

func (t *testInstanceBroker) AllInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	t.AddCall("AllInstances", ctx)
	t.callsChan <- "AllInstances"
	return t.allInstancesFunc(ctx)
}

func (t *testInstanceBroker) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	t.AddCall("MaintainInstance", ctx, args)
	t.callsChan <- "MaintainInstance"
	return nil
}

type testInstance struct {
	instances.Instance
	id string
}

func (i *testInstance) Id() instance.Id {
	return instance.Id(i.id)
}

type testMachine struct {
	mu sync.Mutex

	*apiprovisioner.Machine
	id   string
	life params.Life

	instance     *testInstance
	keepInstance bool

	markForRemoval bool
	constraints    string

	instStatusMsg string
}

func (m *testMachine) Id() string {
	return m.id
}

func (m *testMachine) String() string {
	return m.Id()
}

func (m *testMachine) Life() params.Life {
	return m.life
}

func (m *testMachine) InstanceId() (instance.Id, error) {
	return m.instance.Id(), nil
}

func (m *testMachine) KeepInstance() (bool, error) {
	return m.keepInstance, nil
}

func (m *testMachine) MarkForRemoval() error {
	m.markForRemoval = true
	return nil
}

func (m *testMachine) Tag() names.Tag {
	return m.MachineTag()
}

func (m *testMachine) MachineTag() names.MachineTag {
	return names.NewMachineTag(m.id)
}

func (m *testMachine) SetInstanceStatus(_ status.Status, message string, _ map[string]interface{}) error {
	m.mu.Lock()
	m.instStatusMsg = message
	m.mu.Unlock()
	return nil
}

func (m *testMachine) InstanceStatus() (status.Status, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return status.Status(""), m.instStatusMsg, nil
}

func (m *testMachine) SetStatus(status status.Status, info string, data map[string]interface{}) error {
	return nil
}

func (m *testMachine) Status() (status.Status, string, error) {
	return status.Status(""), "", nil
}

func (m *testMachine) ModelAgentVersion() (*version.Number, error) {
	return &coretesting.FakeVersionNumber, nil
}

func (m *testMachine) SetInstanceInfo(
	id instance.Id, nonce string, characteristics *instance.HardwareCharacteristics,
	networkConfig []params.NetworkConfig, volumes []params.Volume,
	volumeAttachments map[string]params.VolumeAttachmentInfo, charmProfiles []string,
) error {
	return nil
}

func (m *testMachine) ProvisioningInfo() (*params.ProvisioningInfo, error) {
	return &params.ProvisioningInfo{
		ControllerConfig: coretesting.FakeControllerConfig(),
		Series:           jujuversion.SupportedLTS(),
		Constraints:      constraints.MustParse(m.constraints),
	}, nil
}

type testAuthenticationProvider struct {
	*testing.Stub
}

func (m *testAuthenticationProvider) SetupAuthentication(
	machine authentication.TaggedPasswordChanger,
) (*mongo.MongoInfo, *api.Info, error) {
	m.AddCall("SetupAuthentication", machine)
	return nil, nil, nil
}

// startInstanceParamsMatcher is a GoMock matcher that applies a collection of
// conditions to an environs.StartInstanceParams.
// All conditions must be true in order for a positive match.
type startInstanceParamsMatcher struct {
	matchers map[string]func(environs.StartInstanceParams) bool
	failMsg  string
}

func (m *startInstanceParamsMatcher) Matches(params interface{}) bool {
	siParams := params.(environs.StartInstanceParams)
	for msg, match := range m.matchers {
		if !match(siParams) {
			m.failMsg = msg
			return false
		}
	}
	return true
}

func (m *startInstanceParamsMatcher) String() string {
	return m.failMsg
}

func (m *startInstanceParamsMatcher) addMatch(msg string, match func(environs.StartInstanceParams) bool) {
	m.matchers[msg] = match
}

// newAZConstraintStartInstanceParamsMatcher returns a matcher that tests
// whether the candidate environs.StartInstanceParams has a constraints value
// that includes exactly the input zones.
func newAZConstraintStartInstanceParamsMatcher(zones ...string) *startInstanceParamsMatcher {
	match := func(p environs.StartInstanceParams) bool {
		if !p.Constraints.HasZones() {
			return false
		}
		cZones := *p.Constraints.Zones
		if len(cZones) != len(zones) {
			return false
		}
		for _, z := range zones {
			found := false
			for _, cz := range cZones {
				if z == cz {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	return newStartInstanceParamsMatcher(map[string]func(environs.StartInstanceParams) bool{
		fmt.Sprint("AZ constraints:", strings.Join(zones, ", ")): match,
	})
}

func newStartInstanceParamsMatcher(
	matchers map[string]func(environs.StartInstanceParams) bool,
) *startInstanceParamsMatcher {
	if matchers == nil {
		matchers = make(map[string]func(environs.StartInstanceParams) bool)
	}
	return &startInstanceParamsMatcher{matchers: matchers}
}
