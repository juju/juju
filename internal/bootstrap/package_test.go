// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
	network "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/repository"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination bootstrap_mock_test.go github.com/juju/juju/internal/bootstrap AgentBinaryStore,ControllerCharmDeployer,HTTPClient,ApplicationService,IAASApplicationService,CAASApplicationService,ModelConfigService,Downloader,AgentPasswordService,ServiceManager
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination core_charm_mock_test.go github.com/juju/juju/core/charm Repository
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination internal_charm_mock_test.go github.com/juju/juju/internal/charm Charm
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination clock_mock_test.go github.com/juju/clock Clock

type baseSuite struct {
	testhelpers.IsolationSuite

	agentBinaryStore       *MockAgentBinaryStore
	deployer               *MockControllerCharmDeployer
	httpClient             *MockHTTPClient
	objectStore            *MockObjectStore
	agentPasswordService   *MockAgentPasswordService
	applicationService     *MockApplicationService
	iaasApplicationService *MockIAASApplicationService
	caasApplicationService *MockCAASApplicationService
	modelConfigService     *MockModelConfigService
	charmDownloader        *MockDownloader
	charmRepo              *MockRepository
	charm                  *MockCharm

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agentBinaryStore = NewMockAgentBinaryStore(ctrl)
	s.deployer = NewMockControllerCharmDeployer(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)

	s.agentPasswordService = NewMockAgentPasswordService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.iaasApplicationService = NewMockIAASApplicationService(ctrl)
	s.caasApplicationService = NewMockCAASApplicationService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.charmDownloader = NewMockDownloader(ctrl)
	s.charmRepo = NewMockRepository(ctrl)
	s.charm = NewMockCharm(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

func (s *baseSuite) newConfig(c *tc.C) BaseDeployerConfig {
	controllerUUID := uuid.MustNewUUID()

	return BaseDeployerConfig{
		DataDir:              c.MkDir(),
		AgentPasswordService: s.agentPasswordService,
		ApplicationService:   s.applicationService,
		ModelConfigService:   s.modelConfigService,
		ObjectStore:          s.objectStore,
		Constraints:          constraints.Value{},
		ControllerConfig: controller.Config{
			controller.ControllerUUIDKey: controllerUUID.String(),
			controller.IdentityURL:       "https://inferi.com",
			controller.PublicDNSAddress:  "obscura.com",
			controller.APIPort:           1234,
		},
		BootstrapAddresses: network.ProviderAddresses{
			{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
					Type:  network.IPv4Address,
					Scope: network.ScopeMachineLocal,
				},
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "203.0.113.1",
					Type:  network.IPv4Address,
					Scope: network.ScopePublic,
				},
			},
		},
		NewCharmHubRepo: func(repository.CharmHubRepositoryConfig) (corecharm.Repository, error) {
			return s.charmRepo, nil
		},
		NewCharmDownloader: func(h HTTPClient, l logger.Logger) Downloader {
			return s.charmDownloader
		},
		CharmhubHTTPClient: s.httpClient,
		Channel:            charm.Channel{},
		Logger:             s.logger,
		Clock:              clock.WallClock,
	}
}
