// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	agent "github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/flags"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	machine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accessservice "github.com/juju/juju/domain/access/service"
	macaroonerrors "github.com/juju/juju/domain/macaroon/errors"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/testing"
)

type workerSuite struct {
	baseSuite

	adminUserID     user.UUID
	controllerModel coremodel.Model

	states chan string
}

func TestWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) SetUpTest(c *tc.C) {
	s.adminUserID = usertesting.GenUserUUID(c)
	s.controllerModel = coremodel.Model{
		UUID: modeltesting.GenModelUUID(c),
	}
}

func (s *workerSuite) TestKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.ensureBootstrapParams(c)

	s.expectGateUnlock()
	s.expectUser(c)
	s.expectAuthorizedKeys()
	s.expectControllerConfig()
	s.expectAgentConfig()
	s.expectObjectStoreGetter(2)
	s.expectBootstrapFlagSet()
	s.expectSetMachineCloudInstance()
	s.expectSetAPIHostPorts()
	s.expectStateServingInfo()
	s.expectReloadSpaces()
	s.expectInitialiseBakeryConfig(nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)
	s.ensureFinished(c)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestReloadSpacesBeforeControllerCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.ensureBootstrapParams(c)

	s.expectGateUnlock()
	s.expectUser(c)
	s.expectAuthorizedKeys()
	s.expectControllerConfig()
	s.expectAgentConfig()
	s.expectObjectStoreGetter(2)
	s.expectBootstrapFlagSet()
	s.expectSetMachineCloudInstance()
	s.expectSetAPIHostPorts()
	s.expectStateServingInfo()
	controllerCharmDeployerFunc := s.expectReloadSpacesWithFunc(c)
	s.expectInitialiseBakeryConfig(nil)

	w := s.newWorkerWithFunc(c, controllerCharmDeployerFunc)
	defer workertest.DirtyKill(c, w)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestSeedAgentBinary(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that the ControllerModelUUID is used for the namespace for the
	// object store. If it's not the controller model uuid, then the agent
	// binary will not be found.

	s.expectObjectStoreGetter(1)

	var called bool
	w := &bootstrapWorker{
		internalStates: s.states,
		cfg: WorkerConfig{
			ObjectStoreGetter: s.objectStoreGetter,
			AgentBinaryUploader: func(context.Context, string, BinaryAgentStorageService, AgentBinaryStore, objectstore.ObjectStore, logger.Logger) (func(), error) {
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
	c.Assert(called, tc.IsTrue)
	c.Assert(cleanup, tc.NotNil)
}

// TestSeedAuthorizedNilKeys is asserting that if we add a nil slice of
// authorized keys to the controller model that it is safe. This test is here
// assert that we don't break. Specifically because this functionality is being
// added after the fact and may not always be set.
func (s *workerSuite) TestSeedAuthorizedNilKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "admin")).Return(
		user.User{
			UUID: s.adminUserID,
		},
		nil,
	)

	s.keyManagerService.EXPECT().AddPublicKeysForUser(gomock.Any(), s.adminUserID, []string{}).Return(nil)

	w := &bootstrapWorker{
		cfg: WorkerConfig{
			UserService:       s.userService,
			KeyManagerService: s.keyManagerService,
		},
	}

	err := w.seedInitialAuthorizedKeys(c.Context(), nil)
	c.Check(err, tc.ErrorIsNil)
}

func (s *workerSuite) TestSeedBakeryConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()
	w := &bootstrapWorker{
		cfg: WorkerConfig{
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

func (s *workerSuite) TestFilterHostPortsEmptyManagementSpace(c *tc.C) {
	defer s.setupMocks(c).Finish()
	w := &bootstrapWorker{
		internalStates: s.states,
		cfg: WorkerConfig{
			SystemState: s.state,
		},
		logger: s.logger,
	}

	apiHostPorts := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "10.0.0.1"),
	}
	filteredHostPorts := w.filterHostPortsForManagementSpace(c.Context(), "", apiHostPorts, network.SpaceInfos{})
	c.Check(filteredHostPorts, tc.SameContents, apiHostPorts)
}

