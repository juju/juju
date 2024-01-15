// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/worker/provisioner"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type kvmProvisionerSuite struct {
	CommonProvisionerSuite

	containersCh chan []string
}

var _ = gc.Suite(&kvmProvisionerSuite{})

func (s *kvmProvisionerSuite) newKvmProvisioner(c *gc.C, ctrl *gomock.Controller) provisioner.Provisioner {
	mTag := names.NewMachineTag("0")
	defaultPaths := agent.DefaultPaths
	defaultPaths.DataDir = c.MkDir()
	cfg, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             defaultPaths,
			Tag:               mTag,
			UpgradedToVersion: jujuversion.Current,
			Password:          "password",
			Nonce:             "nonce",
			APIAddresses:      []string{"0.0.0.0:12345"},
			CACert:            coretesting.CACert,
			Controller:        coretesting.ControllerTag,
			Model:             coretesting.ModelTag,
		})
	c.Assert(err, jc.ErrorIsNil)

	s.containersCh = make(chan []string)
	m0 := &testMachine{containersCh: s.containersCh}
	s.machinesAPI.EXPECT().Machines(mTag).Return([]apiprovisioner.MachineResult{{
		Machine: m0,
	}}, nil)

	toolsFinder := &mockToolsFinder{}
	w, err := provisioner.NewContainerProvisioner(
		instance.KVM, s.controllerAPI, s.machinesAPI, loggo.GetLogger("test"),
		cfg, s.broker,
		toolsFinder, &mockDistributionGroupFinder{}, &credentialAPIForTest{})
	c.Assert(err, jc.ErrorIsNil)

	s.waitForProvisioner(c)
	return w
}

func (s *kvmProvisionerSuite) TestProvisionerStartStop(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	p := s.newKvmProvisioner(c, ctrl)
	workertest.CleanKill(c, p)
}

func (s *kvmProvisionerSuite) TestDoesNotHaveRetryWatcher(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	p := s.newKvmProvisioner(c, ctrl)
	defer workertest.CleanKill(c, p)

	w, err := provisioner.GetRetryWatcher(p)
	c.Assert(w, gc.IsNil)
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *kvmProvisionerSuite) sendMachineContainersChange(c *gc.C, ids ...string) {
	select {
	case s.containersCh <- ids:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending containers change")
	}
}

func (s *kvmProvisionerSuite) TestContainerStartedAndStopped(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	p := s.newKvmProvisioner(c, ctrl)
	defer workertest.CleanKill(c, p)

	cTag := names.NewMachineTag("0/kvm/666")

	c666 := &testMachine{id: "0/kvm/666"}
	s.broker.EXPECT().AllRunningInstances(gomock.Any()).Return([]instances.Instance{&testInstance{id: "inst-666"}}, nil).Times(2)
	s.machinesAPI.EXPECT().Machines(cTag).Return([]apiprovisioner.MachineResult{{
		Machine: c666,
	}}, nil).Times(2)
	s.machinesAPI.EXPECT().ProvisioningInfo([]names.MachineTag{cTag}).Return(params.ProvisioningInfoResults{
		Results: []params.ProvisioningInfoResult{{
			Result: &params.ProvisioningInfo{
				ControllerConfig: coretesting.FakeControllerConfig(),
				Constraints:      constraints.MustParse("mem=666G"),
				Base:             params.Base{Name: "ubuntu", Channel: "22.04"},
				Jobs:             []model.MachineJob{model.JobHostUnits},
			},
		}},
	}, nil)
	startArg := machineStartInstanceArg(c666.id)
	startArg.Constraints = constraints.MustParse("mem=666G")
	s.broker.EXPECT().StartInstance(gomock.Any(), newDefaultStartInstanceParamsMatcher(c, startArg)).Return(&environs.StartInstanceResult{
		Instance: &testInstance{id: "inst-666"},
	}, nil)

	s.sendMachineContainersChange(c, c666.Id())
	s.checkStartInstance(c, c666)

	s.broker.EXPECT().StopInstances(gomock.Any(), gomock.Any()).Do(func(ctx interface{}, ids ...interface{}) {
		c.Assert(len(ids), gc.Equals, 1)
		c.Assert(ids[0], gc.DeepEquals, instance.Id("inst-666"))
	})

	c666.SetLife(life.Dead)
	s.sendMachineContainersChange(c, c666.Id())
	s.waitForRemovalMark(c, c666)
}

func (s *kvmProvisionerSuite) TestKVMProvisionerObservesConfigChanges(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	p := s.newKvmProvisioner(c, ctrl)
	defer workertest.CleanKill(c, p)

	s.assertProvisionerObservesConfigChanges(c, p, true)
}

func (s *kvmProvisionerSuite) TestKVMProvisionerObservesConfigChangesWorkerCount(c *gc.C) {
	ctrl := s.setUpMocks(c)
	defer ctrl.Finish()

	p := s.newKvmProvisioner(c, ctrl)
	defer workertest.CleanKill(c, p)

	s.assertProvisionerObservesConfigChangesWorkerCount(c, p, true)
}

type credentialAPIForTest struct{}

func (*credentialAPIForTest) InvalidateModelCredential(reason string) error {
	return nil
}
