// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/flags"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accessservice "github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	baseSuite

	states chan string
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestKilled(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.ensureBootstrapParams(c)

	s.expectGateUnlock()
	s.expectUser(c)
	s.expectControllerConfig()
	s.expectAgentConfig()
	s.expectObjectStoreGetter(2)
	s.expectBootstrapFlagSet()
	s.expectSetAPIHostPorts()
	s.expectStateServingInfo()
	s.expectReloadSpaces()
	s.expectInitialiseBakeryConfig()

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
			SystemState: s.state,
			Logger:      s.logger,
		},
	}
	cleanup, err := w.seedAgentBinary(context.Background(), c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(cleanup, gc.NotNil)
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
	filteredHostPorts := w.filterHostPortsForManagementSpace("", apiHostPorts, network.SpaceInfos{})
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
	filteredHostPorts := w.filterHostPortsForManagementSpace("mgmt-space", apiHostPorts, allSpaces)
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
	filteredHostPorts := w.filterHostPortsForManagementSpace("mgmt-space", apiHostPorts, allSpaces)
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
		ApplicationService:      s.applicationService,
		ControllerConfigService: s.controllerConfigService,
		CredentialService:       s.credentialService,
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
		NewEnviron: func(context.Context, environs.OpenParams) (environs.Environ, error) { return nil, nil },
		BootstrapAddresses: func(context.Context, environs.Environ, instance.Id) (network.ProviderAddresses, error) {
			return nil, nil
		},
		BootstrapAddressFinder: func(context.Context, BootstrapAddressesConfig) (network.ProviderAddresses, error) {
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
		}, nil).Times(2)
}

func (s *workerSuite) expectUser(c *gc.C) {
	s.userService.EXPECT().GetUserByName(gomock.Any(), "admin").Return(user.User{
		UUID: usertesting.GenUserUUID(c),
	}, nil)
	s.userService.EXPECT().AddUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u accessservice.AddUserArg) (user.UUID, []byte, error) {
		c.Check(u.Name, gc.Equals, "juju-metrics")
		return usertesting.GenUserUUID(c), nil, nil
	})
}

func (s *workerSuite) expectStateServingInfo() {
	s.agentConfig.EXPECT().StateServingInfo().Return(controller.StateServingInfo{
		APIPort: 42,
	}, true)
}

func (s *workerSuite) expectReloadSpaces() {
	s.networkService.EXPECT().ReloadSpaces(gomock.Any())
}

func (s *workerSuite) expectInitialiseBakeryConfig() {
	s.bakeryConfigService.EXPECT().InitialiseBakeryConfig(gomock.Any())
}

func (s *workerSuite) expectObjectStoreGetter(num int) {
	s.state.EXPECT().ControllerModelUUID().Return(uuid.MustNewUUID().String()).Times(num)
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
		ControllerCharmPath:         "obscura",
		ControllerCharmChannel:      charm.MakePermissiveChannel("", "stable", ""),
	}
	bytes, err := args.Marshal()
	c.Assert(err, jc.ErrorIsNil)

	err = os.WriteFile(filepath.Join(s.dataDir, cloudconfig.FileNameBootstrapParams), bytes, 0644)
	c.Assert(err, jc.ErrorIsNil)
}
