// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertask_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"github.com/kr/pretty"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	environmocks "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	providermocks "github.com/juju/juju/internal/provider/common/mocks"
	"github.com/juju/juju/internal/provisionertask"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

const (
	numProvisionWorkersForTesting = 4
	defaultHarvestMode            = config.HarvestAll
)

func machineInstanceInfoSetter(machineProvisionerAPI apiprovisioner.MachineProvisioner) func(
	ctx context.Context,
	id instance.Id, displayName string, nonce string, characteristics *instance.HardwareCharacteristics,
	networkConfig []params.NetworkConfig, volumes []params.Volume,
	volumeAttachments map[string]params.VolumeAttachmentInfo, charmProfiles []string,
) error {
	return machineProvisionerAPI.SetInstanceInfo
}

type ProvisionerTaskSuite struct {
	testhelpers.IsolationSuite

	setupDone            chan bool
	modelMachinesChanges chan []string
	modelMachinesWatcher watcher.StringsWatcher

	machineErrorRetryChanges chan struct{}
	machineErrorRetryWatcher watcher.NotifyWatcher

	controllerAPI *MockControllerAPI
	machinesAPI   *MockMachinesAPI

	instances      []instances.Instance
	instanceBroker *testInstanceBroker
}

var _ = tc.Suite(&ProvisionerTaskSuite{})

func (s *ProvisionerTaskSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.setupDone = make(chan bool)
	s.modelMachinesChanges = make(chan []string)
	s.modelMachinesWatcher = watchertest.NewMockStringsWatcher(s.modelMachinesChanges)

	s.machineErrorRetryChanges = make(chan struct{})
	s.machineErrorRetryWatcher = watchertest.NewMockNotifyWatcher(s.machineErrorRetryChanges)

	s.instances = []instances.Instance{}
	s.instanceBroker = &testInstanceBroker{
		Stub:      &testhelpers.Stub{},
		callsChan: make(chan string, 2),
		allInstancesFunc: func(ctx context.Context) ([]instances.Instance, error) {
			return s.instances, s.instanceBroker.NextErr()
		},
	}
}

func (s *ProvisionerTaskSuite) TestStartStop(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.modelMachinesWatcher)
	c.Assert(err, tc.ErrorIsNil)
	err = workertest.CheckKilled(c, s.machineErrorRetryWatcher)
	c.Assert(err, tc.ErrorIsNil)
	s.instanceBroker.CheckNoCalls(c)
}

func (s *ProvisionerTaskSuite) TestStopInstancesIgnoresMachinesWithKeep(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	i0 := &testInstance{id: "zero"}
	i1 := &testInstance{id: "one"}
	s.instances = []instances.Instance{
		i0,
		i1,
	}

	m0 := &testMachine{
		c:        c,
		id:       "0",
		life:     life.Dead,
		instance: i0,
	}
	m1 := &testMachine{
		c:            c,
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

	c.Assert(m0.markForRemoval, tc.IsFalse)
	c.Assert(m1.markForRemoval, tc.IsFalse)

	s.sendModelMachinesChange(c, "0", "1")

	s.waitForTask(c, []string{"AllRunningInstances", "StopInstances"})

	workertest.CleanKill(c, task)
	close(s.instanceBroker.callsChan)
	s.instanceBroker.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "AllRunningInstances"},
		{FuncName: "StopInstances", Args: []interface{}{[]instance.Id{"zero"}}},
	})
	c.Assert(m0.markForRemoval, tc.IsTrue)
	c.Assert(m1.markForRemoval, tc.IsTrue)
}

func (s *ProvisionerTaskSuite) TestProvisionerRetries(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	m0 := &testMachine{c: c, id: "0"}
	s.machinesAPI.EXPECT().MachinesWithTransientErrors(gomock.Any()).Return(
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
		provisionertask.RetryStrategy{
			RetryDelay: 0 * time.Second,
			RetryCount: 1,
		},
		numProvisionWorkersForTesting,
	)

	s.sendMachineErrorRetryChange(c)

	s.waitForTask(c, []string{"StartInstance", "StartInstance"})

	workertest.CleanKill(c, task)
	close(s.instanceBroker.callsChan)
	c.Assert(m0.password, tc.Not(tc.Equals), "")
	s.instanceBroker.CheckCallNames(c, "StartInstance", "StartInstance")
}

func (s *ProvisionerTaskSuite) waitForProvisioned(c *tc.C, m *testMachine) {
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		_, err := m.InstanceId(c.Context())
		if err == nil {
			if m.GetPassword() == "" {
				c.Fatalf("provisioned machine %q does not have a password", m.id)
			}
			return
		}
	}
	c.Fatalf("machine %q not started", m.id)
}

func (s *ProvisionerTaskSuite) waitForRemovalMark(c *tc.C, m *testMachine) {
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if m.GetMarkForRemoval() {
			return
		}
	}
	c.Fatalf("machine %q not marked for removal", m.id)
}

func (s *ProvisionerTaskSuite) waitForInstanceStatus(c *tc.C, m *testMachine, status status.Status) string {
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		instStatus, info, err := m.InstanceStatus(c.Context())
		c.Assert(err, tc.ErrorIsNil)
		if instStatus == status {
			return info
		}
	}
	c.Fatalf("machine %q did not have expected status, instead: %v", m.id, m.instStatus)
	return ""
}

var (
	validCloudInitUserData = map[string]interface{}{
		"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false,
	}
	possibleImageMetadata = []*imagemetadata.ImageMetadata{{
		Id:          "image-12334",
		Arch:        "amd64",
		RegionName:  "west",
		RegionAlias: "west",
		Stream:      "proposed",
		Version:     "6.6.6",
	}}
)

func (s *ProvisionerTaskSuite) TestSetUpToStartMachine(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	task := s.newProvisionerTask(c,
		config.HarvestAll,
		&mockDistributionGroupFinder{},
		mockToolsFinder{},
		numProvisionWorkersForTesting,
	)
	defer workertest.CleanKill(c, task)

	m0 := &testMachine{c: c, id: "0"}
	vers := semversion.MustParse("2.99.0")
	res := params.ProvisioningInfoResult{
		Result: &params.ProvisioningInfo{
			Constraints: constraints.MustParse("mem=666G"),
			Base:        params.Base{Name: "ubuntu", Channel: "22.04"},
			Placement:   "foo=bar",
			Tags:        map[string]string{"hello": "world"},
			ImageMetadata: []params.CloudImageMetadata{{
				ImageId: "image-12334",
				Arch:    "amd64",
				Region:  "west",
				Stream:  "proposed",
				Version: "6.6.6",
			}},
			EndpointBindings:            map[string]string{"endpoint": "space"},
			ControllerConfig:            coretesting.FakeControllerConfig(),
			CloudInitUserData:           validCloudInitUserData,
			CharmLXDProfiles:            []string{"p1", "p2"},
			ProvisioningNetworkTopology: params.ProvisioningNetworkTopology{},
		},
	}
	startInstanceParams, err := provisionertask.SetupToStartMachine(task, m0, &vers, res)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(startInstanceParams.InstanceConfig, tc.NotNil)
	c.Assert(startInstanceParams.InstanceConfig.APIInfo, tc.NotNil)
	c.Assert(startInstanceParams.InstanceConfig.APIInfo.Password, tc.Not(tc.Equals), "")
	startInstanceParams.InstanceConfig.APIInfo.Password = ""
	c.Assert(startInstanceParams.InstanceConfig.MachineNonce, tc.Not(tc.Equals), "")
	startInstanceParams.InstanceConfig.MachineNonce = ""
	c.Assert(startInstanceParams.StatusCallback, tc.NotNil)
	startInstanceParams.StatusCallback = nil
	c.Assert(startInstanceParams.Abort, tc.NotNil)
	startInstanceParams.Abort = nil

	want := machineStartInstanceArg("0")
	want.Constraints = constraints.MustParse("mem=666G")
	want.Placement = "foo=bar"
	want.InstanceConfig.Tags = map[string]string{"hello": "world"}
	want.InstanceConfig.CloudInitUserData = validCloudInitUserData
	want.ImageMetadata = possibleImageMetadata
	want.EndpointBindings = map[string]network.Id{"endpoint": "space"}
	want.CharmLXDProfiles = []string{"p1", "p2"}
	c.Assert(startInstanceParams, tc.DeepEquals, *want)
}

