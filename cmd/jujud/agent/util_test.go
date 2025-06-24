// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/clock"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/addons"
	agentconfig "github.com/juju/juju/agent/config"
	agenterrors "github.com/juju/juju/agent/errors"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/mocks"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/upgrades"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/authenticationworker"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/internal/worker/diskmanager"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/internal/worker/machiner"
	"github.com/juju/juju/state"
)

type commonMachineSuite struct {
	AgentSuite
	// FakeJujuXDGDataHomeSuite is needed only because the
	// authenticationworker writes to ~/.ssh.
	coretesting.FakeJujuXDGDataHomeSuite

	cmdRunner *mocks.MockCommandRunner
}

func (s *commonMachineSuite) SetUpSuite(c *tc.C) {
	s.AgentSuite.SetUpSuite(c)
	// Set up FakeJujuXDGDataHomeSuite after AgentSuite since
	// AgentSuite clears all env vars.
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)

	// Stub out executables etc used by workers.
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	s.PatchValue(&authenticationworker.SSHUser, "")
	s.PatchValue(&diskmanager.DefaultListBlockDevices, func(context.Context) ([]blockdevice.BlockDevice, error) {
		return nil, nil
	})
	s.PatchValue(&machiner.GetObservedNetworkConfig, func(_ network.ConfigSource) (network.InterfaceInfos, error) {
		return nil, nil
	})
}

func (s *commonMachineSuite) SetUpTest(c *tc.C) {
	s.AgentSuite.SetUpTest(c)
	// Set up FakeJujuXDGDataHomeSuite after AgentSuite since
	// AgentSuite clears all env vars.
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	testpath := c.MkDir()
	s.PatchEnvPathPrepend(testpath)
	// mock out the start method so we can fake install services without sudo
	fakeCmd(filepath.Join(testpath, "start"))
	fakeCmd(filepath.Join(testpath, "stop"))
}

func fakeCmd(path string) {
	err := os.WriteFile(path, []byte("#!/bin/bash --norc\nexit 0"), 0755)
	if err != nil {
		panic(err)
	}
}

func (s *commonMachineSuite) TearDownTest(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)
	}
	s.AgentSuite.TearDownTest(c)
}

func (s *commonMachineSuite) TearDownSuite(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
	s.AgentSuite.TearDownSuite(c)
}

func NewTestMachineAgentFactory(
	c *tc.C,
	agentConfWriter agentconfig.AgentConfigWriter,
	bufferedLogger *logsender.BufferedLogWriter,
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
		runner, err := worker.NewRunner(worker.RunnerParams{
			Name:          "machine-agent",
			IsFatal:       agenterrors.IsFatal,
			MoreImportant: agenterrors.MoreImportant,
			RestartDelay:  jworker.RestartDelay,
		})
		c.Assert(err, tc.ErrorIsNil)

		prometheusRegistry, err := addons.NewPrometheusRegistry()
		c.Assert(err, tc.ErrorIsNil)
		a := &MachineAgent{
			agentTag:                    agentTag,
			AgentConfigWriter:           agentConfWriter,
			configChangedVal:            voyeur.NewValue(true),
			bufferedLogger:              bufferedLogger,
			workersStarted:              make(chan struct{}),
			dead:                        make(chan struct{}),
			runner:                      runner,
			rootDir:                     rootDir,
			initialUpgradeCheckComplete: gate.NewLock(),
			loopDeviceManager:           &mockLoopDeviceManager{},
			prometheusRegistry:          prometheusRegistry,
			preUpgradeSteps:             preUpgradeSteps,
			upgradeSteps:                upgradeSteps,
			isCaasAgent:                 isCAAS,
			cmdRunner:                   cmdRunner,
		}
		return a, nil
	}
}

func (s *commonMachineSuite) newBufferedLogWriter() *logsender.BufferedLogWriter {
	logger := logsender.NewBufferedLogWriter(1024)
	s.AddCleanup(func(*tc.C) { logger.Close() })
	return logger
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
