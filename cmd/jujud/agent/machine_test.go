// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/lumberjack/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/cmd/jujud/agent/mocks"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/filestorage"
	envstorage "github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/dbaccessor"
	databasetesting "github.com/juju/juju/internal/worker/dbaccessor/testing"
)

type MachineSuite struct {
	commonMachineSuite

	agentStorage envstorage.Storage
}

func TestMachineSuite(t *testing.T) {
	tc.Run(t, &MachineSuite{})
}

// DefaultVersions returns a slice of unique 'versions' for the current
// environment's host architecture. Additionally, it ensures that 'versions'
// for amd64 are returned if that is not the current host's architecture.
func defaultVersions(agentVersion semversion.Number) []semversion.Binary {
	osTypes := set.NewStrings("ubuntu")
	osTypes.Add(coreos.HostOSTypeName())
	var versions []semversion.Binary
	for _, osType := range osTypes.Values() {
		versions = append(versions, semversion.Binary{
			Number:  agentVersion,
			Arch:    arch.HostArch(),
			Release: osType,
		})
		if arch.HostArch() != "amd64" {
			versions = append(versions, semversion.Binary{
				Number:  agentVersion,
				Arch:    "amd64",
				Release: osType,
			})

		}
	}
	return versions
}

func (s *MachineSuite) SetUpTest(c *tc.C) {
	s.ControllerConfigAttrs = map[string]interface{}{
		controller.AuditingEnabled: true,
	}
	s.ControllerModelConfigAttrs = map[string]interface{}{
		"agent-version": coretesting.CurrentVersion().Number.String(),
	}
	s.WithLeaseManager = true
	s.commonMachineSuite.SetUpTest(c)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, tc.ErrorIsNil)
	// Upload tools to both release and devel streams since config will dictate that we
	// end up looking in both places.
	versions := defaultVersions(coretesting.CurrentVersion().Number)
	envtesting.AssertUploadFakeToolsVersions(c, stor, "released", versions...)
	envtesting.AssertUploadFakeToolsVersions(c, stor, "devel", versions...)
	s.agentStorage = stor

	// Restart failed workers much faster for the tests.
	s.PatchValue(&engine.EngineErrorDelay, 100*time.Millisecond)

	// Ensure the dummy provider is initialised - no need to actually bootstrap.
	ctx := envtesting.BootstrapContext(c.Context(), c)
	err = s.Environ.PrepareForBootstrap(ctx, "controller")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MachineSuite) TestParseNonsense(c *tc.C) {
	aCfg := agentconf.NewAgentConf(s.DataDir)
	err := ParseAgentCommand(&machineAgentCommand{agentInitializer: aCfg}, nil)
	c.Assert(err, tc.ErrorMatches, "either machine-id or controller-id must be set")
	err = ParseAgentCommand(&machineAgentCommand{agentInitializer: aCfg}, []string{"--machine-id", "-4004"})
	c.Assert(err, tc.ErrorMatches, "--machine-id option must be a non-negative integer")
	err = ParseAgentCommand(&machineAgentCommand{agentInitializer: aCfg}, []string{"--controller-id", "-4004"})
	c.Assert(err, tc.ErrorMatches, "--controller-id option must be a non-negative integer")
}

