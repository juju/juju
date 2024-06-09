// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	environmocks "github.com/juju/juju/environs/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/worker/provisioner"
	"github.com/juju/juju/internal/worker/provisioner/mocks"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type CommonProvisionerSuite struct {
	jujutesting.IsolationSuite

	controllerAPI *mocks.MockControllerAPI
	machinesAPI   *mocks.MockMachinesAPI
	broker        *environmocks.MockEnviron

	modelConfigCh chan struct{}
	machinesCh    chan []string

	provisionerStarted chan bool
}

func (s *CommonProvisionerSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerAPI = mocks.NewMockControllerAPI(ctrl)
	s.machinesAPI = mocks.NewMockMachinesAPI(ctrl)
	s.broker = environmocks.NewMockEnviron(ctrl)
	s.expectAuth()
	s.expectStartup(c)
	return ctrl
}

func (s *CommonProvisionerSuite) expectStartup(c *gc.C) {
	s.modelConfigCh = make(chan struct{})
	watchCfg := watchertest.NewMockNotifyWatcher(s.modelConfigCh)
	s.controllerAPI.EXPECT().WatchForModelConfigChanges().Return(watchCfg, nil)

	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{config.ProvisionerHarvestModeKey: config.HarvestDestroyed.String()})
	s.controllerAPI.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil).MaxTimes(2)

	s.provisionerStarted = make(chan bool)
	controllerCfg := coretesting.FakeControllerConfig()
	s.controllerAPI.EXPECT().ControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (controller.Config, error) {
		defer close(s.provisionerStarted)
		return controllerCfg, nil
	})
}

func (s *CommonProvisionerSuite) expectAuth() {
	s.controllerAPI.EXPECT().APIAddresses(gomock.Any()).Return([]string{"10.0.0.1"}, nil).AnyTimes()
	s.controllerAPI.EXPECT().ModelUUID().Return(coretesting.ModelTag.Id(), nil).AnyTimes()
	s.controllerAPI.EXPECT().CACert().Return(coretesting.CACert, nil).AnyTimes()
}

func (s *CommonProvisionerSuite) sendModelConfigChange(c *gc.C) {
	select {
	case s.modelConfigCh <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending model config change")
	}
}

func (s *CommonProvisionerSuite) waitForProvisioner(c *gc.C) {
	select {
	case <-s.provisionerStarted:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for provisioner")
	}
}

func (s *CommonProvisionerSuite) checkStartInstance(c *gc.C, m *testMachine) {
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		_, err := m.InstanceId()
		if err == nil {
			return
		}
	}
	c.Fatalf("machine %v not started", m.id)
}

func (s *CommonProvisionerSuite) assertProvisionerObservesConfigChanges(c *gc.C, p provisioner.Provisioner, container bool) {
	// Inject our observer into the provisioner
	cfgObserver := make(chan *config.Config)
	provisioner.SetObserver(p, cfgObserver)

	attrs := coretesting.FakeConfig()
	attrs[config.ProvisionerHarvestModeKey] = config.HarvestDestroyed.String()
	modelCfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	s.controllerAPI.EXPECT().ModelConfig(gomock.Any()).Return(modelCfg, nil)

	if !container {
		s.broker.EXPECT().SetConfig(gomock.Any(), modelCfg).Return(nil)
	}

	s.sendModelConfigChange(c)

	// Wait for the PA to load the new configuration. We wait for the change we expect
	// like this because sometimes we pick up the initial harvest config (destroyed)
	// rather than the one we change to (all).
	var received []int
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case newCfg := <-cfgObserver:
			if newCfg.ProvisionerHarvestMode() == config.HarvestDestroyed {
				return
			}
			received = append(received, newCfg.NumProvisionWorkers())
		case <-timeout:
			if len(received) == 0 {
				c.Fatalf("PA did not action config change")
			} else {
				c.Fatalf("timed out waiting for config to change to '%v', received %+v",
					config.HarvestDestroyed.String(), received)
			}
		}
	}
}

