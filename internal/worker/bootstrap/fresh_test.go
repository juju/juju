// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/controller"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/flags"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accessservice "github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/domain/controllernode"
	"github.com/juju/juju/domain/deployment/charm"
	macaroonerrors "github.com/juju/juju/domain/macaroon/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
)

type freshBootstrapSuite struct {
	baseSuite

	adminUserID     user.UUID
	controllerModel coremodel.Model

	states                 chan string
	removeBootstrapSSHKeys func([]string) error
}

func TestFreshBootstrapSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &freshBootstrapSuite{})
	})
}

func (s *freshBootstrapSuite) SetUpTest(c *tc.C) {
	s.removeBootstrapSSHKeys = func([]string) error { return nil }
	s.adminUserID = usertesting.GenUserUUID(c)
	s.controllerModel = coremodel.Model{
		UUID:      tc.Must0(c, coremodel.NewUUID),
		ModelType: coremodel.IAAS,
	}
}

func (s *freshBootstrapSuite) TestDeleteBootstrapSSHKeysEmpty(c *tc.C) {
	c.Assert(DeleteBootstrapSSHKeys(nil), tc.ErrorIsNil)
}

func (s *freshBootstrapSuite) TestKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.ensureBootstrapParams(c)

	s.expectGateUnlock()
	s.expectUser(c)
	s.expectAuthorisedKeys()
	s.expectControllerConfig()
	s.expectBootstrapFlagSet()
	s.removeBootstrapSSHKeys = func(keys []string) error {
		c.Check(keys, tc.DeepEquals, []string{"bootstrap-ssh-key"})
		return nil
	}
	s.expectReloadSpaces()
	s.expectSeedDefaultStoragePools()
	s.expectInitialiseBakeryConfig(nil)
	s.expectSetAPIHostPorts()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)
	s.ensureFinished(c)

	workertest.CleanKill(c, w)
}

func (s *freshBootstrapSuite) TestDeleteBootstrapSSHKeysError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.ensureBootstrapParams(c)
	s.domainServices.EXPECT().Flag().AnyTimes()
	s.expectUser(c)
	s.expectAuthorisedKeys()
	s.expectControllerConfig()
	s.expectReloadSpaces()
	s.expectSeedDefaultStoragePools()
	s.expectInitialiseBakeryConfig(nil)
	s.expectSetAPIHostPorts()
	s.removeBootstrapSSHKeys = func([]string) error {
		return errors.New("cannot remove bootstrap keys")
	}

	w := s.newWorker(c)
	err := workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, `removing bootstrap SSH keys: cannot remove bootstrap keys`)
}

func (s *freshBootstrapSuite) TestReloadSpacesBeforeControllerCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.ensureBootstrapParams(c)

	s.expectGateUnlock()
	s.expectUser(c)
	s.expectAuthorisedKeys()
	s.expectControllerConfig()
	s.expectBootstrapFlagSet()
	s.expectSetAPIHostPorts()
	s.expectSeedDefaultStoragePools()
	controllerCharmDeployerFunc := s.expectReloadSpacesWithFunc(c)
	s.expectInitialiseBakeryConfig(nil)

	w := s.newWorkerWithFunc(c, controllerCharmDeployerFunc)
	defer workertest.DirtyKill(c, w)

	workertest.CleanKill(c, w)
}

func (s *freshBootstrapSuite) TestSeedAgentBinary(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Agent binary seeding does not need the model object store.
	var called bool
	w := &freshBootstrap{
		cfg: FreshBootstrapConfig{
			AgentBinaryUploader: func(context.Context, string, AgentBinaryStore, logger.Logger) (func(), error) {
				called = true
				return func() {}, nil
			},
			ControllerCharmDeployer: func(context.Context, ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
				return nil, nil
			},
			PopulateControllerCharm: func(context.Context, bootstrap.ControllerCharmDeployer) error {
				return nil
			},
			Logger: s.logger,
		},
	}
	cleanup, err := w.seedAgentBinary(c.Context(), c.MkDir())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
	c.Check(cleanup, tc.NotNil)
}

// TestSeedAuthorisedNilKeys is asserting that if we add a nil slice of
// authorised keys to the controller model that it is safe. This test is here
// assert that we don't break. Specifically because this functionality is being
// added after the fact and may not always be set.
func (s *freshBootstrapSuite) TestSeedAuthorisedNilKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "admin")).Return(
		user.User{
			UUID: s.adminUserID,
		},
		nil,
	)

	s.keyManagerService.EXPECT().AddPublicKeysForUser(gomock.Any(), s.adminUserID).Return(nil)

	w := &freshBootstrap{
		cfg: FreshBootstrapConfig{
			UserService:       s.userService,
			KeyManagerService: s.keyManagerService,
		},
	}

	err := w.seedInitialAuthorisedKeys(c.Context(), nil)
	c.Check(err, tc.ErrorIsNil)
}