func (s *MachineSuite) TestParseUnknown(c *tc.C) {
	aCfg := agentconf.NewAgentConf(s.DataDir)
	a := &machineAgentCommand{agentInitializer: aCfg}
	err := ParseAgentCommand(a, []string{"--machine-id", "42", "blistering barnacles"})
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["blistering barnacles"\]`)
}

func (s *MachineSuite) TestParseSuccess(c *tc.C) {
	ctrl := gomock.NewController(c)
	s.cmdRunner = mocks.NewMockCommandRunner(ctrl)

	create := func() (cmd.Command, agentconf.AgentConf) {
		aCfg := agentconf.NewAgentConf(s.DataDir)
		s.PrimeAgent(c, names.NewMachineTag("42"), initialMachinePassword)
		logger := s.newBufferedLogWriter()
		newDBWorkerFunc := func(context.Context, dbaccessor.DBApp, string, ...dbaccessor.TrackedDBWorkerOption) (dbaccessor.TrackedDB, error) {
			return databasetesting.NewTrackedDB(s.TxnRunnerFactory()), nil
		}
		a := NewMachineAgentCommand(
			nil,
			NewTestMachineAgentFactory(c, aCfg, logger, newDBWorkerFunc, c.MkDir(), s.cmdRunner),
			aCfg,
			aCfg,
		)
		return a, aCfg
	}
	a := CheckAgentCommand(c, s.DataDir, create, []string{"--machine-id", "42", "--log-to-stderr", "--data-dir", s.DataDir})
	c.Assert(a.(*machineAgentCommand).machineId, tc.Equals, "42")
}

func (s *MachineSuite) TestUseLumberjack(c *tc.C) {
	ctx := cmdtesting.Context(c)
	agentConf := FakeAgentConfig{}
	logger := s.newBufferedLogWriter()

	ctrl := gomock.NewController(c)
	s.cmdRunner = mocks.NewMockCommandRunner(ctrl)

	newDBWorkerFunc := func(context.Context, dbaccessor.DBApp, string, ...dbaccessor.TrackedDBWorkerOption) (dbaccessor.TrackedDB, error) {
		return databasetesting.NewTrackedDB(s.TxnRunnerFactory()), nil
	}
	a := NewMachineAgentCommand(
		ctx,
		NewTestMachineAgentFactory(c, &agentConf, logger, newDBWorkerFunc, c.MkDir(), s.cmdRunner),
		agentConf,
		agentConf,
	)
	// little hack to set the data that Init expects to already be set
	a.(*machineAgentCommand).machineId = "42"

	err := a.Init(nil)
	c.Assert(err, tc.IsNil)

	l, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, tc.IsTrue)
	c.Check(l.MaxAge, tc.Equals, 0)
	c.Check(l.MaxBackups, tc.Equals, 2)
	c.Check(l.Filename, tc.Equals, filepath.FromSlash("/var/log/juju/machine-42.log"))
	c.Check(l.MaxSize, tc.Equals, 100)
}

func (s *MachineSuite) TestDontUseLumberjack(c *tc.C) {
	ctx := cmdtesting.Context(c)
	agentConf := FakeAgentConfig{}
	logger := s.newBufferedLogWriter()

	ctrl := gomock.NewController(c)
	s.cmdRunner = mocks.NewMockCommandRunner(ctrl)

	newDBWorkerFunc := func(context.Context, dbaccessor.DBApp, string, ...dbaccessor.TrackedDBWorkerOption) (dbaccessor.TrackedDB, error) {
		return databasetesting.NewTrackedDB(s.TxnRunnerFactory()), nil
	}
	a := NewMachineAgentCommand(
		ctx,
		NewTestMachineAgentFactory(c, &agentConf, logger, newDBWorkerFunc, c.MkDir(), s.cmdRunner),
		agentConf,
		agentConf,
	)
	// little hack to set the data that Init expects to already be set
	a.(*machineAgentCommand).machineId = "42"

	// set the value that normally gets set by the flag parsing
	a.(*machineAgentCommand).logToStdErr = true

	err := a.Init(nil)
	c.Assert(err, tc.IsNil)

	_, ok := ctx.Stderr.(*lumberjack.Logger)
	c.Assert(ok, tc.IsFalse)
}

func (s *MachineSuite) TestRunStop(c *tc.C) {
	m, _, _ := s.primeAgent(c)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	done := make(chan error)
	go func() {
		done <- a.Run(cmdtesting.Context(c))
	}()
	err := a.Stop()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(<-done, tc.ErrorIsNil)
}

func (s *MachineSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
	
 - Test agent tools version set when upgrading a controller
 - Test agent tools version set when upgrading a model
 - Test upgrade request upgrading a model
 - Test the machine agent includes the api address updater worker
 - Test the machine agent includes the disk manager worker
 - Test the machine agent includes the machine storage worker
 - Test the machine agent is running correct workers when not migrating
 - Test upgrade is not triggered if not required
 - Test config ignore-machine-addresses is not ignored for machines and containers
`)
}
