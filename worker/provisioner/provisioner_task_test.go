// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	stdcontext "context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller/authentication"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/common/mocks"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/provisioner"
)

type ProvisionerTaskSuite struct {
	testing.IsolationSuite

	setupDone            chan bool
	modelMachinesChanges chan []string
	modelMachinesWatcher watcher.StringsWatcher

	machineErrorRetryChanges chan struct{}
	machineErrorRetryWatcher watcher.NotifyWatcher

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

	s.setupDone = make(chan bool)
	s.modelMachinesChanges = make(chan []string)
	s.modelMachinesWatcher = watchertest.NewMockStringsWatcher(s.modelMachinesChanges)

	s.machineErrorRetryChanges = make(chan struct{})
	s.machineErrorRetryWatcher = watchertest.NewMockNotifyWatcher(s.machineErrorRetryChanges)

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
			return s.instances, s.instanceBroker.NextErr()
		},
	}

	s.callCtx = &context.CloudCallContext{
		Context: stdcontext.TODO(),
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
		life:     life.Dead,
		instance: i0,
	}
	m1 := &testMachine{
		id:           "1",
		life:         life.Dead,
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

	s.waitForTask(c, []string{"AllRunningInstances", "StopInstances"})

	workertest.CleanKill(c, task)
	close(s.instanceBroker.callsChan)
	s.machineGetter.CheckCallNames(c, "Machines")
	s.instanceBroker.CheckCalls(c, []testing.StubCall{
		{"AllRunningInstances", []interface{}{s.callCtx}},
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

func (s *ProvisionerTaskSuite) TestEvenZonePlacement(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// There are 3 available usedZones, so test with 4 machines
	// to ensure even spread across usedZones.
	machines := []*testMachine{{
		id: "0",
	}, {
		id: "1",
	}, {
		id: "2",
	}, {
		id: "3",
	}}
	broker := s.setUpZonedEnviron(ctrl, machines...)
	azConstraints := newAZConstraintStartInstanceParamsMatcher()
	broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil).Times(len(machines))

	// We need to coordinate access to usedZones by the worker, executing the
	// expectations below on a separate Goroutine, and the test logic.
	zoneLock := sync.Mutex{}
	var usedZones []string

	expectedIds := set.NewStrings()
	for _, m := range machines {
		expectedIds.Add(m.id)
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(&environs.StartInstanceResult{
			Instance: &testInstance{id: "instance-" + m.id},
		}, nil).Do(func(ctx, params interface{}) {
			zoneLock.Lock()
			usedZones = append(usedZones, params.(environs.StartInstanceParams).AvailabilityZone)
			zoneLock.Unlock()
		})
	}
	broker.EXPECT().StopInstances(s.callCtx, gomock.Any()).Do(func(ctx interface{}, ids ...interface{}) {
		for _, id := range ids {
			expectedIds.Remove(fmt.Sprintf("%s", id))
		}
		c.Assert(expectedIds.Size(), gc.Equals, 0)
	})

	task := s.newProvisionerTaskWithBroker(c, broker, nil)
	s.sendModelMachinesChange(c, expectedIds.Values()...)

	shortAttempt := &utils.AttemptStrategy{
		Total: coretesting.LongWait,
		Delay: 10 * time.Millisecond,
	}
	for a := shortAttempt.Start(); a.Next(); {
		zoneLock.Lock()
		if len(usedZones) == 4 {
			break
		}
		zoneLock.Unlock()
	}
	zoneCounts := make(map[string]int)
	for _, z := range usedZones {
		count := zoneCounts[z] + 1
		zoneCounts[z] = count
	}
	for z, count := range zoneCounts {
		if count == 0 || count > 2 {
			c.Fatalf("expected either 1 or 2 machines for %v, got %d", z, count)
		}
	}
	c.Assert(set.NewStrings(usedZones...).SortedValues(), jc.DeepEquals, []string{"az1", "az2", "az3"})

	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestMultipleSpaceConstraints(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	m0 := &testMachine{
		id:          "0",
		constraints: "spaces=alpha,beta",
		topology: params.ProvisioningNetworkTopology{
			SubnetAZs: map[string][]string{
				"subnet-1": {"az-1"},
				"subnet-2": {"az-2"},
			},
			SpaceSubnets: map[string][]string{
				"alpha": {"subnet-1"},
				"beta":  {"subnet-2"},
			},
		},
	}
	broker := s.setUpZonedEnviron(ctrl, m0)
	spaceConstraints := newSpaceConstraintStartInstanceParamsMatcher("alpha", "beta")

	spaceConstraints.addMatch("subnets-to-zones", func(p environs.StartInstanceParams) bool {
		if len(p.SubnetsToZones) != 2 {
			return false
		}

		// Order independence.
		for _, subZones := range p.SubnetsToZones {
			for sub, zones := range subZones {
				var zone string

				switch sub {
				case "subnet-1":
					zone = "az-1"
				case "subnet-2":
					zone = "az-2"
				default:
					return false
				}

				if len(zones) != 1 || zones[0] != zone {
					return false
				}
			}
		}

		return true
	})

	broker.EXPECT().DeriveAvailabilityZones(s.callCtx, spaceConstraints).Return([]string{}, nil)

	// Use satisfaction of this call as the synchronisation point.
	started := make(chan struct{})
	broker.EXPECT().StartInstance(s.callCtx, spaceConstraints).Return(&environs.StartInstanceResult{
		Instance: &testInstance{id: "instance-0"},
	}, nil).Do(func(_ ...interface{}) {
		go func() { started <- struct{}{} }()
	})
	task := s.newProvisionerTaskWithBroker(c, broker, nil)
	broker.EXPECT().StopInstances(s.callCtx, []instance.Id{"0"})

	s.sendModelMachinesChange(c, "0")

	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no matching call to StartInstance")
	}

	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsNoZoneAvailable(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	m0 := &testMachine{
		id:          "0",
		constraints: "zones=az9",
	}
	broker := s.setUpZonedEnviron(ctrl, m0)

	// Constraint for availability zone az9 can not be satisfied;
	// this broker only knows of az1, az2, az3.
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az9")
	broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil)

	broker.EXPECT().StopInstances(s.callCtx, []instance.Id{"0"})
	task := s.newProvisionerTaskWithBroker(c, broker, nil)
	s.sendModelMachinesChange(c, "0")
	s.waitForWorkerSetup(c, "worker not set up")

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

	m0 := &testMachine{
		id:          "0",
		constraints: "zones=az1",
	}
	broker := s.setUpZonedEnviron(ctrl, m0)
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
		Instance: &testInstance{id: "instance-0"},
	}, nil).Do(func(_ ...interface{}) {
		go func() { started <- struct{}{} }()
	})
	broker.EXPECT().StopInstances(s.callCtx, []instance.Id{"0"})

	task := s.newProvisionerTaskWithBroker(c, broker, nil)

	s.sendModelMachinesChange(c, "0")

	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no matching call to StartInstance")
	}

	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsNoDistributionGroupRetry(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := s.setUpZonedEnviron(ctrl)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az1")

	failedErr := errors.Errorf("oh no")
	// Use satisfaction of this call as the synchronisation point.
	started := make(chan struct{})
	gomock.InOrder(
		broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil),
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(nil, failedErr),
		broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil),
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(&environs.StartInstanceResult{
			Instance: &testInstance{id: "instance-1"},
		}, nil).Do(func(_ ...interface{}) {
			go func() { started <- struct{}{} }()
		}),
	)

	task := s.newProvisionerTaskWithBroker(c, broker, nil)

	m0 := &testMachine{
		id:          "0",
		constraints: "zones=az1",
	}
	s.machineStatusResults = []apiprovisioner.MachineStatusResult{{Machine: m0, Status: params.StatusResult{}}}
	s.sendMachineErrorRetryChange(c)
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

	m0 := &testMachine{
		id:          "0",
		constraints: "zones=az1,az2",
	}
	broker := s.setUpZonedEnviron(ctrl, m0)

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
		Instance: &testInstance{id: "instance-0"},
	}, nil).Do(func(_ ...interface{}) {
		go func() { started <- struct{}{} }()
	})
	broker.EXPECT().StopInstances(s.callCtx, []instance.Id{"0"})

	// Another machine from the same distribution group is already in az1,
	// so we expect the machine to be created in az2.
	task := s.newProvisionerTaskWithBroker(c, broker, map[names.MachineTag][]string{
		names.NewMachineTag("0"): {"az1"},
	})

	s.sendModelMachinesChange(c, "0")
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no matching call to StartInstance")
	}

	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsWithDistributionGroupRetry(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := s.setUpZonedEnviron(ctrl)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az1", "az2")

	// Use satisfaction of this call as the synchronisation point.
	failedErr := errors.Errorf("oh no")
	started := make(chan struct{})
	gomock.InOrder(
		broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil),
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(nil, failedErr),
		broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil),
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(&environs.StartInstanceResult{
			Instance: &testInstance{id: "instance-1"},
		}, nil).Do(func(_ ...interface{}) {
			go func() { started <- struct{}{} }()
		}),
	)

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
	s.sendMachineErrorRetryChange(c)

	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no matching call to StartInstance")
	}

	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneRestrictiveConstraintsWithDistributionGroupRetry(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := s.setUpZonedEnviron(ctrl)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az2")

	// Use satisfaction of this call as the synchronisation point.
	failedErr := errors.Errorf("oh no")
	started := make(chan struct{})
	gomock.InOrder(
		broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil),
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(nil, failedErr),
		broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil),
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(&environs.StartInstanceResult{
			Instance: &testInstance{id: "instance-2"},
		}, nil).Do(func(_ ...interface{}) {
			go func() { started <- struct{}{} }()
		}),
	)

	// Another machine from the same distribution group is already in az1,
	// so we expect the machine to be created in az2.
	task := s.newProvisionerTaskWithBroker(c, broker, map[names.MachineTag][]string{
		names.NewMachineTag("0"): {"az2"},
		names.NewMachineTag("1"): {"az3"},
	})

	m0 := &testMachine{
		id:          "0",
		constraints: "zones=az2",
	}
	s.machineStatusResults = []apiprovisioner.MachineStatusResult{
		{Machine: m0, Status: params.StatusResult{}},
	}
	s.sendMachineErrorRetryChange(c)
	s.sendMachineErrorRetryChange(c)

	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no matching call to StartInstance")
	}

	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestPopulateAZMachinesErrorWorkerStopped(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockZonedEnviron(ctrl)
	broker.EXPECT().AllRunningInstances(s.callCtx).Return(nil, errors.New("boom")).Do(func(_ ...interface{}) {
		go func() { close(s.setupDone) }()
	})

	task := s.newProvisionerTaskWithBroker(c, broker, map[names.MachineTag][]string{
		names.NewMachineTag("0"): {"az1"},
	})
	s.sendModelMachinesChange(c, "0")
	s.waitForWorkerSetup(c, "worker not set up")

	err := workertest.CheckKill(c, task)
	c.Assert(err, gc.ErrorMatches, "failed to process updated machines: .* boom")
}

