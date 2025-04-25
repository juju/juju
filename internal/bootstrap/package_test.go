// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/clock"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/repository"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination bootstrap_mock_test.go github.com/juju/juju/internal/bootstrap AgentBinaryStorage,AgentBinaryStore,ControllerCharmDeployer,HTTPClient,CloudService,CloudServiceGetter,Machine,MachineGetter,ApplicationService,ModelConfigService,Downloader,AgentPasswordService,RelationService
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination core_charm_mock_test.go github.com/juju/juju/core/charm Repository
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination internal_charm_mock_test.go github.com/juju/juju/internal/charm Charm
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination clock_mock_test.go github.com/juju/clock Clock

func Test(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	agentBinaryStore     *MockAgentBinaryStore
	storage              *MockAgentBinaryStorage
	deployer             *MockControllerCharmDeployer
	httpClient           *MockHTTPClient
	objectStore          *MockObjectStore
	agentPasswordService *MockAgentPasswordService
	applicationService   *MockApplicationService
	modelConfigService   *MockModelConfigService
	relationService      *MockRelationService
	charmDownloader      *MockDownloader
	charmRepo            *MockRepository
	charm                *MockCharm

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agentBinaryStore = NewMockAgentBinaryStore(ctrl)
	s.storage = NewMockAgentBinaryStorage(ctrl)
	s.deployer = NewMockControllerCharmDeployer(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)

	s.agentPasswordService = NewMockAgentPasswordService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.relationService = NewMockRelationService(ctrl)
	s.charmDownloader = NewMockDownloader(ctrl)
	s.charmRepo = NewMockRepository(ctrl)
	s.charm = NewMockCharm(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

func (s *baseSuite) newConfig(c *gc.C) BaseDeployerConfig {
	controllerUUID := uuid.MustNewUUID()

	return BaseDeployerConfig{
		DataDir:              c.MkDir(),
		AgentPasswordService: s.agentPasswordService,
		ApplicationService:   s.applicationService,
		ModelConfigService:   s.modelConfigService,
		RelationService:      s.relationService,
		ObjectStore:          s.objectStore,
		Constraints:          constraints.Value{},
		ControllerConfig: controller.Config{
			controller.ControllerUUIDKey: controllerUUID.String(),
			controller.IdentityURL:       "https://inferi.com",
			controller.PublicDNSAddress:  "obscura.com",
			controller.APIPort:           1234,
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