func (s *freshBootstrapSuite) TestSeedBakeryConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()
	w := &freshBootstrap{
		cfg: FreshBootstrapConfig{
			BakeryConfigService: s.bakeryConfigService,
		},
	}

	s.expectInitialiseBakeryConfig(nil)
	err := w.seedMacaroonConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	s.expectInitialiseBakeryConfig(macaroonerrors.BakeryConfigAlreadyInitialised)
	err = w.seedMacaroonConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	s.expectInitialiseBakeryConfig(errors.Errorf("boom"))
	err = w.seedMacaroonConfig(c.Context())
	c.Assert(err, tc.Not(tc.ErrorIsNil))
}

func (s *freshBootstrapSuite) TestSeedStoragePools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectSeedDefaultStoragePools()
	s.storageService.EXPECT().CreateStoragePool(
		gomock.Any(),
		"loop-pool",
		domainstorage.ProviderType("loop"),
		map[string]any{"foo": "bar"},
	)

	w := &freshBootstrap{
		cfg: FreshBootstrapConfig{
			ModelInfoService: s.modelInfoService,
			StorageService:   s.storageService,
			Logger:           s.logger,
		},
	}
	err := w.seedStoragePools(c.Context(), map[string]storage.Attrs{
		"loop-pool": {
			"name": "loop-pool",
			"type": "loop",
			"foo":  "bar",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *freshBootstrapSuite) TestSetControllerApplicationPassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	applicationUUID := coreapplication.UUID("controller-application-uuid")
	s.applicationService.EXPECT().GetApplicationUUIDByName(gomock.Any(), "controller").Return(applicationUUID, nil)
	s.agentPasswordService.EXPECT().SetApplicationPassword(gomock.Any(), applicationUUID, "application-password")

	w := &freshBootstrap{cfg: FreshBootstrapConfig{
		ApplicationService:   s.applicationService,
		AgentPasswordService: s.agentPasswordService,
		ApplicationPassword:  "application-password",
	}}
	c.Assert(w.setControllerApplicationPassword(c.Context()), tc.ErrorIsNil)
}

func (s *freshBootstrapSuite) newWorker(c *tc.C) worker.Worker {
	return s.newWorkerWithFunc(c,
		func(context.Context, ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
			return nil, nil
		})
}

func (s *freshBootstrapSuite) newWorkerWithFunc(c *tc.C, controllerCharmDeployerFunc ControllerCharmDeployerFunc) worker.Worker {
	operation, err := NewFreshBootstrap(FreshBootstrapConfig{
		RemoveBootstrapSSHKeys:     s.removeBootstrapSSHKeys,
		DataDir:                    s.dataDir,
		APIPort:                    42,
		CharmhubHTTPClient:         s.httpClient,
		ControllerAgentBinaryStore: s.controllerAgentBinaryStore,
		UserService:                s.userService,
		AgentPasswordService:       s.agentPasswordService,
		ApplicationService:         s.applicationService,
		ControllerNodeService:      s.controllerNodeService,
		ModelConfigService:         s.modelConfigService,
		ModelInfoService:           s.modelInfoService,
		MachineService:             s.machineService,
		ControllerModel:            s.controllerModel,
		KeyManagerService:          s.keyManagerService,
		ControllerConfigService:    s.controllerConfigService,
		StorageService:             s.storageService,
		NetworkService:             s.networkService,
		BakeryConfigService:        s.bakeryConfigService,
		PopulateControllerCharm: func(context.Context, bootstrap.ControllerCharmDeployer) error {
			return nil
		},
		AgentBinaryUploader: func(context.Context, string, AgentBinaryStore, logger.Logger) (func(), error) {
			return func() {}, nil
		},
		ControllerCharmDeployer: controllerCharmDeployerFunc,
		AgentPassword:           "password",
		BootstrapAddressFinder: func(context.Context, instance.Id) (network.ProviderAddresses, error) {
			return nil, nil
		},
		AgentFinalizer: func(ctx context.Context, aps AgentPasswordService, ms MachineService, sip instancecfg.StateInitializationParams, password string) error {
			return nil
		},
		Logger: s.logger,
		Clock:  clock.WallClock,
	})
	c.Assert(err, tc.ErrorIsNil)
	w, err := newWorker(WorkerConfig{
		Operation:           operation,
		FlagService:         s.flagService,
		BootstrapUnlocker:   s.bootstrapUnlocker,
		ControllerModelUUID: s.controllerModel.UUID,
		StatusHistory:       s.statusHistory,
		Logger:              s.logger,
		Clock:               clock.WallClock,
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *freshBootstrapSuite) setupMocks(c *tc.C) *gomock.Controller {
	// Buffer both state transitions. The worker can report "started" and
	// "completed" before the test drains the first event.
	s.states = make(chan string, 2)

	ctrl := s.baseSuite.setupMocks(c)

	return ctrl
}

func (s *freshBootstrapSuite) ensureStartup(c *tc.C) {
	s.ensureState(c, stateStarted)
}

func (s *freshBootstrapSuite) ensureFinished(c *tc.C) {
	s.ensureState(c, stateCompleted)
}

func (s *freshBootstrapSuite) ensureState(c *tc.C, st string) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, st)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for %s", st)
	}
}

