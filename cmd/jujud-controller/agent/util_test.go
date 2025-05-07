// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v4"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/addons"
	agentconfig "github.com/juju/juju/agent/config"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/cmd/jujud-controller/agent/agenttest"
	"github.com/juju/juju/cmd/jujud-controller/agent/mocks"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/mongo/mongometrics"
	"github.com/juju/juju/internal/mongo/mongotest"
	"github.com/juju/juju/internal/provider/dummy"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/upgrades"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/authenticationworker"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/internal/worker/dbaccessor/testing"
	"github.com/juju/juju/internal/worker/diskmanager"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/machiner"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

const (
	initialMachinePassword = "machine-password-1234567890"
)

var fastDialOpts = api.DialOpts{
	Timeout:    coretesting.LongWait,
	RetryDelay: coretesting.ShortWait,
}

type commonMachineSuite struct {
	fakeEnsureMongo *agenttest.FakeEnsureMongo
	AgentSuite
	// FakeJujuXDGDataHomeSuite is needed only because the
	// authenticationworker writes to ~/.ssh.
	coretesting.FakeJujuXDGDataHomeSuite

	cmdRunner *mocks.MockCommandRunner
}

func (s *commonMachineSuite) SetUpSuite(c *gc.C) {
	s.AgentSuite.SetUpSuite(c)
	// Set up FakeJujuXDGDataHomeSuite after AgentSuite since
	// AgentSuite clears all env vars.
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)

	// Stub out executables etc used by workers.
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	s.PatchValue(&stateWorkerDialOpts, mongotest.DialOpts())
	s.PatchValue(&authenticationworker.SSHUser, "")
	s.PatchValue(&diskmanager.DefaultListBlockDevices, func(context.Context) ([]blockdevice.BlockDevice, error) {
		return nil, nil
	})
	s.PatchValue(&machiner.GetObservedNetworkConfig, func(_ network.ConfigSource) (network.InterfaceInfos, error) {
		return nil, nil
	})
}

func (s *commonMachineSuite) SetUpTest(c *gc.C) {
	s.AgentSuite.SetUpTest(c)
	// Set up FakeJujuXDGDataHomeSuite after AgentSuite since
	// AgentSuite clears all env vars.
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	testpath := c.MkDir()
	s.PatchEnvPathPrepend(testpath)
	// mock out the start method so we can fake install services without sudo
	fakeCmd(filepath.Join(testpath, "start"))
	fakeCmd(filepath.Join(testpath, "stop"))

	s.fakeEnsureMongo = agenttest.InstallFakeEnsureMongo(s, s.DataDir)
}

func (s *commonMachineSuite) assertChannelActive(c *gc.C, aChannel chan struct{}, intent string) {
	// Wait for channel to be active.
	select {
	case <-aChannel:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for %v", intent)
	}
}

func fakeCmd(path string) {
	err := os.WriteFile(path, []byte("#!/bin/bash --norc\nexit 0"), 0755)
	if err != nil {
		panic(err)
	}
}

