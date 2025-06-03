// Copyright 2012-2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/agent/machiner"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/machinemanager"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/jujud-controller/agent/agenttest"
	"github.com/juju/juju/cmd/jujud-controller/agent/model"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	envstorage "github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/migrationmaster"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/exec"
	"github.com/juju/utils/v4/symlink"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/juju/worker/v4/workertest"
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

// FIXME: Delete all these tests and reimplement according to skip comments.

func (s *MachineLegacySuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Testing that the controller runs the cleaner worker by removing an application and watching it's unit disapear.
  This is a very silly test.
- Testing that the controller runs the instance poller by doing a great song and dance to add tools, deploy units of an
  application etc using the dummy provider. It then checks that the deployed machine's addresses are updated by said
  poller. This is also a very silly test.
`)
}

func (s *MachineLegacySuite) SetUpTest(c *tc.C) {
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

func (s *MachineLegacySuite) TestManageModelAuditsAPI(c *tc.C) {
	c.Skip("This test relies on pubsub to notify all workers that the apiserver details has changed. This needs to be an integration test, not a unit test.")

	password := "shhh..."
	user := names.NewUserTag("username")

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()

	err := controllerConfigService.UpdateControllerConfig(c.Context(), map[string]interface{}{
		"audit-log-exclude-methods": "Client.FullStatus",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	s.assertJob(c, state.JobManageModel, nil, func(conf agent.Config, _ *MachineAgent) {
		logPath := filepath.Join(conf.LogDir(), "audit.log")

		makeAPIRequest := func(doRequest func(*apiclient.Client)) {
			apiInfo, ok := conf.APIInfo()
			c.Assert(ok, tc.IsTrue)
			apiInfo.Tag = user
			apiInfo.Password = password
			st, err := api.Open(c.Context(), apiInfo, fastDialOpts)
			c.Assert(err, tc.ErrorIsNil)
			defer st.Close()
			doRequest(apiclient.NewClient(st, loggertesting.WrapCheckLog(c)))
		}
		makeMachineAPIRequest := func(doRequest func(*machinemanager.Client)) {
			apiInfo, ok := conf.APIInfo()
			c.Assert(ok, tc.IsTrue)
			apiInfo.Tag = user
			apiInfo.Password = password
			st, err := api.Open(c.Context(), apiInfo, fastDialOpts)
			c.Assert(err, tc.ErrorIsNil)
			defer st.Close()
			doRequest(machinemanager.NewClient(st))
		}

		// Make requests in separate API connections so they're separate conversations.
		makeAPIRequest(func(client *apiclient.Client) {
			_, err = client.Status(c.Context(), nil)
			c.Assert(err, tc.ErrorIsNil)
		})
		makeMachineAPIRequest(func(client *machinemanager.Client) {
			_, err = client.AddMachines(c.Context(), []params.AddMachineParams{{
				Jobs: []coremodel.MachineJob{"JobHostUnits"},
			}})
			c.Assert(err, tc.ErrorIsNil)
		})

		// Check that there's a call to Client.AddMachinesV2 in the
		// log, but no call to Client.FullStatus.
		records := readAuditLog(c, logPath)
		c.Assert(records, tc.HasLen, 3)
		c.Assert(records[1].Request, tc.NotNil)
		c.Assert(records[1].Request.Facade, tc.Equals, "MachineManager")
		c.Assert(records[1].Request.Method, tc.Equals, "AddMachines")

		// Now update the controller config to remove the exclusion.
		err := controllerConfigService.UpdateControllerConfig(c.Context(), map[string]interface{}{
			"audit-log-exclude-methods": "",
		}, nil)
		c.Assert(err, tc.ErrorIsNil)

		prevRecords := len(records)

		// We might need to wait until the controller config change is
		// propagated to the apiserver.
		for a := coretesting.LongAttempt.Start(); a.Next(); {
			makeAPIRequest(func(client *apiclient.Client) {
				_, err = client.Status(c.Context(), nil)
				c.Assert(err, tc.ErrorIsNil)
			})
			// Check to see whether there are more logged requests.
			records = readAuditLog(c, logPath)
			if prevRecords < len(records) {
				break
			}
		}
		// Now there should also be a call to Client.FullStatus (and a response).
		lastRequest := records[len(records)-2]
		c.Assert(lastRequest.Request, tc.NotNil)
		c.Assert(lastRequest.Request.Facade, tc.Equals, "Client")
		c.Assert(lastRequest.Request.Method, tc.Equals, "FullStatus")
	})
}

func (s *MachineLegacySuite) TestHostedModelWorkers(c *tc.C) {
	c.Skip("These rely on model databases, which aren't available in the agent tests. See addendum.")

	// The dummy provider blows up in the face of multi-model
	// scenarios so patch in a minimal environs.Environ that's good
	// enough to allow the model workers to run.
	s.PatchValue(&newEnvirons, func(context.Context, environs.OpenParams) (environs.Environ, error) {
		return &minModelWorkersEnviron{}, nil
	})

	tracker := agenttest.NewEngineTracker()
	instrumented := TrackModels(c, tracker, iaasModelManifolds)
	s.PatchValue(&iaasModelManifolds, instrumented)

	matcher := agenttest.NewWorkerMatcher(c, tracker, s.DefaultModelUUID.String(),
		append(alwaysModelWorkers, aliveModelWorkers...))
	s.assertJob(c, state.JobManageModel, nil, func(agent.Config, *MachineAgent) {
		agenttest.WaitMatch(c, matcher.Check, ReallyLongWait)
	})
}

func (s *MachineLegacySuite) TestWorkersForHostedModelWithInvalidCredential(c *tc.C) {
	c.Skip("This test relies on pubsub to notify all workers that the apiserver details has changed. This needs to be an integration test, not a unit test.")

	// The dummy provider blows up in the face of multi-model
	// scenarios so patch in a minimal environs.Environ that's good
	// enough to allow the model workers to run.
	loggo.GetLogger("juju.worker.dependency").SetLogLevel(loggo.TRACE)
	s.PatchValue(&newEnvirons, func(context.Context, environs.OpenParams) (environs.Environ, error) {
		return &minModelWorkersEnviron{}, nil
	})

	// invalidate cloud credential for this model
	domainServices := s.ControllerDomainServices(c)
	err := domainServices.Credential().InvalidateCredential(c.Context(), testing.DefaultCredentialId, "coz i can")
	c.Assert(err, tc.ErrorIsNil)

	tracker := agenttest.NewEngineTracker()
	instrumented := TrackModels(c, tracker, iaasModelManifolds)
	s.PatchValue(&iaasModelManifolds, instrumented)

	expectedWorkers := append(alwaysModelWorkers, aliveModelWorkers...)
	// Since this model's cloud credential is no longer valid,
	// only the workers that don't require a valid credential should remain.
	remainingWorkers := set.NewStrings(expectedWorkers...).Difference(
		set.NewStrings(requireValidCredentialModelWorkers...))

	matcher := agenttest.NewWorkerMatcher(c, tracker, s.DefaultModelUUID.String(), remainingWorkers.SortedValues())
	s.assertJob(c, state.JobManageModel, nil, func(agent.Config, *MachineAgent) {
		agenttest.WaitMatch(c, matcher.Check, ReallyLongWait)
	})
}

func (s *MachineLegacySuite) TestWorkersForHostedModelWithDeletedCredential(c *tc.C) {
	c.Skip("This test relies on pubsub to notify all workers that the apiserver details has changed. This needs to be an integration test, not a unit test.")

	// The dummy provider blows up in the face of multi-model
	// scenarios so patch in a minimal environs.Environ that's good
	// enough to allow the model workers to run.
	loggo.GetLogger("juju.worker.dependency").SetLogLevel(loggo.TRACE)
	s.PatchValue(&newEnvirons, func(context.Context, environs.OpenParams) (environs.Environ, error) {
		return &minModelWorkersEnviron{}, nil
	})

	ctx := c.Context()
	key := credential.Key{
		Cloud: "dummy",
		Owner: user.AdminUserName,
		Name:  "another",
	}
	domainServices := s.ControllerDomainServices(c)
	err := domainServices.Credential().UpdateCloudCredential(ctx, key, cloud.NewCredential(cloud.UserPassAuthType, nil))
	c.Assert(err, tc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	st := f.MakeModel(c, &factory.ModelParams{
		ConfigAttrs: coretesting.Attrs{
			"max-action-results-age":  "2h",
			"max-action-results-size": "4M",
			"logging-config":          "juju=debug;juju.worker.dependency=trace",
		},
		CloudCredential: names.NewCloudCredentialTag("dummy/admin/another"),
	})
	defer func() {
		err := st.Close()
		c.Check(err, tc.ErrorIsNil)
	}()

	// remove cloud credential used by this model but keep model reference to it
	err = domainServices.Credential().RemoveCloudCredential(ctx, key)
	c.Assert(err, tc.ErrorIsNil)

	tracker := agenttest.NewEngineTracker()
	instrumented := TrackModels(c, tracker, iaasModelManifolds)
	s.PatchValue(&iaasModelManifolds, instrumented)

	expectedWorkers := append(alwaysModelWorkers, aliveModelWorkers...)
	// Since this model's cloud credential is no longer valid,
	// only the workers that don't require a valid credential should remain.
	remainingWorkers := set.NewStrings(expectedWorkers...).Difference(
		set.NewStrings(requireValidCredentialModelWorkers...))
	matcher := agenttest.NewWorkerMatcher(c, tracker, s.DefaultModelUUID.String(), remainingWorkers.SortedValues())

	s.assertJob(c, state.JobManageModel, nil, func(agent.Config, *MachineAgent) {
		agenttest.WaitMatch(c, matcher.Check, ReallyLongWait)
	})
}

func (s *MachineLegacySuite) TestMigratingModelWorkers(c *tc.C) {
	c.Skip("These rely on model databases, which aren't available in the agent tests. See addendum.")

	st, closer := s.setupNewModel(c)
	defer closer()

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
	c.Assert(err, tc.ErrorIsNil)

	matcher := agenttest.NewWorkerMatcher(c, tracker, s.DefaultModelUUID.String(),
		append(alwaysModelWorkers, migratingModelWorkers...))
	s.assertJob(c, state.JobManageModel, nil, func(agent.Config, *MachineAgent) {
		agenttest.WaitMatch(c, matcher.Check, ReallyLongWait)
	})
}

func (s *MachineLegacySuite) TestDyingModelCleanedUp(c *tc.C) {
	c.Skip("This test relies on pubsub to notify all workers that the apiserver details has changed. This needs to be an integration test, not a unit test.")

	st, closer := s.setupNewModel(c)
	defer closer()

	timeout := time.After(ReallyLongWait)
	s.assertJob(c, state.JobManageModel, nil,
		func(agent.Config, *MachineAgent) {
			m, err := st.Model()
			c.Assert(err, tc.ErrorIsNil)
			watch := m.Watch()
			defer workertest.CleanKill(c, watch)

			err = m.Destroy(state.DestroyModelParams{})
			c.Assert(err, tc.ErrorIsNil)
			for {
				select {
				case <-watch.Changes():
					err := m.Refresh()
					if err == nil {
						continue // still there
					} else if errors.Is(err, errors.NotFound) {
						return // successfully removed
					}
					c.Assert(err, tc.ErrorIsNil) // guaranteed fail
				case <-timeout:
					c.Fatalf("timed out waiting for workers")
				}
			}
		})
}

func (s *MachineLegacySuite) TestMachineAgentSymlinks(c *tc.C) {
	stm, _, _ := s.primeAgent(c, state.JobManageModel)
	ctrl, a := s.newAgent(c, stm)
	defer ctrl.Finish()
	defer a.Stop()
	done := s.waitForOpenState(c, a)

	// Symlinks should have been created
	for _, link := range jujudSymlinks {
		_, err := os.Stat(utils.EnsureBaseDir(a.rootDir, link))
		c.Assert(err, tc.ErrorIsNil, tc.Commentf(link))
	}

	s.waitStopped(c, state.JobManageModel, a, done)
}

func (s *MachineLegacySuite) TestMachineAgentSymlinkJujuExecExists(c *tc.C) {
	stm, _, _ := s.primeAgent(c, state.JobManageModel)
	ctrl, a := s.newAgent(c, stm)
	defer ctrl.Finish()
	defer a.Stop()

	// Pre-create the symlinks, but pointing to the incorrect location.
	a.rootDir = c.MkDir()
	for _, link := range jujudSymlinks {
		fullLink := utils.EnsureBaseDir(a.rootDir, link)
		c.Assert(os.MkdirAll(filepath.Dir(fullLink), os.FileMode(0755)), tc.ErrorIsNil)
		c.Assert(symlink.New("/nowhere/special", fullLink), tc.ErrorIsNil, tc.Commentf(link))
	}

	// Start the agent and wait for it be running.
	done := s.waitForOpenState(c, a)

	// juju-exec symlink should have been recreated.
	for _, link := range jujudSymlinks {
		fullLink := utils.EnsureBaseDir(a.rootDir, link)
		linkTarget, err := symlink.Read(fullLink)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(linkTarget, tc.Not(tc.Equals), "/nowhere/special", tc.Commentf(link))
	}

	s.waitStopped(c, state.JobManageModel, a, done)
}

func (s *MachineLegacySuite) TestManageModelServesAPI(c *tc.C) {
	c.Skip("This test relies on pubsub to notify all workers that the apiserver details has changed. This needs to be an integration test, not a unit test.")

	s.assertJob(c, state.JobManageModel, nil, func(conf agent.Config, a *MachineAgent) {
		apiInfo, ok := conf.APIInfo()
		c.Assert(ok, tc.IsTrue)
		st, err := api.Open(c.Context(), apiInfo, fastDialOpts)
		c.Assert(err, tc.ErrorIsNil)
		defer st.Close()
		m, err := apimachiner.NewClient(st).Machine(c.Context(), conf.Tag().(names.MachineTag))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(m.Life(), tc.Equals, life.Alive)
	})
}

func (s *MachineLegacySuite) TestIAASControllerPatchUpdateManagerFile(c *tc.C) {
	c.Skip("This test relies on pubsub to notify all workers that the apiserver details has changed. This needs to be an integration test, not a unit test.")

	s.assertJob(c, state.JobManageModel,
		func() {
			s.cmdRunner.EXPECT().RunCommands(exec.RunParams{
				Commands: "[ ! -f /etc/update-manager/release-upgrades ] || sed -i '/Prompt=/ s/=.*/=never/' /etc/update-manager/release-upgrades",
			}).Return(&exec.ExecResponse{Code: 0}, nil).AnyTimes()
		},
		func(conf agent.Config, a *MachineAgent) {
			apiInfo, ok := conf.APIInfo()
			c.Assert(ok, tc.IsTrue)
			st, err := api.Open(c.Context(), apiInfo, fastDialOpts)
			c.Assert(err, tc.ErrorIsNil)
			defer func() { _ = st.Close() }()
			err = a.machineStartup(c.Context(), st, loggertesting.WrapCheckLog(c))
			c.Assert(err, tc.ErrorIsNil)
		},
	)
}

func (s *MachineLegacySuite) TestIAASControllerPatchUpdateManagerFileErrored(c *tc.C) {
	c.Skip("This test relies on pubsub to notify all workers that the apiserver details has changed. This needs to be an integration test, not a unit test.")

	s.assertJob(c, state.JobManageModel,
		func() {
			s.cmdRunner.EXPECT().RunCommands(exec.RunParams{
				Commands: "[ ! -f /etc/update-manager/release-upgrades ] || sed -i '/Prompt=/ s/=.*/=never/' /etc/update-manager/release-upgrades",
			}).Return(nil, errors.New("unknown error")).MinTimes(1)
		},
		func(conf agent.Config, a *MachineAgent) {
			apiInfo, ok := conf.APIInfo()
			c.Assert(ok, tc.IsTrue)
			st, err := api.Open(c.Context(), apiInfo, fastDialOpts)
			c.Assert(err, tc.ErrorIsNil)
			defer func() { _ = st.Close() }()
			err = a.machineStartup(c.Context(), st, loggertesting.WrapCheckLog(c))
			c.Assert(err, tc.ErrorMatches, `unknown error`)
		},
	)
}

func (s *MachineLegacySuite) TestIAASControllerPatchUpdateManagerFileNonZeroExitCode(c *tc.C) {
	c.Skip("This test relies on pubsub to notify all workers that the apiserver details has changed. This needs to be an integration test, not a unit test.")

	s.assertJob(c, state.JobManageModel,
		func() {
			s.cmdRunner.EXPECT().RunCommands(exec.RunParams{
				Commands: "[ ! -f /etc/update-manager/release-upgrades ] || sed -i '/Prompt=/ s/=.*/=never/' /etc/update-manager/release-upgrades",
			}).Return(&exec.ExecResponse{Code: 1, Stderr: []byte(`unknown error`)}, nil).MinTimes(1)
		},
		func(conf agent.Config, a *MachineAgent) {
			apiInfo, ok := conf.APIInfo()
			c.Assert(ok, tc.IsTrue)
			st, err := api.Open(c.Context(), apiInfo, fastDialOpts)
			c.Assert(err, tc.ErrorIsNil)
			defer func() { _ = st.Close() }()
			err = a.machineStartup(c.Context(), st, loggertesting.WrapCheckLog(c))
			c.Assert(err, tc.ErrorMatches, `cannot patch /etc/update-manager/release-upgrades: unknown error`)
		},
	)
}

func (s *MachineLegacySuite) TestControllerModelWorkers(c *tc.C) {
	c.Skip("These rely on model databases, which aren't available in the agent tests. See addendum.")

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

func (s *MachineLegacySuite) TestModelWorkersRespectSingularResponsibilityFlag(c *tc.C) {
	c.Skip("This test relies on pubsub to notify all workers that the apiserver details has changed. This needs to be an integration test, not a unit test.")
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

func (s *MachineLegacySuite) assertJob(
	c *tc.C,
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
	c *tc.C, job state.MachineJob,
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

	logger.Debugf(context.TODO(), "new agent %#v", a)

	// All state jobs currently also run an APIWorker, so no
	// need to check for that here, like in assertJob.
	done := s.waitForOpenState(c, a)

	if postCheck != nil {
		postCheck(conf, a)
	}
	s.waitStopped(c, job, a, done)
}

func (s *MachineLegacySuite) waitForOpenState(c *tc.C, a *MachineAgent) chan error {
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
		c.Assert(agentAPI, tc.NotNil)
		return done
	case <-time.After(coretesting.LongWait):
		c.Fatalf("API not opened")
	}
	c.Fatal("fail if called")
	return nil
}

func (s *MachineLegacySuite) setupNewModel(c *tc.C) (newSt *state.State, closer func()) {
	// Create a new environment, tests can now watch if workers start for it.
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	newSt = f.MakeModel(c, &factory.ModelParams{
		UUID: s.DefaultModelUUID,
		ConfigAttrs: coretesting.Attrs{
			"max-action-results-age":  "2h",
			"max-action-results-size": "4M",
		},
	})
	return newSt, func() {
		err := newSt.Close()
		c.Check(err, tc.ErrorIsNil)
	}
}

func (s *MachineLegacySuite) waitStopped(c *tc.C, job state.MachineJob, a *MachineAgent, done chan error) {
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
		c.Assert(err, tc.ErrorIsNil)
	}

	select {
	case err := <-done:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for agent to terminate")
	}
}

func (s *MachineLegacySuite) claimSingularLease(c *tc.C) {
	modelUUID := s.ControllerModelUUID()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		q := `
INSERT INTO lease (uuid, lease_type_id, model_uuid, name, holder, start, expiry)
VALUES (?, 0, ?, ?, 'machine-999-lxd-99', datetime('now'), datetime('now', '+100 seconds'))`[1:]
		_, err := tx.ExecContext(ctx, q, uuid.MustNewUUID().String(), modelUUID, modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}