func (s *ProvisionerTaskSuite) TestProvisionerSetsErrorStatusWhenNoToolsAreAvailable(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	task := s.newProvisionerTask(c,
		config.HarvestAll,
		&mockDistributionGroupFinder{},
		mockToolsFinder{},
		numProvisionWorkersForTesting,
	)
	defer workertest.CleanKill(c, task)

	m0 := &testMachine{
		c:            c,
		id:           "0",
		agentVersion: semversion.MustParse("6.6.6"),
	}

	s.expectMachines(m0)
	s.expectProvisioningInfo(m0)
	s.sendModelMachinesChange(c, "0")

	// Ensure machine error status was set, and the error matches
	msg := s.waitForInstanceStatus(c, m0, status.ProvisioningError)
	c.Check(msg, tc.Equals, "no matching agent binaries available")
}

func (s *ProvisionerTaskSuite) TestEvenZonePlacement(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	// There are 3 available usedZones, so test with 4 machines
	// to ensure even spread across usedZones.
	machines := []*testMachine{{
		c:  c,
		id: "0",
	}, {
		c:  c,
		id: "1",
	}, {
		c:  c,
		id: "2",
	}, {
		c:  c,
		id: "3",
	}}
	broker := s.setUpZonedEnviron(ctrl, machines...)
	azConstraints := newAZConstraintStartInstanceParamsMatcher()
	broker.EXPECT().DeriveAvailabilityZones(gomock.Any(), azConstraints).Return([]string{}, nil).Times(len(machines))

	// We need to coordinate access to usedZones by the worker, executing the
	// expectations below on a separate Goroutine, and the test logic.
	zoneLock := sync.Mutex{}
	var usedZones []string

	for _, m := range machines {
		broker.EXPECT().StartInstance(gomock.Any(), azConstraints).
			DoAndReturn(func(ctx context.Context, params environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
				zoneLock.Lock()
				usedZones = append(usedZones, params.AvailabilityZone)
				zoneLock.Unlock()
				return &environs.StartInstanceResult{
					Instance: &testInstance{id: "instance-" + m.id},
				}, nil
			})
	}

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, defaultHarvestMode)
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
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(set.NewStrings(usedZones...).SortedValues(), tc.DeepEquals, []string{"az1", "az2", "az3"})

	for _, m := range machines {
		c.Assert(m.password, tc.Not(tc.Equals), "")
	}
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestMultipleSpaceConstraints(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	m0 := &testMachine{
		c:           c,
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

	broker.EXPECT().DeriveAvailabilityZones(gomock.Any(), spaceConstraints).Return([]string{}, nil)

	// Use satisfaction of this call as the synchronisation point.
	broker.EXPECT().StartInstance(gomock.Any(), spaceConstraints).Return(&environs.StartInstanceResult{
		Instance: &testInstance{id: "instance-0"},
	}, nil)
	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, defaultHarvestMode)

	s.sendModelMachinesChange(c, "0")
	s.waitForProvisioned(c, m0)
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsNoZoneAvailable(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	m0 := &testMachine{
		c:           c,
		id:          "0",
		constraints: "zones=az9",
	}
	broker := s.setUpZonedEnviron(ctrl, m0)

	// Constraint for availability zone az9 can not be satisfied;
	// this broker only knows of az1, az2, az3.
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az9")
	broker.EXPECT().DeriveAvailabilityZones(gomock.Any(), azConstraints).Return([]string{}, nil)

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, defaultHarvestMode)
	s.sendModelMachinesChange(c, "0")
	s.waitForWorkerSetup(c)

	// Wait for instance status to be set.
	msg := s.waitForInstanceStatus(c, m0, status.ProvisioningError)
	c.Check(msg, tc.Equals, "suitable availability zone for machine 0 not found")

	c.Assert(m0.password, tc.Not(tc.Equals), "")
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsNoDistributionGroup(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	m0 := &testMachine{
		c:           c,
		id:          "0",
		constraints: "zones=az1",
	}
	broker := s.setUpZonedEnviron(ctrl, m0)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az1")
	broker.EXPECT().DeriveAvailabilityZones(gomock.Any(), azConstraints).Return([]string{}, nil)

	// For the call to start instance, we expect the same zone constraint to
	// be present, but we also expect that the zone in start instance params
	// matches the constraint, based on being available in this broker.
	azConstraintsAndDerivedZone := newAZConstraintStartInstanceParamsMatcher("az1")
	azConstraintsAndDerivedZone.addMatch("availability zone: az1", func(p environs.StartInstanceParams) bool {
		return p.AvailabilityZone == "az1"
	})

	// Use satisfaction of this call as the synchronisation point.
	broker.EXPECT().StartInstance(gomock.Any(), azConstraints).Return(&environs.StartInstanceResult{
		Instance: &testInstance{id: "instance-0"},
	}, nil)
	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, defaultHarvestMode)

	s.sendModelMachinesChange(c, "0")
	s.waitForProvisioned(c, m0)
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsNoDistributionGroupRetry(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	m0 := &testMachine{
		c:           c,
		id:          "0",
		constraints: "zones=az1",
	}
	s.expectProvisioningInfo(m0)
	s.machinesAPI.EXPECT().MachinesWithTransientErrors(gomock.Any()).Return(
		[]apiprovisioner.MachineStatusResult{{Machine: m0, Status: params.StatusResult{}}}, nil).MinTimes(1)

	broker := s.setUpZonedEnviron(ctrl)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az1")

	failedErr := errors.Errorf("oh no")
	// Use satisfaction of this call as the synchronisation point.
	gomock.InOrder(
		broker.EXPECT().DeriveAvailabilityZones(gomock.Any(), azConstraints).Return([]string{}, nil).AnyTimes(), // may be called multiple times due to the changes in the provisioner task main logic.
		broker.EXPECT().StartInstance(gomock.Any(), azConstraints).Return(nil, failedErr),
		broker.EXPECT().DeriveAvailabilityZones(gomock.Any(), azConstraints).Return([]string{}, nil).AnyTimes(), // may be called multiple times due to the changes in the provisioner task main logic.
		broker.EXPECT().StartInstance(gomock.Any(), azConstraints).Return(&environs.StartInstanceResult{
			Instance: &testInstance{id: "instance-1"},
		}, nil),
	)

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, defaultHarvestMode)

	s.sendMachineErrorRetryChange(c)
	s.sendMachineErrorRetryChange(c)
	s.waitForProvisioned(c, m0)
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsWithDistributionGroup(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	m0 := &testMachine{
		c:           c,
		id:          "0",
		constraints: "zones=az1,az2",
	}

	broker := s.setUpZonedEnviron(ctrl, m0)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az1", "az2")
	broker.EXPECT().DeriveAvailabilityZones(gomock.Any(), azConstraints).Return([]string{}, nil)

	// For the call to start instance, we expect the same zone constraints to
	// be present, but we also expect that the zone in start instance params
	// was selected from the constraints, based on a machine from the same
	// distribution group already being in one of the zones.
	azConstraintsAndDerivedZone := newAZConstraintStartInstanceParamsMatcher("az1", "az2")
	azConstraintsAndDerivedZone.addMatch("availability zone: az2", func(p environs.StartInstanceParams) bool {
		return p.AvailabilityZone == "az2"
	})

	// Use satisfaction of this call as the synchronisation point.
	broker.EXPECT().StartInstance(gomock.Any(), azConstraints).Return(&environs.StartInstanceResult{
		Instance: &testInstance{id: "instance-0"},
	}, nil)

	// Another machine from the same distribution group is already in az1,
	// so we expect the machine to be created in az2.
	task := s.newProvisionerTaskWithBroker(c, broker, map[names.MachineTag][]string{
		names.NewMachineTag("0"): {"az1"},
	}, numProvisionWorkersForTesting, defaultHarvestMode)

	s.sendModelMachinesChange(c, "0")
	s.waitForProvisioned(c, m0)
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneConstraintsWithDistributionGroupRetry(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	m0 := &testMachine{
		c:           c,
		id:          "0",
		constraints: "zones=az1,az2",
	}
	s.expectProvisioningInfo(m0)
	s.machinesAPI.EXPECT().MachinesWithTransientErrors(gomock.Any()).Return(
		[]apiprovisioner.MachineStatusResult{{Machine: m0, Status: params.StatusResult{}}}, nil).MinTimes(1)

	broker := s.setUpZonedEnviron(ctrl)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az1", "az2")

	// Use satisfaction of this call as the synchronisation point.
	failedErr := errors.Errorf("oh no")
	gomock.InOrder(
		broker.EXPECT().DeriveAvailabilityZones(gomock.Any(), azConstraints).Return([]string{}, nil).AnyTimes(), // may be called multiple times due to the changes in the provisioner task main logic.
		broker.EXPECT().StartInstance(gomock.Any(), azConstraints).Return(nil, failedErr),
		broker.EXPECT().DeriveAvailabilityZones(gomock.Any(), azConstraints).Return([]string{}, nil).AnyTimes(), // may be called multiple times due to the changes in the provisioner task main logic.
		broker.EXPECT().StartInstance(gomock.Any(), azConstraints).Return(&environs.StartInstanceResult{
			Instance: &testInstance{id: "instance-1"},
		}, nil),
	)

	// Another machine from the same distribution group is already in az1,
	// so we expect the machine to be created in az2.
	task := s.newProvisionerTaskWithBroker(c, broker, map[names.MachineTag][]string{
		names.NewMachineTag("0"): {"az1"},
	}, numProvisionWorkersForTesting, defaultHarvestMode)

	s.sendMachineErrorRetryChange(c)
	s.sendMachineErrorRetryChange(c)
	s.waitForProvisioned(c, m0)
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestZoneRestrictiveConstraintsWithDistributionGroupRetry(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	m0 := &testMachine{
		c:           c,
		id:          "0",
		constraints: "zones=az2",
	}
	s.expectProvisioningInfo(m0)
	s.machinesAPI.EXPECT().MachinesWithTransientErrors(gomock.Any()).Return(
		[]apiprovisioner.MachineStatusResult{{Machine: m0, Status: params.StatusResult{}}}, nil).MinTimes(1)

	broker := s.setUpZonedEnviron(ctrl)
	azConstraints := newAZConstraintStartInstanceParamsMatcher("az2")

	// Use satisfaction of this call as the synchronisation point.
	failedErr := errors.Errorf("oh no")
	gomock.InOrder(
		broker.EXPECT().DeriveAvailabilityZones(gomock.Any(), azConstraints).Return([]string{}, nil).AnyTimes(), // may be called multiple times due to the changes in the provisioner task main logic.
		broker.EXPECT().StartInstance(gomock.Any(), azConstraints).Return(nil, failedErr),
		broker.EXPECT().DeriveAvailabilityZones(gomock.Any(), azConstraints).Return([]string{}, nil).AnyTimes(), // may be called multiple times due to the changes in the provisioner task main logic.
		broker.EXPECT().StartInstance(gomock.Any(), azConstraints).Return(&environs.StartInstanceResult{
			Instance: &testInstance{id: "instance-2"},
		}, nil),
	)

	// Another machine from the same distribution group is already in az1,
	// so we expect the machine to be created in az2.
	task := s.newProvisionerTaskWithBroker(c, broker, map[names.MachineTag][]string{
		names.NewMachineTag("0"): {"az2"},
		names.NewMachineTag("1"): {"az3"},
	}, numProvisionWorkersForTesting, defaultHarvestMode)

	s.sendMachineErrorRetryChange(c)
	s.sendMachineErrorRetryChange(c)
	s.waitForProvisioned(c, m0)
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestPopulateAZMachinesErrorWorkerStopped(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := providermocks.NewMockZonedEnviron(ctrl)
	broker.EXPECT().AllRunningInstances(gomock.Any()).DoAndReturn(func(ctx context.Context) ([]instances.Instance, error) {
		go func() { close(s.setupDone) }()
		return nil, errors.New("boom")
	})

	task := s.newProvisionerTaskWithBroker(c, broker, map[names.MachineTag][]string{
		names.NewMachineTag("0"): {"az1"},
	}, numProvisionWorkersForTesting, defaultHarvestMode)
	s.sendModelMachinesChange(c, "0")
	s.waitForWorkerSetup(c)

	err := workertest.CheckKill(c, task)
	c.Assert(err, tc.ErrorMatches, "processing updated machines: getting all instances from broker: boom")
}

func (s *ProvisionerTaskSuite) TestDedupStopRequests(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	// m0 is a machine that should be terminated.
	i0 := &testInstance{id: "0"}
	s.instances = []instances.Instance{i0}
	m0 := &testMachine{
		c:        c,
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
	broker.EXPECT().StopInstances(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, ids ...instance.Id) error {
		c.Assert(len(ids), tc.Equals, 1)
		c.Assert(ids[0], tc.DeepEquals, instance.Id("0"))

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

		return nil
	})

	task := s.newProvisionerTaskWithBrokerAndEventCb(c, broker, nil, numProvisionWorkersForTesting, defaultHarvestMode, barrierCb)

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

func (s *ProvisionerTaskSuite) TestDeferStopRequestsForMachinesStillProvisioning(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	s.instances = []instances.Instance{&testInstance{id: "0"}}

	// m0 is a machine that should be started.
	m0 := &testMachine{
		c:           c,
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
	broker.EXPECT().DeriveAvailabilityZones(gomock.Any(), azConstraints).Return([]string{}, nil).AnyTimes()
	gomock.InOrder(
		broker.EXPECT().StartInstance(gomock.Any(), azConstraints).DoAndReturn(func(ctx context.Context, params environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
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
			m0.SetLife(life.Dead)
			s.sendModelMachinesChange(c, "0")
			select {
			case <-barrier:
			case <-time.After(coretesting.LongWait):
				c.Errorf("timed out waiting for second processed-machines event")
			}
			return &environs.StartInstanceResult{
				Instance: &testInstance{id: "instance-0"},
			}, nil
		}),
		broker.EXPECT().StopInstances(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, ids ...instance.Id) error {
			c.Assert(len(ids), tc.Equals, 1)
			c.Assert(ids[0], tc.DeepEquals, instance.Id("0"))

			// Signal the test to shut down the worker.
			close(doneCh)
			return nil
		}),
	)

	task := s.newProvisionerTaskWithBrokerAndEventCb(c, broker, nil, numProvisionWorkersForTesting, defaultHarvestMode, barrierCb)

	// Send change for machine 0
	s.sendModelMachinesChange(c, "0")

	// This ensures that the worker pool within the provisioner gets cleanly
	// shutdown and that any pending requests are processed.
	select {
	case <-doneCh:
	case <-time.After(3 * coretesting.LongWait):
		c.Errorf("timed out waiting for work to complete")
	}
	c.Assert(m0.password, tc.Not(tc.Equals), "")
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestResizeWorkerPool(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	barrier := make(chan struct{}, 1)
	var barrierCb = func(evt string) {
		if evt == "resized-worker-pool" {
			close(barrier)
		}
	}

	broker := s.setUpZonedEnviron(ctrl)
	task := s.newProvisionerTaskWithBrokerAndEventCb(c, broker, nil, numProvisionWorkersForTesting, defaultHarvestMode, barrierCb)

	// Resize the pool
	task.SetNumProvisionWorkers(numProvisionWorkersForTesting + 1)

	<-barrier
	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestUpdatedZonesReflectedInAZMachineSlice(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	s.instances = []instances.Instance{&testInstance{id: "0"}}
	m0 := &testMachine{c: c, id: "0", life: life.Alive}
	s.expectMachines(m0)
	s.expectProvisioningInfo(m0)

	broker := providermocks.NewMockZonedEnviron(ctrl)
	exp := broker.EXPECT()

	exp.AllRunningInstances(gomock.Any()).Return(s.instances, nil).MinTimes(1)
	exp.InstanceAvailabilityZoneNames(gomock.Any(), []instance.Id{s.instances[0].Id()}).
		DoAndReturn(func(context.Context, []instance.Id) (map[instance.Id]string, error) {
			close(s.setupDone)
			return map[instance.Id]string{}, nil
		})

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
		exp.AvailabilityZones(gomock.Any()).Return(network.AvailabilityZones{az1}, nil),
		exp.AvailabilityZones(gomock.Any()).Return(network.AvailabilityZones{az1, az2, az3}, nil),
		exp.AvailabilityZones(gomock.Any()).Return(network.AvailabilityZones{az1}, nil),
	)

	step := make(chan struct{}, 1)

	// We really don't care about these calls.
	// StartInstance is just a synchronisation point.
	exp.DeriveAvailabilityZones(gomock.Any(), gomock.Any()).Return([]string{}, nil).AnyTimes()
	exp.StartInstance(gomock.Any(), gomock.Any()).Return(&environs.StartInstanceResult{
		Instance: &testInstance{id: "instance-0"},
	}, nil).MinTimes(1).Do(func(context.Context, environs.StartInstanceParams) {
		select {
		case step <- struct{}{}:
		case <-time.After(testhelpers.LongWait):
			c.Fatalf("timed out writing to step channel")
		}
	})

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, defaultHarvestMode)

	syncStep := func() {
		select {
		case <-step:
		case <-time.After(testhelpers.LongWait):
			c.Fatalf("timed out reading from step channel")
		}
	}

	s.sendModelMachinesChange(c, "0")

	// After the first change, there is only one AZ in the tracker.
	syncStep()
	azm := provisionertask.GetCopyAvailabilityZoneMachines(task)
	c.Assert(azm, tc.HasLen, 1)
	c.Assert(azm[0].ZoneName, tc.Equals, "az1")

	m0.SetUnprovisioned()
	s.sendModelMachinesChange(c, "0")

	// After the second change, we see all 3 AZs.
	syncStep()
	azm = provisionertask.GetCopyAvailabilityZoneMachines(task)
	c.Assert(azm, tc.HasLen, 3)
	c.Assert([]string{azm[0].ZoneName, azm[1].ZoneName, azm[2].ZoneName}, tc.SameContents, []string{"az1", "az2", "az3"})

	m0.SetUnprovisioned()
	s.sendModelMachinesChange(c, "0")

	// At this point, we will have had a deployment to one of the zones added
	// in the prior step. This means one will be removed from tracking,
	// but the one we deployed to will not be deleted.
	syncStep()
	azm = provisionertask.GetCopyAvailabilityZoneMachines(task)
	c.Assert(azm, tc.HasLen, 2)

	workertest.CleanKill(c, task)
}

func (s *ProvisionerTaskSuite) TestHarvestUnknownReapsOnlyUnknown(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	inst0 := &testInstance{id: "0"}
	m0 := &testMachine{c: c, id: "0", life: life.Alive, instance: inst0}
	instUnknown := &testInstance{id: "unknown"}
	s.instances = []instances.Instance{inst0, instUnknown}

	s.expectMachines(m0)

	broker := environmocks.NewMockEnviron(ctrl)
	exp := broker.EXPECT()
	exp.AllRunningInstances(gomock.Any()).Return(s.instances, nil).MinTimes(1)
	// Only stop the unknown instance.
	exp.StopInstances(gomock.Any(), []instance.Id{"unknown"}).Return(nil)

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, config.HarvestUnknown)
	defer workertest.CleanKill(c, task)

	m0.SetLife(life.Dead)
	s.sendModelMachinesChange(c, "0")
	s.waitForRemovalMark(c, m0)
}

func (s *ProvisionerTaskSuite) TestHarvestDestroyedReapsOnlyDestroyed(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	inst0 := &testInstance{id: "0"}
	m0 := &testMachine{c: c, id: "0", life: life.Alive, instance: inst0}
	instUnknown := &testInstance{id: "unknown"}
	s.instances = []instances.Instance{inst0, instUnknown}

	s.expectMachines(m0)

	broker := environmocks.NewMockEnviron(ctrl)
	exp := broker.EXPECT()
	exp.AllRunningInstances(gomock.Any()).Return(s.instances, nil).MinTimes(1)
	// Only stop the dead instance.
	exp.StopInstances(gomock.Any(), []instance.Id{"0"}).Return(nil).AnyTimes()

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, config.HarvestDestroyed)
	defer workertest.CleanKill(c, task)

	// This results in no action.
	s.sendModelMachinesChange(c, "0")

	m0.SetLife(life.Dead)
	s.sendModelMachinesChange(c, "0")
	s.waitForRemovalMark(c, m0)
}

func (s *ProvisionerTaskSuite) TestHarvestAllReapsAllTheThings(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	inst0 := &testInstance{id: "0"}
	m0 := &testMachine{c: c, id: "0", life: life.Alive, instance: inst0}
	instUnknown := &testInstance{id: "unknown"}
	s.instances = []instances.Instance{inst0, instUnknown}

	s.expectMachines(m0)

	broker := environmocks.NewMockEnviron(ctrl)
	exp := broker.EXPECT()
	exp.AllRunningInstances(gomock.Any()).Return(s.instances, nil).MinTimes(1)
	// Stop both instances.
	exp.StopInstances(gomock.Any(), []instance.Id{"0", "unknown"}).Return(nil)

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, config.HarvestAll)
	defer workertest.CleanKill(c, task)

	m0.SetLife(life.Dead)
	s.sendModelMachinesChange(c, "0")
	s.waitForRemovalMark(c, m0)
}

func (s *ProvisionerTaskSuite) TestProvisionerStopRetryingIfDying(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	m0 := &testMachine{c: c, id: "0"}
	s.machinesAPI.EXPECT().MachinesWithTransientErrors(gomock.Any()).Return(
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
		provisionertask.RetryStrategy{
			RetryDelay: 10 * time.Second,
			RetryCount: 1,
		},
		numProvisionWorkersForTesting,
	)

	s.sendMachineErrorRetryChange(c)

	s.waitForTask(c, []string{"StartInstance"})

	workertest.CleanKill(c, task)
	close(s.instanceBroker.callsChan)
	c.Assert(m0.password, tc.Not(tc.Equals), "")
	s.instanceBroker.CheckCallNames(c, "StartInstance")

	statusInfo, _, err := m0.Status(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statusInfo, tc.Equals, status.Pending)
	statusInfo, _, err = m0.InstanceStatus(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	if statusInfo != status.Pending && statusInfo != status.Provisioning {
		c.Errorf("statusInfo.Status was %q not one of %q or %q",
			statusInfo, status.Pending, status.Provisioning)
	}
}

func (s *ProvisionerTaskSuite) TestMachineErrorsRetainInstances(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	// create a machine
	inst0 := &testInstance{id: "0"}
	// create an instance out of band
	instUnknown := &testInstance{id: "unknown"}

	s.instances = []instances.Instance{inst0, instUnknown}
	s.machinesAPI.EXPECT().Machines(gomock.Any(), names.NewMachineTag("0")).Return([]apiprovisioner.MachineResult{{
		Err: &params.Error{Code: "some error"},
	}}, nil).MinTimes(1)

	// start the provisioner and ensure it doesn't kill any
	// instances if there are errors getting machines.
	task := s.newProvisionerTask(c,
		config.HarvestAll,
		&mockDistributionGroupFinder{},
		mockToolsFinder{},
		numProvisionWorkersForTesting,
	)
	s.sendModelMachinesChange(c, "0")
	c.Assert(worker.Stop(task), tc.ErrorMatches, ".*getting machine.*")
}

func (s *ProvisionerTaskSuite) TestProvisioningMachinesWithRequestedRootDisk(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	m0 := &testMachine{c: c, id: "0", life: life.Alive}
	s.expectMachines(m0)

	s.machinesAPI.EXPECT().ProvisioningInfo(gomock.Any(), []names.MachineTag{names.NewMachineTag("0")}).Return(
		params.ProvisioningInfoResults{Results: []params.ProvisioningInfoResult{{
			Result: &params.ProvisioningInfo{
				ControllerConfig: coretesting.FakeControllerConfig(),
				Base:             params.Base{Name: "ubuntu", Channel: "22.04"},
				RootDisk: &params.VolumeParams{
					Provider:   "static",
					Attributes: map[string]interface{}{"persistent": true},
				},
			},
		}}}, nil)

	broker := environmocks.NewMockEnviron(ctrl)
	exp := broker.EXPECT()

	exp.AllRunningInstances(gomock.Any()).Return(s.instances, nil).MinTimes(1)

	startArg := machineStartInstanceArg("0")
	startArg.RootDisk = &storage.VolumeParams{
		Provider:   "static",
		Attributes: map[string]interface{}{"persistent": true},
	}
	exp.StartInstance(gomock.Any(), newDefaultStartInstanceParamsMatcher(c, startArg)).Return(&environs.StartInstanceResult{
		Instance: &testInstance{id: "instance-0"},
	}, nil)

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, config.HarvestAll)
	defer workertest.CleanKill(c, task)

	s.sendModelMachinesChange(c, "0")
	s.waitForProvisioned(c, m0)
}

func (s *ProvisionerTaskSuite) TestProvisioningMachinesWithRequestedVolumes(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	m0 := &testMachine{c: c, id: "0", life: life.Alive}
	s.expectMachines(m0)

	s.machinesAPI.EXPECT().ProvisioningInfo(gomock.Any(), []names.MachineTag{names.NewMachineTag("0")}).Return(
		params.ProvisioningInfoResults{Results: []params.ProvisioningInfoResult{{
			Result: &params.ProvisioningInfo{
				ControllerConfig: coretesting.FakeControllerConfig(),
				Base:             params.Base{Name: "ubuntu", Channel: "22.04"},
				Volumes: []params.VolumeParams{{
					VolumeTag: "volume-0",
					Size:      1024,
					Provider:  "static",
					Attachment: &params.VolumeAttachmentParams{
						MachineTag: "machine-0",
						ReadOnly:   true,
					},
				}, {
					VolumeTag:  "volume-1",
					Size:       2048,
					Provider:   "persistent-pool",
					Attributes: map[string]interface{}{"persistent": true},
					Attachment: &params.VolumeAttachmentParams{
						MachineTag: "machine-0",
					},
				}},
				VolumeAttachments: []params.VolumeAttachmentParams{{
					VolumeTag:  "volume-1",
					MachineTag: "machine-0",
					Provider:   "static",
					ReadOnly:   true,
					VolumeId:   "666",
				}},
			},
		}}}, nil)

	broker := environmocks.NewMockEnviron(ctrl)
	exp := broker.EXPECT()

	exp.AllRunningInstances(gomock.Any()).Return(s.instances, nil).MinTimes(1)

	mTag := names.NewMachineTag("0")
	startArg := machineStartInstanceArg("0")
	startArg.Volumes = []storage.VolumeParams{{
		Tag:      names.NewVolumeTag("0"),
		Size:     1024,
		Provider: "static",
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine:  mTag,
				ReadOnly: true,
			},
			Volume: names.NewVolumeTag("0"),
		},
	}, {
		Tag:        names.NewVolumeTag("1"),
		Size:       2048,
		Provider:   "persistent-pool",
		Attributes: map[string]interface{}{"persistent": true},
		Attachment: &storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Machine: mTag,
			},
			Volume: names.NewVolumeTag("1"),
		},
	}}
	startArg.VolumeAttachments = []storage.VolumeAttachmentParams{{
		AttachmentParams: storage.AttachmentParams{
			Machine:  mTag,
			Provider: "static",
			ReadOnly: true,
		},
		VolumeId: "666",
		Volume:   names.NewVolumeTag("1"),
	}}
	exp.StartInstance(gomock.Any(), newDefaultStartInstanceParamsMatcher(c, startArg)).Return(&environs.StartInstanceResult{
		Instance: &testInstance{id: "instance-0"},
	}, nil)

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, config.HarvestAll)
	defer workertest.CleanKill(c, task)

	s.sendModelMachinesChange(c, "0")
	s.waitForProvisioned(c, m0)
}