func (s *workerSuite) TestHostPortsNotInSpaceNoFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()
	w := &bootstrapWorker{
		internalStates: s.states,
		cfg: WorkerConfig{
			SystemState: s.state,
		},
		logger: s.logger,
	}

	apiHostPorts := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "10.0.0.1"),
	}
	allSpaces := network.SpaceInfos{
		{
			Name: "other-space",
			Subnets: []network.SubnetInfo{
				{
					CIDR: "10.0.0.0/24",
				},
			},
		},
		{
			ID:   "mgmt-space",
			Name: "mgmt-space",
			Subnets: []network.SubnetInfo{
				{
					CIDR: "10.1.0.0/24",
				},
			},
		},
	}
	filteredHostPorts := w.filterHostPortsForManagementSpace(c.Context(), "mgmt-space", apiHostPorts, allSpaces)
	c.Check(filteredHostPorts, tc.SameContents, apiHostPorts)
}

func (s *workerSuite) TestHostPortsSameSpaceThenFilter(c *tc.C) {
	defer s.setupMocks(c).Finish()
	w := &bootstrapWorker{
		internalStates: s.states,
		cfg: WorkerConfig{
			SystemState: s.state,
		},
		logger: s.logger,
	}

	spaceHostPorts := []network.SpaceHostPort{
		{
			SpaceAddress: network.SpaceAddress{
				SpaceID: "mgmt-space",
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
				},
			},
			NetPort: 1234,
		},
		{
			SpaceAddress: network.SpaceAddress{
				SpaceID: "other-space",
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.2",
				},
			},
			NetPort: 1234,
		},
	}
	apiHostPorts := []network.SpaceHostPorts{
		spaceHostPorts,
	}

	allSpaces := network.SpaceInfos{
		{
			ID:   "mgmt-space",
			Name: "mgmt-space",
			Subnets: []network.SubnetInfo{
				{
					CIDR: "10.0.0.0/24",
				},
			},
		},
	}

	expected := []network.SpaceHostPorts{
		{
			{
				SpaceAddress: network.SpaceAddress{
					SpaceID: "mgmt-space",
					MachineAddress: network.MachineAddress{
						Value: "10.0.0.1",
					},
				},
				NetPort: 1234,
			},
		},
	}
	filteredHostPorts := w.filterHostPortsForManagementSpace(c.Context(), "mgmt-space", apiHostPorts, allSpaces)
	c.Check(filteredHostPorts, tc.SameContents, expected)
}

func (s *workerSuite) TestSeedStoragePools(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageService.EXPECT().CreateStoragePool(gomock.Any(), "loop-pool", provider.LoopProviderType, map[string]any{"foo": "bar"})

	w := &bootstrapWorker{
		internalStates: s.states,
		cfg: WorkerConfig{
			ObjectStoreGetter: s.objectStoreGetter,
			ProviderRegistry:  provider.CommonStorageProviders(),
			StorageService:    s.storageService,
			SystemState:       s.state,
			Logger:            s.logger,
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

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	return s.newWorkerWithFunc(c,
		func(context.Context, ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
			return nil, nil
		})
}

func (s *workerSuite) newWorkerWithFunc(c *tc.C, controllerCharmDeployerFunc ControllerCharmDeployerFunc) worker.Worker {
	w, err := newWorker(WorkerConfig{
		Logger:                     s.logger,
		Agent:                      s.agent,
		ObjectStoreGetter:          s.objectStoreGetter,
		BootstrapUnlocker:          s.bootstrapUnlocker,
		CharmhubHTTPClient:         s.httpClient,
		ControllerAgentBinaryStore: s.controllerAgentBinaryStore,
		SystemState:                s.state,
		UserService:                s.userService,
		AgentPasswordService:       s.agentPasswordService,
		ApplicationService:         s.applicationService,
		ControllerNodeService:      s.controllerNodeService,
		ModelConfigService:         s.modelConfigService,
		MachineService:             s.machineService,
		ControllerModel:            s.controllerModel,
		KeyManagerService:          s.keyManagerService,
		ControllerConfigService:    s.controllerConfigService,
		StorageService:             s.storageService,
		ProviderRegistry:           provider.CommonStorageProviders(),
		CloudService:               s.cloudService,
		NetworkService:             s.networkService,
		BakeryConfigService:        s.bakeryConfigService,
		FlagService:                s.flagService,
		PopulateControllerCharm: func(context.Context, bootstrap.ControllerCharmDeployer) error {
			return nil
		},
		AgentBinaryUploader: func(context.Context, string, BinaryAgentStorageService, AgentBinaryStore, objectstore.ObjectStore, logger.Logger) (func(), error) {
			return func() {}, nil
		},
		ControllerCharmDeployer: controllerCharmDeployerFunc,
		BootstrapAddressFinder: func(context.Context, instance.Id) (network.ProviderAddresses, error) {
			return nil, nil
		},
		Clock: clock.WallClock,
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	ctrl := s.baseSuite.setupMocks(c)

	return ctrl
}

func (s *workerSuite) ensureStartup(c *tc.C) {
	s.ensureState(c, stateStarted)
}

func (s *workerSuite) ensureFinished(c *tc.C) {
	s.ensureState(c, stateCompleted)
}

func (s *workerSuite) ensureState(c *tc.C, st string) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, st)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for %s", st)
	}
}

func (s *workerSuite) expectControllerConfig() {
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).
		Return(controller.Config{
			controller.ControllerUUIDKey:   "test-uuid",
			controller.JujuManagementSpace: "mgmt-space",
		}, nil).Times(3)
}