// setUpZonedEnviron creates a mock environ with instances based on those set
// on the test suite, and 3 availability zones.
func (s *ProvisionerTaskSuite) setUpZonedEnviron(ctrl *gomock.Controller, machines ...*testMachine) *mocks.MockZonedEnviron {
	broker := mocks.NewMockZonedEnviron(ctrl)
	if len(machines) == 0 {
		return broker
	}
	for _, m := range machines {
		inst := &testInstance{id: m.id}
		s.instances = append(s.instances, inst)
		s.machinesResults = append(s.machinesResults, apiprovisioner.MachineResult{Machine: m})
		s.machineStatusResults = append(s.machineStatusResults, apiprovisioner.MachineStatusResult{Machine: m, Status: params.StatusResult{
			Status: "pending",
		}})
	}

	instanceIds := make([]instance.Id, len(s.instances))
	for i, inst := range s.instances {
		instanceIds[i] = inst.Id()
	}

	// Environ has 3 availability zones: az1, az2, az3.
	zones := make(network.AvailabilityZones, 3)
	for i := 0; i < 3; i++ {
		az := mocks.NewMockAvailabilityZone(ctrl)
		az.EXPECT().Name().Return(fmt.Sprintf("az%d", i+1))
		az.EXPECT().Available().Return(true)
		zones[i] = az
	}

	exp := broker.EXPECT()
	exp.AllRunningInstances(s.callCtx).Return(s.instances, nil).Times(2)
	exp.InstanceAvailabilityZoneNames(s.callCtx, instanceIds).Return(map[instance.Id]string{}, nil).Do(func(_ ...interface{}) {
		close(s.setupDone)
	})
	exp.AvailabilityZones(s.callCtx).Return(zones, nil)
	return broker
}

