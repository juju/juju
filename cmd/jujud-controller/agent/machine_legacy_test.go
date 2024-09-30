// Copyright 2012-2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/exec"
	"github.com/juju/utils/v4/symlink"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/api/base"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/machinemanager"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/jujud-controller/agent/agenttest"
	"github.com/juju/juju/cmd/jujud-controller/agent/model"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/filestorage"
	envstorage "github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/provider/dummy"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/charmrevision"
	"github.com/juju/juju/internal/worker/instancepoller"
	"github.com/juju/juju/internal/worker/migrationmaster"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// MachineLegacySuite is an integration test suite that requires access to
// state sync point. The sync point has be added to allow these tests to pass.
// Going forward we do not want to implement that sync point for dqlite. This
// means that these tests need to be refactor to either actual unit tests or
// bash integration tests. Once the state package is gone, these will no longer
// function to work.
//
// Do not edit them to make the sync point work better. They're legacy and
// should be treated as such, until we cut them over.
//
// Addendum: These tests also only work on the controller database. Now that
// models are being integrated, we're having to setup those during these tests.
// We're recreating the dbaccessor in the tests. We're in a sunk cost fallacy
// situation. Skipping over the tests, as we're going to be removing
// them in favour of tests that will perform the same tests on a REAL
// bootstrapped controller and model.

type MachineLegacySuite struct {
	// The duplication of the MachineSuite is important. We don't want to break
	// the MachineSuite based on the following legacy tests.
	// Do not be tempted in swapping this for MachineSuite.
	commonMachineSuite

	agentStorage envstorage.Storage
}

var _ = gc.Suite(&MachineLegacySuite{})