func (s *ProvisionerTaskSuite) TestProvisioningDoesNotProvisionTheSameMachineAfterRestart(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	// Start with an already provisioned machine.
	inst0 := &testInstance{id: "0"}
	s.instances = []instances.Instance{inst0}
	m0 := &testMachine{c: c, id: "0", life: life.Alive, instance: inst0}

	broker := environmocks.NewMockEnviron(ctrl)
	exp := broker.EXPECT()
	exp.AllRunningInstances(gomock.Any()).Return(s.instances, nil).MinTimes(1)

	done := make(chan bool)
	s.machinesAPI.EXPECT().Machines(gomock.Any(), names.NewMachineTag("0")).
		DoAndReturn(func(context.Context, ...names.MachineTag) ([]apiprovisioner.MachineResult, error) {
			go func() { done <- true }()
			return []apiprovisioner.MachineResult{{
				Machine: m0,
			}}, nil
		})

	// Ensure event is ready as provisioner starts up.
	go func() {
		s.sendModelMachinesChange(c, "0")
	}()

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, config.HarvestAll)
	defer workertest.CleanKill(c, task)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout waiting for provisioner")
	}
}

func (s *ProvisionerTaskSuite) TestDyingMachines(c *tc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	// Start with an already dying provisioned machine.
	inst0 := &testInstance{id: "0"}
	m0 := &testMachine{c: c, id: "0", life: life.Dying, instance: inst0}

	// And a dying unprovisioned one.
	m1 := &testMachine{c: c, id: "1", life: life.Dying}

	s.instances = []instances.Instance{inst0}

	broker := environmocks.NewMockEnviron(ctrl)
	exp := broker.EXPECT()
	exp.AllRunningInstances(gomock.Any()).Return(s.instances, nil).MinTimes(1)
	s.expectMachines(m0, m1)

	task := s.newProvisionerTaskWithBroker(c, broker, nil, numProvisionWorkersForTesting, config.HarvestAll)
	defer workertest.CleanKill(c, task)

	s.sendModelMachinesChange(c, "0", "1")

	// It reaps the unprovisioned one.
	s.waitForRemovalMark(c, m1)

	// verify the other one's still fine
	c.Assert(m0.markForRemoval, tc.IsFalse)
}

