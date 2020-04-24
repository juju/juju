// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"runtime"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/kvm/mock"
	kvmtesting "github.com/juju/juju/container/kvm/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/provisioner"
)

type kvmProvisionerSuite struct {
	CommonProvisionerSuite
	kvmtesting.TestSuite

	events chan mock.Event
}

var _ = gc.Suite(&kvmProvisionerSuite{})

func (s *kvmProvisionerSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping kvm tests on windows")
	}
	s.CommonProvisionerSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
}

func (s *kvmProvisionerSuite) TearDownSuite(c *gc.C) {
	s.TestSuite.TearDownSuite(c)
	s.CommonProvisionerSuite.TearDownSuite(c)
}

func (s *kvmProvisionerSuite) SetUpTest(c *gc.C) {
	s.CommonProvisionerSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)

	s.events = make(chan mock.Event, 25)
	s.ContainerFactory.AddListener(s.events)
}

func (s *kvmProvisionerSuite) nextEvent(c *gc.C) mock.Event {
	select {
	case event := <-s.events:
		return event
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no event arrived")
	}
	panic("not reachable")
}

func (s *kvmProvisionerSuite) expectStarted(c *gc.C, machine *state.Machine) string {
	s.State.StartSync()
	event := s.nextEvent(c)
	c.Assert(event.Action, gc.Equals, mock.Started)
	err := machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	s.waitInstanceId(c, machine, instance.Id(event.InstanceId))
	return event.InstanceId
}

func (s *kvmProvisionerSuite) expectStopped(c *gc.C, instId string) {
	s.State.StartSync()
	event := s.nextEvent(c)
	c.Assert(event.Action, gc.Equals, mock.Stopped)
	c.Assert(event.InstanceId, gc.Equals, instId)
}

func (s *kvmProvisionerSuite) expectNoEvents(c *gc.C) {
	select {
	case event := <-s.events:
		c.Fatalf("unexpected event %#v", event)
	case <-time.After(coretesting.ShortWait):
		return
	}
}

func (s *kvmProvisionerSuite) TearDownTest(c *gc.C) {
	close(s.events)
	s.TestSuite.TearDownTest(c)
	s.CommonProvisionerSuite.TearDownTest(c)
}

func (s *kvmProvisionerSuite) newKvmProvisioner(c *gc.C) provisioner.Provisioner {
	broker := &mockBroker{Environ: s.Environ, retryCount: make(map[string]int),
		startInstanceFailureInfo: map[string]mockBrokerFailures{
			"3": {whenSucceed: 2, err: fmt.Errorf("error: some error")},
			"4": {whenSucceed: 2, err: fmt.Errorf("error: some error")},
		},
	}
	machineTag := names.NewMachineTag("0")
	agentConfig := s.AgentConfigForTag(c, machineTag)
	toolsFinder := (*provisioner.GetToolsFinder)(s.provisioner)
	w, err := provisioner.NewContainerProvisioner(
		instance.KVM, s.provisioner, loggo.GetLogger("test"),
		agentConfig, broker,
		toolsFinder, &mockDistributionGroupFinder{}, &credentialAPIForTest{})
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *kvmProvisionerSuite) TestProvisionerStartStop(c *gc.C) {
	p := s.newKvmProvisioner(c)
	workertest.CleanKill(c, p)
}

func (s *kvmProvisionerSuite) TestDoesNotStartEnvironMachines(c *gc.C) {
	p := s.newKvmProvisioner(c)
	defer workertest.CleanKill(c, p)

	// Check that an instance is not provisioned when the machine is created.
	_, err := s.State.AddMachine(series.DefaultSupportedLTS(), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.expectNoEvents(c)
}

func (s *kvmProvisionerSuite) TestDoesNotHaveRetryWatcher(c *gc.C) {
	p := s.newKvmProvisioner(c)
	defer workertest.CleanKill(c, p)

	w, err := provisioner.GetRetryWatcher(p)
	c.Assert(w, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *kvmProvisionerSuite) addContainer(c *gc.C) *state.Machine {
	template := state.MachineTemplate{
		Series: series.DefaultSupportedLTS(),
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, "0", instance.KVM)
	c.Assert(err, jc.ErrorIsNil)
	return container
}

func (s *kvmProvisionerSuite) TestContainerStartedAndStopped(c *gc.C) {
	if arch.NormaliseArch(runtime.GOARCH) != arch.AMD64 {
		c.Skip("Test only enabled on amd64, see bug lp:1572145")
	}
	p := s.newKvmProvisioner(c)
	defer workertest.CleanKill(c, p)

	container := s.addContainer(c)

	// TODO(jam): 2016-12-22 recent changes to check for networking changes
	// when starting a container cause this test to start failing, because
	// the Dummy provider does not support Networking configuration.
	_, _, err := s.provisioner.HostChangesForContainer(container.MachineTag())
	c.Assert(err, gc.ErrorMatches, "dummy provider network config not supported.*")
	c.Skip("dummy provider doesn't support network config. https://pad.lv/1651974")
	instId := s.expectStarted(c, container)

	// ...and removed, along with the machine, when the machine is Dead.
	c.Assert(container.EnsureDead(), gc.IsNil)
	s.expectStopped(c, instId)
	s.waitForRemovalMark(c, container)
}

func (s *kvmProvisionerSuite) TestKVMProvisionerObservesConfigChanges(c *gc.C) {
	p := s.newKvmProvisioner(c)
	defer workertest.CleanKill(c, p)
	s.assertProvisionerObservesConfigChanges(c, p)
}
