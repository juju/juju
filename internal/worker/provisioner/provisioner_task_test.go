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

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/controller/authentication"
	corebase "github.com/juju/juju/core/base"
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
	"github.com/juju/juju/internal/worker/provisioner"
	"github.com/juju/juju/internal/worker/provisioner/mocks"
	providermocks "github.com/juju/juju/provider/common/mocks"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

const numProvisionWorkersForTesting = 4

type ProvisionerTaskSuite struct {
	testing.IsolationSuite

	setupDone            chan bool
	modelMachinesChanges chan []string
	modelMachinesWatcher watcher.StringsWatcher

	machineErrorRetryChanges chan struct{}
	machineErrorRetryWatcher watcher.NotifyWatcher

	taskAPI *mocks.MockTaskAPI

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
	// We expect no calls to the task API.
	defer s.setUpMocks(c).Finish()

	task := s.newProvisionerTask(c,
		config.HarvestAll,
		&mockDistributionGroupFinder{},
		mockToolsFinder{},
		numProvisionWorkersForTesting,
	)
	workertest.CheckAlive(c, task)
	workertest.CleanKill(c, task)

	err := workertest.CheckKilled(c, task)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.modelMachinesWatcher)
	c.Assert(err, jc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.machineErrorRetryWatcher)
	c.Assert(err, jc.ErrorIsNil)
	s.instanceBroker.CheckNoCalls(c)
}

func (s *ProvisionerTaskSuite) TestStopInstancesIgnoresMachinesWithKeep(c *gc.C) {
	defer s.setUpMocks(c).Finish()

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

	s.expectMachines(m0, m1)

	task := s.newProvisionerTask(c,
		config.HarvestAll,
		&mockDistributionGroupFinder{},
		mockToolsFinder{},
		numProvisionWorkersForTesting,
	)
	defer workertest.CleanKill(c, task)

	c.Assert(m0.markForRemoval, jc.IsFalse)
	c.Assert(m1.markForRemoval, jc.IsFalse)

	s.sendModelMachinesChange(c, "0", "1")

	s.waitForTask(c, []string{"AllRunningInstances", "StopInstances"})

	workertest.CleanKill(c, task)
	close(s.instanceBroker.callsChan)
	s.instanceBroker.CheckCalls(c, []testing.StubCall{
		{"AllRunningInstances", []interface{}{s.callCtx}},
		{"StopInstances", []interface{}{s.callCtx, []instance.Id{"zero"}}},
	})
	c.Assert(m0.markForRemoval, jc.IsTrue)
	c.Assert(m1.markForRemoval, jc.IsTrue)
}

func (s *ProvisionerTaskSuite) TestProvisionerRetries(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	m0 := &testMachine{id: "0"}
	s.taskAPI.EXPECT().MachinesWithTransientErrors().Return(
		[]apiprovisioner.MachineStatusResult{{Machine: m0, Status: params.StatusResult{}}}, nil)
	s.expectProvisioningInfo(m0)

	s.instanceBroker.SetErrors(
		errors.New("errors 1"),
		errors.New("errors 2"),
	)

	task := s.newProvisionerTaskWithRetry(c,
		config.HarvestAll,
		&mockDistributionGroupFinder{},
		mockToolsFinder{},
		provisioner.NewRetryStrategy(0*time.Second, 1),
		numProvisionWorkersForTesting,
	)

	s.sendMachineErrorRetryChange(c)

	s.waitForTask(c, []string{"StartInstance", "StartInstance"})

	workertest.CleanKill(c, task)
	close(s.instanceBroker.callsChan)
	s.auth.CheckCallNames(c, "SetupAuthentication")
	s.instanceBroker.CheckCallNames(c, "StartInstance", "StartInstance")
}