func (s *ProvisionerTaskSuite) waitForWorkerSetup(c *gc.C, msg string) {
	select {
	case <-s.setupDone:
	case <-time.After(coretesting.LongWait):
		c.Fatalf(msg)
	}
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
		loggo.GetLogger("test"),
		harvestingMethod,
		s.machineGetter,
		distributionGroupFinder,
		toolsFinder,
		s.modelMachinesWatcher,
		s.machineErrorRetryWatcher,
		s.instanceBroker,
		s.auth,
		imagemetadata.ReleasedStream,
		retryStrategy,
		func(_ stdcontext.Context) context.ProviderCallContext { return s.callCtx },
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
		loggo.GetLogger("test"),
		config.HarvestAll,
		s.machineGetter,
		&mockDistributionGroupFinder{groups: distributionGroups},
		mockToolsFinder{},
		s.modelMachinesWatcher,
		s.machineErrorRetryWatcher,
		broker,
		s.auth,
		imagemetadata.ReleasedStream,
		provisioner.NewRetryStrategy(0*time.Second, 0),
		func(_ stdcontext.Context) context.ProviderCallContext { return s.callCtx },
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

func (t *testInstanceBroker) AllRunningInstances(ctx context.ProviderCallContext) ([]instances.Instance, error) {
	t.AddCall("AllRunningInstances", ctx)
	t.callsChan <- "AllRunningInstances"
	return t.allInstancesFunc(ctx)
}

type testInstance struct {
	instances.Instance
	id string
}

func (i *testInstance) Id() instance.Id {
	return instance.Id(i.id)
}

type testMachine struct {
	*apiprovisioner.Machine

	mu sync.Mutex

	id             string
	life           life.Value
	instance       *testInstance
	keepInstance   bool
	markForRemoval bool
	constraints    string
	instStatusMsg  string
	modStatusMsg   string
	topology       params.ProvisioningNetworkTopology
}

func (m *testMachine) Id() string {
	return m.id
}

func (m *testMachine) String() string {
	return m.Id()
}

func (m *testMachine) Life() life.Value {
	return m.life
}

func (m *testMachine) InstanceId() (instance.Id, error) {
	if m.instance == nil {
		return "", params.Error{Code: "not provisioned"}
	}
	return m.instance.Id(), nil
}

func (m *testMachine) InstanceNames() (instance.Id, string, error) {
	instId, err := m.InstanceId()
	return instId, "", err
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
	return "", m.instStatusMsg, nil
}

func (m *testMachine) SetModificationStatus(_ status.Status, message string, _ map[string]interface{}) error {
	m.mu.Lock()
	m.modStatusMsg = message
	m.mu.Unlock()
	return nil
}

func (m *testMachine) ModificationStatus() (status.Status, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return "", m.modStatusMsg, nil
}

func (m *testMachine) SetStatus(_ status.Status, _ string, _ map[string]interface{}) error {
	return nil
}

func (m *testMachine) Status() (status.Status, string, error) {
	return "pending", "", nil
}

func (m *testMachine) ModelAgentVersion() (*version.Number, error) {
	return &coretesting.FakeVersionNumber, nil
}

func (m *testMachine) SetInstanceInfo(
	_ instance.Id, _ string, _ string, _ *instance.HardwareCharacteristics, _ []params.NetworkConfig, _ []params.Volume,
	_ map[string]params.VolumeAttachmentInfo, _ []string,
) error {
	return nil
}

func (m *testMachine) ProvisioningInfo() (*params.ProvisioningInfo, error) {
	return &params.ProvisioningInfo{
		ControllerConfig:            coretesting.FakeControllerConfig(),
		Series:                      jujuversion.DefaultSupportedLTS(),
		Constraints:                 constraints.MustParse(m.constraints),
		ProvisioningNetworkTopology: m.topology,
	}, nil
}

type testAuthenticationProvider struct {
	*testing.Stub
}

func (m *testAuthenticationProvider) SetupAuthentication(
	machine authentication.TaggedPasswordChanger,
) (*api.Info, error) {
	m.AddCall("SetupAuthentication", machine)
	return nil, nil
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
			return len(zones) == 0
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

func newSpaceConstraintStartInstanceParamsMatcher(spaces ...string) *startInstanceParamsMatcher {
	match := func(p environs.StartInstanceParams) bool {
		if !p.Constraints.HasSpaces() {
			return false
		}
		cSpaces := p.Constraints.IncludeSpaces()
		if len(cSpaces) != len(spaces) {
			return false
		}
		for _, s := range spaces {
			found := false
			for _, cs := range spaces {
				if s == cs {
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
		fmt.Sprint("space constraints:", strings.Join(spaces, ", ")): match,
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
