// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/set"
	"github.com/juju/utils/symlink"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/charmrepo"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiaddresser "github.com/juju/juju/api/addresser"
	apideployer "github.com/juju/juju/api/deployer"
	apienvironment "github.com/juju/juju/api/environment"
	apifirewaller "github.com/juju/juju/api/firewaller"
	apiinstancepoller "github.com/juju/juju/api/instancepoller"
	apimetricsmanager "github.com/juju/juju/api/metricsmanager"
	apinetworker "github.com/juju/juju/api/networker"
	apirsyslog "github.com/juju/juju/api/rsyslog"
	charmtesting "github.com/juju/juju/apiserver/charmrevisionupdater/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cert"
	agenttesting "github.com/juju/juju/cmd/jujud/agent/testing"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/cmd/jujud/util/password"
	lxctesting "github.com/juju/juju/container/lxc/testing"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/utils/ssh"
	sshtesting "github.com/juju/juju/utils/ssh/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/addresser"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/authenticationworker"
	"github.com/juju/juju/worker/certupdater"
	"github.com/juju/juju/worker/deployer"
	"github.com/juju/juju/worker/diskmanager"
	"github.com/juju/juju/worker/instancepoller"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/machiner"
	"github.com/juju/juju/worker/networker"
	"github.com/juju/juju/worker/peergrouper"
	"github.com/juju/juju/worker/proxyupdater"
	"github.com/juju/juju/worker/resumer"
	"github.com/juju/juju/worker/rsyslog"
	"github.com/juju/juju/worker/singular"
	"github.com/juju/juju/worker/storageprovisioner"
	"github.com/juju/juju/worker/upgrader"
)

var (
	_ = gc.Suite(&MachineSuite{})
	_ = gc.Suite(&MachineWithCharmsSuite{})
	_ = gc.Suite(&mongoSuite{})
)

func TestPackage(t *testing.T) {
	// TODO(waigani) 2014-03-19 bug 1294458
	// Refactor to use base suites

	// Change the path to "juju-run", so that the
	// tests don't try to write to /usr/local/bin.
	JujuRun = mktemp("juju-run", "")
	defer os.Remove(JujuRun)

	coretesting.MgoTestPackage(t)
}

type commonMachineSuite struct {
	singularRecord *singularRunnerRecord
	lxctesting.TestSuite
	fakeEnsureMongo *agenttesting.FakeEnsureMongo
	AgentSuite
}

func (s *commonMachineSuite) SetUpSuite(c *gc.C) {
	s.AgentSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
	// We're not interested in whether EnsureJujudPassword works here since we test it somewhere else
	s.PatchValue(&password.EnsureJujudPassword, func() error { return nil })
}

func (s *commonMachineSuite) TearDownSuite(c *gc.C) {
	s.TestSuite.TearDownSuite(c)
	s.AgentSuite.TearDownSuite(c)
}

func (s *commonMachineSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
	s.AgentSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
	s.AgentSuite.PatchValue(&charmrepo.CacheDir, c.MkDir())
	s.AgentSuite.PatchValue(&stateWorkerDialOpts, mongo.DefaultDialOpts())

	os.Remove(JujuRun) // ignore error; may not exist
	// Patch ssh user to avoid touching ~ubuntu/.ssh/authorized_keys.
	s.AgentSuite.PatchValue(&authenticationworker.SSHUser, "")

	testpath := c.MkDir()
	s.AgentSuite.PatchEnvPathPrepend(testpath)
	// mock out the start method so we can fake install services without sudo
	fakeCmd(filepath.Join(testpath, "start"))
	fakeCmd(filepath.Join(testpath, "stop"))

	s.AgentSuite.PatchValue(&upstart.InitDir, c.MkDir())

	s.singularRecord = newSingularRunnerRecord()
	s.AgentSuite.PatchValue(&newSingularRunner, s.singularRecord.newSingularRunner)
	s.AgentSuite.PatchValue(&peergrouperNew, func(st *state.State) (worker.Worker, error) {
		return newDummyWorker(), nil
	})

	s.fakeEnsureMongo = agenttesting.InstallFakeEnsureMongo(s)
	s.AgentSuite.PatchValue(&maybeInitiateMongoServer, s.fakeEnsureMongo.InitiateMongo)
}

func fakeCmd(path string) {
	err := ioutil.WriteFile(path, []byte("#!/bin/bash --norc\nexit 0"), 0755)
	if err != nil {
		panic(err)
	}
}

func (s *commonMachineSuite) TearDownTest(c *gc.C) {
	s.TestSuite.TearDownTest(c)
	s.AgentSuite.TearDownTest(c)
}

// primeAgent adds a new Machine to run the given jobs, and sets up the
// machine agent's directory.  It returns the new machine, the
// agent's configuration and the tools currently running.
func (s *commonMachineSuite) primeAgent(
	c *gc.C, vers version.Binary,
	jobs ...state.MachineJob) (m *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools) {

	m, err := s.State.AddMachine("quantal", jobs...)
	c.Assert(err, jc.ErrorIsNil)

	pinger, err := m.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		err := pinger.Stop()
		c.Check(err, jc.ErrorIsNil)
	})

	return s.configureMachine(c, m.Id(), vers)
}

func (s *commonMachineSuite) configureMachine(c *gc.C, machineId string, vers version.Binary) (
	machine *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools,
) {
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	// Add a machine and ensure it is provisioned.
	inst, md := jujutesting.AssertStartInstance(c, s.Environ, machineId)
	c.Assert(m.SetProvisioned(inst.Id(), agent.BootstrapNonce, md), jc.ErrorIsNil)

	// Add an address for the tests in case the maybeInitiateMongoServer
	// codepath is exercised.
	s.setFakeMachineAddresses(c, m)

	// Set up the new machine.
	err = m.SetAgentVersion(vers)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetPassword(initialMachinePassword)
	c.Assert(err, jc.ErrorIsNil)
	tag := m.Tag()
	if m.IsManager() {
		err = m.SetMongoPassword(initialMachinePassword)
		c.Assert(err, jc.ErrorIsNil)
		agentConfig, tools = s.AgentSuite.PrimeStateAgent(c, tag, initialMachinePassword, vers)
		info, ok := agentConfig.StateServingInfo()
		c.Assert(ok, jc.IsTrue)
		ssi := cmdutil.ParamsStateServingInfoToStateStateServingInfo(info)
		err = s.State.SetStateServingInfo(ssi)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		agentConfig, tools = s.PrimeAgent(c, tag, initialMachinePassword, vers)
	}
	err = agentConfig.Write()
	c.Assert(err, jc.ErrorIsNil)
	return m, agentConfig, tools
}

// newAgent returns a new MachineAgent instance
func (s *commonMachineSuite) newAgent(c *gc.C, m *state.Machine) *MachineAgent {
	agentConf := agentConf{dataDir: s.DataDir()}
	agentConf.ReadConfig(names.NewMachineTag(m.Id()).String())
	logsCh, err := logsender.InstallBufferedLogWriter(1024)
	c.Assert(err, jc.ErrorIsNil)
	machineAgentFactory := MachineAgentFactoryFn(
		&agentConf, logsCh, &mockLoopDeviceManager{},
	)
	return machineAgentFactory(m.Id())
}

func (s *MachineSuite) TestParseSuccess(c *gc.C) {
	create := func() (cmd.Command, AgentConf) {
		agentConf := agentConf{dataDir: s.DataDir()}
		a := NewMachineAgentCmd(
			nil,
			MachineAgentFactoryFn(
				&agentConf, nil, &mockLoopDeviceManager{},
			),
			&agentConf,
			&agentConf,
		)
		a.(*machineAgentCmd).logToStdErr = true

		return a, &agentConf
	}
	a := CheckAgentCommand(c, create, []string{"--machine-id", "42"})
	c.Assert(a.(*machineAgentCmd).machineId, gc.Equals, "42")
}

type MachineSuite struct {
	commonMachineSuite
	metricAPI *mockMetricAPI
}

var perEnvSingularWorkers = []string{
	"cleaner",
	"minunitsworker",
	"addresserworker",
	"environ-provisioner",
	"charm-revision-updater",
	"instancepoller",
	"firewaller",
}

const initialMachinePassword = "machine-password-1234567890"

func (s *MachineSuite) SetUpTest(c *gc.C) {
	s.commonMachineSuite.SetUpTest(c)
	s.metricAPI = newMockMetricAPI()
	s.PatchValue(&getMetricAPI, func(_ api.Connection) apimetricsmanager.MetricsManagerClient {
		return s.metricAPI
	})
	s.AddCleanup(func(*gc.C) { s.metricAPI.Stop() })
	// Most of these tests normally finish sub-second on a fast machine.
	// If any given test hits a minute, we have almost certainly become
	// wedged, so dump the logs.
	coretesting.DumpTestLogsAfter(time.Minute, c, s)
}