func (s *commonMachineSuite) TearDownTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
	// MgoServer.Reset() is done during the embedded MgoSuite TearDownSuite().
	// But we need to do it for every test in this suite to keep
	// the tests happy.
	if mgotesting.MgoServer.Addr() != "" {
		err := retry.Call(retry.CallArgs{
			Func: mgotesting.MgoServer.Reset,
			// Only interested in retrying the intermittent
			// 'unexpected message'.
			IsFatalError: func(err error) bool {
				return !strings.HasSuffix(err.Error(), "unexpected message")
			},
			Delay:    time.Millisecond,
			Clock:    clock.WallClock,
			Attempts: 5,
			NotifyFunc: func(lastError error, attempt int) {
				logger.Infof(context.TODO(), "retrying MgoServer.Reset() after attempt %d: %v", attempt, lastError)
			},
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	s.AgentSuite.TearDownTest(c)
}

func (s *commonMachineSuite) TearDownSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
	s.AgentSuite.TearDownSuite(c)
}

// primeAgent adds a new Machine to run the given jobs, and sets up the
// machine agent's directory.  It returns the new machine, the
// agent's configuration and the tools currently running.
func (s *commonMachineSuite) primeAgent(c *gc.C, jobs ...state.MachineJob) (
	m *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools,
) {
	vers := coretesting.CurrentVersion()
	return s.primeAgentVersion(c, vers, jobs...)
}

// TODO(wallyworld) - we need the dqlite model database to be available.
//func (s *commonMachineSuite) createMachine(c *gc.C, machineId string) string {
//	db := s.DB()
//
//	netNodeUUID := uuid.MustNewUUID().String()
//	_, err := db.ExecContext(context.Background(), "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
//	c.Assert(err, jc.ErrorIsNil)
//	machineUUID := uuid.MustNewUUID().String()
//	_, err = db.ExecContext(context.Background(), `
//INSERT INTO machine (uuid, life_id, machine_id, net_node_uuid)
//VALUES (?, ?, ?, ?)
//`, machineUUID, life.Alive, machineId, netNodeUUID)
//	c.Assert(err, jc.ErrorIsNil)
//	return machineUUID
//}

// primeAgentVersion is similar to primeAgent, but permits the
// caller to specify the version.Binary to prime with.
func (s *commonMachineSuite) primeAgentVersion(c *gc.C, vers semversion.Binary, jobs ...state.MachineJob) (m *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools) {
	m, err := s.ControllerModel(c).State().AddMachine(state.UbuntuBase("12.10"), jobs...)
	c.Assert(err, jc.ErrorIsNil)
	// TODO(wallyworld) - we need the dqlite model database to be available.
	// s.createMachine(c, m.Id())
	return s.primeAgentWithMachine(c, m, vers)
}

func (s *commonMachineSuite) primeAgentWithMachine(c *gc.C, m *state.Machine, vers semversion.Binary) (*state.Machine, agent.ConfigSetterWriter, *tools.Tools) {
	return s.configureMachine(c, m.Id(), vers)
}

func (s *commonMachineSuite) configureMachine(c *gc.C, machineId string, vers semversion.Binary) (
	machineState *state.Machine, agentConfig agent.ConfigSetterWriter, tools *tools.Tools,
) {
	m, err := s.ControllerModel(c).State().Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	// Add a machine and ensure it is provisioned.
	inst, md := jujutesting.AssertStartInstance(c, s.Environ, s.ControllerUUID, machineId)
	c.Assert(m.SetProvisioned(inst.Id(), "", agent.BootstrapNonce, md), jc.ErrorIsNil)
	// Double write to machine domain.
	machineService := s.ControllerDomainServices(c).Machine()
	machineUUID, err := machineService.CreateMachine(context.Background(), machine.Name(m.Id()))
	c.Assert(err, jc.ErrorIsNil)
	err = machineService.SetMachineCloudInstance(context.Background(), machineUUID, inst.Id(), "", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Add an address for the tests in case the initiateMongoServer
	// codepath is exercised.
	s.setFakeMachineAddresses(c, m, inst.Id())

	// Set up the new machine.
	err = m.SetAgentVersion(vers)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetPassword(initialMachinePassword)
	c.Assert(err, jc.ErrorIsNil)
	tag := m.Tag()
	if m.IsManager() {
		err = m.SetMongoPassword(initialMachinePassword)
		c.Assert(err, jc.ErrorIsNil)
		agentConfig, tools = s.PrimeStateAgentVersion(c, tag, initialMachinePassword, vers)
		info, ok := agentConfig.StateServingInfo()
		c.Assert(ok, jc.IsTrue)
		err = s.ControllerModel(c).State().SetStateServingInfo(info)
		c.Assert(err, jc.ErrorIsNil)
	} else {
		agentConfig, tools = s.PrimeAgentVersion(c, tag, initialMachinePassword, vers)
	}
	err = agentConfig.Write()
	c.Assert(err, jc.ErrorIsNil)
	return m, agentConfig, tools
}

func NewTestMachineAgentFactory(
	c *gc.C,
	agentConfWriter agentconfig.AgentConfigWriter,
	newDBWorkerFunc dbaccessor.NewDBWorkerFunc,
	rootDir string,
	cmdRunner CommandRunner,
) machineAgentFactoryFnType {
	preUpgradeSteps := func(state.ModelType) upgrades.PreUpgradeStepsFunc {
		return func(agent.Config, bool) error {
			return nil
		}
	}
	upgradeSteps := func(from semversion.Number, targets []upgrades.Target, context upgrades.Context) error {
		return nil
	}

	return func(agentTag names.Tag, isCAAS bool) (*MachineAgent, error) {
		prometheusRegistry, err := addons.NewPrometheusRegistry()
		c.Assert(err, jc.ErrorIsNil)
		a := &MachineAgent{
			agentTag:          agentTag,
			AgentConfigWriter: agentConfWriter,
			configChangedVal:  voyeur.NewValue(true),
			workersStarted:    make(chan struct{}),
			dead:              make(chan struct{}),
			runner: worker.NewRunner(worker.RunnerParams{
				IsFatal:       agenterrors.IsFatal,
				MoreImportant: agenterrors.MoreImportant,
				RestartDelay:  jworker.RestartDelay,
			}),
			rootDir:                     rootDir,
			initialUpgradeCheckComplete: gate.NewLock(),
			loopDeviceManager:           &mockLoopDeviceManager{},
			newDBWorkerFunc:             newDBWorkerFunc,
			prometheusRegistry:          prometheusRegistry,
			mongoTxnCollector:           mongometrics.NewTxnCollector(),
			mongoDialCollector:          mongometrics.NewDialCollector(),
			preUpgradeSteps:             preUpgradeSteps,
			upgradeSteps:                upgradeSteps,
			isCaasAgent:                 isCAAS,
			cmdRunner:                   cmdRunner,
		}
		return a, nil
	}
}

// newAgent returns a new MachineAgent instance
func (s *commonMachineSuite) newAgent(c *gc.C, m *state.Machine) (*gomock.Controller, *MachineAgent) {
	ctrl := gomock.NewController(c)
	s.cmdRunner = mocks.NewMockCommandRunner(ctrl)

	agentConf := agentconf.NewAgentConf(s.DataDir)
	agentConf.ReadConfig(names.NewMachineTag(m.Id()).String())
	newDBWorkerFunc := func(context.Context, dbaccessor.DBApp, string, ...dbaccessor.TrackedDBWorkerOption) (dbaccessor.TrackedDB, error) {
		return testing.NewTrackedDB(s.TxnRunnerFactory()), nil
	}
	machineAgentFactory := NewTestMachineAgentFactory(c, agentConf, newDBWorkerFunc, c.MkDir(), s.cmdRunner)
	machineAgent, err := machineAgentFactory(m.Tag(), false)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl, machineAgent
}

func (s *commonMachineSuite) setFakeMachineAddresses(c *gc.C, machine *state.Machine, instanceId instance.Id) {
	controllerConfig := coretesting.FakeControllerConfig()

	addrs := network.NewSpaceAddresses("0.1.2.3")
	err := machine.SetProviderAddresses(controllerConfig, addrs...)
	c.Assert(err, jc.ErrorIsNil)
	// Set the addresses in the environ instance as well so that if the instance poller
	// runs it won't overwrite them.
	c.Assert(err, jc.ErrorIsNil)
	insts, err := s.Environ.Instances(context.Background(), []instance.Id{instanceId})
	c.Assert(err, jc.ErrorIsNil)
	dummy.SetInstanceAddresses(insts[0], network.NewMachineAddresses([]string{"0.1.2.3"}).AsProviderAddresses())
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

func newSignal() *signal {
	return &signal{ch: make(chan struct{})}
}

type signal struct {
	mu sync.Mutex
	ch chan struct{}
}

func (s *signal) triggered() <-chan struct{} {
	return s.ch
}

func (s *signal) assertTriggered(c *gc.C, thing string) {
	select {
	case <-s.triggered():
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for %s", thing)
	}
}

func (s *signal) trigger() {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.ch:
		// Already closed.
	default:
		close(s.ch)
	}
}

type runner interface {
	Run(*cmd.Context) error
	Stop() error
}

// runWithTimeout runs an agent and waits
// for it to complete within a reasonable time.
func runWithTimeout(c *gc.C, r runner) error {
	done := make(chan error)
	go func() {
		done <- r.Run(cmdtesting.Context(c))
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(coretesting.LongWait):
	}
	err := r.Stop()
	return fmt.Errorf("timed out waiting for agent to finish; stop error: %v", err)
}

type FakeConfig struct {
	agent.ConfigSetter
	values map[string]string
}

func (FakeConfig) LogDir() string {
	return filepath.FromSlash("/var/log/juju/")
}

func (FakeConfig) Tag() names.Tag {
	return names.NewMachineTag("42")
}

func (f FakeConfig) Value(key string) string {
	if f.values == nil {
		return ""
	}
	return f.values[key]
}

func (f FakeConfig) AgentLogfileMaxSizeMB() int {
	return 100
}

func (f FakeConfig) AgentLogfileMaxBackups() int {
	return 2
}

type FakeAgentConfig struct {
	agentconf.AgentConf
	values map[string]string
}

func (FakeAgentConfig) ReadConfig(string) error { return nil }

func (a FakeAgentConfig) CurrentConfig() agent.Config {
	return FakeConfig{values: a.values}
}

func (FakeAgentConfig) ChangeConfig(mutate agent.ConfigMutator) error {
	return mutate(FakeConfig{})
}

func (FakeAgentConfig) CheckArgs([]string) error { return nil }

// minModelWorkersEnviron implements just enough of environs.Environ
// to allow model workers to run.
type minModelWorkersEnviron struct {
	environs.Environ
	environs.LXDProfiler
}

func (e *minModelWorkersEnviron) Config() *config.Config {
	attrs := coretesting.FakeConfig()
	cfg, err := config.New(config.UseDefaults, attrs)
	if err != nil {
		panic(err)
	}
	return cfg
}

func (e *minModelWorkersEnviron) SetConfig(context.Context, *config.Config) error {
	return nil
}

func (e *minModelWorkersEnviron) AllRunningInstances(ctx context.Context) ([]instances.Instance, error) {
	return nil, nil
}

func (e *minModelWorkersEnviron) Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
	return nil, environs.ErrNoInstances
}

func (*minModelWorkersEnviron) MaybeWriteLXDProfile(pName string, put lxdprofile.Profile) error {
	return nil
}

func (*minModelWorkersEnviron) LXDProfileNames(containerName string) ([]string, error) {
	return nil, nil
}

func (*minModelWorkersEnviron) AssignLXDProfiles(instId string, profilesNames []string, profilePosts []lxdprofile.ProfilePost) (current []string, err error) {
	return profilesNames, nil
}
