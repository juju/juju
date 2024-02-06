// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/lumberjack/v2"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/utils/v3/ssh"
	sshtesting "github.com/juju/utils/v3/ssh/testing"
	"github.com/juju/utils/v3/symlink"
	"github.com/juju/worker/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	"github.com/juju/juju/cmd/jujud/agent/mocks"
	"github.com/juju/juju/container/kvm"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/mongo"
	_ "github.com/juju/juju/secrets/provider/all"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/diskmanager"
	"github.com/juju/juju/worker/machiner"
	"github.com/juju/juju/worker/storageprovisioner"
)

type MachineSuite struct {
	commonMachineSuite
}

var _ = gc.Suite(&MachineSuite{})

func (s *MachineSuite) SetUpTest(c *gc.C) {
	s.ControllerConfigAttrs = map[string]interface{}{
		controller.AuditingEnabled: true,
	}
	s.commonMachineSuite.SetUpTest(c)

	// Restart failed workers much faster for the tests.
	s.PatchValue(&engine.EngineErrorDelay, 100*time.Millisecond)

	// Most of these tests normally finish sub-second on a fast machine.
	// If any given test hits a minute, we have almost certainly become
	// wedged, so dump the logs.
	coretesting.DumpTestLogsAfter(time.Minute, c, s)
}

func (s *MachineSuite) TestParseNonsense(c *gc.C) {
	aCfg := agentconf.NewAgentConf(s.DataDir())
	err := ParseAgentCommand(&machineAgentCmd{agentInitializer: aCfg}, nil)
	c.Assert(err, gc.ErrorMatches, "either machine-id or controller-id must be set")
	err = ParseAgentCommand(&machineAgentCmd{agentInitializer: aCfg}, []string{"--machine-id", "-4004"})
	c.Assert(err, gc.ErrorMatches, "--machine-id option must be a non-negative integer")
	err = ParseAgentCommand(&machineAgentCmd{agentInitializer: aCfg}, []string{"--controller-id", "-4004"})
	c.Assert(err, gc.ErrorMatches, "--controller-id option must be a non-negative integer")
}