func (s *MachineSuite) TestParseNonsense(c *gc.C) {
	for _, args := range [][]string{
		{},
		{"--machine-id", "-4004"},
	} {
		var agentConf agentConf
		err := ParseAgentCommand(&machineAgentCmd{agentInitializer: &agentConf}, args)
		c.Assert(err, gc.ErrorMatches, "--machine-id option must be set, and expects a non-negative integer")
	}
}

func (s *MachineSuite) TestParseUnknown(c *gc.C) {
	var agentConf agentConf
	a := &machineAgentCmd{agentInitializer: &agentConf}
	err := ParseAgentCommand(a, []string{"--machine-id", "42", "blistering barnacles"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["blistering barnacles"\]`)
}

func (s *MachineSuite) TestRunInvalidMachineId(c *gc.C) {
	c.Skip("agents don't yet distinguish between temporary and permanent errors")
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	err := s.newAgent(c, m).Run(nil)
	c.Assert(err, gc.ErrorMatches, "some error")
}

func (s *MachineSuite) TestUseLumberjack(c *gc.C) {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, gc.IsNil)

	agentConf := FakeAgentConfig{}

	a := NewMachineAgentCmd(
		ctx,
		MachineAgentFactoryFn(
			agentConf, nil, &mockLoopDeviceManager{},
		),
		agentConf,
		agentConf,
	)
	// little hack to set the data that Init expects to already be set
	a.(*machineAgentCmd).machineId = "42"

	err = a.Init(nil)
	c.Assert(err, gc.IsNil)

	l, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, jc.IsTrue)
	c.Check(l.MaxAge, gc.Equals, 0)
	c.Check(l.MaxBackups, gc.Equals, 2)
	c.Check(l.Filename, gc.Equals, filepath.FromSlash("/var/log/juju/machine-42.log"))
	c.Check(l.MaxSize, gc.Equals, 300)
}

func (s *MachineSuite) TestDontUseLumberjack(c *gc.C) {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, gc.IsNil)

	agentConf := FakeAgentConfig{}

	a := NewMachineAgentCmd(
		ctx,
		MachineAgentFactoryFn(
			agentConf, nil,
			&mockLoopDeviceManager{},
		),
		agentConf,
		agentConf,
	)
	// little hack to set the data that Init expects to already be set
	a.(*machineAgentCmd).machineId = "42"

	// set the value that normally gets set by the flag parsing
	a.(*machineAgentCmd).logToStdErr = true

	err = a.Init(nil)
	c.Assert(err, gc.IsNil)

	_, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, jc.IsFalse)
}

func (s *MachineSuite) TestRunStop(c *gc.C) {
	m, ac, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	err := a.Stop()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(<-done, jc.ErrorIsNil)
	c.Assert(charmrepo.CacheDir, gc.Equals, filepath.Join(ac.DataDir(), "charmcache"))
}

func (s *MachineSuite) TestWithDeadMachine(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	err := m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestWithRemovedMachine(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	err := m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = m.Remove()
	c.Assert(err, jc.ErrorIsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestDyingMachine(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	defer func() {
		c.Check(a.Stop(), jc.ErrorIsNil)
	}()
	// Wait for configuration to be finished
	<-a.WorkersStarted()
	err := m.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	select {
	case err := <-done:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(watcher.Period * 5 / 4):
		// TODO(rog) Fix this so it doesn't wait for so long.
		// https://bugs.launchpad.net/juju-core/+bug/1163983
		c.Fatalf("timed out waiting for agent to terminate")
	}
	err = m.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Life(), gc.Equals, state.Dead)
}

func (s *MachineSuite) TestHostUnits(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	ctx, reset := patchDeployContext(c, s.BackingState)
	defer reset()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// check that unassigned units don't trigger any deployments.
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	u0, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	u1, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	ctx.waitDeployed(c)

	// assign u0, check it's deployed.
	err = u0.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	ctx.waitDeployed(c, u0.Name())

	// "start the agent" for u0 to prevent short-circuited remove-on-destroy;
	// check that it's kept deployed despite being Dying.
	err = u0.SetAgentStatus(state.StatusIdle, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = u0.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	ctx.waitDeployed(c, u0.Name())

	// add u1 to the machine, check it's deployed.
	err = u1.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	ctx.waitDeployed(c, u0.Name(), u1.Name())

	// make u0 dead; check the deployer recalls the unit and removes it from
	// state.
	err = u0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	ctx.waitDeployed(c, u1.Name())

	// The deployer actually removes the unit just after
	// removing its deployment, so we need to poll here
	// until it actually happens.
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		err := u0.Refresh()
		if err == nil && attempt.HasNext() {
			continue
		}
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}

	// short-circuit-remove u1 after it's been deployed; check it's recalled
	// and removed from state.
	err = u1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = u1.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	ctx.waitDeployed(c)
}

func patchDeployContext(c *gc.C, st *state.State) (*fakeContext, func()) {
	ctx := &fakeContext{
		inited:   make(chan struct{}),
		deployed: make(set.Strings),
	}
	orig := newDeployContext
	newDeployContext = func(dst *apideployer.State, agentConfig agent.Config) deployer.Context {
		ctx.st = st
		ctx.agentConfig = agentConfig
		close(ctx.inited)
		return ctx
	}
	return ctx, func() { newDeployContext = orig }
}

func (s *commonMachineSuite) setFakeMachineAddresses(c *gc.C, machine *state.Machine) {
	addrs := network.NewAddresses("0.1.2.3")
	err := machine.SetProviderAddresses(addrs...)
	c.Assert(err, jc.ErrorIsNil)
	// Set the addresses in the environ instance as well so that if the instance poller
	// runs it won't overwrite them.
	instId, err := machine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	insts, err := s.Environ.Instances([]instance.Id{instId})
	c.Assert(err, jc.ErrorIsNil)
	dummy.SetInstanceAddresses(insts[0], addrs)
}

func (s *MachineSuite) TestManageEnviron(c *gc.C) {
	usefulVersion := version.Current
	usefulVersion.Series = "quantal" // to match the charm created below
	envtesting.AssertUploadFakeToolsVersions(
		c, s.DefaultToolsStorage, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), usefulVersion)
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	op := make(chan dummy.Operation, 200)
	dummy.Listen(op)

	a := s.newAgent(c, m)
	// Make sure the agent is stopped even if the test fails.
	defer a.Stop()
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()

	// See state server runners start
	r0 := s.singularRecord.nextRunner(c)
	r0.waitForWorker(c, "txnpruner")

	r1 := s.singularRecord.nextRunner(c)
	r1.waitForWorkers(c, perEnvSingularWorkers)

	// Check that the provisioner and firewaller are alive by doing
	// a rudimentary check that it responds to state changes.

	// Add one unit to a service; it should get allocated a machine
	// and then its ports should be opened.
	charm := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "test-service", charm)
	err := svc.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	units, err := juju.AddUnits(s.State, svc, 1, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(opRecvTimeout(c, s.State, op, dummy.OpStartInstance{}), gc.NotNil)

	// Wait for the instance id to show up in the state.
	s.waitProvisioned(c, units[0])
	err = units[0].OpenPort("tcp", 999)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(opRecvTimeout(c, s.State, op, dummy.OpOpenPorts{}), gc.NotNil)

	// Check that the metrics workers have started by adding metrics
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for metric cleanup API to be called")
	case <-s.metricAPI.CleanupCalled():
	}
	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for metric sender API to be called")
	case <-s.metricAPI.SendCalled():
	}

	err = a.Stop()
	c.Assert(err, jc.ErrorIsNil)

	select {
	case err := <-done:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for agent to terminate")
	}
}

func (s *MachineSuite) TestManageEnvironRunsResumer(c *gc.C) {
	started := make(chan struct{})
	s.AgentSuite.PatchValue(&newResumer, func(st resumer.TransactionResumer) *resumer.Resumer {
		close(started)
		return resumer.NewResumer(st)
	})

	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()

	// Wait for the worker that starts before the resumer to start.
	_ = s.singularRecord.nextRunner(c)
	r := s.singularRecord.nextRunner(c)
	r.waitForWorker(c, "charm-revision-updater")

	// Now make sure the resumer starts.
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("resumer worker not started as expected")
	}
}

func (s *MachineSuite) TestManageEnvironStartsInstancePoller(c *gc.C) {
	started := make(chan struct{})
	s.AgentSuite.PatchValue(&newInstancePoller, func(st *apiinstancepoller.API) worker.Worker {
		close(started)
		return instancepoller.NewWorker(st)
	})

	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()

	// Wait for the worker that starts before the instancepoller to
	// start.
	_ = s.singularRecord.nextRunner(c)
	r := s.singularRecord.nextRunner(c)
	r.waitForWorker(c, "charm-revision-updater")

	// Now make sure the resumer starts.
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("instancepoller worker not started as expected")
	}
}

const startWorkerWait = 250 * time.Millisecond

func (s *MachineSuite) TestManageEnvironDoesNotRunFirewallerWhenModeIsNone(c *gc.C) {
	s.PatchValue(&getFirewallMode, func(api.Connection) (string, error) {
		return config.FwNone, nil
	})
	started := make(chan struct{})
	s.AgentSuite.PatchValue(&newFirewaller, func(st *apifirewaller.State) (worker.Worker, error) {
		close(started)
		return newDummyWorker(), nil
	})

	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()

	// Wait for the worker that starts before the firewaller to start.
	_ = s.singularRecord.nextRunner(c)
	r := s.singularRecord.nextRunner(c)
	r.waitForWorker(c, "charm-revision-updater")

	// Now make sure the firewaller doesn't start.
	select {
	case <-started:
		c.Fatalf("firewaller worker unexpectedly started")
	case <-time.After(startWorkerWait):
	}
}

func (s *MachineSuite) TestManageEnvironRunsInstancePoller(c *gc.C) {
	s.AgentSuite.PatchValue(&instancepoller.ShortPoll, 500*time.Millisecond)
	usefulVersion := version.Current
	usefulVersion.Series = "quantal" // to match the charm created below
	envtesting.AssertUploadFakeToolsVersions(
		c, s.DefaultToolsStorage,
		s.Environ.Config().AgentStream(),
		s.Environ.Config().AgentStream(),
		usefulVersion,
	)
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()

	// Add one unit to a service;
	charm := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "test-service", charm)
	units, err := juju.AddUnits(s.State, svc, 1, "")
	c.Assert(err, jc.ErrorIsNil)

	m, instId := s.waitProvisioned(c, units[0])
	insts, err := s.Environ.Instances([]instance.Id{instId})
	c.Assert(err, jc.ErrorIsNil)
	addrs := network.NewAddresses("1.2.3.4")
	dummy.SetInstanceAddresses(insts[0], addrs)
	dummy.SetInstanceStatus(insts[0], "running")

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if !a.HasNext() {
			c.Logf("final machine addresses: %#v", m.Addresses())
			c.Fatalf("timed out waiting for machine to get address")
		}
		err := m.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		instStatus, err := m.InstanceStatus()
		c.Assert(err, jc.ErrorIsNil)
		if reflect.DeepEqual(m.Addresses(), addrs) && instStatus == "running" {
			break
		}
	}
}

func (s *MachineSuite) TestManageEnvironRunsPeergrouper(c *gc.C) {
	started := make(chan struct{}, 1)
	s.AgentSuite.PatchValue(&peergrouperNew, func(st *state.State) (worker.Worker, error) {
		c.Check(st, gc.NotNil)
		select {
		case started <- struct{}{}:
		default:
		}
		return newDummyWorker(), nil
	})
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for peergrouper worker to be started")
	}
}

func (s *MachineSuite) testAddresserNewWorkerResult(c *gc.C, expectFinished bool) {
	// TODO(dimitern): Fix this in a follow-up.
	c.Skip("Test temporarily disabled as flaky - see bug lp:1488576")

	started := make(chan struct{})
	s.PatchValue(&newAddresser, func(api *apiaddresser.API) (worker.Worker, error) {
		close(started)
		w, err := addresser.NewWorker(api)
		c.Check(err, jc.ErrorIsNil)
		if expectFinished {
			// When the address-allocation feature flag is disabled.
			c.Check(w, gc.FitsTypeOf, worker.FinishedWorker{})
		} else {
			// When the address-allocation feature flag is enabled.
			c.Check(w, gc.Not(gc.FitsTypeOf), worker.FinishedWorker{})
		}
		return w, err
	})

	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()

	// Wait for the worker that starts before the addresser to start.
	_ = s.singularRecord.nextRunner(c)
	r := s.singularRecord.nextRunner(c)
	r.waitForWorker(c, "cleaner")

	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for addresser to start")
	}
}

func (s *MachineSuite) TestAddresserWorkerDoesNotStopWhenAddressDeallocationSupported(c *gc.C) {
	s.SetFeatureFlags(feature.AddressAllocation)
	s.testAddresserNewWorkerResult(c, false)
}

func (s *MachineSuite) TestAddresserWorkerStopsWhenAddressDeallocationNotSupported(c *gc.C) {
	s.SetFeatureFlags()
	s.testAddresserNewWorkerResult(c, true)
}

func (s *MachineSuite) TestManageEnvironRunsDbLogPrunerIfFeatureFlagEnabled(c *gc.C) {
	s.SetFeatureFlags("db-log")

	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()

	runner := s.singularRecord.nextRunner(c)
	runner.waitForWorker(c, "dblogpruner")
}

func (s *MachineSuite) TestManageEnvironDoesntRunDbLogPrunerByDefault(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()

	// Wait for the txnpruner to be started. This is started just after
	// dblogpruner would be started.
	runner := s.singularRecord.nextRunner(c)
	started := set.NewStrings(runner.waitForWorker(c, "txnpruner")...)
	c.Assert(started.Contains("dblogpruner"), jc.IsFalse)
}

func (s *MachineSuite) TestManageEnvironRunsStatusHistoryPruner(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()

	runner := s.singularRecord.nextRunner(c)
	runner.waitForWorker(c, "statushistorypruner")
}

func (s *MachineSuite) TestManageEnvironCallsUseMultipleCPUs(c *gc.C) {
	// If it has been enabled, the JobManageEnviron agent should call utils.UseMultipleCPUs
	usefulVersion := version.Current
	usefulVersion.Series = "quantal"
	envtesting.AssertUploadFakeToolsVersions(
		c, s.DefaultToolsStorage, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), usefulVersion)
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	calledChan := make(chan struct{}, 1)
	s.AgentSuite.PatchValue(&useMultipleCPUs, func() { calledChan <- struct{}{} })
	// Now, start the agent, and observe that a JobManageEnviron agent
	// calls UseMultipleCPUs
	a := s.newAgent(c, m)
	defer a.Stop()
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()
	// Wait for configuration to be finished
	<-a.WorkersStarted()
	select {
	case <-calledChan:
	case <-time.After(coretesting.LongWait):
		c.Errorf("we failed to call UseMultipleCPUs()")
	}
	c.Check(a.Stop(), jc.ErrorIsNil)
	// However, an agent that just JobHostUnits doesn't call UseMultipleCPUs
	m2, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a2 := s.newAgent(c, m2)
	defer a2.Stop()
	go func() {
		c.Check(a2.Run(nil), jc.ErrorIsNil)
	}()
	// Wait until all the workers have been started, and then kill everything
	<-a2.workersStarted
	c.Check(a2.Stop(), jc.ErrorIsNil)
	select {
	case <-calledChan:
		c.Errorf("we should not have called UseMultipleCPUs()")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *MachineSuite) waitProvisioned(c *gc.C, unit *state.Unit) (*state.Machine, instance.Id) {
	c.Logf("waiting for unit %q to be provisioned", unit)
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	w := m.Watch()
	defer w.Stop()
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timed out waiting for provisioning")
		case _, ok := <-w.Changes():
			c.Assert(ok, jc.IsTrue)
			err := m.Refresh()
			c.Assert(err, jc.ErrorIsNil)
			if instId, err := m.InstanceId(); err == nil {
				c.Logf("unit provisioned with instance %s", instId)
				return m, instId
			} else {
				c.Check(err, jc.Satisfies, errors.IsNotProvisioned)
			}
		}
	}
}

func (s *MachineSuite) testUpgradeRequest(c *gc.C, agent runner, tag string, currentTools *tools.Tools) {
	newVers := version.Current
	newVers.Patch++
	newTools := envtesting.AssertUploadFakeToolsVersions(
		c, s.DefaultToolsStorage, s.Environ.Config().AgentStream(), s.Environ.Config().AgentStream(), newVers)[0]
	err := s.State.SetEnvironAgentVersion(newVers.Number)
	c.Assert(err, jc.ErrorIsNil)
	err = runWithTimeout(agent)
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: tag,
		OldTools:  currentTools.Version,
		NewTools:  newTools.Version,
		DataDir:   s.DataDir(),
	})
}

func (s *MachineSuite) TestUpgradeRequest(c *gc.C) {
	m, _, currentTools := s.primeAgent(c, version.Current, state.JobManageEnviron, state.JobHostUnits)
	a := s.newAgent(c, m)
	s.testUpgradeRequest(c, a, m.Tag().String(), currentTools)
	c.Assert(a.isAgentUpgradePending(), jc.IsTrue)
}

func (s *MachineSuite) TestNoUpgradeRequired(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron, state.JobHostUnits)
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() { done <- a.Run(nil) }()
	select {
	case <-a.initialAgentUpgradeCheckComplete:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout waiting for upgrade check")
	}
	defer a.Stop() // in case of failure
	s.waitStopped(c, state.JobManageEnviron, a, done)
	c.Assert(a.isAgentUpgradePending(), jc.IsFalse)
}

var fastDialOpts = api.DialOpts{
	Timeout:    coretesting.LongWait,
	RetryDelay: coretesting.ShortWait,
}

func (s *MachineSuite) waitStopped(c *gc.C, job state.MachineJob, a *MachineAgent, done chan error) {
	err := a.Stop()
	if job == state.JobManageEnviron {
		// When shutting down, the API server can be shut down before
		// the other workers that connect to it, so they get an error so
		// they then die, causing Stop to return an error.  It's not
		// easy to control the actual error that's received in this
		// circumstance so we just log it rather than asserting that it
		// is not nil.
		if err != nil {
			c.Logf("error shutting down state manager: %v", err)
		}
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}

	select {
	case err := <-done:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for agent to terminate")
	}
}

func (s *MachineSuite) assertJobWithAPI(
	c *gc.C,
	job state.MachineJob,
	test func(agent.Config, api.Connection),
) {
	s.assertAgentOpensState(c, &reportOpenedAPI, job, func(cfg agent.Config, st interface{}) {
		test(cfg, st.(api.Connection))
	})
}

func (s *MachineSuite) assertJobWithState(
	c *gc.C,
	job state.MachineJob,
	test func(agent.Config, *state.State),
) {
	paramsJob := job.ToParams()
	if !paramsJob.NeedsState() {
		c.Fatalf("%v does not use state", paramsJob)
	}
	s.assertAgentOpensState(c, &reportOpenedState, job, func(cfg agent.Config, st interface{}) {
		test(cfg, st.(*state.State))
	})
}

// assertAgentOpensState asserts that a machine agent started with the
// given job will call the function pointed to by reportOpened. The
// agent's configuration and the value passed to reportOpened are then
// passed to the test function for further checking.
func (s *MachineSuite) assertAgentOpensState(
	c *gc.C,
	reportOpened *func(io.Closer),
	job state.MachineJob,
	test func(agent.Config, interface{}),
) {
	stm, conf, _ := s.primeAgent(c, version.Current, job)
	a := s.newAgent(c, stm)
	defer a.Stop()
	logger.Debugf("new agent %#v", a)

	// All state jobs currently also run an APIWorker, so no
	// need to check for that here, like in assertJobWithState.

	agentAPIs := make(chan io.Closer, 1)
	s.AgentSuite.PatchValue(reportOpened, func(st io.Closer) {
		select {
		case agentAPIs <- st:
		default:
		}
	})

	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()

	select {
	case agentAPI := <-agentAPIs:
		c.Assert(agentAPI, gc.NotNil)
		test(conf, agentAPI)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("API not opened")
	}

	s.waitStopped(c, job, a, done)
}

func (s *MachineSuite) TestManageEnvironServesAPI(c *gc.C) {
	s.assertJobWithState(c, state.JobManageEnviron, func(conf agent.Config, agentState *state.State) {
		st, err := api.Open(conf.APIInfo(), fastDialOpts)
		c.Assert(err, jc.ErrorIsNil)
		defer st.Close()
		m, err := st.Machiner().Machine(conf.Tag().(names.MachineTag))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Life(), gc.Equals, params.Alive)
	})
}

func (s *MachineSuite) assertAgentSetsToolsVersion(c *gc.C, job state.MachineJob) {
	vers := version.Current
	vers.Minor = version.Current.Minor + 1
	m, _, _ := s.primeAgent(c, vers, job)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

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
			c.Logf("(%v vs. %v)", agentTools.Version, version.Current)
			if agentTools.Version.Minor != version.Current.Minor {
				continue
			}
			c.Assert(agentTools.Version, gc.DeepEquals, version.Current)
			done = true
		}
	}
}

func (s *MachineSuite) TestAgentSetsToolsVersionManageEnviron(c *gc.C) {
	s.assertAgentSetsToolsVersion(c, state.JobManageEnviron)
}

func (s *MachineSuite) TestAgentSetsToolsVersionHostUnits(c *gc.C) {
	s.assertAgentSetsToolsVersion(c, state.JobHostUnits)
}

func (s *MachineSuite) TestManageEnvironRunsCleaner(c *gc.C) {
	s.assertJobWithState(c, state.JobManageEnviron, func(conf agent.Config, agentState *state.State) {
		// Create a service and unit, and destroy the service.
		service := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
		unit, err := service.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		err = service.Destroy()
		c.Assert(err, jc.ErrorIsNil)

		// Check the unit was not yet removed.
		err = unit.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		w := unit.Watch()
		defer w.Stop()

		// Trigger a sync on the state used by the agent, and wait
		// for the unit to be removed.
		agentState.StartSync()
		timeout := time.After(coretesting.LongWait)
		for done := false; !done; {
			select {
			case <-timeout:
				c.Fatalf("unit not cleaned up")
			case <-time.After(coretesting.ShortWait):
				s.State.StartSync()
			case <-w.Changes():
				err := unit.Refresh()
				if errors.IsNotFound(err) {
					done = true
				} else {
					c.Assert(err, jc.ErrorIsNil)
				}
			}
		}
	})
}

func (s *MachineSuite) TestJobManageEnvironRunsMinUnitsWorker(c *gc.C) {
	s.assertJobWithState(c, state.JobManageEnviron, func(conf agent.Config, agentState *state.State) {
		// Ensure that the MinUnits worker is alive by doing a simple check
		// that it responds to state changes: add a service, set its minimum
		// number of units to one, wait for the worker to add the missing unit.
		service := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
		err := service.SetMinUnits(1)
		c.Assert(err, jc.ErrorIsNil)
		w := service.Watch()
		defer w.Stop()

		// Trigger a sync on the state used by the agent, and wait for the unit
		// to be created.
		agentState.StartSync()
		timeout := time.After(coretesting.LongWait)
		for {
			select {
			case <-timeout:
				c.Fatalf("unit not created")
			case <-time.After(coretesting.ShortWait):
				s.State.StartSync()
			case <-w.Changes():
				units, err := service.AllUnits()
				c.Assert(err, jc.ErrorIsNil)
				if len(units) == 1 {
					return
				}
			}
		}
	})
}

func (s *MachineSuite) TestMachineAgentRunsAuthorisedKeysWorker(c *gc.C) {
	//TODO(bogdanteleaga): Fix once we get authentication worker up on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: authentication worker not yet implemented on windows")
	}
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Update the keys in the environment.
	sshKey := sshtesting.ValidKeyOne.Key + " user@host"
	err := s.BackingState.UpdateEnvironConfig(map[string]interface{}{"authorized-keys": sshKey}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for ssh keys file to be updated.
	s.State.StartSync()
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

// opRecvTimeout waits for any of the given kinds of operation to
// be received from ops, and times out if not.
func opRecvTimeout(c *gc.C, st *state.State, opc <-chan dummy.Operation, kinds ...dummy.Operation) dummy.Operation {
	st.StartSync()
	for {
		select {
		case op := <-opc:
			for _, k := range kinds {
				if reflect.TypeOf(op) == reflect.TypeOf(k) {
					return op
				}
			}
			c.Logf("discarding unknown event %#v", op)
		case <-time.After(15 * time.Second):
			c.Fatalf("time out wating for operation")
		}
	}
}

func (s *MachineSuite) TestOpenStateFailsForJobHostUnits(c *gc.C) {
	s.assertJobWithAPI(c, state.JobHostUnits, func(conf agent.Config, st api.Connection) {
		s.AssertCannotOpenState(c, conf.Tag(), conf.DataDir())
	})
}

func (s *MachineSuite) TestOpenStateFailsForJobManageNetworking(c *gc.C) {
	s.assertJobWithAPI(c, state.JobManageNetworking, func(conf agent.Config, st api.Connection) {
		s.AssertCannotOpenState(c, conf.Tag(), conf.DataDir())
	})
}

func (s *MachineSuite) TestOpenStateWorksForJobManageEnviron(c *gc.C) {
	s.assertJobWithAPI(c, state.JobManageEnviron, func(conf agent.Config, st api.Connection) {
		s.AssertCanOpenState(c, conf.Tag(), conf.DataDir())
	})
}

func (s *MachineSuite) TestOpenAPIStateWorksForJobHostUnits(c *gc.C) {
	machine, conf, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	s.runOpenAPISTateTest(c, machine, conf)
}

func (s *MachineSuite) TestOpenAPIStateWorksForJobManageNetworking(c *gc.C) {
	machine, conf, _ := s.primeAgent(c, version.Current, state.JobManageNetworking)
	s.runOpenAPISTateTest(c, machine, conf)
}

func (s *MachineSuite) TestOpenAPIStateWorksForJobManageEnviron(c *gc.C) {
	machine, conf, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	s.runOpenAPISTateTest(c, machine, conf)
}

func (s *MachineSuite) runOpenAPISTateTest(c *gc.C, machine *state.Machine, conf agent.Config) {
	configPath := agent.ConfigPath(conf.DataDir(), conf.Tag())

	// Set a failing password...
	confW, err := agent.ReadConfig(configPath)
	c.Assert(err, jc.ErrorIsNil)
	confW.SetPassword("not-set-on-state-server")

	// ...and also make sure the api info points to the testing api
	// server (and not, as for JobManageEnviron machines, to the port
	// chosen for the agent's own API server to run on. This is usually
	// sane, but inconvenient here because we're not running the full
	// agent and so the configured API server is not actually there).
	apiInfo := s.APIInfo(c)
	hostPorts, err := network.ParseHostPorts(apiInfo.Addrs...)
	c.Assert(err, jc.ErrorIsNil)
	confW.SetAPIHostPorts([][]network.HostPort{hostPorts})
	err = confW.Write()
	c.Assert(err, jc.ErrorIsNil)

	// Check that it successfully connects with the conf's old password.
	assertOpen := func() {
		tagString := conf.Tag().String()
		agent := NewAgentConf(conf.DataDir())
		err := agent.ReadConfig(tagString)
		c.Assert(err, jc.ErrorIsNil)
		st, gotEntity, err := apicaller.OpenAPIState(agent)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(st, gc.NotNil)
		st.Close()
		c.Assert(gotEntity.Tag(), gc.Equals, tagString)
	}
	assertOpen()

	// Check that the initial password is no longer valid.
	assertPassword := func(password string, valid bool) {
		err := machine.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(machine.PasswordValid(password), gc.Equals, valid)
	}
	assertPassword(initialMachinePassword, false)

	// Read the configuration and check that we can connect with it.
	confR, err := agent.ReadConfig(configPath)
	c.Assert(err, gc.IsNil)
	newPassword := confR.APIInfo().Password
	assertPassword(newPassword, true)

	// Double-check that we can open a fresh connection with the stored
	// conf ... and that the password hasn't been changed again.
	assertOpen()
	assertPassword(newPassword, true)
}

func (s *MachineSuite) TestMachineAgentSymlinkJujuRun(c *gc.C) {
	_, err := os.Stat(JujuRun)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	s.assertJobWithAPI(c, state.JobManageEnviron, func(conf agent.Config, st api.Connection) {
		// juju-run should have been created
		_, err := os.Stat(JujuRun)
		c.Assert(err, jc.ErrorIsNil)
	})
}

func (s *MachineSuite) TestMachineAgentSymlinkJujuRunExists(c *gc.C) {
	if runtime.GOOS == "windows" {
		// Cannot make symlink to nonexistent file on windows or
		// create a file point a symlink to it then remove it
		c.Skip("Cannot test this on windows")
	}
	err := symlink.New("/nowhere/special", JujuRun)
	c.Assert(err, jc.ErrorIsNil)
	_, err = os.Stat(JujuRun)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	s.assertJobWithAPI(c, state.JobManageEnviron, func(conf agent.Config, st api.Connection) {
		// juju-run should have been recreated
		_, err := os.Stat(JujuRun)
		c.Assert(err, jc.ErrorIsNil)
		link, err := symlink.Read(JujuRun)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(link, gc.Not(gc.Equals), "/nowhere/special")
	})
}

func (s *MachineSuite) TestProxyUpdater(c *gc.C) {
	s.assertProxyUpdater(c, true)
	s.assertProxyUpdater(c, false)
}

func (s *MachineSuite) assertProxyUpdater(c *gc.C, expectWriteSystemFiles bool) {
	// Patch out the func that decides whether we should write system files.
	var gotConf agent.Config
	s.AgentSuite.PatchValue(&shouldWriteProxyFiles, func(conf agent.Config) bool {
		gotConf = conf
		return expectWriteSystemFiles
	})

	// Make sure there are some proxy settings to write.
	expectSettings := proxy.Settings{
		Http:  "http proxy",
		Https: "https proxy",
		Ftp:   "ftp proxy",
	}
	updateAttrs := config.ProxyConfigMap(expectSettings)
	err := s.State.UpdateEnvironConfig(updateAttrs, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Patch out the actual worker func.
	started := make(chan struct{})
	mockNew := func(api *apienvironment.Facade, writeSystemFiles bool) worker.Worker {
		// Direct check of the behaviour flag.
		c.Check(writeSystemFiles, gc.Equals, expectWriteSystemFiles)
		// Indirect check that we get a functional API.
		conf, err := api.EnvironConfig()
		if c.Check(err, jc.ErrorIsNil) {
			actualSettings := conf.ProxySettings()
			c.Check(actualSettings, jc.DeepEquals, expectSettings)
		}
		return worker.NewSimpleWorker(func(_ <-chan struct{}) error {
			close(started)
			return nil
		})
	}
	s.AgentSuite.PatchValue(&proxyupdater.New, mockNew)

	s.primeAgent(c, version.Current, state.JobHostUnits)
	s.assertJobWithAPI(c, state.JobHostUnits, func(conf agent.Config, st api.Connection) {
		for {
			select {
			case <-time.After(coretesting.LongWait):
				c.Fatalf("timeout while waiting for proxy updater to start")
			case <-started:
				c.Assert(gotConf, jc.DeepEquals, conf)
				return
			}
		}
	})
}

func (s *MachineSuite) TestMachineAgentUninstall(c *gc.C) {
	m, ac, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	err := m.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	a := s.newAgent(c, m)
	err = runWithTimeout(a)
	c.Assert(err, jc.ErrorIsNil)
	// juju-run should have been removed on termination
	_, err = os.Stat(JujuRun)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	// data-dir should have been removed on termination
	_, err = os.Stat(ac.DataDir())
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *MachineSuite) TestMachineAgentRsyslogManageEnviron(c *gc.C) {
	s.testMachineAgentRsyslogConfigWorker(c, state.JobManageEnviron, rsyslog.RsyslogModeAccumulate)
}

func (s *MachineSuite) TestMachineAgentRsyslogHostUnits(c *gc.C) {
	s.testMachineAgentRsyslogConfigWorker(c, state.JobHostUnits, rsyslog.RsyslogModeForwarding)
}

func (s *MachineSuite) testMachineAgentRsyslogConfigWorker(c *gc.C, job state.MachineJob, expectedMode rsyslog.RsyslogMode) {
	created := make(chan rsyslog.RsyslogMode, 1)
	s.AgentSuite.PatchValue(&cmdutil.NewRsyslogConfigWorker, func(_ *apirsyslog.State, _ agent.Config, mode rsyslog.RsyslogMode) (worker.Worker, error) {
		created <- mode
		return newDummyWorker(), nil
	})
	s.assertJobWithAPI(c, job, func(conf agent.Config, st api.Connection) {
		select {
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timeout while waiting for rsyslog worker to be created")
		case mode := <-created:
			c.Assert(mode, gc.Equals, expectedMode)
		}
	})
}

func (s *MachineSuite) TestMachineAgentRunsAPIAddressUpdaterWorker(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Update the API addresses.
	updatedServers := [][]network.HostPort{
		network.NewHostPorts(1234, "localhost"),
	}
	err := s.BackingState.SetAPIHostPorts(updatedServers)
	c.Assert(err, jc.ErrorIsNil)

	// Wait for config to be updated.
	s.BackingState.StartSync()
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
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
	started := make(chan struct{})
	newWorker := func(diskmanager.ListBlockDevicesFunc, diskmanager.BlockDeviceSetter) worker.Worker {
		close(started)
		return worker.NewNoOpWorker()
	}
	s.PatchValue(&newDiskManager, newWorker)

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for worker to be started.
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for diskmanager worker to start")
	}
}

func (s *MachineSuite) TestDiskManagerWorkerUpdatesState(c *gc.C) {
	expected := []storage.BlockDevice{{DeviceName: "whatever"}}
	s.PatchValue(&diskmanager.DefaultListBlockDevices, func() ([]storage.BlockDevice, error) {
		return expected, nil
	})

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for state to be updated.
	s.BackingState.StartSync()
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		devices, err := s.BackingState.BlockDevices(m.MachineTag())
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
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)

	started := make(chan struct{})
	newWorker := func(
		scope names.Tag,
		storageDir string,
		_ storageprovisioner.VolumeAccessor,
		_ storageprovisioner.FilesystemAccessor,
		_ storageprovisioner.LifecycleManager,
		_ storageprovisioner.EnvironAccessor,
		_ storageprovisioner.MachineAccessor,
		_ storageprovisioner.StatusSetter,
		_ clock.Clock,
	) worker.Worker {
		c.Check(scope, gc.Equals, m.Tag())
		// storageDir is not empty for machine scoped storage provisioners
		c.Assert(storageDir, gc.Not(gc.Equals), "")
		close(started)
		return worker.NewNoOpWorker()
	}
	s.PatchValue(&newStorageWorker, newWorker)

	// Start the machine agent.
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for worker to be started.
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for storage worker to start")
	}
}

func (s *MachineSuite) TestMachineAgentRunsEnvironStorageWorker(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)

	var numWorkers, machineWorkers, environWorkers uint32
	started := make(chan struct{})
	newWorker := func(
		scope names.Tag,
		storageDir string,
		_ storageprovisioner.VolumeAccessor,
		_ storageprovisioner.FilesystemAccessor,
		_ storageprovisioner.LifecycleManager,
		_ storageprovisioner.EnvironAccessor,
		_ storageprovisioner.MachineAccessor,
		_ storageprovisioner.StatusSetter,
		_ clock.Clock,
	) worker.Worker {
		// storageDir is empty for environ storage provisioners
		if storageDir == "" {
			c.Check(scope, gc.Equals, s.State.EnvironTag())
			c.Check(atomic.AddUint32(&environWorkers, 1), gc.Equals, uint32(1))
			atomic.AddUint32(&numWorkers, 1)
		}
		if storageDir != "" {
			c.Check(scope, gc.Equals, m.Tag())
			c.Check(atomic.AddUint32(&machineWorkers, 1), gc.Equals, uint32(1))
			atomic.AddUint32(&numWorkers, 1)
		}
		if atomic.LoadUint32(&environWorkers) == 1 && atomic.LoadUint32(&machineWorkers) == 1 {
			close(started)
		}
		return worker.NewNoOpWorker()
	}
	s.PatchValue(&newStorageWorker, newWorker)

	// Start the machine agent.
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for worker to be started.
	select {
	case <-started:
		c.Assert(atomic.LoadUint32(&numWorkers), gc.Equals, uint32(2))
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for storage worker to start")
	}
}

func (s *MachineSuite) TestMachineAgentRunsCertificateUpdateWorkerForStateServer(c *gc.C) {
	started := make(chan struct{})
	newUpdater := func(certupdater.AddressWatcher, certupdater.StateServingInfoGetter, certupdater.EnvironConfigGetter,
		certupdater.APIHostPortsGetter, certupdater.StateServingInfoSetter, chan params.StateServingInfo,
	) worker.Worker {
		close(started)
		return worker.NewNoOpWorker()
	}
	s.PatchValue(&newCertificateUpdater, newUpdater)

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Wait for worker to be started.
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for certificate update worker to start")
	}
}

func (s *MachineSuite) TestMachineAgentDoesNotRunsCertificateUpdateWorkerForNonStateServer(c *gc.C) {
	started := make(chan struct{})
	newUpdater := func(certupdater.AddressWatcher, certupdater.StateServingInfoGetter, certupdater.EnvironConfigGetter,
		certupdater.APIHostPortsGetter, certupdater.StateServingInfoSetter, chan params.StateServingInfo,
	) worker.Worker {
		close(started)
		return worker.NewNoOpWorker()
	}
	s.PatchValue(&newCertificateUpdater, newUpdater)

	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	// Ensure the worker is not started.
	select {
	case <-started:
		c.Fatalf("certificate update worker unexpectedly started")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *MachineSuite) TestCertificateUpdateWorkerUpdatesCertificate(c *gc.C) {
	// Set up the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	a.ReadConfig(names.NewMachineTag(m.Id()).String())

	// Set up check that certificate has been updated.
	updated := make(chan struct{})
	go func() {
		for {
			stateInfo, _ := a.CurrentConfig().StateServingInfo()
			srvCert, err := cert.ParseCert(stateInfo.Cert)
			c.Assert(err, jc.ErrorIsNil)
			sanIPs := make([]string, len(srvCert.IPAddresses))
			for i, ip := range srvCert.IPAddresses {
				sanIPs[i] = ip.String()
			}
			if len(sanIPs) == 1 && sanIPs[0] == "0.1.2.3" {
				close(updated)
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	// Wait for certificate to be updated.
	select {
	case <-updated:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for certificate to be updated")
	}
}

func (s *MachineSuite) TestCertificateDNSUpdated(c *gc.C) {
	// Disable the certificate work so it doesn't update the certificate.
	newUpdater := func(certupdater.AddressWatcher, certupdater.StateServingInfoGetter, certupdater.EnvironConfigGetter,
		certupdater.APIHostPortsGetter, certupdater.StateServingInfoSetter, chan params.StateServingInfo,
	) worker.Worker {
		return worker.NewNoOpWorker()
	}
	s.PatchValue(&newCertificateUpdater, newUpdater)

	// Set up the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)

	// Set up check that certificate has been updated when the agent starts.
	updated := make(chan struct{})
	expectedDnsNames := set.NewStrings("local", "juju-apiserver", "juju-mongodb")
	go func() {
		for {
			stateInfo, _ := a.CurrentConfig().StateServingInfo()
			srvCert, err := cert.ParseCert(stateInfo.Cert)
			c.Assert(err, jc.ErrorIsNil)
			certDnsNames := set.NewStrings(srvCert.DNSNames...)
			if !expectedDnsNames.Difference(certDnsNames).IsEmpty() {
				continue
			}
			pemContent, err := ioutil.ReadFile(filepath.Join(s.DataDir(), "server.pem"))
			c.Assert(err, jc.ErrorIsNil)
			if string(pemContent) == stateInfo.Cert+"\n"+stateInfo.PrivateKey {
				close(updated)
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	// Wait for certificate to be updated.
	select {
	case <-updated:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for certificate to be updated")
	}
}

func (s *MachineSuite) TestMachineAgentNetworkerMode(c *gc.C) {
	tests := []struct {
		about          string
		managedNetwork bool
		jobs           []state.MachineJob
		intrusiveMode  bool
	}{{
		about:          "network management enabled, network management job set",
		managedNetwork: true,
		jobs:           []state.MachineJob{state.JobHostUnits, state.JobManageNetworking},
		intrusiveMode:  true,
	}, {
		about:          "network management disabled, network management job set",
		managedNetwork: false,
		jobs:           []state.MachineJob{state.JobHostUnits, state.JobManageNetworking},
		intrusiveMode:  false,
	}, {
		about:          "network management enabled, network management job not set",
		managedNetwork: true,
		jobs:           []state.MachineJob{state.JobHostUnits},
		intrusiveMode:  false,
	}, {
		about:          "network management disabled, network management job not set",
		managedNetwork: false,
		jobs:           []state.MachineJob{state.JobHostUnits},
		intrusiveMode:  false,
	}}
	// Perform tests.
	for i, test := range tests {
		c.Logf("test #%d: %s", i, test.about)

		modeCh := make(chan bool, 1)
		s.AgentSuite.PatchValue(&newNetworker, func(
			st apinetworker.State,
			conf agent.Config,
			intrusiveMode bool,
			configBaseDir string,
		) (*networker.Networker, error) {
			select {
			case modeCh <- intrusiveMode:
			default:
			}
			return networker.NewNetworker(st, conf, intrusiveMode, configBaseDir)
		})

		attrs := coretesting.Attrs{"disable-network-management": !test.managedNetwork}
		err := s.BackingState.UpdateEnvironConfig(attrs, nil, nil)
		c.Assert(err, jc.ErrorIsNil)

		m, _, _ := s.primeAgent(c, version.Current, test.jobs...)
		a := s.newAgent(c, m)
		defer a.Stop()
		doneCh := make(chan error)
		go func() {
			doneCh <- a.Run(nil)
		}()

		select {
		case intrusiveMode := <-modeCh:
			if intrusiveMode != test.intrusiveMode {
				c.Fatalf("expected networker intrusive mode = %v, got mode = %v", test.intrusiveMode, intrusiveMode)
			}
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for the networker to start")
		}
		s.waitStopped(c, state.JobManageNetworking, a, doneCh)
	}
}

func (s *MachineSuite) TestMachineAgentIgnoreAddresses(c *gc.C) {
	for _, expectedIgnoreValue := range []bool{true, false} {
		ignoreAddressCh := make(chan bool, 1)
		s.AgentSuite.PatchValue(&newMachiner, func(
			accessor machiner.MachineAccessor,
			conf agent.Config,
			ignoreMachineAddresses bool,
		) worker.Worker {
			select {
			case ignoreAddressCh <- ignoreMachineAddresses:
			default:
			}
			return machiner.NewMachiner(accessor, conf, ignoreMachineAddresses)
		})

		attrs := coretesting.Attrs{"ignore-machine-addresses": expectedIgnoreValue}
		err := s.BackingState.UpdateEnvironConfig(attrs, nil, nil)
		c.Assert(err, jc.ErrorIsNil)

		m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
		a := s.newAgent(c, m)
		defer a.Stop()
		doneCh := make(chan error)
		go func() {
			doneCh <- a.Run(nil)
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

func (s *MachineSuite) TestMachineAgentUpgradeMongo(c *gc.C) {
	m, agentConfig, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	agentConfig.SetUpgradedToVersion(version.MustParse("1.18.0"))
	err := agentConfig.Write()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.MongoSession().DB("admin").RemoveUser(m.Tag().String())
	c.Assert(err, jc.ErrorIsNil)

	s.fakeEnsureMongo.ServiceInstalled = true
	s.fakeEnsureMongo.ReplicasetInitiated = false

	s.AgentSuite.PatchValue(&ensureMongoAdminUser, func(p mongo.EnsureAdminUserParams) (bool, error) {
		err := s.State.MongoSession().DB("admin").AddUser(p.User, p.Password, false)
		c.Assert(err, jc.ErrorIsNil)
		return true, nil
	})

	stateOpened := make(chan interface{}, 1)
	s.AgentSuite.PatchValue(&reportOpenedState, func(st io.Closer) {
		select {
		case stateOpened <- st:
		default:
		}
	})

	// Start the machine agent, and wait for state to be opened.
	a := s.newAgent(c, m)
	done := make(chan error)
	go func() { done <- a.Run(nil) }()
	defer a.Stop() // in case of failure
	select {
	case st := <-stateOpened:
		c.Assert(st, gc.NotNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("state not opened")
	}
	s.waitStopped(c, state.JobManageEnviron, a, done)
	c.Assert(s.fakeEnsureMongo.EnsureCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.InitiateCount, gc.Equals, 1)
}

func (s *MachineSuite) TestMachineAgentSetsPrepareRestore(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	c.Check(a.IsRestorePreparing(), jc.IsFalse)
	c.Check(a.IsRestoreRunning(), jc.IsFalse)
	err := a.PrepareRestore()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a.IsRestorePreparing(), jc.IsTrue)
	c.Assert(a.IsRestoreRunning(), jc.IsFalse)
	err = a.PrepareRestore()
	c.Assert(err, gc.ErrorMatches, "already in restore mode")
}

func (s *MachineSuite) TestMachineAgentSetsRestoreInProgress(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	c.Check(a.IsRestorePreparing(), jc.IsFalse)
	c.Check(a.IsRestoreRunning(), jc.IsFalse)
	err := a.PrepareRestore()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a.IsRestorePreparing(), jc.IsTrue)
	err = a.BeginRestore()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a.IsRestoreRunning(), jc.IsTrue)
	err = a.BeginRestore()
	c.Assert(err, gc.ErrorMatches, "already restoring")
}

func (s *MachineSuite) TestMachineAgentRestoreRequiresPrepare(c *gc.C) {
	// Start the machine agent.
	m, _, _ := s.primeAgent(c, version.Current, state.JobHostUnits)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()
	c.Check(a.IsRestorePreparing(), jc.IsFalse)
	c.Check(a.IsRestoreRunning(), jc.IsFalse)
	err := a.BeginRestore()
	c.Assert(err, gc.ErrorMatches, "not in restore mode, cannot begin restoration")
	c.Assert(a.IsRestoreRunning(), jc.IsFalse)
}

func (s *MachineSuite) TestNewEnvironmentStartsNewWorkers(c *gc.C) {
	_, closer := s.setUpNewEnvironment(c)
	defer closer()
	expectedWorkers, closer := s.setUpAgent(c)
	defer closer()

	r1 := s.singularRecord.nextRunner(c)
	workers := r1.waitForWorker(c, "firewaller")
	c.Assert(workers, jc.SameContents, expectedWorkers)
}

func (s *MachineSuite) TestNewStorageWorkerIsScopedToNewEnviron(c *gc.C) {
	st, closer := s.setUpNewEnvironment(c)
	defer closer()

	// Check that newStorageWorker is called and the environ tag is scoped to
	// that of the new environment tag.
	started := make(chan struct{})
	newWorker := func(
		scope names.Tag,
		storageDir string,
		_ storageprovisioner.VolumeAccessor,
		_ storageprovisioner.FilesystemAccessor,
		_ storageprovisioner.LifecycleManager,
		_ storageprovisioner.EnvironAccessor,
		_ storageprovisioner.MachineAccessor,
		_ storageprovisioner.StatusSetter,
		_ clock.Clock,
	) worker.Worker {
		// storageDir is empty for environ storage provisioners
		if storageDir == "" {
			// If this is the worker for the new environment,
			// close the channel.
			if scope == st.EnvironTag() {
				close(started)
			}
		}
		return worker.NewNoOpWorker()
	}
	s.PatchValue(&newStorageWorker, newWorker)

	_, closer = s.setUpAgent(c)
	defer closer()

	// Wait for newStorageWorker to be started.
	select {
	case <-started:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for storage worker to start")
	}
}

func (s *MachineSuite) setUpNewEnvironment(c *gc.C) (newSt *state.State, closer func()) {
	// Create a new environment, tests can now watch if workers start for it.
	newSt = s.Factory.MakeEnvironment(c, &factory.EnvParams{
		ConfigAttrs: map[string]interface{}{
			"state-server": false,
		},
		Prepare: true,
	})
	return newSt, func() {
		newSt.Close()
	}
}

func (s *MachineSuite) setUpAgent(c *gc.C) (expectedWorkers []string, closer func()) {
	expectedWorkers = make([]string, 0, len(perEnvSingularWorkers)+1)
	for _, w := range perEnvSingularWorkers {
		expectedWorkers = append(expectedWorkers, w)
		if w == "environ-provisioner" {
			expectedWorkers = append(expectedWorkers, "environ-storageprovisioner")
		}
	}
	s.PatchValue(&watcher.Period, 100*time.Millisecond)

	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	go func() { c.Check(a.Run(nil), jc.ErrorIsNil) }()

	_ = s.singularRecord.nextRunner(c) // Don't care about this one for this test.

	// Wait for the workers for the initial env to start. The
	// firewaller is the last worker started for a new environment.
	r0 := s.singularRecord.nextRunner(c)
	workers := r0.waitForWorker(c, "firewaller")
	c.Assert(workers, jc.SameContents, expectedWorkers)

	return expectedWorkers, func() {
		c.Check(a.Stop(), jc.ErrorIsNil)
	}
}

func (s *MachineSuite) TestReplicasetInitiation(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("state servers on windows aren't supported")
	}

	s.fakeEnsureMongo.ReplicasetInitiated = false

	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	agentConfig := a.CurrentConfig()

	err := a.ensureMongoServer(agentConfig)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeEnsureMongo.EnsureCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.InitiateCount, gc.Equals, 1)
}

func (s *MachineSuite) TestReplicasetAlreadyInitiated(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("state servers on windows aren't supported")
	}

	s.fakeEnsureMongo.ReplicasetInitiated = true

	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	agentConfig := a.CurrentConfig()

	err := a.ensureMongoServer(agentConfig)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeEnsureMongo.EnsureCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.InitiateCount, gc.Equals, 0)
}

func (s *MachineSuite) TestReplicasetInitForNewStateServer(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("state servers on windows aren't supported")
	}

	s.fakeEnsureMongo.ServiceInstalled = false
	s.fakeEnsureMongo.ReplicasetInitiated = true

	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)
	a := s.newAgent(c, m)
	agentConfig := a.CurrentConfig()

	err := a.ensureMongoServer(agentConfig)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.fakeEnsureMongo.EnsureCount, gc.Equals, 1)
	c.Assert(s.fakeEnsureMongo.InitiateCount, gc.Equals, 0)
}

// MachineWithCharmsSuite provides infrastructure for tests which need to
// work with charms.
type MachineWithCharmsSuite struct {
	commonMachineSuite
	charmtesting.CharmSuite

	machine *state.Machine
}

func (s *MachineWithCharmsSuite) SetUpSuite(c *gc.C) {
	s.commonMachineSuite.SetUpSuite(c)
	s.CharmSuite.SetUpSuite(c, &s.commonMachineSuite.JujuConnSuite)
}

func (s *MachineWithCharmsSuite) TearDownSuite(c *gc.C) {
	s.commonMachineSuite.TearDownSuite(c)
	s.CharmSuite.TearDownSuite(c)
}

func (s *MachineWithCharmsSuite) SetUpTest(c *gc.C) {
	s.commonMachineSuite.SetUpTest(c)
	s.CharmSuite.SetUpTest(c)
}

func (s *MachineWithCharmsSuite) TearDownTest(c *gc.C) {
	s.commonMachineSuite.TearDownTest(c)
	s.CharmSuite.TearDownTest(c)
}

func (s *MachineWithCharmsSuite) TestManageEnvironRunsCharmRevisionUpdater(c *gc.C) {
	m, _, _ := s.primeAgent(c, version.Current, state.JobManageEnviron)

	s.SetupScenario(c)

	a := s.newAgent(c, m)
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	checkRevision := func() bool {
		curl := charm.MustParseURL("cs:quantal/mysql")
		placeholder, err := s.State.LatestPlaceholderCharm(curl)
		return err == nil && placeholder.String() == curl.WithRevision(23).String()
	}
	success := false
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if success = checkRevision(); success {
			break
		}
	}
	c.Assert(success, jc.IsTrue)
}

type mongoSuite struct {
	coretesting.BaseSuite
}

func (s *mongoSuite) TestStateWorkerDialSetsWriteMajority(c *gc.C) {
	s.testStateWorkerDialSetsWriteMajority(c, true)
}

func (s *mongoSuite) TestStateWorkerDialDoesNotSetWriteMajorityWithoutReplsetConfig(c *gc.C) {
	s.testStateWorkerDialSetsWriteMajority(c, false)
}

func (s *mongoSuite) testStateWorkerDialSetsWriteMajority(c *gc.C, configureReplset bool) {
	inst := gitjujutesting.MgoInstance{
		EnableJournal: true,
		Params:        []string{"--replSet", "juju"},
	}
	err := inst.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)
	defer inst.Destroy()

	var expectedWMode string
	dialOpts := stateWorkerDialOpts
	if configureReplset {
		info := inst.DialInfo()
		args := peergrouper.InitiateMongoParams{
			DialInfo:       info,
			MemberHostPort: inst.Addr(),
		}
		err = peergrouper.MaybeInitiateMongoServer(args)
		c.Assert(err, jc.ErrorIsNil)
		expectedWMode = "majority"
	} else {
		dialOpts.Direct = true
	}

	mongoInfo := mongo.Info{
		Addrs:  []string{inst.Addr()},
		CACert: coretesting.CACert,
	}
	session, err := mongo.DialWithInfo(mongoInfo, dialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	safe := session.Safe()
	c.Assert(safe, gc.NotNil)
	c.Assert(safe.WMode, gc.Equals, expectedWMode)
	c.Assert(safe.J, jc.IsTrue) // always enabled
}

type shouldWriteProxyFilesSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&shouldWriteProxyFilesSuite{})

func (s *shouldWriteProxyFilesSuite) TestAll(c *gc.C) {
	tests := []struct {
		description  string
		providerType string
		machineId    string
		expect       bool
	}{{
		description:  "local provider machine 0 must not write",
		providerType: "local",
		machineId:    "0",
		expect:       false,
	}, {
		description:  "local provider other machine must write 1",
		providerType: "local",
		machineId:    "0/kvm/0",
		expect:       true,
	}, {
		description:  "local provider other machine must write 2",
		providerType: "local",
		machineId:    "123",
		expect:       true,
	}, {
		description:  "other provider machine 0 must write",
		providerType: "anything",
		machineId:    "0",
		expect:       true,
	}, {
		description:  "other provider other machine must write 1",
		providerType: "dummy",
		machineId:    "0/kvm/0",
		expect:       true,
	}, {
		description:  "other provider other machine must write 2",
		providerType: "blahblahblah",
		machineId:    "123",
		expect:       true,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.description)
		mockConf := &mockAgentConfig{
			providerType: test.providerType,
			tag:          names.NewMachineTag(test.machineId),
		}
		c.Check(shouldWriteProxyFiles(mockConf), gc.Equals, test.expect)
	}
}

type mockAgentConfig struct {
	agent.Config
	providerType string
	tag          names.Tag
}

func (m *mockAgentConfig) Tag() names.Tag {
	return m.tag
}

func (m *mockAgentConfig) Value(key string) string {
	if key == agent.ProviderType {
		return m.providerType
	}
	return ""
}

type singularRunnerRecord struct {
	runnerC chan *fakeSingularRunner
}

func newSingularRunnerRecord() *singularRunnerRecord {
	return &singularRunnerRecord{
		runnerC: make(chan *fakeSingularRunner, 5),
	}
}

func (r *singularRunnerRecord) newSingularRunner(runner worker.Runner, conn singular.Conn) (worker.Runner, error) {
	sr, err := singular.New(runner, conn)
	if err != nil {
		return nil, err
	}
	fakeRunner := &fakeSingularRunner{
		Runner: sr,
		startC: make(chan string, 64),
	}
	r.runnerC <- fakeRunner
	return fakeRunner, nil
}

// nextRunner blocks until a new singular runner is created.
func (r *singularRunnerRecord) nextRunner(c *gc.C) *fakeSingularRunner {
	for {
		select {
		case r := <-r.runnerC:
			return r
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for singular runner to be created")
		}
	}
}

type fakeSingularRunner struct {
	worker.Runner
	startC chan string
}

func (r *fakeSingularRunner) StartWorker(name string, start func() (worker.Worker, error)) error {
	r.startC <- name
	return r.Runner.StartWorker(name, start)
}

// waitForWorker waits for a given worker to be started, returning all
// workers started while waiting.
func (r *fakeSingularRunner) waitForWorker(c *gc.C, target string) []string {
	var seen []string
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case workerName := <-r.startC:
			seen = append(seen, workerName)
			if workerName == target {
				return seen
			}
		case <-timeout:
			c.Fatal("timed out waiting for " + target)
		}
	}
}

// waitForWorkers waits for a given worker to be started, returning all
// workers started while waiting.
func (r *fakeSingularRunner) waitForWorkers(c *gc.C, targets []string) []string {
	var seen []string
	seenTargets := make(map[string]bool)
	numSeenTargets := 0
	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case workerName := <-r.startC:
			if seenTargets[workerName] == true {
				c.Fatal("worker started twice: " + workerName)
			}
			seenTargets[workerName] = true
			numSeenTargets++
			seen = append(seen, workerName)
			if numSeenTargets == len(targets) {
				return seen
			}
		case <-timeout:
			c.Fatalf("timed out waiting for %v", targets)
		}
	}
}

type mockMetricAPI struct {
	stop          chan struct{}
	cleanUpCalled chan struct{}
	sendCalled    chan struct{}
}

func newMockMetricAPI() *mockMetricAPI {
	return &mockMetricAPI{
		stop:          make(chan struct{}),
		cleanUpCalled: make(chan struct{}),
		sendCalled:    make(chan struct{}),
	}
}

func (m *mockMetricAPI) CleanupOldMetrics() error {
	go func() {
		select {
		case m.cleanUpCalled <- struct{}{}:
		case <-m.stop:
			break
		}
	}()
	return nil
}

func (m *mockMetricAPI) SendMetrics() error {
	go func() {
		select {
		case m.sendCalled <- struct{}{}:
		case <-m.stop:
			break
		}
	}()
	return nil
}

func (m *mockMetricAPI) SendCalled() <-chan struct{} {
	return m.sendCalled
}

func (m *mockMetricAPI) CleanupCalled() <-chan struct{} {
	return m.cleanUpCalled
}

func (m *mockMetricAPI) Stop() {
	close(m.stop)
}

func mkdtemp(prefix string) string {
	d, err := ioutil.TempDir("", prefix)
	if err != nil {
		panic(err)
	}
	return d
}

func mktemp(prefix string, content string) string {
	f, err := ioutil.TempFile("", prefix)
	if err != nil {
		panic(err)
	}
	_, err = f.WriteString(content)
	if err != nil {
		panic(err)
	}
	f.Close()
	return f.Name()
}

type mockLoopDeviceManager struct {
	detachLoopDevicesArgRootfs string
	detachLoopDevicesArgPrefix string
}

func (m *mockLoopDeviceManager) DetachLoopDevices(rootfs, prefix string) error {
	m.detachLoopDevicesArgRootfs = rootfs
	m.detachLoopDevicesArgPrefix = prefix
	return nil
}
