// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	stdcontext "context"
	"time"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/cmd/internal/agent/agentconf"
	"github.com/juju/juju/cmd/jujud-controller/agent/agenttest"
	"github.com/juju/juju/cmd/jujud-controller/agent/mocks"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/environs/filestorage"
	envstorage "github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/container/kvm"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/worker/dbaccessor"
	databasetesting "github.com/juju/juju/internal/worker/dbaccessor/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type MachineSuite struct {
	commonMachineSuite

	agentStorage envstorage.Storage
}

var _ = gc.Suite(&MachineSuite{})

// noopRevisionUpdater creates a stub to prevent outbound requests to the
// charmhub store and the charmstore. As these are meant to be unit tests, we
// should strive to remove outbound calls to external services.
type noopRevisionUpdater struct{}

func (noopRevisionUpdater) UpdateLatestRevisions() error {
	return nil
}

// DefaultVersions returns a slice of unique 'versions' for the current
// environment's host architecture. Additionally, it ensures that 'versions'
// for amd64 are returned if that is not the current host's architecture.
func defaultVersions(agentVersion version.Number) []version.Binary {
	osTypes := set.NewStrings("ubuntu")
	osTypes.Add(coreos.HostOSTypeName())
	var versions []version.Binary
	for _, osType := range osTypes.Values() {
		versions = append(versions, version.Binary{
			Number:  agentVersion,
			Arch:    arch.HostArch(),
			Release: osType,
		})
		if arch.HostArch() != "amd64" {
			versions = append(versions, version.Binary{
				Number:  agentVersion,
				Arch:    "amd64",
				Release: osType,
			})

		}
	}
	return versions
}

func (s *MachineSuite) SetUpTest(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	// Upload tools to both release and devel streams since config will dictate that we
	// end up looking in both places.
	versions := defaultVersions(coretesting.CurrentVersion().Number)
	envtesting.AssertUploadFakeToolsVersions(c, stor, "released", "released", versions...)
	envtesting.AssertUploadFakeToolsVersions(c, stor, "devel", "devel", versions...)
	s.agentStorage = stor

	// Restart failed workers much faster for the tests.
	s.PatchValue(&engine.EngineErrorDelay, 100*time.Millisecond)

	// Most of these tests normally finish sub-second on a fast machine.
	// If any given test hits a minute, we have almost certainly become
	// wedged, so dump the logs.
	coretesting.DumpTestLogsAfter(time.Minute, c, s)

	// Ensure the dummy provider is initialised - no need to actually bootstrap.
	ctx := envtesting.BootstrapContext(stdcontext.Background(), c)
	err = s.Environ.PrepareForBootstrap(ctx, "controller")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineSuite) TestParseNonsense(c *gc.C) {
	aCfg := agentconf.NewAgentConf(s.DataDir)
	err := ParseAgentCommand(&machineAgentCommand{agentInitializer: aCfg}, nil)
	c.Assert(err, gc.ErrorMatches, "either machine-id or controller-id must be set")
	err = ParseAgentCommand(&machineAgentCommand{agentInitializer: aCfg}, []string{"--machine-id", "-4004"})
	c.Assert(err, gc.ErrorMatches, "--machine-id option must be a non-negative integer")
	err = ParseAgentCommand(&machineAgentCommand{agentInitializer: aCfg}, []string{"--controller-id", "-4004"})
	c.Assert(err, gc.ErrorMatches, "--controller-id option must be a non-negative integer")
}

func (s *MachineSuite) TestParseUnknown(c *gc.C) {
	aCfg := agentconf.NewAgentConf(s.DataDir)
	a := &machineAgentCommand{agentInitializer: aCfg}
	err := ParseAgentCommand(a, []string{"--machine-id", "42", "blistering barnacles"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["blistering barnacles"\]`)
}

func (s *MachineSuite) TestParseSuccess(c *gc.C) {
	ctrl := gomock.NewController(c)
	s.cmdRunner = mocks.NewMockCommandRunner(ctrl)

	create := func() (cmd.Command, agentconf.AgentConf) {
		aCfg := agentconf.NewAgentConf(s.DataDir)
		s.PrimeAgent(c, names.NewMachineTag("42"), initialMachinePassword)
		logger := s.newBufferedLogWriter()
		newDBWorkerFunc := func(stdcontext.Context, dbaccessor.DBApp, string, ...dbaccessor.TrackedDBWorkerOption) (dbaccessor.TrackedDB, error) {
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
	c.Assert(a.(*machineAgentCommand).machineId, gc.Equals, "42")
}

func (s *MachineSuite) TestRunStop(c *gc.C) {
	m, _, _ := s.primeAgent(c, state.JobHostUnits)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	err := a.Stop()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(<-done, jc.ErrorIsNil)
}

func (s *MachineSuite) TestAgentSetsToolsVersionManageModel(c *gc.C) {
	s.assertAgentSetsToolsVersion(c, state.JobManageModel)
}

func (s *MachineSuite) TestAgentSetsToolsVersionHostUnits(c *gc.C) {
	s.assertAgentSetsToolsVersion(c, state.JobHostUnits)
}

func (s *MachineSuite) TestMachineWorkers(c *gc.C) {
	// TODO(wallyworld) - we need the dqlite model database to be available.
	c.Skip("we need to seed the dqlite database with machine data")
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