// setUpZonedEnviron creates a mock broker with instances based on those set
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
	exp.AllRunningInstances(gomock.Any()).Return(s.instances, nil).MinTimes(1)
	exp.InstanceAvailabilityZoneNames(gomock.Any(), instanceIds).
		DoAndReturn(func(context.Context, []instance.Id) (map[instance.Id]string, error) {
			close(s.setupDone)
			return map[instance.Id]string{}, nil
		})
	exp.AvailabilityZones(gomock.Any()).Return(zones, nil).MinTimes(1)
	return broker
}

func (s *ProvisionerTaskSuite) waitForWorkerSetup(c *tc.C) {
	select {
	case <-s.setupDone:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("worker not set up")
	}
}

func (s *ProvisionerTaskSuite) waitForTask(c *tc.C, expectedCalls []string) {
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

func (s *ProvisionerTaskSuite) sendModelMachinesChange(c *tc.C, ids ...string) {
	select {
	case s.modelMachinesChanges <- ids:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending model machines change")
	}
}

func (s *ProvisionerTaskSuite) sendMachineErrorRetryChange(c *tc.C) {
	select {
	case s.machineErrorRetryChanges <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending machine error retry change")
	}
}

func (s *ProvisionerTaskSuite) newProvisionerTask(
	c *tc.C,
	harvestingMethod config.HarvestMode,
	distributionGroupFinder provisionertask.DistributionGroupFinder,
	toolsFinder provisionertask.ToolsFinder,
	numProvisionWorkers int,
) provisionertask.ProvisionerTask {
	return s.newProvisionerTaskWithRetry(c,
		harvestingMethod,
		distributionGroupFinder,
		toolsFinder,
		provisionertask.RetryStrategy{
			RetryDelay: 0 * time.Second,
			RetryCount: 0,
		},
		numProvisionWorkers,
	)
}

func (s *ProvisionerTaskSuite) newProvisionerTaskWithRetry(
	c *tc.C,
	harvestingMethod config.HarvestMode,
	distributionGroupFinder provisionertask.DistributionGroupFinder,
	toolsFinder provisionertask.ToolsFinder,
	retryStrategy provisionertask.RetryStrategy,
	numProvisionWorkers int,
) provisionertask.ProvisionerTask {
	w, err := provisionertask.NewProvisionerTask(provisionertask.TaskConfig{
		ControllerUUID:               coretesting.ControllerTag.Id(),
		HostTag:                      names.NewMachineTag("0"),
		Logger:                       loggertesting.WrapCheckLog(c),
		HarvestMode:                  harvestingMethod,
		ControllerAPI:                s.controllerAPI,
		MachinesAPI:                  s.machinesAPI,
		DistributionGroupFinder:      distributionGroupFinder,
		ToolsFinder:                  toolsFinder,
		MachineWatcher:               s.modelMachinesWatcher,
		RetryWatcher:                 s.machineErrorRetryWatcher,
		Broker:                       s.instanceBroker,
		ImageStream:                  imagemetadata.ReleasedStream,
		RetryStartInstanceStrategy:   retryStrategy,
		NumProvisionWorkers:          numProvisionWorkers,
		GetMachineInstanceInfoSetter: machineInstanceInfoSetter,
	})
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *ProvisionerTaskSuite) newProvisionerTaskWithBroker(
	c *tc.C,
	broker environs.InstanceBroker,
	distributionGroups map[names.MachineTag][]string,
	numProvisionWorkers int,
	harvestingMethod config.HarvestMode,
) provisionertask.ProvisionerTask {
	return s.newProvisionerTaskWithBrokerAndEventCb(c, broker, distributionGroups, numProvisionWorkers, harvestingMethod, nil)
}

func (s *ProvisionerTaskSuite) newProvisionerTaskWithBrokerAndEventCb(
	c *tc.C,
	broker environs.InstanceBroker,
	distributionGroups map[names.MachineTag][]string,
	numProvisionWorkers int,
	harvestingMethod config.HarvestMode,
	evtCb func(string),
) provisionertask.ProvisionerTask {
	task, err := provisionertask.NewProvisionerTask(provisionertask.TaskConfig{
		ControllerUUID:          coretesting.ControllerTag.Id(),
		HostTag:                 names.NewMachineTag("0"),
		Logger:                  loggertesting.WrapCheckLog(c),
		HarvestMode:             harvestingMethod,
		ControllerAPI:           s.controllerAPI,
		MachinesAPI:             s.machinesAPI,
		DistributionGroupFinder: &mockDistributionGroupFinder{groups: distributionGroups},
		ToolsFinder:             mockToolsFinder{},
		MachineWatcher:          s.modelMachinesWatcher,
		RetryWatcher:            s.machineErrorRetryWatcher,
		Broker:                  broker,
		ImageStream:             imagemetadata.ReleasedStream,
		RetryStartInstanceStrategy: provisionertask.RetryStrategy{
			RetryDelay: 0 * time.Second,
			RetryCount: 0,
		},
		NumProvisionWorkers:          numProvisionWorkers,
		EventProcessedCb:             evtCb,
		GetMachineInstanceInfoSetter: machineInstanceInfoSetter,
	})
	c.Assert(err, tc.ErrorIsNil)
	return task
}

func (s *ProvisionerTaskSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerAPI = NewMockControllerAPI(ctrl)
	s.machinesAPI = NewMockMachinesAPI(ctrl)
	s.expectAuth()
	return ctrl
}

func (s *ProvisionerTaskSuite) expectAuth() {
	s.controllerAPI.EXPECT().APIAddresses(gomock.Any()).Return([]string{"10.0.0.1"}, nil).AnyTimes()
	s.controllerAPI.EXPECT().ModelUUID(gomock.Any()).Return(coretesting.ModelTag.Id(), nil).AnyTimes()
	s.controllerAPI.EXPECT().CACert(gomock.Any()).Return(coretesting.CACert, nil).AnyTimes()
}

func (s *ProvisionerTaskSuite) expectMachines(machines ...*testMachine) {
	tags := transform.Slice(machines, func(m *testMachine) names.MachineTag {
		return names.NewMachineTag(m.id)
	})

	mResults := transform.Slice(machines, func(m *testMachine) apiprovisioner.MachineResult {
		return apiprovisioner.MachineResult{Machine: m}
	})

	s.machinesAPI.EXPECT().Machines(gomock.Any(), tags).Return(mResults, nil).MinTimes(1)
}

func (s *ProvisionerTaskSuite) expectProvisioningInfo(machines ...*testMachine) {
	tags := transform.Slice(machines, func(m *testMachine) names.MachineTag {
		return names.NewMachineTag(m.id)
	})

	base := jujuversion.DefaultSupportedLTSBase()

	piResults := transform.Slice(machines, func(m *testMachine) params.ProvisioningInfoResult {
		return params.ProvisioningInfoResult{
			Result: &params.ProvisioningInfo{
				ControllerConfig:            coretesting.FakeControllerConfig(),
				Base:                        params.Base{Name: base.OS, Channel: base.Channel.String()},
				Constraints:                 constraints.MustParse(m.constraints),
				ProvisioningNetworkTopology: m.topology,
			},
			Error: nil,
		}
	})

	s.machinesAPI.EXPECT().ProvisioningInfo(gomock.Any(), tags).Return(
		params.ProvisioningInfoResults{Results: piResults}, nil).AnyTimes()
}

type MachineClassifySuite struct {
}

var _ = tc.Suite(&MachineClassifySuite{})

type machineClassificationTest struct {
	description    string
	life           life.Value
	status         status.Status
	idErr          string
	ensureDeadErr  string
	expectErrCode  string
	expectErrFmt   string
	statusErr      string
	classification provisionertask.MachineClassification
}

var machineClassificationTestsNoMaintenance = machineClassificationTest{
	description:    "Machine doesn't need maintaining",
	life:           life.Alive,
	status:         status.Started,
	classification: provisionertask.None,
}

func (s *MachineClassifySuite) TestMachineClassification(c *tc.C) {
	test := func(t machineClassificationTest, id string) {
		// Run a sub-test from the test table
		s2e := func(s string) error {
			// Little helper to turn a non-empty string into a useful error for "ErrorMatches"
			if s != "" {
				return &params.Error{Code: s}
			}
			return nil
		}

		c.Logf("%s: %s", id, t.description)
		machine := testMachine{
			c:             c,
			life:          t.life,
			instStatus:    t.status,
			machineStatus: t.status,
			id:            id,
			idErr:         s2e(t.idErr),
			ensureDeadErr: s2e(t.ensureDeadErr),
			statusErr:     s2e(t.statusErr),
		}
		classification, err := provisionertask.ClassifyMachine(c.Context(), loggertesting.WrapCheckLog(c), &machine)
		if err != nil {
			c.Assert(err, tc.ErrorMatches, fmt.Sprintf(t.expectErrFmt, machine.Id()))
		} else {
			c.Assert(err, tc.Equals, s2e(t.expectErrCode))
		}
		c.Assert(classification, tc.Equals, t.classification)
	}

	test(machineClassificationTestsNoMaintenance, "0")
}

type testInstanceBroker struct {
	*testhelpers.Stub

	callsChan        chan string
	allInstancesFunc func(ctx context.Context) ([]instances.Instance, error)
}

func (t *testInstanceBroker) StartInstance(_ context.Context, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	t.AddCall("StartInstance", args)
	t.callsChan <- "StartInstance"
	return nil, t.NextErr()
}

func (t *testInstanceBroker) StopInstances(_ context.Context, ids ...instance.Id) error {
	t.AddCall("StopInstances", ids)
	t.callsChan <- "StopInstances"
	return t.NextErr()
}

func (t *testInstanceBroker) AllInstances(ctx context.Context) ([]instances.Instance, error) {
	t.AddCall("AllInstances")
	t.callsChan <- "AllInstances"
	return t.allInstancesFunc(ctx)
}

func (t *testInstanceBroker) AllRunningInstances(ctx context.Context) ([]instances.Instance, error) {
	t.AddCall("AllRunningInstances")
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

	c *tc.C

	mu sync.Mutex

	id             string
	life           life.Value
	agentVersion   semversion.Number
	instance       *testInstance
	keepInstance   bool
	markForRemoval bool
	constraints    string
	machineStatus  status.Status
	instStatus     status.Status
	instStatusMsg  string
	modStatusMsg   string
	password       string
	topology       params.ProvisioningNetworkTopology

	containersCh chan []string

	idErr         error
	ensureDeadErr error
	statusErr     error
}

func (m *testMachine) Id() string {
	return m.id
}

func (m *testMachine) String() string {
	return m.Id()
}

func (m *testMachine) Life() life.Value {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.life
}

func (m *testMachine) SetLife(life life.Value) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.life = life
}