func (s *freshBootstrapSuite) expectControllerConfig() {
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).
		Return(controller.Config{
			controller.ControllerUUIDKey:   "test-uuid",
			controller.JujuManagementSpace: "mgmt-space",
		}, nil).Times(3)
}

func (s *freshBootstrapSuite) expectUser(c *tc.C) {
	s.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "admin")).Return(user.User{
		UUID: s.adminUserID,
	}, nil).Times(2)
	s.userService.EXPECT().AddUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u accessservice.AddUserArg) (user.UUID, []byte, error) {
		c.Check(u.Name, tc.Equals, usertesting.GenNewName(c, "juju-metrics"))
		return usertesting.GenUserUUID(c), nil, nil
	})
	s.userService.EXPECT().AddExternalUser(gomock.Any(), usertesting.GenNewName(c, "everyone@external"), "", gomock.Any())
}

func (s *freshBootstrapSuite) expectAuthorisedKeys() {
	s.keyManagerService.EXPECT().AddPublicKeysForUser(gomock.Any(), s.adminUserID, []string{}).Return(nil)
}

func (s *freshBootstrapSuite) expectReloadSpaces() {
	s.networkService.EXPECT().ReloadSpaces(gomock.Any())
}

func (s *freshBootstrapSuite) expectReloadSpacesWithFunc(c *tc.C) ControllerCharmDeployerFunc {
	seedControllerCharm := false
	h := func(context.Context, ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
		seedControllerCharm = true
		return nil, nil
	}

	s.networkService.EXPECT().ReloadSpaces(gomock.Any()).DoAndReturn(
		func(ctx context.Context) error {
			c.Check(seedControllerCharm, tc.IsFalse, tc.Commentf("seedControllerCharm called before ReloadSpaces, kubernetes bootstrap will fail"))
			return nil
		},
	)
	return h
}

func (s *freshBootstrapSuite) expectInitialiseBakeryConfig(err error) {
	s.bakeryConfigService.EXPECT().InitialiseBakeryConfig(gomock.Any()).Return(err)
}

func (s *freshBootstrapSuite) expectBootstrapFlagSet() {
	s.flagService.EXPECT().SetFlag(gomock.Any(), flags.BootstrapFlag, true, flags.BootstrapFlagDescription).Return(nil)
}

func (s *freshBootstrapSuite) expectSeedDefaultStoragePools() {
	s.modelInfoService.EXPECT().SeedDefaultStoragePools(gomock.Any())
}

func (s *freshBootstrapSuite) expectSetAPIHostPorts() {
	spaceName := network.SpaceName("mgmt-space")
	args := controllernode.SetAPIAddressArgs{
		APIPort: 42,
		Addresses: map[string]controllernode.APIAddressSet{
			"0": {},
		},
	}
	s.networkService.EXPECT().GetAllSpaces(gomock.Any()).Return(nil, nil)
	s.networkService.EXPECT().SpaceByName(gomock.Any(), spaceName).Return(nil, networkerrors.SpaceNotFound)
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), args)
}

func (s *freshBootstrapSuite) ensureBootstrapParams(c *tc.C) {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig())
	c.Assert(err, tc.ErrorIsNil)

	args := instancecfg.StateInitializationParams{
		BootstrapSSHAuthorizedKeys:  []string{"bootstrap-ssh-key"},
		ControllerModelConfig:       cfg,
		BootstrapMachineConstraints: constraints.MustParse("mem=1G"),
		BootstrapMachineInstanceId:  instance.Id("i-deadbeef"),
		ControllerCharmPath:         "obscura",
		ControllerCharmChannel:      charm.MakePermissiveChannel("", "stable", ""),
	}
	bytes, err := args.Marshal()
	c.Assert(err, tc.ErrorIsNil)

	err = os.WriteFile(filepath.Join(s.dataDir, cloudconfig.FileNameBootstrapParams), bytes, 0644)
	c.Assert(err, tc.ErrorIsNil)
}
