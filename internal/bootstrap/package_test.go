// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/juju/internal/charm"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charm/services"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination bootstrap_mock_test.go github.com/juju/juju/internal/bootstrap AgentBinaryStorage,ControllerCharmDeployer,HTTPClient,CloudService,CloudServiceGetter,OperationApplier,Machine,MachineGetter,StateBackend,Application,Charm,Unit,Model,CharmUploader,ApplicationService
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination charm_mock_test.go github.com/juju/juju/core/charm Repository
//go:generate go run go.uber.org/mock/mockgen -typed -package bootstrap -destination downloader_mock_test.go github.com/juju/juju/apiserver/facades/client/charms/interfaces Downloader

func Test(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	storage            *MockAgentBinaryStorage
	deployer           *MockControllerCharmDeployer
	httpClient         *MockHTTPClient
	objectStore        *MockObjectStore
	unit               *MockUnit
	model              *MockModel
	application        *MockApplication
	stateBackend       *MockStateBackend
	applicationService *MockApplicationService
	charmUploader      *MockCharmUploader
	charmDownloader    *MockDownloader
	charmRepo          *MockRepository
	charm              *MockCharm

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.storage = NewMockAgentBinaryStorage(ctrl)
	s.deployer = NewMockControllerCharmDeployer(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)

	s.unit = NewMockUnit(ctrl)
	s.model = NewMockModel(ctrl)
	s.application = NewMockApplication(ctrl)
	s.stateBackend = NewMockStateBackend(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.charmUploader = NewMockCharmUploader(ctrl)
	s.charmDownloader = NewMockDownloader(ctrl)
	s.charmRepo = NewMockRepository(ctrl)
	s.charm = NewMockCharm(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

func (s *baseSuite) newConfig(c *gc.C) BaseDeployerConfig {
	controllerUUID := uuid.MustNewUUID()

	return BaseDeployerConfig{
		DataDir:            c.MkDir(),
		StateBackend:       s.stateBackend,
		CharmUploader:      s.charmUploader,
		ApplicationService: s.applicationService,
		ObjectStore:        s.objectStore,
		Constraints:        constraints.Value{},
		ControllerConfig: controller.Config{
			controller.ControllerUUIDKey: controllerUUID.String(),
			controller.IdentityURL:       "https://inferi.com",
			controller.PublicDNSAddress:  "obscura.com",
			controller.APIPort:           1234,
		},
		NewCharmRepo: func(services.CharmRepoFactoryConfig) (corecharm.Repository, error) {
			return s.charmRepo, nil
		},
		NewCharmDownloader: func(services.CharmDownloaderConfig) (Downloader, error) {
			return s.charmDownloader, nil
		},
		CharmhubHTTPClient: s.httpClient,
		Channel:            charm.Channel{},
		Logger:             s.logger,
	}
}

func ptr[T any](v T) *T {
	return &v
}