func (m *testMachine) WatchContainers(_ context.Context, cType instance.ContainerType) (watcher.StringsWatcher, error) {
	if m.containersCh == nil {
		return nil, errors.Errorf("unexpected call to watch %q containers on %q", cType, m.id)
	}
	w := watchertest.NewMockStringsWatcher(m.containersCh)
	return w, nil
}

func (m *testMachine) InstanceId(context.Context) (instance.Id, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.instance == nil {
		return "", params.Error{Code: "not provisioned"}
	}
	return m.instance.Id(), nil
}

func (m *testMachine) InstanceNames() (instance.Id, string, error) {
	instId, err := m.InstanceId(m.c.Context())
	return instId, "", err
}

func (m *testMachine) KeepInstance(context.Context) (bool, error) {
	return m.keepInstance, nil
}

func (m *testMachine) MarkForRemoval(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markForRemoval = true
	return nil
}

func (m *testMachine) GetMarkForRemoval() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.markForRemoval
}

func (m *testMachine) Tag() names.Tag {
	return m.MachineTag()
}

func (m *testMachine) MachineTag() names.MachineTag {
	return names.NewMachineTag(m.id)
}

func (m *testMachine) SetInstanceStatus(ctx context.Context, status status.Status, message string, _ map[string]interface{}) error {
	m.mu.Lock()
	m.instStatus = status
	m.instStatusMsg = message
	m.mu.Unlock()
	return nil
}

