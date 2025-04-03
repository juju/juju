// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) SetUpTest(c *gc.C) {
	s.adminUserID = usertesting.GenUserUUID(c)
	s.controllerModel = coremodel.Model{
		UUID: modeltesting.GenModelUUID(c),
	}
}

func (s *workerSuite) TestKilled(c *gc.C) {
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

func (s *workerSuite) TestSeedAgentBinary(c *gc.C) {
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
			AgentBinaryUploader: func(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, logger.Logger) (func(), error) {
				called = true
				return func() {}, nil
			},
			ControllerCharmDeployer: func(ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
				return nil, nil
			},
			PopulateControllerCharm: func(context.Context, bootstrap.ControllerCharmDeployer) error {
				return nil
			},
			SystemState:     s.state,
			ControllerModel: s.controllerModel,
			Logger:          s.logger,
		},
	}
	cleanup, err := w.seedAgentBinary(context.Background(), c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(cleanup, gc.NotNil)
}

// TestSeedAuthorizedNilKeys is asserting that if we add a nil slice of
// authorized keys to the controller model that it is safe. This test is here
// assert that we don't break. Specifically because this functionality is being
// added after the fact and may not always be set.
func (s *workerSuite) TestSeedAuthorizedNilKeys(c *gc.C) {
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

	err := w.seedInitialAuthorizedKeys(context.Background(), nil)
	c.Check(err, jc.ErrorIsNil)
}

func (s *workerSuite) TestSeedBakeryConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()
	w := &bootstrapWorker{
		cfg: WorkerConfig{
			BakeryConfigService: s.bakeryConfigService,
		},
	}

	s.expectInitialiseBakeryConfig(nil)
	err := w.seedMacaroonConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	s.expectInitialiseBakeryConfig(macaroonerrors.BakeryConfigAlreadyInitialised)
	err = w.seedMacaroonConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	s.expectInitialiseBakeryConfig(errors.Errorf("boom"))
	err = w.seedMacaroonConfig(context.Background())
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *workerSuite) TestFilterHostPortsEmptyManagementSpace(c *gc.C) {
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
	filteredHostPorts := w.filterHostPortsForManagementSpace(context.Background(), "", apiHostPorts, network.SpaceInfos{})
	c.Check(filteredHostPorts, jc.SameContents, apiHostPorts)
}

func (s *workerSuite) TestHostPortsNotInSpaceNoFilter(c *gc.C) {
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
	filteredHostPorts := w.filterHostPortsForManagementSpace(context.Background(), "mgmt-space", apiHostPorts, allSpaces)
	c.Check(filteredHostPorts, jc.SameContents, apiHostPorts)
}

func (s *workerSuite) TestHostPortsSameSpaceThenFilter(c *gc.C) {
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
	filteredHostPorts := w.filterHostPortsForManagementSpace(context.Background(), "mgmt-space", apiHostPorts, allSpaces)
	c.Check(filteredHostPorts, jc.SameContents, expected)
}

func (s *workerSuite) TestSeedStoragePools(c *gc.C) {
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
	err := w.seedStoragePools(context.Background(), map[string]storage.Attrs{
		"loop-pool": {
			"name": "loop-pool",
			"type": "loop",
			"foo":  "bar",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	w, err := newWorker(WorkerConfig{
		Logger:                  s.logger,
		Agent:                   s.agent,
		ObjectStoreGetter:       s.objectStoreGetter,
		BootstrapUnlocker:       s.bootstrapUnlocker,
		CharmhubHTTPClient:      s.httpClient,
		SystemState:             s.state,
		UserService:             s.userService,
		PasswordService:         s.passwordService,
		ApplicationService:      s.applicationService,
		ModelConfigService:      s.modelConfigService,
		MachineService:          s.machineService,
		ControllerModel:         s.controllerModel,
		KeyManagerService:       s.keyManagerService,
		ControllerConfigService: s.controllerConfigService,
		StorageService:          s.storageService,
		ProviderRegistry:        provider.CommonStorageProviders(),
		CloudService:            s.cloudService,
		NetworkService:          s.networkService,
		BakeryConfigService:     s.bakeryConfigService,
		FlagService:             s.flagService,
		PopulateControllerCharm: func(context.Context, bootstrap.ControllerCharmDeployer) error {
			return nil
		},
		AgentBinaryUploader: func(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, logger.Logger) (func(), error) {
			return func() {}, nil
		},
		ControllerCharmDeployer: func(ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
			return nil, nil
		},
		BootstrapAddressFinder: func(context.Context, instance.Id) (network.ProviderAddresses, error) {
			return nil, nil
		},
	}, s.states)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	ctrl := s.baseSuite.setupMocks(c)

	return ctrl
}

func (s *workerSuite) ensureStartup(c *gc.C) {
	s.ensureState(c, stateStarted)
}

func (s *workerSuite) ensureFinished(c *gc.C) {
	s.ensureState(c, stateCompleted)
}

func (s *workerSuite) ensureState(c *gc.C, st string) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, st)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for %s", st)
	}
}

func (s *workerSuite) expectControllerConfig() {
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).
		Return(controller.Config{
			controller.ControllerUUIDKey: "test-uuid",
		}, nil).Times(3)
}

func (s *workerSuite) expectUser(c *gc.C) {
	s.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "admin")).Return(user.User{
		UUID: s.adminUserID,
	}, nil).Times(2)
	s.userService.EXPECT().AddUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u accessservice.AddUserArg) (user.UUID, []byte, error) {
		c.Check(u.Name, gc.Equals, usertesting.GenNewName(c, "juju-metrics"))
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
	s.machineService.EXPECT().SetMachineCloudInstance(gomock.Any(), "deadbeef", instance.Id("i-deadbeef"), "", nil)
}

func (s *workerSuite) expectReloadSpaces() {
	s.networkService.EXPECT().ReloadSpaces(gomock.Any())
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
	s.networkService.EXPECT().GetAllSpaces(gomock.Any())
	s.state.EXPECT().SetAPIHostPorts(controller.Config{
		controller.ControllerUUIDKey: "test-uuid",
	}, gomock.Any(), gomock.Any())
}

func (s *workerSuite) ensureBootstrapParams(c *gc.C) {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)

	args := instancecfg.StateInitializationParams{
		ControllerModelConfig:       cfg,
		BootstrapMachineConstraints: constraints.MustParse("mem=1G"),
		BootstrapMachineInstanceId:  instance.Id("i-deadbeef"),
		ControllerCharmPath:         "obscura",
		ControllerCharmChannel:      charm.MakePermissiveChannel("", "stable", ""),
	}
	bytes, err := args.Marshal()
	c.Assert(err, jc.ErrorIsNil)

	err = os.WriteFile(filepath.Join(s.dataDir, cloudconfig.FileNameBootstrapParams), bytes, 0644)
	c.Assert(err, jc.ErrorIsNil)
}
