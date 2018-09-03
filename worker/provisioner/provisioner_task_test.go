// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/workertest"

	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/instance"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/provisioner"
)

type ProvisionerTaskSuite struct {
	testing.IsolationSuite

	modelMachinesChanges chan []string
	modelMachinesWatcher watcher.StringsWatcher

	machineErrorRetryChanges chan struct{}
	machineErrorRetryWatcher watcher.NotifyWatcher

	machinesResults      []apiprovisioner.MachineResult
	machineStatusResults []apiprovisioner.MachineStatusResult
	machineGetter        *testMachineGetter

	instances       []instance.Instance
	instanceBrocker *testInstanceBrocker

	callCtx           *context.CloudCallContext
	invalidCredential bool
}

var _ = gc.Suite(&ProvisionerTaskSuite{})

func (s *ProvisionerTaskSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

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

	s.instances = []instance.Instance{}
	s.instanceBrocker = &testInstanceBrocker{
		Stub: &testing.Stub{},
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
		apiprovisioner.MachineResult{
			Machine: m0,
		},
		apiprovisioner.MachineResult{
			Machine: m1,
		},
	}

	s.sendModelMachinesChange(c, "0", "1")

	s.machineGetter.CheckCallNames(c, "Machines")

	// Only one instance is being stopped on provider but both are marked for removal in Juju api.
	s.instanceBrocker.CheckCalls(c, []testing.StubCall{
		{"AllInstances", []interface{}{s.callCtx}},
		{"StopInstances", []interface{}{s.callCtx, []instance.Id{"zero"}}}})
	c.Assert(m0.markForRemoval, jc.IsTrue)
	c.Assert(m1.markForRemoval, jc.IsTrue)
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

	retryStrategy := provisioner.NewRetryStrategy(0*time.Second, 0)

	w, err := provisioner.NewProvisionerTask(
		"controller-UUID",
		names.NewMachineTag("0"),
		harvestingMethod,
		s.machineGetter,
		distributionGroupFinder,
		toolsFinder,
		s.modelMachinesWatcher,
		s.machineErrorRetryWatcher,
		s.instanceBrocker,
		// auth,
		nil,
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

	allInstancesFunc func(ctx context.ProviderCallContext) ([]instance.Instance, error)
}

func (t *testInstanceBrocker) StartInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	t.AddCall("StartInstance", ctx, args)
	return nil, nil
}

func (t *testInstanceBrocker) StopInstances(ctx context.ProviderCallContext, ids ...instance.Id) error {
	t.AddCall("StopInstances", ctx, ids)
	return nil
}

func (t *testInstanceBrocker) AllInstances(ctx context.ProviderCallContext) ([]instance.Instance, error) {
	t.AddCall("AllInstances", ctx)
	return t.allInstancesFunc(ctx)
}

func (t *testInstanceBrocker) MaintainInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) error {
	t.AddCall("MaintainInstance", ctx, args)
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