func (m *testMachine) InstanceStatus(context.Context) (status.Status, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.instStatus == "" {
		return "pending", "", nil
	}
	return m.instStatus, m.instStatusMsg, nil
}

func (m *testMachine) SetModificationStatus(_ context.Context, _ status.Status, message string, _ map[string]interface{}) error {
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

func (m *testMachine) SetStatus(_ context.Context, status status.Status, _ string, _ map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.machineStatus = status
	return nil
}

func (m *testMachine) Status(context.Context) (status.Status, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.machineStatus == "" {
		return "pending", "", nil
	}
	return m.machineStatus, "", nil
}

func (m *testMachine) ModelAgentVersion(context.Context) (*semversion.Number, error) {
	if m.agentVersion == semversion.Zero {
		return &coretesting.FakeVersionNumber, nil
	}
	return &m.agentVersion, nil
}

func (m *testMachine) SetUnprovisioned() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instance = nil
}

func (m *testMachine) SetInstanceInfo(
	_ context.Context,
	instId instance.Id, _ string, _ string, _ *instance.HardwareCharacteristics, _ []params.NetworkConfig, _ []params.Volume,
	_ map[string]params.VolumeAttachmentInfo, _ []string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instance = &testInstance{id: string(instId)}
	return nil
}

func (m *testMachine) SetPassword(_ context.Context, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.password = password
	return nil
}