func (s *ProvisionerTaskSuite) TestEvenZonePlacement(c *gc.C) {
	ctrl := s.setUpMocks(c)
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

	for _, m := range machines {
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(&environs.StartInstanceResult{
			Instance: &testInstance{id: "instance-" + m.id},
		}, nil).Do(func(ctx, params interface{}) {
			zoneLock.Lock()
			usedZones = append(usedZones, params.(environs.StartInstanceParams).AvailabilityZone)
			zoneLock.Unlock()
		})
	}

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting)
	s.sendModelMachinesChange(c, "0", "1", "2", "3")

	retryCallArgs := retry.CallArgs{
		Clock:       clock.WallClock,
		MaxDuration: coretesting.LongWait,
		Delay:       10 * time.Millisecond,
		Func: func() error {
			zoneLock.Lock()
			if len(usedZones) == 4 {
				return nil
			}
			zoneLock.Unlock()
			return errors.Errorf("Not ready yet")
		},
	}
	err := retry.Call(retryCallArgs)
	c.Assert(err, jc.ErrorIsNil)

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
	ctrl := s.setUpMocks(c)
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
	}, nil).Do(func(context.ProviderCallContext, environs.StartInstanceParams) {
		go func() { started <- struct{}{} }()
	})
	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting)

	s.sendModelMachinesChange(c, "0")

	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no matching call to StartInstance")
	}

	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsNoZoneAvailable(c *gc.C) {
	ctrl := s.setUpMocks(c)
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

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting)
	s.sendModelMachinesChange(c, "0")
	s.waitForWorkerSetup(c)

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
	ctrl := s.setUpMocks(c)
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
	}, nil).Do(func(context.ProviderCallContext, environs.StartInstanceParams) {
		go func() { started <- struct{}{} }()
	})

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting)

	s.sendModelMachinesChange(c, "0")

	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no matching call to StartInstance")
	}

	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsNoDistributionGroupRetry(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	m0 := &testMachine{
		id:          "0",
		constraints: "zones=az1",
	}
	s.expectProvisioningInfo(m0)
	s.taskAPI.EXPECT().MachinesWithTransientErrors().Return(
		[]apiprovisioner.MachineStatusResult{{Machine: m0, Status: params.StatusResult{}}}, nil).MinTimes(1)

	broker := s.setUpZonedEnviron(ctrl)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az1")

	failedErr := errors.Errorf("oh no")
	// Use satisfaction of this call as the synchronisation point.
	started := make(chan struct{})
	gomock.InOrder(
		broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil).AnyTimes(), // may be called multiple times due to the changes in the provisioner task main logic.
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(nil, failedErr),
		broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil).AnyTimes(), // may be called multiple times due to the changes in the provisioner task main logic.
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(&environs.StartInstanceResult{
			Instance: &testInstance{id: "instance-1"},
		}, nil).Do(func(context.ProviderCallContext, environs.StartInstanceParams) {
			go func() { started <- struct{}{} }()
		}),
	)

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting)

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
	ctrl := s.setUpMocks(c)
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
	}, nil).Do(func(context.ProviderCallContext, environs.StartInstanceParams) {
		go func() { started <- struct{}{} }()
	})

	// Another machine from the same distribution group is already in az1,
	// so we expect the machine to be created in az2.
	task := s.newProvisionerTaskWithBroker(c, broker, map[names.MachineTag][]string{
		names.NewMachineTag("0"): {"az1"},
	}, numProvisionWorkersForTesting)

	s.sendModelMachinesChange(c, "0")
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no matching call to StartInstance")
	}

	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsWithDistributionGroupRetry(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	m0 := &testMachine{
		id:          "0",
		constraints: "zones=az1,az2",
	}
	s.expectProvisioningInfo(m0)
	s.taskAPI.EXPECT().MachinesWithTransientErrors().Return(
		[]apiprovisioner.MachineStatusResult{{Machine: m0, Status: params.StatusResult{}}}, nil).MinTimes(1)

	broker := s.setUpZonedEnviron(ctrl)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az1", "az2")

	// Use satisfaction of this call as the synchronisation point.
	failedErr := errors.Errorf("oh no")
	started := make(chan struct{})
	gomock.InOrder(
		broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil).AnyTimes(), // may be called multiple times due to the changes in the provisioner task main logic.
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(nil, failedErr),
		broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil).AnyTimes(), // may be called multiple times due to the changes in the provisioner task main logic.
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(&environs.StartInstanceResult{
			Instance: &testInstance{id: "instance-1"},
		}, nil).Do(func(context.ProviderCallContext, environs.StartInstanceParams) {
			go func() { started <- struct{}{} }()
		}),
	)

	// Another machine from the same distribution group is already in az1,
	// so we expect the machine to be created in az2.
	task := s.newProvisionerTaskWithBroker(c, broker, map[names.MachineTag][]string{
		names.NewMachineTag("0"): {"az1"},
	}, numProvisionWorkersForTesting)

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
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	m0 := &testMachine{
		id:          "0",
		constraints: "zones=az2",
	}
	s.expectProvisioningInfo(m0)
	s.taskAPI.EXPECT().MachinesWithTransientErrors().Return(
		[]apiprovisioner.MachineStatusResult{{Machine: m0, Status: params.StatusResult{}}}, nil).MinTimes(1)

	broker := s.setUpZonedEnviron(ctrl)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az2")

	// Use satisfaction of this call as the synchronisation point.
	failedErr := errors.Errorf("oh no")
	started := make(chan struct{})
	gomock.InOrder(
		broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil).AnyTimes(), // may be called multiple times due to the changes in the provisioner task main logic.
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(nil, failedErr),
		broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil).AnyTimes(), // may be called multiple times due to the changes in the provisioner task main logic.
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(&environs.StartInstanceResult{
			Instance: &testInstance{id: "instance-2"},
		}, nil).Do(func(context.ProviderCallContext, environs.StartInstanceParams) {
			go func() { started <- struct{}{} }()
		}),
	)

	// Another machine from the same distribution group is already in az1,
	// so we expect the machine to be created in az2.
	task := s.newProvisionerTaskWithBroker(c, broker, map[names.MachineTag][]string{
		names.NewMachineTag("0"): {"az2"},
		names.NewMachineTag("1"): {"az3"},
	}, numProvisionWorkersForTesting)

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

	broker := providermocks.NewMockZonedEnviron(ctrl)
	broker.EXPECT().AllRunningInstances(s.callCtx).Return(nil, errors.New("boom")).Do(func(context.ProviderCallContext) {
		go func() { close(s.setupDone) }()
	})

	task := s.newProvisionerTaskWithBroker(c, broker, map[names.MachineTag][]string{
		names.NewMachineTag("0"): {"az1"},
	}, numProvisionWorkersForTesting)
	s.sendModelMachinesChange(c, "0")
	s.waitForWorkerSetup(c)

	err := workertest.CheckKill(c, task)
	c.Assert(err, gc.ErrorMatches, "processing updated machines: getting all instances from broker: boom")
}