func (s *MachineLegacySuite) SetUpTest(c *gc.C) {
	c.Skip(`
These tests require the model database. We haven't plumbed that in yet.
I've added to the risk register to ensure that we come back around and
write correct integration tests for these.

For now, we're going to skip these tests.
`)

	s.ControllerConfigAttrs = map[string]interface{}{
		controller.AuditingEnabled: true,
		// We need to clear the JujuDBSnapChannel config value as the agent
		// hasn't been correctly primed with the right value. Changing the
		// value in the tests also breaks other suites that expect it to be
		// empty.
		// This test suite is truly horrid!
		controller.JujuDBSnapChannel: "",
		controller.ObjectStoreType:   objectstore.FileBackend,
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
	ctx := envtesting.BootstrapContext(context.Background(), c)
	err = s.Environ.PrepareForBootstrap(ctx, "controller")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineLegacySuite) TestManageModelAuditsAPI(c *gc.C) {
	password := "shhh..."
	user := names.NewUserTag("username")

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()

	err := controllerConfigService.UpdateControllerConfig(context.Background(), map[string]interface{}{
		"audit-log-exclude-methods": "Client.FullStatus",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.assertJob(c, state.JobManageModel, nil, func(conf agent.Config, _ *MachineAgent) {
		logPath := filepath.Join(conf.LogDir(), "audit.log")

		makeAPIRequest := func(doRequest func(*apiclient.Client)) {
			apiInfo, ok := conf.APIInfo()
			c.Assert(ok, jc.IsTrue)
			apiInfo.Tag = user
			apiInfo.Password = password
			st, err := api.Open(context.Background(), apiInfo, fastDialOpts)
			c.Assert(err, jc.ErrorIsNil)
			defer st.Close()
			doRequest(apiclient.NewClient(st, loggertesting.WrapCheckLog(c)))
		}
		makeMachineAPIRequest := func(doRequest func(*machinemanager.Client)) {
			apiInfo, ok := conf.APIInfo()
			c.Assert(ok, jc.IsTrue)
			apiInfo.Tag = user
			apiInfo.Password = password
			st, err := api.Open(context.Background(), apiInfo, fastDialOpts)
			c.Assert(err, jc.ErrorIsNil)
			defer st.Close()
			doRequest(machinemanager.NewClient(st))
		}

		// Make requests in separate API connections so they're separate conversations.
		makeAPIRequest(func(client *apiclient.Client) {
			_, err = client.Status(context.Background(), nil)
			c.Assert(err, jc.ErrorIsNil)
		})
		makeMachineAPIRequest(func(client *machinemanager.Client) {
			_, err = client.AddMachines(context.Background(), []params.AddMachineParams{{
				Jobs: []coremodel.MachineJob{"JobHostUnits"},
			}})
			c.Assert(err, jc.ErrorIsNil)
		})

		// Check that there's a call to Client.AddMachinesV2 in the
		// log, but no call to Client.FullStatus.
		records := readAuditLog(c, logPath)
		c.Assert(records, gc.HasLen, 3)
		c.Assert(records[1].Request, gc.NotNil)
		c.Assert(records[1].Request.Facade, gc.Equals, "MachineManager")
		c.Assert(records[1].Request.Method, gc.Equals, "AddMachines")

		// Now update the controller config to remove the exclusion.
		err := controllerConfigService.UpdateControllerConfig(context.Background(), map[string]interface{}{
			"audit-log-exclude-methods": "",
		}, nil)
		c.Assert(err, jc.ErrorIsNil)

		prevRecords := len(records)

		// We might need to wait until the controller config change is
		// propagated to the apiserver.
		for a := coretesting.LongAttempt.Start(); a.Next(); {
			makeAPIRequest(func(client *apiclient.Client) {
				_, err = client.Status(context.Background(), nil)
				c.Assert(err, jc.ErrorIsNil)
			})
			// Check to see whether there are more logged requests.
			records = readAuditLog(c, logPath)
			if prevRecords < len(records) {
				break
			}
		}
		// Now there should also be a call to Client.FullStatus (and a response).
		lastRequest := records[len(records)-2]
		c.Assert(lastRequest.Request, gc.NotNil)
		c.Assert(lastRequest.Request.Facade, gc.Equals, "Client")
		c.Assert(lastRequest.Request.Method, gc.Equals, "FullStatus")
	})
}

func (s *MachineLegacySuite) TestHostedModelWorkers(c *gc.C) {
	c.Skip("These rely on model databases, which aren't available in the agent tests. See addendum.")

	s.PatchValue(&charmrevision.NewAPIFacade, func(base.APICaller) (charmrevision.Facade, error) {
		return noopRevisionUpdater{}, nil
	})

	// The dummy provider blows up in the face of multi-model
	// scenarios so patch in a minimal environs.Environ that's good
	// enough to allow the model workers to run.
	s.PatchValue(&newEnvirons, func(context.Context, environs.OpenParams) (environs.Environ, error) {
		return &minModelWorkersEnviron{}, nil
	})

	st, closer := s.setupNewModel(c)
	defer closer()

	uuid := st.ModelUUID()

	tracker := agenttest.NewEngineTracker()
	instrumented := TrackModels(c, tracker, iaasModelManifolds)
	s.PatchValue(&iaasModelManifolds, instrumented)

	matcher := agenttest.NewWorkerMatcher(c, tracker, uuid,
		append(alwaysModelWorkers, aliveModelWorkers...))
	s.assertJob(c, state.JobManageModel, nil, func(agent.Config, *MachineAgent) {
		agenttest.WaitMatch(c, matcher.Check, ReallyLongWait)
	})
}

func (s *MachineLegacySuite) TestWorkersForHostedModelWithInvalidCredential(c *gc.C) {
	// The dummy provider blows up in the face of multi-model
	// scenarios so patch in a minimal environs.Environ that's good
	// enough to allow the model workers to run.
	loggo.GetLogger("juju.worker.dependency").SetLogLevel(loggo.TRACE)
	s.PatchValue(&newEnvirons, func(context.Context, environs.OpenParams) (environs.Environ, error) {
		return &minModelWorkersEnviron{}, nil
	})

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	st := f.MakeModel(c, &factory.ModelParams{
		ConfigAttrs: coretesting.Attrs{
			"max-status-history-age":  "2h",
			"max-status-history-size": "4M",
			"max-action-results-age":  "2h",
			"max-action-results-size": "4M",
		},
		CloudCredential: testing.DefaultCredentialTag,
	})
	defer func() {
		err := st.Close()
		c.Check(err, jc.ErrorIsNil)
	}()

	uuid := st.ModelUUID()

	// invalidate cloud credential for this model
	domainServices := s.ControllerDomainServices(c)
	err := domainServices.Credential().InvalidateCredential(context.Background(), testing.DefaultCredentialId, "coz i can")
	c.Assert(err, jc.ErrorIsNil)

	tracker := agenttest.NewEngineTracker()
	instrumented := TrackModels(c, tracker, iaasModelManifolds)
	s.PatchValue(&iaasModelManifolds, instrumented)

	expectedWorkers := append(alwaysModelWorkers, aliveModelWorkers...)
	// Since this model's cloud credential is no longer valid,
	// only the workers that don't require a valid credential should remain.
	remainingWorkers := set.NewStrings(expectedWorkers...).Difference(
		set.NewStrings(requireValidCredentialModelWorkers...))

	matcher := agenttest.NewWorkerMatcher(c, tracker, uuid, remainingWorkers.SortedValues())
	s.assertJob(c, state.JobManageModel, nil, func(agent.Config, *MachineAgent) {
		agenttest.WaitMatch(c, matcher.Check, ReallyLongWait)
	})
}

func (s *MachineLegacySuite) TestWorkersForHostedModelWithDeletedCredential(c *gc.C) {
	// The dummy provider blows up in the face of multi-model
	// scenarios so patch in a minimal environs.Environ that's good
	// enough to allow the model workers to run.
	loggo.GetLogger("juju.worker.dependency").SetLogLevel(loggo.TRACE)
	s.PatchValue(&newEnvirons, func(context.Context, environs.OpenParams) (environs.Environ, error) {
		return &minModelWorkersEnviron{}, nil
	})

	ctx := context.Background()
	key := credential.Key{
		Cloud: "dummy",
		Owner: user.AdminUserName,
		Name:  "another",
	}
	domainServices := s.ControllerDomainServices(c)
	err := domainServices.Credential().UpdateCloudCredential(ctx, key, cloud.NewCredential(cloud.UserPassAuthType, nil))
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	st := f.MakeModel(c, &factory.ModelParams{
		ConfigAttrs: coretesting.Attrs{
			"max-status-history-age":  "2h",
			"max-status-history-size": "4M",
			"max-action-results-age":  "2h",
			"max-action-results-size": "4M",
			"logging-config":          "juju=debug;juju.worker.dependency=trace",
		},
		CloudCredential: names.NewCloudCredentialTag("dummy/admin/another"),
	})
	defer func() {
		err := st.Close()
		c.Check(err, jc.ErrorIsNil)
	}()

	uuid := st.ModelUUID()

	// remove cloud credential used by this model but keep model reference to it
	err = domainServices.Credential().RemoveCloudCredential(ctx, key)
	c.Assert(err, jc.ErrorIsNil)

	tracker := agenttest.NewEngineTracker()
	instrumented := TrackModels(c, tracker, iaasModelManifolds)
	s.PatchValue(&iaasModelManifolds, instrumented)

	expectedWorkers := append(alwaysModelWorkers, aliveModelWorkers...)
	// Since this model's cloud credential is no longer valid,
	// only the workers that don't require a valid credential should remain.
	remainingWorkers := set.NewStrings(expectedWorkers...).Difference(
		set.NewStrings(requireValidCredentialModelWorkers...))
	matcher := agenttest.NewWorkerMatcher(c, tracker, uuid, remainingWorkers.SortedValues())

	s.assertJob(c, state.JobManageModel, nil, func(agent.Config, *MachineAgent) {
		agenttest.WaitMatch(c, matcher.Check, ReallyLongWait)
	})
}

func (s *MachineLegacySuite) TestMigratingModelWorkers(c *gc.C) {
	c.Skip("These rely on model databases, which aren't available in the agent tests. See addendum.")

	st, closer := s.setupNewModel(c)
	defer closer()
	modelUUID := st.ModelUUID()

	tracker := agenttest.NewEngineTracker()

	// Replace the real migrationmaster worker with a fake one which
	// does nothing. This is required to make this test be reliable as
	// the environment required for the migrationmaster to operate
	// correctly is too involved to set up from here.
	//
	// TODO(mjs) - an alternative might be to provide a fake Facade
	// and api.Open to the real migrationmaster but this test is
	// awfully far away from the low level details of the worker.
	origModelManifolds := iaasModelManifolds
	modelManifoldsDisablingMigrationMaster := func(config model.ManifoldsConfig) dependency.Manifolds {
		config.NewMigrationMaster = func(config migrationmaster.Config) (worker.Worker, error) {
			return &nullWorker{dead: make(chan struct{})}, nil
		}
		return origModelManifolds(config)
	}
	instrumented := TrackModels(c, tracker, modelManifoldsDisablingMigrationMaster)
	s.PatchValue(&iaasModelManifolds, instrumented)

	targetControllerTag := names.NewControllerTag(uuid.MustNewUUID().String())
	_, err := st.CreateMigration(state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag: targetControllerTag,
			Addrs:         []string{"1.2.3.4:5555"},
			CACert:        "cert",
			AuthTag:       names.NewUserTag("user"),
			Password:      "password",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	matcher := agenttest.NewWorkerMatcher(c, tracker, modelUUID,
		append(alwaysModelWorkers, migratingModelWorkers...))
	s.assertJob(c, state.JobManageModel, nil, func(agent.Config, *MachineAgent) {
		agenttest.WaitMatch(c, matcher.Check, ReallyLongWait)
	})
}

func (s *MachineLegacySuite) TestDyingModelCleanedUp(c *gc.C) {
	st, closer := s.setupNewModel(c)
	defer closer()

	timeout := time.After(ReallyLongWait)
	s.assertJob(c, state.JobManageModel, nil,
		func(agent.Config, *MachineAgent) {
			m, err := st.Model()
			c.Assert(err, jc.ErrorIsNil)
			watch := m.Watch()
			defer workertest.CleanKill(c, watch)

			err = m.Destroy(state.DestroyModelParams{})
			c.Assert(err, jc.ErrorIsNil)
			for {
				select {
				case <-watch.Changes():
					err := m.Refresh()
					if err == nil {
						continue // still there
					} else if errors.Is(err, errors.NotFound) {
						return // successfully removed
					}
					c.Assert(err, jc.ErrorIsNil) // guaranteed fail
				case <-timeout:
					c.Fatalf("timed out waiting for workers")
				}
			}
		})
}

func (s *MachineLegacySuite) TestMachineAgentSymlinks(c *gc.C) {
	stm, _, _ := s.primeAgent(c, state.JobManageModel)
	ctrl, a := s.newAgent(c, stm)
	defer ctrl.Finish()
	defer a.Stop()
	done := s.waitForOpenState(c, a)

	// Symlinks should have been created
	for _, link := range jujudSymlinks {
		_, err := os.Stat(utils.EnsureBaseDir(a.rootDir, link))
		c.Assert(err, jc.ErrorIsNil, gc.Commentf(link))
	}

	s.waitStopped(c, state.JobManageModel, a, done)
}

func (s *MachineLegacySuite) TestMachineAgentSymlinkJujuExecExists(c *gc.C) {
	stm, _, _ := s.primeAgent(c, state.JobManageModel)
	ctrl, a := s.newAgent(c, stm)
	defer ctrl.Finish()
	defer a.Stop()

	// Pre-create the symlinks, but pointing to the incorrect location.
	a.rootDir = c.MkDir()
	for _, link := range jujudSymlinks {
		fullLink := utils.EnsureBaseDir(a.rootDir, link)
		c.Assert(os.MkdirAll(filepath.Dir(fullLink), os.FileMode(0755)), jc.ErrorIsNil)
		c.Assert(symlink.New("/nowhere/special", fullLink), jc.ErrorIsNil, gc.Commentf(link))
	}

	// Start the agent and wait for it be running.
	done := s.waitForOpenState(c, a)

	// juju-exec symlink should have been recreated.
	for _, link := range jujudSymlinks {
		fullLink := utils.EnsureBaseDir(a.rootDir, link)
		linkTarget, err := symlink.Read(fullLink)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(linkTarget, gc.Not(gc.Equals), "/nowhere/special", gc.Commentf(link))
	}

	s.waitStopped(c, state.JobManageModel, a, done)
}

func (s *MachineLegacySuite) TestManageModelServesAPI(c *gc.C) {
	s.assertJob(c, state.JobManageModel, nil, func(conf agent.Config, a *MachineAgent) {
		apiInfo, ok := conf.APIInfo()
		c.Assert(ok, jc.IsTrue)
		st, err := api.Open(context.Background(), apiInfo, fastDialOpts)
		c.Assert(err, jc.ErrorIsNil)
		defer st.Close()
		m, err := apimachiner.NewClient(st).Machine(context.Background(), conf.Tag().(names.MachineTag))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Life(), gc.Equals, life.Alive)
	})
}

func (s *MachineLegacySuite) TestIAASControllerPatchUpdateManagerFile(c *gc.C) {
	s.assertJob(c, state.JobManageModel,
		func() {
			s.cmdRunner.EXPECT().RunCommands(exec.RunParams{
				Commands: "[ ! -f /etc/update-manager/release-upgrades ] || sed -i '/Prompt=/ s/=.*/=never/' /etc/update-manager/release-upgrades",
			}).Return(&exec.ExecResponse{Code: 0}, nil).AnyTimes()
		},
		func(conf agent.Config, a *MachineAgent) {
			apiInfo, ok := conf.APIInfo()
			c.Assert(ok, jc.IsTrue)
			st, err := api.Open(context.Background(), apiInfo, fastDialOpts)
			c.Assert(err, jc.ErrorIsNil)
			defer func() { _ = st.Close() }()
			err = a.machineStartup(context.Background(), st, loggertesting.WrapCheckLog(c))
			c.Assert(err, jc.ErrorIsNil)
		},
	)
}

func (s *MachineLegacySuite) TestIAASControllerPatchUpdateManagerFileErrored(c *gc.C) {
	s.assertJob(c, state.JobManageModel,
		func() {
			s.cmdRunner.EXPECT().RunCommands(exec.RunParams{
				Commands: "[ ! -f /etc/update-manager/release-upgrades ] || sed -i '/Prompt=/ s/=.*/=never/' /etc/update-manager/release-upgrades",
			}).Return(nil, errors.New("unknown error")).MinTimes(1)
		},
		func(conf agent.Config, a *MachineAgent) {
			apiInfo, ok := conf.APIInfo()
			c.Assert(ok, jc.IsTrue)
			st, err := api.Open(context.Background(), apiInfo, fastDialOpts)
			c.Assert(err, jc.ErrorIsNil)
			defer func() { _ = st.Close() }()
			err = a.machineStartup(context.Background(), st, loggertesting.WrapCheckLog(c))
			c.Assert(err, gc.ErrorMatches, `unknown error`)
		},
	)
}

func (s *MachineLegacySuite) TestIAASControllerPatchUpdateManagerFileNonZeroExitCode(c *gc.C) {
	s.assertJob(c, state.JobManageModel,
		func() {
			s.cmdRunner.EXPECT().RunCommands(exec.RunParams{
				Commands: "[ ! -f /etc/update-manager/release-upgrades ] || sed -i '/Prompt=/ s/=.*/=never/' /etc/update-manager/release-upgrades",
			}).Return(&exec.ExecResponse{Code: 1, Stderr: []byte(`unknown error`)}, nil).MinTimes(1)
		},
		func(conf agent.Config, a *MachineAgent) {
			apiInfo, ok := conf.APIInfo()
			c.Assert(ok, jc.IsTrue)
			st, err := api.Open(context.Background(), apiInfo, fastDialOpts)
			c.Assert(err, jc.ErrorIsNil)
			defer func() { _ = st.Close() }()
			err = a.machineStartup(context.Background(), st, loggertesting.WrapCheckLog(c))
			c.Assert(err, gc.ErrorMatches, `cannot patch /etc/update-manager/release-upgrades: unknown error`)
		},
	)
}

func (s *MachineLegacySuite) TestManageModelRunsCleaner(c *gc.C) {
	c.Skip("These rely on model databases, which aren't available in the agent tests. See addendum.")

	s.assertJob(c, state.JobManageModel, nil, func(conf agent.Config, a *MachineAgent) {
		// Create an application and unit, and destroy the app.
		f, release := s.NewFactory(c, s.ControllerModelUUID())
		defer release()
		app := f.MakeApplication(c, &factory.ApplicationParams{
			Name:  "wordpress",
			Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
		})
		unit, err := app.AddUnit(s.modelConfigService(c), state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = app.Destroy(testing.NewObjectStore(c, s.ControllerModelUUID()))
		c.Assert(err, jc.ErrorIsNil)

		// Check the unit was not yet removed.
		err = unit.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		w := unit.Watch()
		defer worker.Stop(w)

		// Wait for the unit to be removed.
		timeout := time.After(coretesting.LongWait)
		for done := false; !done; {
			select {
			case <-timeout:
				c.Fatalf("unit not cleaned up")
			case <-w.Changes():
				err := unit.Refresh()
				if errors.Is(err, errors.NotFound) {
					done = true
				} else {
					c.Assert(err, jc.ErrorIsNil)
				}
			}
		}
	})
}

func (s *MachineLegacySuite) TestJobManageModelRunsMinUnitsWorker(c *gc.C) {
	c.Skip("These rely on model databases, which aren't available in the agent tests. See addendum.")

	s.assertJob(c, state.JobManageModel, nil, func(_ agent.Config, _ *MachineAgent) {
		// Ensure that the MinUnits worker is alive by doing a simple check
		// that it responds to state changes: add an application, set its minimum
		// number of units to one, wait for the worker to add the missing unit.
		f, release := s.NewFactory(c, s.ControllerModelUUID())
		defer release()
		app := f.MakeApplication(c, &factory.ApplicationParams{
			Name:  "wordpress",
			Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
		})
		err := app.SetMinUnits(1)
		c.Assert(err, jc.ErrorIsNil)
		w := app.Watch()
		defer worker.Stop(w)

		// Wait for the unit to be created.
		timeout := time.After(longerWait)
		for {
			select {
			case <-timeout:
				c.Fatalf("unit not created")
			case <-w.Changes():
				units, err := app.AllUnits()
				c.Assert(err, jc.ErrorIsNil)
				if len(units) == 1 {
					return
				}
			}
		}
	})
}

func (s *MachineLegacySuite) TestControllerModelWorkers(c *gc.C) {
	c.Skip("These rely on model databases, which aren't available in the agent tests. See addendum.")

	s.PatchValue(&charmrevision.NewAPIFacade, func(base.APICaller) (charmrevision.Facade, error) {
		return noopRevisionUpdater{}, nil
	})

	uuid := s.ControllerModelUUID()

	tracker := agenttest.NewEngineTracker()
	instrumented := TrackModels(c, tracker, iaasModelManifolds)
	s.PatchValue(&iaasModelManifolds, instrumented)

	expectedWorkers := append(alwaysModelWorkers, aliveModelWorkers...)

	matcher := agenttest.NewWorkerMatcher(c, tracker, uuid, expectedWorkers)
	s.assertJob(c, state.JobManageModel, nil,
		func(agent.Config, *MachineAgent) {
			agenttest.WaitMatch(c, matcher.Check, longerWait)
		},
	)
}

func (s *MachineLegacySuite) TestModelWorkersRespectSingularResponsibilityFlag(c *gc.C) {
	// Grab responsibility for the model on behalf of another machine.
	s.claimSingularLease(c)

	// Then run a normal model-tracking test, just checking for
	// a different set of workers.
	tracker := agenttest.NewEngineTracker()
	instrumented := TrackModels(c, tracker, iaasModelManifolds)
	s.PatchValue(&iaasModelManifolds, instrumented)

	matcher := agenttest.NewWorkerMatcher(c, tracker, s.ControllerModelUUID(), alwaysModelWorkers)
	s.assertJob(c, state.JobManageModel, nil, func(agent.Config, *MachineAgent) {
		agenttest.WaitMatch(c, matcher.Check, longerWait)
	})
}

func (s *MachineLegacySuite) TestManageModelRunsInstancePoller(c *gc.C) {
	c.Skip("These rely on model databases, which aren't available in the agent tests. See addendum.")

	jujutesting.PatchExecutableAsEchoArgs(c, s, "ovs-vsctl", 0)
	s.AgentSuite.PatchValue(&instancepoller.ShortPoll, 500*time.Millisecond)
	s.AgentSuite.PatchValue(&instancepoller.ShortPollCap, 500*time.Millisecond)

	stream := s.Environ.Config().AgentStream()
	usefulVersion := version.Binary{
		Number:  jujuversion.Current,
		Arch:    arch.HostArch(),
		Release: "ubuntu",
	}
	envtesting.AssertUploadFakeToolsVersions(c, s.agentStorage, stream, stream, usefulVersion)

	m, _, _ := s.primeAgent(c, state.JobManageModel)
	ctrl, a := s.newAgent(c, m)
	defer ctrl.Finish()

	s.cmdRunner.EXPECT().RunCommands(exec.RunParams{
		Commands: "[ ! -f /etc/update-manager/release-upgrades ] || sed -i '/Prompt=/ s/=.*/=never/' /etc/update-manager/release-upgrades",
	}).AnyTimes().Return(&exec.ExecResponse{Code: 0}, nil)

	defer func() { _ = a.Stop() }()
	go func() {
		c.Check(a.Run(cmdtesting.Context(c)), jc.ErrorIsNil)
	}()

	// Wait for the workers to start. This ensures that the central
	// hub referred to in startAddressPublisher has been assigned,
	// and we will not fail race tests with concurrent access.
	select {
	case <-a.WorkersStarted():
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for agent workers to start")
	}

	startAddressPublisher(s, c, a)

	// Add one unit to an application;
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	arch := arch.HostArch()
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "test-application",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "dummy"}),
		CharmOrigin: &state.CharmOrigin{
			Source: "charm-hub",
			Platform: &state.Platform{
				Architecture: arch,
				OS:           "ubuntu",
				Channel:      "22.04",
			}},
		Constraints: constraints.MustParse("arch=" + arch),
	})
	unit, err := app.AddUnit(s.modelConfigService(c), state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.ControllerModel(c).State().AssignUnit(s.modelConfigService(c), unit, state.AssignNew)
	c.Assert(err, jc.ErrorIsNil)

	m, instId := s.waitProvisioned(c, unit)
	insts, err := s.Environ.Instances(envcontext.WithoutCredentialInvalidator(context.Background()), []instance.Id{instId})
	c.Assert(err, jc.ErrorIsNil)

	dummy.SetInstanceStatus(insts[0], "running")

	strategy := &utils.AttemptStrategy{
		Total: 60 * time.Second,
		Delay: coretesting.ShortWait,
	}
	for attempt := strategy.Start(); attempt.Next(); {
		if !attempt.HasNext() {
			c.Logf("final machine addresses: %#v", m.Addresses())
			c.Fatalf("timed out waiting for machine to get address")
		}
		err := m.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		instStatus, err := m.InstanceStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Logf("found status is %q %q", instStatus.Status, instStatus.Message)

		// The dummy provider always returns 3 devices with one address each.
		// We don't care what they are, just that the instance-poller retrieved
		// them and set them against the machine in state.
		if len(m.Addresses()) == 3 && instStatus.Message == "running" {
			break
		}
		c.Logf("waiting for machine %q address to be updated", m.Id())
	}
}

func (s *MachineLegacySuite) waitProvisioned(c *gc.C, unit *state.Unit) (*state.Machine, instance.Id) {
	c.Logf("waiting for unit %q to be provisioned", unit)
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.ControllerModel(c).State().Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	w := m.Watch()
	defer worker.Stop(w)
	timeout := time.After(longerWait)
	for {
		select {
		case <-timeout:
			c.Fatalf("timed out waiting for provisioning")
		case _, ok := <-w.Changes():
			c.Assert(ok, jc.IsTrue)
			err := m.Refresh()
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

func (s *MachineLegacySuite) assertJob(
	c *gc.C,
	job state.MachineJob,
	preCheck func(),
	postCheck func(agent.Config, *MachineAgent),
) {
	paramsJob := job.ToParams()
	if !paramsJob.NeedsState() {
		c.Fatalf("%v does not use state", paramsJob)
	}
	s.assertAgentOpensState(c, job, preCheck, postCheck)
}

// assertAgentOpensState asserts that a machine agent started with the
// given job. The agent's configuration and the agent's state.State are
// then passed to the test function for further checking.
func (s *MachineLegacySuite) assertAgentOpensState(
	c *gc.C, job state.MachineJob,
	preCheck func(),
	postCheck func(agent.Config, *MachineAgent),
) {
	stm, conf, _ := s.primeAgent(c, job)
	ctrl, a := s.newAgent(c, stm)
	defer ctrl.Finish()
	defer a.Stop()

	if preCheck != nil {
		preCheck()
	} else if job == state.JobManageModel {
		s.cmdRunner.EXPECT().RunCommands(exec.RunParams{
			Commands: "[ ! -f /etc/update-manager/release-upgrades ] || sed -i '/Prompt=/ s/=.*/=never/' /etc/update-manager/release-upgrades",
		}).AnyTimes().Return(&exec.ExecResponse{Code: 0}, nil)
	}

	logger.Debugf("new agent %#v", a)

	// All state jobs currently also run an APIWorker, so no
	// need to check for that here, like in assertJob.
	done := s.waitForOpenState(c, a)
	startAddressPublisher(s, c, a)

	if postCheck != nil {
		postCheck(conf, a)
	}
	s.waitStopped(c, job, a, done)
}

func (s *MachineLegacySuite) waitForOpenState(c *gc.C, a *MachineAgent) chan error {
	agentAPIs := make(chan struct{}, 1)
	s.AgentSuite.PatchValue(&reportOpenedState, func(st *state.State) {
		select {
		case agentAPIs <- struct{}{}:
		default:
		}
	})

	done := make(chan error)
	go func() {
		done <- a.Run(cmdtesting.Context(c))
	}()

	select {
	case agentAPI := <-agentAPIs:
		c.Assert(agentAPI, gc.NotNil)
		return done
	case <-time.After(coretesting.LongWait):
		c.Fatalf("API not opened")
	}
	c.Fatal("fail if called")
	return nil
}

func (s *MachineLegacySuite) setupNewModel(c *gc.C) (newSt *state.State, closer func()) {
	// Create a new environment, tests can now watch if workers start for it.
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	newSt = f.MakeModel(c, &factory.ModelParams{
		ConfigAttrs: coretesting.Attrs{
			"max-status-history-age":  "2h",
			"max-status-history-size": "4M",
			"max-action-results-age":  "2h",
			"max-action-results-size": "4M",
		},
	})
	return newSt, func() {
		err := newSt.Close()
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *MachineLegacySuite) waitStopped(c *gc.C, job state.MachineJob, a *MachineAgent, done chan error) {
	err := a.Stop()
	if job == state.JobManageModel {
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
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for agent to terminate")
	}
}

func (s *MachineLegacySuite) claimSingularLease(c *gc.C) {
	modelUUID := s.ControllerModelUUID()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		q := `
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
VALUES (?, 0, ?, ?, 'machine-999-lxd-99', datetime('now'), datetime('now', '+100 seconds'))`[1:]
		_, err := tx.ExecContext(ctx, q, uuid.MustNewUUID().String(), modelUUID, modelUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}