func (m *testMachine) GetPassword() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.password
}

func (m *testMachine) EnsureDead(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markForRemoval = true
	return nil
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

var (
	startInstanceArgTemplate = environs.StartInstanceParams{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Tools:          tools.List{{Version: semversion.MustParseBinary("2.99.0-ubuntu-amd64")}},
	}
	instanceConfigTemplate = instancecfg.InstanceConfig{
		ControllerTag:    coretesting.ControllerTag,
		ControllerConfig: coretesting.FakeControllerConfig(),
		Jobs:             []model.MachineJob{model.JobHostUnits},
		APIInfo: &api.Info{
			ModelTag: coretesting.ModelTag,
			Addrs:    []string{"10.0.0.1"},
			CACert:   coretesting.CACert,
		},
		Base:               corebase.MustParseBaseFromString("ubuntu@22.04"),
		TransientDataDir:   "/var/run/juju",
		DataDir:            "/var/lib/juju",
		LogDir:             "/var/log/juju",
		MetricsSpoolDir:    "/var/lib/juju/metricspool",
		CloudInitOutputLog: "/var/log/cloud-init-output.log",
		ImageStream:        "released",
	}
)

func machineStartInstanceArg(id string) *environs.StartInstanceParams {
	result := startInstanceArgTemplate
	instCfg := instanceConfigTemplate
	result.InstanceConfig = &instCfg
	tag := names.NewMachineTag(id)
	result.InstanceConfig.APIInfo.Tag = tag
	result.InstanceConfig.MachineId = id
	result.InstanceConfig.MachineAgentServiceName = fmt.Sprintf("jujud-%s", tag)
	return &result
}

func newDefaultStartInstanceParamsMatcher(c *tc.C, want *environs.StartInstanceParams) *startInstanceParamsMatcher {
	match := func(p environs.StartInstanceParams) bool {
		p.Abort = nil
		p.StatusCallback = nil
		p.CleanupCallback = nil
		if p.InstanceConfig != nil {
			cfgCopy := *p.InstanceConfig
			// The api password and machine nonce are generated to random values.
			// Just ensure they are not empty and tweak it so that the compare succeeds.
			if cfgCopy.APIInfo != nil {
				if cfgCopy.APIInfo.Password == "" {
					return false
				}
				cfgCopy.APIInfo.Password = want.InstanceConfig.APIInfo.Password
			}
			if cfgCopy.MachineNonce == "" {
				return false
			}
			cfgCopy.MachineNonce = ""
			p.InstanceConfig = &cfgCopy
		}
		if len(p.EndpointBindings) == 0 {
			p.EndpointBindings = nil
		}
		if len(p.Volumes) == 0 {
			p.Volumes = nil
		}
		if len(p.VolumeAttachments) == 0 {
			p.VolumeAttachments = nil
		}
		if len(p.ImageMetadata) == 0 {
			p.ImageMetadata = nil
		}
		match := reflect.DeepEqual(p, *want)
		if !match {
			c.Logf("got: %s\n", pretty.Sprint(p))
		}
		return match
	}
	m := newStartInstanceParamsMatcher(map[string]func(environs.StartInstanceParams) bool{
		fmt.Sprintf("core start params: %s\n", pretty.Sprint(*want)): match,
	})
	return m
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

type mockDistributionGroupFinder struct {
	groups map[names.MachineTag][]string
}

func (mock *mockDistributionGroupFinder) DistributionGroupByMachineId(
	ctx context.Context,
	tags ...names.MachineTag,
) ([]apiprovisioner.DistributionGroupResult, error) {
	result := make([]apiprovisioner.DistributionGroupResult, len(tags))
	if len(mock.groups) == 0 {
		for i := range tags {
			result[i] = apiprovisioner.DistributionGroupResult{MachineIds: []string{}}
		}
	} else {
		for i, tag := range tags {
			if dg, ok := mock.groups[tag]; ok {
				result[i] = apiprovisioner.DistributionGroupResult{MachineIds: dg}
			} else {
				result[i] = apiprovisioner.DistributionGroupResult{
					MachineIds: []string{}, Err: &params.Error{Code: params.CodeNotFound, Message: "Fail"}}
			}
		}
	}
	return result, nil
}

type mockToolsFinder struct {
}

func (f mockToolsFinder) FindTools(ctx context.Context, number semversion.Number, os string, a string) (tools.List, error) {
	if number.Compare(semversion.MustParse("6.6.6")) == 0 {
		return nil, tools.ErrNoMatches
	}
	v, err := semversion.ParseBinary(fmt.Sprintf("%s-%s-%s", number, os, arch.HostArch()))
	if err != nil {
		return nil, err
	}
	if a == "" {
		return nil, errors.New("missing arch")
	}
	v.Arch = a
	return tools.List{&tools.Tools{Version: v}}, nil
}