func (s *CommonProvisionerSuite) assertProvisionerObservesConfigChangesWorkerCount(c *gc.C, p provisioner.Provisioner, container bool) {
	// Inject our observer into the provisioner
	cfgObserver := make(chan *config.Config)
	provisioner.SetObserver(p, cfgObserver)

	attrs := coretesting.FakeConfig().Merge(coretesting.Attrs{
		config.ProvisionerHarvestModeKey: config.HarvestDestroyed.String(),
	})
	if container {
		attrs[config.NumContainerProvisionWorkersKey] = 20
	} else {
		attrs[config.NumProvisionWorkersKey] = 20
	}
	modelCfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	s.controllerAPI.EXPECT().ModelConfig(gomock.Any()).Return(modelCfg, nil)

	if !container {
		s.broker.EXPECT().SetConfig(gomock.Any(), modelCfg).Return(nil)
	}

	s.sendModelConfigChange(c)

	// Wait for the PA to load the new configuration. We wait for the change we expect
	// like this because sometimes we pick up the initial harvest config (destroyed)
	// rather than the one we change to (all).
	var received []int
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case newCfg := <-cfgObserver:
			if container {
				if newCfg.NumContainerProvisionWorkers() == 20 {
					return
				}
				received = append(received, newCfg.NumContainerProvisionWorkers())
			} else {
				if newCfg.NumProvisionWorkers() == 20 {
					return
				}
				received = append(received, newCfg.NumProvisionWorkers())
			}
		case <-timeout:
			if len(received) == 0 {
				c.Fatalf("PA did not action config change")
			} else {
				c.Fatalf("timed out waiting for config to change to '%v', received %+v",
					20, received)
			}
		}
	}
}

// waitForRemovalMark waits for the supplied machine to be marked for removal.
func (s *CommonProvisionerSuite) waitForRemovalMark(c *gc.C, m *testMachine) {
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if m.GetMarkForRemoval() {
			return
		}
	}
	c.Fatalf("machine %q not marked for removal", m.id)
}

func (s *CommonProvisionerSuite) expectMachinesWatcher() {
	s.machinesCh = make(chan []string)
	mw := watchertest.NewMockStringsWatcher(s.machinesCh)
	s.machinesAPI.EXPECT().WatchModelMachines().Return(mw, nil)

	rw := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	s.machinesAPI.EXPECT().WatchMachineErrorRetry().Return(rw, nil)
}

func (s *CommonProvisionerSuite) newEnvironProvisioner(c *gc.C) provisioner.Provisioner {
	c.Assert(s.machinesAPI, gc.NotNil)
	s.expectMachinesWatcher()

	machineTag := names.NewMachineTag("0")
	defaultPaths := agent.DefaultPaths
	defaultPaths.DataDir = c.MkDir()
	agentConfig, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             defaultPaths,
			Tag:               machineTag,
			UpgradedToVersion: jujuversion.Current,
			Password:          "password",
			Nonce:             "nonce",
			APIAddresses:      []string{"0.0.0.0:12345"},
			CACert:            coretesting.CACert,
			Controller:        coretesting.ControllerTag,
			Model:             coretesting.ModelTag,
		})
	c.Assert(err, jc.ErrorIsNil)

	w, err := provisioner.NewEnvironProvisioner(
		s.controllerAPI, s.machinesAPI,
		mockToolsFinder{},
		&mockDistributionGroupFinder{},
		agentConfig,
		loggertesting.WrapCheckLog(c),
		s.broker, &credentialAPIForTest{})
	c.Assert(err, jc.ErrorIsNil)

	s.waitForProvisioner(c)
	return w
}

type ProvisionerSuite struct {
	CommonProvisionerSuite
}

var _ = gc.Suite(&ProvisionerSuite{})

func (s *ProvisionerSuite) sendModelMachinesChange(c *gc.C, ids ...string) {
	select {
	case s.machinesCh <- ids:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending model machines change")
	}
}

func (s *ProvisionerSuite) TestProvisionerStartStop(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	p := s.newEnvironProvisioner(c)
	workertest.CleanKill(c, p)
}