func (s *workerSuite) expectUser(c *tc.C) {
	s.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "admin")).Return(user.User{
		UUID: s.adminUserID,
	}, nil).Times(2)
	s.userService.EXPECT().AddUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u accessservice.AddUserArg) (user.UUID, []byte, error) {
		c.Check(u.Name, tc.Equals, usertesting.GenNewName(c, "juju-metrics"))
		return usertesting.GenUserUUID(c), nil, nil
	})
	s.userService.EXPECT().AddExternalUser(gomock.Any(), usertesting.GenNewName(c, "everyone@external"), "", gomock.Any())
}

func (s *workerSuite) expectAuthorizedKeys() {
	s.keyManagerService.EXPECT().AddPublicKeysForUser(gomock.Any(), s.adminUserID, []string{}).Return(nil)
}

func (s *workerSuite) expectStateServingInfo() {
	s.agentConfig.EXPECT().StateServingInfo().Return(controller.StateServingInfo{
		APIPort: 42,
	}, true)
}

func (s *workerSuite) expectSetMachineCloudInstance() {
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name(agent.BootstrapControllerId)).Return("deadbeef", nil)
	s.machineService.EXPECT().SetMachineCloudInstance(gomock.Any(), machine.UUID("deadbeef"), instance.Id("i-deadbeef"), "", agent.BootstrapNonce, nil)
}

func (s *workerSuite) expectReloadSpaces() {
	s.networkService.EXPECT().ReloadSpaces(gomock.Any())
}

func (s *workerSuite) expectReloadSpacesWithFunc(c *tc.C) ControllerCharmDeployerFunc {
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

func (s *workerSuite) expectInitialiseBakeryConfig(err error) {
	s.bakeryConfigService.EXPECT().InitialiseBakeryConfig(gomock.Any()).Return(err)
}

func (s *workerSuite) expectObjectStoreGetter(num int) {
	s.objectStoreGetter.EXPECT().GetObjectStore(gomock.Any(), gomock.Any()).Return(s.objectStore, nil).Times(num)
}

func (s *workerSuite) expectBootstrapFlagSet() {
	s.flagService.EXPECT().SetFlag(gomock.Any(), flags.BootstrapFlag, true, flags.BootstrapFlagDescription).Return(nil)
}

func (s *workerSuite) expectSetAPIHostPorts() {
	spaceName := network.SpaceName("mgmt-space")
	mgmtSpace := &network.SpaceInfo{
		Name: spaceName,
		Subnets: []network.SubnetInfo{
			{
				CIDR: "10.0.0.0/24",
			},
		},
	}
	s.networkService.EXPECT().SpaceByName(gomock.Any(), spaceName).Return(mgmtSpace, nil)
	s.controllerNodeService.EXPECT().SetAPIAddresses(gomock.Any(), "0", gomock.Any(), mgmtSpace)

	s.networkService.EXPECT().GetAllSpaces(gomock.Any())
	s.state.EXPECT().SetAPIHostPorts(controller.Config{
		controller.ControllerUUIDKey:   "test-uuid",
		controller.JujuManagementSpace: "mgmt-space",
	}, gomock.Any(), gomock.Any())
}

func (s *workerSuite) ensureBootstrapParams(c *tc.C) {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig())
	c.Assert(err, tc.ErrorIsNil)

	args := instancecfg.StateInitializationParams{
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