func (s *MachineSuite) TestParseUnknown(c *gc.C) {
	aCfg := agentconf.NewAgentConf(s.DataDir())
	a := &machineAgentCmd{agentInitializer: aCfg}
	err := ParseAgentCommand(a, []string{"--machine-id", "42", "blistering barnacles"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["blistering barnacles"\]`)
}

func (s *MachineSuite) TestParseSuccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	s.cmdRunner = mocks.NewMockCommandRunner(ctrl)

	create := func() (cmd.Command, agentconf.AgentConf) {
		aCfg := agentconf.NewAgentConf(s.DataDir())
		s.PrimeAgent(c, names.NewMachineTag("42"), initialMachinePassword)
		logger := s.newBufferedLogWriter()
		a := NewMachineAgentCmd(
			nil,
			NewTestMachineAgentFactory(c, aCfg, logger, c.MkDir(), s.cmdRunner),
			aCfg,
			aCfg,
		)
		return a, aCfg
	}
	a := CheckAgentCommand(c, s.DataDir(), create, []string{"--machine-id", "42", "--log-to-stderr", "--data-dir", s.DataDir()})
	c.Assert(a.(*machineAgentCmd).machineId, gc.Equals, "42")
}

func (s *MachineSuite) TestRunInvalidMachineId(c *gc.C) {
	c.Skip("agents don't yet distinguish between temporary and permanent errors")
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	_, agent := s.newAgent(c, m)
	err := agent.Run(nil)
	c.Assert(err, gc.ErrorMatches, "some error")
}

func (s *MachineSuite) TestUseLumberjack(c *gc.C) {
	ctx := cmdtesting.Context(c)
	agentConf := FakeAgentConfig{}
	logger := s.newBufferedLogWriter()

	ctrl := gomock.NewController(c)
	s.cmdRunner = mocks.NewMockCommandRunner(ctrl)

	a := NewMachineAgentCmd(
		ctx,
		NewTestMachineAgentFactory(c, &agentConf, logger, c.MkDir(), s.cmdRunner),
		agentConf,
		agentConf,
	)
	// little hack to set the data that Init expects to already be set
	a.(*machineAgentCmd).machineId = "42"

	err := a.Init(nil)
	c.Assert(err, gc.IsNil)

	l, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, jc.IsTrue)
	c.Check(l.MaxAge, gc.Equals, 0)
	c.Check(l.MaxBackups, gc.Equals, 2)
	c.Check(l.Filename, gc.Equals, filepath.FromSlash("/var/log/juju/machine-42.log"))
	c.Check(l.MaxSize, gc.Equals, 100)
}

func (s *MachineSuite) TestDontUseLumberjack(c *gc.C) {
	ctx := cmdtesting.Context(c)
	agentConf := FakeAgentConfig{}
	logger := s.newBufferedLogWriter()

	ctrl := gomock.NewController(c)
	s.cmdRunner = mocks.NewMockCommandRunner(ctrl)

	a := NewMachineAgentCmd(
		ctx,
		NewTestMachineAgentFactory(c, &agentConf, logger, c.MkDir(), s.cmdRunner),
		agentConf,
		agentConf,
	)
	// little hack to set the data that Init expects to already be set
	a.(*machineAgentCmd).machineId = "42"

	// set the value that normally gets set by the flag parsing
	a.(*machineAgentCmd).logToStdErr = true

	err := a.Init(nil)
	c.Assert(err, gc.IsNil)

	_, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, jc.IsFalse)
}

func (s *MachineSuite) TestRunStop(c *gc.C) {
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	done := make(chan error)
	go func() {
		done <- a.Run(cmdtesting.Context(c))
	}()
	err := a.Stop()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(<-done, jc.ErrorIsNil)
}

func (s *MachineSuite) TestDyingMachine(c *gc.C) {
	c.Skip("https://bugs.launchpad.net/juju/+bug/1881979")
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	done := make(chan error)
	go func() {
		done <- a.Run(cmdtesting.Context(c))
	}()
	defer func() {
		c.Check(a.Stop(), jc.ErrorIsNil)
	}()
	// Wait for configuration to be finished
	<-a.WorkersStarted()
	err := m.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// Tearing down the dependency engine can take a non-trivial amount of
	// time.
	select {
	case err := <-done:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		// This test intermittently fails and we haven't been able to determine
		// why it gets wedged. So we will dump the goroutines before the fatal call.
		buff := bytes.Buffer{}
		err = pprof.Lookup("goroutine").WriteTo(&buff, 1)
		c.Check(err, jc.ErrorIsNil)
		c.Logf("\nagent didn't stop, here's what it was doing\n\n%s", buff)
		c.Fatalf("timed out waiting for agent to terminate")
	}
	err = m.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dead)
}

func (s *MachineSuite) TestCallsUseMultipleCPUs(c *gc.C) {
	// All machine agents call UseMultipleCPUs.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	calledChan := make(chan struct{}, 1)
	s.AgentSuite.PatchValue(&useMultipleCPUs, func() { calledChan <- struct{}{} })
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	defer a.Stop()

	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()

	// Wait for configuration to be finished
	<-a.WorkersStarted()
	s.assertChannelActive(c, calledChan, "UseMultipleCPUs() to be called")
	c.Check(a.Stop(), jc.ErrorIsNil)
}

func (s *MachineSuite) testUpgradeRequest(c *gc.C, agent runner, tag string, currentTools *tools.Tools) {
	newVers := coretesting.CurrentVersion()
	newVers.Patch++
	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, s.DefaultToolsStorage, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), newVers)[0]
	err := s.State.SetModelAgentVersion(newVers.Number, nil, true)
	c.Assert(err, jc.ErrorIsNil)
	err = runWithTimeout(c, agent)
	envtesting.CheckUpgraderReadyError(c, err, &agenterrors.UpgradeReadyError{
		AgentName: tag,
		OldTools:  currentTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir(),
	})
}

func (s *MachineSuite) TestUpgradeRequest(c *gc.C) {
	c.Skip("fix machine upgrade test when not controller")
	m, _, currentTools := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	s.testUpgradeRequest(c, a, m.Tag().String(), currentTools)
	c.Assert(a.initialUpgradeCheckComplete.IsUnlocked(), jc.IsFalse)
}

func (s *MachineSuite) TestNoUpgradeRequired(c *gc.C) {
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	done := make(chan error)
	go func() { done <- a.Run(cmdtesting.Context(c)) }()
	select {
	case <-a.initialUpgradeCheckComplete.Unlocked():
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout waiting for upgrade check")
	}
	defer a.Stop() // in case of failure
	s.waitStopped(c, state.JobHostUnits, a, done)
	c.Assert(a.initialUpgradeCheckComplete.IsUnlocked(), jc.IsTrue)
}

func (s *MachineSuite) waitStopped(c *gc.C, job state.MachineJob, a *MachineAgent, done chan error) {
	c.Assert(a.Stop(), jc.ErrorIsNil)

	select {
	case err := <-done:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for agent to terminate")
	}
}

func (s *MachineSuite) assertAgentSetsToolsVersion(c *gc.C, job state.MachineJob) {
	s.PatchValue(&mongo.IsMaster, func(session *mgo.Session, obj mongo.WithAddresses) (bool, error) {
		addr := obj.Addresses()
		for _, a := range addr {
			if a.Value == "0.1.2.3" {
				return true, nil
			}
		}
		return false, nil
	})
	vers := coretesting.CurrentVersion()
	vers.Minor--
	m, _, _ := s.primeAgentVersion(c, vers, job)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	ctx := cmdtesting.Context(c)
	go func() { c.Check(a.Run(ctx), jc.ErrorIsNil) }()
	defer func() {
		logger.Infof("stopping machine agent")
		c.Check(a.Stop(), jc.ErrorIsNil)
		logger.Infof("stopped machine agent")
	}()

	timeout := time.After(coretesting.LongWait)
	for done := false; !done; {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for agent version to be set")
		case <-time.After(coretesting.ShortWait):
			c.Log("Refreshing")
			err := m.Refresh()
			c.Assert(err, jc.ErrorIsNil)
			c.Log("Fetching agent tools")
			agentTools, err := m.AgentTools()
			c.Assert(err, jc.ErrorIsNil)
			c.Logf("(%v vs. %v)", agentTools.Version, jujuversion.Current)
			if agentTools.Version.Minor != jujuversion.Current.Minor {
				continue
			}
			c.Assert(agentTools.Version.Number, gc.DeepEquals, jujuversion.Current)
			done = true
		}
	}
}

func (s *MachineSuite) TestAgentSetsToolsVersionHostUnits(c *gc.C) {
	s.assertAgentSetsToolsVersion(c, state.JobHostUnits)
}

func (s *MachineSuite) TestMachineAgentRunsAuthorisedKeysWorker(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Update the keys in the environment.
	sshKey := sshtesting.ValidKeyOne.Key + " user@host"
	err := s.Model.UpdateModelConfig(map[string]interface{}{"authorized-keys": sshKey}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for ssh keys file to be updated.
	timeout := time.After(coretesting.LongWait)
	sshKeyWithCommentPrefix := sshtesting.ValidKeyOne.Key + " Juju:user@host"
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for authorised ssh keys to change")
		case <-time.After(coretesting.ShortWait):
			keys, err := ssh.ListKeys(authenticationworker.SSHUser, ssh.FullKeys)
			c.Assert(err, jc.ErrorIsNil)
			keysStr := strings.Join(keys, "\n")
			if sshKeyWithCommentPrefix != keysStr {
				continue
			}
			return
		}
	}
}

func (s *MachineSuite) TestMachineAgentSymlinks(c *gc.C) {
	stm, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, stm)
	defer ctrl.Finish()
	defer a.Stop()
	doneCh := make(chan error)
	go func() {
		doneCh <- a.Run(cmdtesting.Context(c))
	}()

	<-a.WorkersStarted()

	// Symlinks should have been created
	for _, link := range jujudSymlinks {
		_, err := os.Stat(utils.EnsureBaseDir(a.rootDir, link))
		c.Assert(err, jc.ErrorIsNil, gc.Commentf(link))
	}

	s.waitStopped(c, state.JobHostUnits, a, doneCh)
}

func (s *MachineSuite) TestMachineAgentSymlinkJujuExecExists(c *gc.C) {
	// Pre-create the symlinks, but pointing to the incorrect location.
	rootDir := c.MkDir()
	for _, link := range jujudSymlinks {
		fullLink := utils.EnsureBaseDir(rootDir, link)
		c.Assert(os.MkdirAll(filepath.Dir(fullLink), os.FileMode(0755)), jc.ErrorIsNil)
		c.Assert(symlink.New("/nowhere/special", fullLink), jc.ErrorIsNil, gc.Commentf(link))
	}

	stm, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, stm)
	a.rootDir = rootDir
	defer ctrl.Finish()
	defer a.Stop()
	doneCh := make(chan error)
	go func() {
		doneCh <- a.Run(cmdtesting.Context(c))
	}()

	<-a.WorkersStarted()

	// juju-exec symlink should have been recreated.
	for _, link := range jujudSymlinks {
		fullLink := utils.EnsureBaseDir(a.rootDir, link)
		linkTarget, err := symlink.Read(fullLink)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(linkTarget, gc.Not(gc.Equals), "/nowhere/special", gc.Commentf(link))
	}

	s.waitStopped(c, state.JobHostUnits, a, doneCh)
}

func (s *MachineSuite) TestMachineAgentRunsAPIAddressUpdaterWorker(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Update the API addresses.
	updatedServers := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "localhost"),
	}
	err := s.BackingState.SetAPIHostPorts(updatedServers)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for config to be updated.
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if !attempt.HasNext() {
			break
		}
		addrs, err := a.CurrentConfig().APIAddresses()
		c.Assert(err, jc.ErrorIsNil)
		if reflect.DeepEqual(addrs, []string{"localhost:1234"}) {
			return
		}
	}
	c.Fatalf("timeout while waiting for agent config to change")
}

func (s *MachineSuite) TestMachineAgentRunsDiskManagerWorker(c *gc.C) {
	// Patch out the worker func before starting the agent.
	started := newSignal()
	newWorker := func(diskmanager.ListBlockDevicesFunc, diskmanager.BlockDeviceSetter) worker.Worker {
		started.trigger()
		return jworker.NewNoOpWorker()
	}
	s.PatchValue(&diskmanager.NewWorker, newWorker)

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	started.assertTriggered(c, "diskmanager worker to start")
}

func (s *MachineSuite) TestDiskManagerWorkerUpdatesState(c *gc.C) {
	expected := []storage.BlockDevice{{DeviceName: "whatever"}}
	s.PatchValue(&diskmanager.DefaultListBlockDevices, func() ([]storage.BlockDevice, error) {
		return expected, nil
	})

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	sb, err := state.NewStorageBackend(s.BackingState)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for state to be updated.
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		devices, err := sb.BlockDevices(m.MachineTag())
		c.Assert(err, jc.ErrorIsNil)
		if len(devices) > 0 {
			c.Assert(devices, gc.HasLen, 1)
			c.Assert(devices[0].DeviceName, gc.Equals, expected[0].DeviceName)
			return
		}
	}
	c.Fatalf("timeout while waiting for block devices to be recorded")
}

func (s *MachineSuite) TestMachineAgentRunsMachineStorageWorker(c *gc.C) {
	m, _, _ := s.primeAgent(c, state.JobHostUnits)

	started := newSignal()
	newWorker := func(config storageprovisioner.Config) (worker.Worker, error) {
		c.Check(config.Scope, gc.Equals, m.Tag())
		c.Check(config.Validate(), jc.ErrorIsNil)
		started.trigger()
		return jworker.NewNoOpWorker(), nil
	}
	s.PatchValue(&storageprovisioner.NewStorageProvisioner, newWorker)

	// Start the machine agent.
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	started.assertTriggered(c, "storage worker to start")
}

func (s *MachineSuite) setupIgnoreAddresses(c *gc.C, expectedIgnoreValue bool) chan bool {
	ignoreAddressCh := make(chan bool, 1)
	s.AgentSuite.PatchValue(&machiner.NewMachiner, func(cfg machiner.Config) (worker.Worker, error) {
		select {
		case ignoreAddressCh <- cfg.ClearMachineAddressesOnStart:
		default:
		}

		// The test just cares that NewMachiner is called with the correct
		// value, nothing else is done with the worker.
		return newDummyWorker(), nil
	})

	attrs := coretesting.Attrs{"ignore-machine-addresses": expectedIgnoreValue}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)
	return ignoreAddressCh
}

func (s *MachineSuite) TestMachineAgentIgnoreAddresses(c *gc.C) {
	for _, expectedIgnoreValue := range []bool{true, false} {
		ignoreAddressCh := s.setupIgnoreAddresses(c, expectedIgnoreValue)

		m, _, _ := s.primeAgent(c, state.JobHostUnits)
		ctrl, a := s.newAgent(c, m)
		defer ctrl.Finish()
		defer a.Stop()
		doneCh := make(chan error)
		go func() {
			doneCh <- a.Run(cmdtesting.Context(c))
		}()

		select {
		case ignoreMachineAddresses := <-ignoreAddressCh:
			if ignoreMachineAddresses != expectedIgnoreValue {
				c.Fatalf("expected ignore-machine-addresses = %v, got = %v", expectedIgnoreValue, ignoreMachineAddresses)
			}
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for the machiner to start")
		}
		s.waitStopped(c, state.JobHostUnits, a, doneCh)
	}
}

func (s *MachineSuite) TestMachineAgentIgnoreAddressesContainer(c *gc.C) {
	ignoreAddressCh := s.setupIgnoreAddresses(c, true)

	parent, err := s.State.AddMachine(state.UbuntuBase("20.04"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.AddMachineInsideMachine(
		state.MachineTemplate{
			Base: state.UbuntuBase("22.04"),
			Jobs: []state.MachineJob{state.JobHostUnits},
		},
		parent.Id(),
		instance.LXD,
	)
	c.Assert(err, jc.ErrorIsNil)

	vers := coretesting.CurrentVersion()
	s.primeAgentWithMachine(c, m, vers)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	defer a.Stop()
	doneCh := make(chan error)
	go func() {
		doneCh <- a.Run(cmdtesting.Context(c))
	}()

	select {
	case ignoreMachineAddresses := <-ignoreAddressCh:
		if ignoreMachineAddresses {
			c.Fatalf("expected ignore-machine-addresses = false, got = true")
		}
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for the machiner to start")
	}
	s.waitStopped(c, state.JobHostUnits, a, doneCh)
}

func (s *MachineSuite) TestMachineWorkers(c *gc.C) {
	testing.PatchExecutableAsEchoArgs(c, s, "ovs-vsctl", 0)

	tracker := agenttest.NewEngineTracker()
	instrumented := TrackMachines(c, tracker, iaasMachineManifolds)
	s.PatchValue(&iaasMachineManifolds, instrumented)

	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	go func() { c.Check(a.Run(cmdtesting.Context(c)), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for it to stabilise, running as normal.
	matcher := agenttest.NewWorkerMatcher(c, tracker, a.Tag().String(),
		append(alwaysMachineWorkers, notMigratingMachineWorkers...))

	// Indicate that this machine supports KVM containers rather than doing
	// detection that may return true/false based on the machine running tests.
	s.PatchValue(&kvm.IsKVMSupported, func() (bool, error) { return true, nil })

	agenttest.WaitMatch(c, matcher.Check, coretesting.LongWait)
}