func (s *ProvisionerSuite) TestMachineStartedAndStopped(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	p := s.newEnvironProvisioner(c)
	defer workertest.CleanKill(c, p)

	// Check that an instance is provisioned when the machine is created...

	mTag := names.NewMachineTag("666")
	m666 := &testMachine{id: "666"}

	s.broker.EXPECT().AllRunningInstances(gomock.Any()).Return([]instances.Instance{&testInstance{id: "inst-666"}}, nil).Times(2)
	s.machinesAPI.EXPECT().Machines(gomock.Any(), mTag).Return([]apiprovisioner.MachineResult{{
		Machine: m666,
	}}, nil).Times(2)
	s.machinesAPI.EXPECT().ProvisioningInfo([]names.MachineTag{mTag}).Return(params.ProvisioningInfoResults{
		Results: []params.ProvisioningInfoResult{{
			Result: &params.ProvisioningInfo{
				ControllerConfig: coretesting.FakeControllerConfig(),
				Base:             params.Base{Name: "ubuntu", Channel: "22.04"},
				Jobs:             []model.MachineJob{model.JobHostUnits},
			},
		}},
	}, nil)
	startArg := machineStartInstanceArg(mTag.Id())
	s.broker.EXPECT().StartInstance(gomock.Any(), newDefaultStartInstanceParamsMatcher(c, startArg)).Return(&environs.StartInstanceResult{
		Instance: &testInstance{id: "inst-666"},
	}, nil)

	s.sendModelMachinesChange(c, mTag.Id())
	s.checkStartInstance(c, m666)

	// ...and removed, along with the machine, when the machine is Dead.
	s.broker.EXPECT().StopInstances(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx envcontext.ProviderCallContext, ids ...instance.Id) error {
		c.Assert(len(ids), gc.Equals, 1)
		c.Assert(ids[0], gc.DeepEquals, instance.Id("inst-666"))
		return nil
	})

	m666.SetLife(life.Dead)
	s.sendModelMachinesChange(c, mTag.Id())
	s.waitForRemovalMark(c, m666)
}

func (s *ProvisionerSuite) TestEnvironProvisionerObservesConfigChanges(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	p := s.newEnvironProvisioner(c)
	defer workertest.CleanKill(c, p)
	s.assertProvisionerObservesConfigChanges(c, p, false)
}

func (s *ProvisionerSuite) TestEnvironProvisionerObservesConfigChangesWorkerCount(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	p := s.newEnvironProvisioner(c)
	defer workertest.CleanKill(c, p)
	s.assertProvisionerObservesConfigChangesWorkerCount(c, p, false)
}

type MachineClassifySuite struct {
}

var _ = gc.Suite(&MachineClassifySuite{})

type machineClassificationTest struct {
	description    string
	life           life.Value
	status         status.Status
	idErr          string
	ensureDeadErr  string
	expectErrCode  string
	expectErrFmt   string
	statusErr      string
	classification provisioner.MachineClassification
}

var machineClassificationTestsNoMaintenance = machineClassificationTest{
	description:    "Machine doesn't need maintaining",
	life:           life.Alive,
	status:         status.Started,
	classification: provisioner.None,
}

func (s *MachineClassifySuite) TestMachineClassification(c *gc.C) {
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
			life:          t.life,
			instStatus:    t.status,
			machineStatus: t.status,
			id:            id,
			idErr:         s2e(t.idErr),
			ensureDeadErr: s2e(t.ensureDeadErr),
			statusErr:     s2e(t.statusErr),
		}
		classification, err := provisioner.ClassifyMachine(loggertesting.WrapCheckLog(c), &machine)
		if err != nil {
			c.Assert(err, gc.ErrorMatches, fmt.Sprintf(t.expectErrFmt, machine.Id()))
		} else {
			c.Assert(err, gc.Equals, s2e(t.expectErrCode))
		}
		c.Assert(classification, gc.Equals, t.classification)
	}

	test(machineClassificationTestsNoMaintenance, "0")
}

type mockDistributionGroupFinder struct {
	groups map[names.MachineTag][]string
}

func (mock *mockDistributionGroupFinder) DistributionGroupByMachineId(
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

func (f mockToolsFinder) FindTools(number version.Number, os string, a string) (coretools.List, error) {
	if number.Compare(version.MustParse("6.6.6")) == 0 {
		return nil, coretools.ErrNoMatches
	}
	v, err := version.ParseBinary(fmt.Sprintf("%s-%s-%s", number, os, arch.HostArch()))
	if err != nil {
		return nil, err
	}
	if a == "" {
		return nil, errors.New("missing arch")
	}
	v.Arch = a
	return coretools.List{&coretools.Tools{Version: v}}, nil
}
