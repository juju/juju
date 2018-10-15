// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"reflect"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/api"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller/authentication"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/instance"
	jujuversion "github.com/juju/juju/juju/version"
	"github.com/juju/juju/mongo"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/provisioner"
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

	instances       []instance.Instance
	instanceBrocker *testInstanceBrocker

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

	s.instances = []instance.Instance{}
	s.instanceBrocker = &testInstanceBrocker{
		Stub:      &testing.Stub{},
		callsChan: make(chan string, 2),
		allInstancesFunc: func(ctx context.ProviderCallContext) ([]instance.Instance, error) {
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
	s.instanceBrocker.CheckNoCalls(c)
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
	s.instances = []instance.Instance{
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
	close(s.instanceBrocker.callsChan)
	s.machineGetter.CheckCallNames(c, "Machines")
	s.instanceBrocker.CheckCalls(c, []testing.StubCall{
		{"AllInstances", []interface{}{s.callCtx}},
		{"StopInstances", []interface{}{s.callCtx, []instance.Id{"zero"}}},
	})
	c.Assert(m0.markForRemoval, jc.IsTrue)
	c.Assert(m1.markForRemoval, jc.IsTrue)
}

func (s *ProvisionerTaskSuite) waitForTask(c *gc.C, expectedCalls []string) {
	calls := []string{}
	for {
		select {
		case call := <-s.instanceBrocker.callsChan:
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

func (s *ProvisionerTaskSuite) TestProvisionerRetries(c *gc.C) {
	s.instanceBrocker.SetErrors(
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
	close(s.instanceBrocker.callsChan)
	s.machineGetter.CheckCallNames(c, "MachinesWithTransientErrors")
	s.auth.CheckCallNames(c, "SetupAuthentication")
	s.instanceBrocker.CheckCallNames(c, "StartInstance", "StartInstance")
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
		s.instanceBrocker,
		s.auth,
		imagemetadata.ReleasedStream,
		retryStrategy,
		s.callCtx,
	)
	c.Assert(err, jc.ErrorIsNil)
	return w
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

type testInstanceBrocker struct {
	*testing.Stub

	callsChan chan string

	allInstancesFunc func(ctx context.ProviderCallContext) ([]instance.Instance, error)
}

func (t *testInstanceBrocker) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	t.AddCall("StartInstance", ctx, args)
	t.callsChan <- "StartInstance"
	return nil, t.NextErr()
}

func (t *testInstanceBrocker) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	t.AddCall("StopInstances", ctx, ids)
	t.callsChan <- "StopInstances"
	return t.NextErr()
}

func (t *testInstanceBrocker) AllInstances(ctx context.ProviderCallContext) ([]instance.Instance, error) {
	t.AddCall("AllInstances", ctx)
	t.callsChan <- "AllInstances"
	return t.allInstancesFunc(ctx)
}

func (t *testInstanceBrocker) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	t.AddCall("MaintainInstance", ctx, args)
	t.callsChan <- "MaintainInstance"
	return nil
}

type testInstance struct {
	instance.Instance
	id string
}

func (i *testInstance) Id() instance.Id {
	return instance.Id(i.id)
}

type testMachine struct {
	*apiprovisioner.Machine
	id   string
	life params.Life

	instance     *testInstance
	keepInstance bool

	markForRemoval bool
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

func (m *testMachine) SetInstanceStatus(status status.Status, message string, data map[string]interface{}) error {
	return nil
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
	}, nil
}

type testAuthenticationProvider struct {
	*testing.Stub
}

func (m *testAuthenticationProvider) SetupAuthentication(machine authentication.TaggedPasswordChanger) (*mongo.MongoInfo, *api.Info, error) {
	m.AddCall("SetupAuthentication", machine)
	return nil, nil, nil
}