func (s *ProvisionerTaskSuite) TestDedupStopRequests(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	// m0 is a machine that should be terminated.
	i0 := &testInstance{id: "0"}
	s.instances = []instances.Instance{i0}
	m0 := &testMachine{
		id:       "0",
		life:     life.Dead,
		instance: i0,
	}
	broker := s.setUpZonedEnviron(ctrl, m0)

	// This is a complex scenario. Here is how everything is set up:
	//
	// We will register an event processed callback as a synchronization
	// point.
	//
	// The first machine change event will trigger a StopInstance call
	// against the broker. While in that call (i.e. the machine is still
	// being stopped from the provisioner's perspective), we will trigger
	// another machine change event for the same machine and wait until it
	// has been processed and the event processed callback invoked.
	//
	// Then, doneCh which instructs the test to perform a CleanKill for
	// the worker and make sure that no errors got reported.
	//
	// This verifies that machines being stopped are ignored when processing
	// machine changes concurrently.

	doneCh := make(chan struct{})
	barrier := make(chan struct{}, 1)
	var barrierCb = func(evt string) {
		if evt == "processed-machines" {
			barrier <- struct{}{}
		}
	}

	// StopInstances should only be called once for m0.
	broker.EXPECT().StopInstances(s.callCtx, gomock.Any()).Do(func(ctx interface{}, ids ...interface{}) {
		c.Assert(len(ids), gc.Equals, 1)
		c.Assert(ids[0], gc.DeepEquals, instance.Id("0"))

		// While one of the pool workers is executing this code, we
		// will wait until the machine change event gets processed
		// and the main loop is ready to process the next event.
		select {
		case <-barrier:
		case <-time.After(coretesting.LongWait):
			c.Errorf("timed out waiting for first processed-machines event")
		}

		// Trigger another change while machine 0 is still being stopped
		// and wait until the event has been processed by the provisioner
		// main loop before returning
		s.sendModelMachinesChange(c, "0")
		select {
		case <-barrier:
		case <-time.After(coretesting.LongWait):
			c.Errorf("timed out waiting for second processed-machines event")
		}
		close(doneCh)
	})

	task := s.newProvisionerTaskWithBrokerAndEventCb(c, broker, nil, numProvisionWorkersForTesting, barrierCb)

	s.sendModelMachinesChange(c, "0")

	// This ensures that the worker pool within the provisioner gets cleanly
	// shutdown and that any pending requests are processed.
	select {
	case <-doneCh:
	case <-time.After(3 * coretesting.LongWait):
		c.Errorf("timed out waiting for work to complete")
	}
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestDeferStopRequestsForMachinesStillProvisioning(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	s.instances = []instances.Instance{&testInstance{id: "0"}}

	// m0 is a machine that should be started.
	m0 := &testMachine{
		id:          "0",
		life:        life.Alive,
		constraints: "zones=az1",
	}

	broker := s.setUpZonedEnviron(ctrl, m0)

	// This is a complex scenario to ensure the provisioner works as expected
	// when the equivalent of "juju add-machine; juju remove-machine 0" is
	// executed. Here is how everything is set up:
	//
	// We will register an event processed callback as a synchronization
	// point.
	//
	// Machine 0 is alive but not yes started. Processing the first machine
	// change will trigger a StartInstance call against the broker.  While
	// in that call (i.e. the machine is still being started from the
	// provisioner's perspective), we will set the machine as dead, queue a
	// change event for the same machine and wait until it has been
	// processed and the event processed callback invoked.
	//
	// The change event for the dead machine should not immediately trigger
	// a StopInstance call but rather the provisioner will detect that the
	// machine is still being started and defer the stopping of the machine
	// until the machine gets started (immediately when StartInstance
	// returns).
	//
	// Finally, doneCh which instructs the test to perform a CleanKill for
	// the worker and make sure that no errors got reported.

	doneCh := make(chan struct{})
	barrier := make(chan struct{}, 1)
	var barrierCb = func(evt string) {
		if evt == "processed-machines" {
			barrier <- struct{}{}
		}
	}

	azConstraints := newAZConstraintStartInstanceParamsMatcher("az1")
	broker.EXPECT().DeriveAvailabilityZones(s.callCtx, azConstraints).Return([]string{}, nil).AnyTimes()
	gomock.InOrder(
		broker.EXPECT().StartInstance(s.callCtx, azConstraints).Return(&environs.StartInstanceResult{
			Instance: &testInstance{id: "instance-0"},
		}, nil).Do(func(ctx, params interface{}) {
			// While one of the pool workers is executing this code, we
			// will wait until the machine change event gets processed
			// and the main loop is ready to process the next event.
			select {
			case <-barrier:
			case <-time.After(coretesting.LongWait):
				c.Errorf("timed out waiting for first processed-machines event")
			}

			// While the machine is still starting, flag it as dead,
			// trigger another change and wait for it to be processed.
			// We expect that the defer stop flag is going to be set for
			// the machine and a StopInstance call to be issued once we
			// return.
			m0.life = life.Dead
			s.sendModelMachinesChange(c, "0")
			select {
			case <-barrier:
			case <-time.After(coretesting.LongWait):
				c.Errorf("timed out waiting for second processed-machines event")
			}
		}),
		broker.EXPECT().StopInstances(s.callCtx, gomock.Any()).Do(func(ctx interface{}, ids ...interface{}) {
			c.Assert(len(ids), gc.Equals, 1)
			c.Assert(ids[0], gc.DeepEquals, instance.Id("0"))

			// Signal the test to shut down the worker.
			close(doneCh)
		}),
	)

	task := s.newProvisionerTaskWithBrokerAndEventCb(c, broker, nil, numProvisionWorkersForTesting, barrierCb)

	// Send change for machine 0
	s.sendModelMachinesChange(c, "0")

	// This ensures that the worker pool within the provisioner gets cleanly
	// shutdown and that any pending requests are processed.
	select {
	case <-doneCh:
	case <-time.After(3 * coretesting.LongWait):
		c.Errorf("timed out waiting for work to complete")
	}
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestResizeWorkerPool(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	barrier := make(chan struct{}, 1)
	var barrierCb = func(evt string) {
		if evt == "resized-worker-pool" {
			close(barrier)
		}
	}

	broker := s.setUpZonedEnviron(ctrl)
	task := s.newProvisionerTaskWithBrokerAndEventCb(c, broker, nil, numProvisionWorkersForTesting, barrierCb)

	// Resize the pool
	task.SetNumProvisionWorkers(numProvisionWorkersForTesting + 1)

	<-barrier
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestUpdatedZonesReflectedInAZMachineSlice(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	s.instances = []instances.Instance{&testInstance{id: "0"}}
	m0 := &testMachine{id: "0", life: life.Alive}
	s.expectMachines(m0)
	s.expectProvisioningInfo(m0)

	broker := providermocks.NewMockZonedEnviron(ctrl)
	exp := broker.EXPECT()

	exp.AllRunningInstances(s.callCtx).Return(s.instances, nil).MinTimes(1)
	exp.InstanceAvailabilityZoneNames(s.callCtx, []instance.Id{s.instances[0].Id()}).Return(
		map[instance.Id]string{}, nil).Do(func(context.ProviderCallContext, []instance.Id) { close(s.setupDone) })

	az1 := providermocks.NewMockAvailabilityZone(ctrl)
	az1.EXPECT().Name().Return("az1").MinTimes(1)
	az1.EXPECT().Available().Return(true).MinTimes(1)

	az2 := providermocks.NewMockAvailabilityZone(ctrl)
	az2.EXPECT().Name().Return("az2").MinTimes(1)
	az2.EXPECT().Available().Return(true).MinTimes(1)

	az3 := providermocks.NewMockAvailabilityZone(ctrl)
	az3.EXPECT().Name().Return("az3").MinTimes(1)
	az3.EXPECT().Available().Return(true).MinTimes(1)

	// Return 1 availability zone on the first call, then 3, then 1 again.
	// See steps below punctuated by sending machine changes.
	gomock.InOrder(
		exp.AvailabilityZones(s.callCtx).Return(network.AvailabilityZones{az1}, nil),
		exp.AvailabilityZones(s.callCtx).Return(network.AvailabilityZones{az1, az2, az3}, nil),
		exp.AvailabilityZones(s.callCtx).Return(network.AvailabilityZones{az1}, nil),
	)

	step := make(chan struct{}, 1)

	// We really don't care about these calls.
	// StartInstance is just a synchronisation point.
	exp.DeriveAvailabilityZones(s.callCtx, gomock.Any()).Return([]string{}, nil).AnyTimes()
	exp.StartInstance(s.callCtx, gomock.Any()).Return(&environs.StartInstanceResult{
		Instance: &testInstance{id: "instance-0"},
	}, nil).AnyTimes().Do(func(context.ProviderCallContext, environs.StartInstanceParams) {
		select {
		case step <- struct{}{}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out writing to step channel")
		}
	})

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting)

	syncStep := func() {
		select {
		case <-step:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out reading from step channel")
		}
	}

	s.sendModelMachinesChange(c, "0")

	// After the first change, there is only one AZ in the tracker.
	syncStep()
	azm := provisioner.GetCopyAvailabilityZoneMachines(task)
	c.Assert(azm, gc.HasLen, 1)
	c.Assert(azm[0].ZoneName, gc.Equals, "az1")

	s.sendModelMachinesChange(c, "0")

	// After the second change, we see all 3 AZs.
	syncStep()
	azm = provisioner.GetCopyAvailabilityZoneMachines(task)
	c.Assert(azm, gc.HasLen, 3)
	c.Assert([]string{azm[0].ZoneName, azm[1].ZoneName, azm[2].ZoneName}, jc.SameContents, []string{"az1", "az2", "az3"})

	s.sendModelMachinesChange(c, "0")

	// At this point, we will have had a deployment to one of the zones added
	// in the prior step. This means one will be removed from tracking,
	// but the one we deployed to will not be deleted.
	syncStep()
	azm = provisioner.GetCopyAvailabilityZoneMachines(task)
	c.Assert(azm, gc.HasLen, 2)

	workertest.CleanKill(c, task)
}

// setUpZonedEnviron creates a mock environ with instances based on those set
// on the test suite, and 3 availability zones.
func (s *ProvisionerTaskSuite) setUpZonedEnviron(ctrl *gomock.Controller, machines ...*testMachine) *providermocks.MockZonedEnviron {
	broker := providermocks.NewMockZonedEnviron(ctrl)
	if len(machines) == 0 {
		return broker
	}

	s.expectMachines(machines...)
	s.expectProvisioningInfo(machines...)

	instanceIds := make([]instance.Id, len(s.instances))
	for i, inst := range s.instances {
		instanceIds[i] = inst.Id()
	}

	// Environ has 3 availability zones: az1, az2, az3.
	zones := make(network.AvailabilityZones, 3)
	for i := 0; i < 3; i++ {
		az := providermocks.NewMockAvailabilityZone(ctrl)
		az.EXPECT().Name().Return(fmt.Sprintf("az%d", i+1)).MinTimes(1)
		az.EXPECT().Available().Return(true).MinTimes(1)
		zones[i] = az
	}

	exp := broker.EXPECT()
	exp.AllRunningInstances(s.callCtx).Return(s.instances, nil).MinTimes(1)
	exp.InstanceAvailabilityZoneNames(s.callCtx, instanceIds).Return(map[instance.Id]string{}, nil).Do(
		func(context.ProviderCallContext, []instance.Id) { close(s.setupDone) },
	)
	exp.AvailabilityZones(s.callCtx).Return(zones, nil).MinTimes(1)
	return broker
}

func (s *ProvisionerTaskSuite) waitForWorkerSetup(c *gc.C) {
	select {
	case <-s.setupDone:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("worker not set up")
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
	numProvisionWorkers int,
) provisioner.ProvisionerTask {
	return s.newProvisionerTaskWithRetry(c,
		harvestingMethod,
		distributionGroupFinder,
		toolsFinder,
		provisioner.NewRetryStrategy(0*time.Second, 0),
		numProvisionWorkers,
	)
}

func (s *ProvisionerTaskSuite) newProvisionerTaskWithRetry(
	c *gc.C,
	harvestingMethod config.HarvestMode,
	distributionGroupFinder provisioner.DistributionGroupFinder,
	toolsFinder provisioner.ToolsFinder,
	retryStrategy provisioner.RetryStrategy,
	numProvisionWorkers int,
) provisioner.ProvisionerTask {
	w, err := provisioner.NewProvisionerTask(provisioner.TaskConfig{
		ControllerUUID:             coretesting.ControllerTag.Id(),
		HostTag:                    names.NewMachineTag("0"),
		Logger:                     loggo.GetLogger("test"),
		HarvestMode:                harvestingMethod,
		TaskAPI:                    s.taskAPI,
		DistributionGroupFinder:    distributionGroupFinder,
		ToolsFinder:                toolsFinder,
		MachineWatcher:             s.modelMachinesWatcher,
		RetryWatcher:               s.machineErrorRetryWatcher,
		Broker:                     s.instanceBroker,
		Auth:                       s.auth,
		ImageStream:                imagemetadata.ReleasedStream,
		RetryStartInstanceStrategy: retryStrategy,
		CloudCallContextFunc:       func(_ stdcontext.Context) context.ProviderCallContext { return s.callCtx },
		NumProvisionWorkers:        numProvisionWorkers,
	})
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *ProvisionerTaskSuite) newProvisionerTaskWithBroker(c *gc.C, broker environs.InstanceBroker, distributionGroups map[names.MachineTag][]string, numProvisionWorkers int) provisioner.ProvisionerTask {
	return s.newProvisionerTaskWithBrokerAndEventCb(c, broker, distributionGroups, numProvisionWorkers, nil)
}

func (s *ProvisionerTaskSuite) newProvisionerTaskWithBrokerAndEventCb(
	c *gc.C,
	broker environs.InstanceBroker,
	distributionGroups map[names.MachineTag][]string,
	numProvisionWorkers int,
	evtCb func(string),
) provisioner.ProvisionerTask {
	task, err := provisioner.NewProvisionerTask(provisioner.TaskConfig{
		ControllerUUID:             coretesting.ControllerTag.Id(),
		HostTag:                    names.NewMachineTag("0"),
		Logger:                     loggo.GetLogger("test"),
		HarvestMode:                config.HarvestAll,
		TaskAPI:                    s.taskAPI,
		DistributionGroupFinder:    &mockDistributionGroupFinder{groups: distributionGroups},
		ToolsFinder:                mockToolsFinder{},
		MachineWatcher:             s.modelMachinesWatcher,
		RetryWatcher:               s.machineErrorRetryWatcher,
		Broker:                     broker,
		Auth:                       s.auth,
		ImageStream:                imagemetadata.ReleasedStream,
		RetryStartInstanceStrategy: provisioner.NewRetryStrategy(0*time.Second, 0),
		CloudCallContextFunc:       func(_ stdcontext.Context) context.ProviderCallContext { return s.callCtx },
		NumProvisionWorkers:        numProvisionWorkers,
		EventProcessedCb:           evtCb,
	})
	c.Assert(err, jc.ErrorIsNil)
	return task
}

func (s *ProvisionerTaskSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.taskAPI = mocks.NewMockTaskAPI(ctrl)
	return ctrl
}

func (s *ProvisionerTaskSuite) expectMachines(machines ...*testMachine) {
	tags := transform.Slice(machines, func(m *testMachine) names.MachineTag {
		return names.NewMachineTag(m.id)
	})

	mResults := transform.Slice(machines, func(m *testMachine) apiprovisioner.MachineResult {
		return apiprovisioner.MachineResult{Machine: m}
	})

	s.taskAPI.EXPECT().Machines(tags).Return(mResults, nil).MinTimes(1)
}

func (s *ProvisionerTaskSuite) expectProvisioningInfo(machines ...*testMachine) {
	tags := transform.Slice(machines, func(m *testMachine) names.MachineTag {
		return names.NewMachineTag(m.id)
	})

	base, _ := corebase.GetBaseFromSeries(jujuversion.DefaultSupportedLTS())

	piResults := transform.Slice(machines, func(m *testMachine) params.ProvisioningInfoResult {
		machineCons := constraints.MustParse(m.constraints)
		paramCons := params.Value{
			Arch:             machineCons.Arch,
			Container:        machineCons.Container,
			CpuCores:         machineCons.CpuCores,
			CpuPower:         machineCons.CpuPower,
			Mem:              machineCons.Mem,
			RootDisk:         machineCons.RootDisk,
			RootDiskSource:   machineCons.RootDiskSource,
			Tags:             machineCons.Tags,
			InstanceRole:     machineCons.InstanceRole,
			InstanceType:     machineCons.InstanceType,
			Spaces:           machineCons.Spaces,
			VirtType:         machineCons.VirtType,
			Zones:            machineCons.Zones,
			AllocatePublicIP: machineCons.AllocatePublicIP,
			ImageID:          machineCons.ImageID,
		}

		return params.ProvisioningInfoResult{
			Result: &params.ProvisioningInfo{
				ControllerConfig:            coretesting.FakeControllerConfig(),
				Base:                        params.Base{Name: base.OS, Channel: base.Channel.String()},
				Constraints:                 paramCons,
				ProvisioningNetworkTopology: m.topology,
			},
			Error: nil,
		}
	})

	s.taskAPI.EXPECT().ProvisioningInfo(tags).Return(
		params.ProvisioningInfoResults{Results: piResults}, nil).AnyTimes()
}

type testInstanceBroker struct {
	*testing.Stub

	callsChan        chan string
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
