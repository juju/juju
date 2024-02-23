// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/juju/charm"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/internal/charm/services"
	"github.com/juju/juju/internal/uuid"
	jujujujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination bootstrap_mock_test.go github.com/juju/juju/internal/bootstrap AgentBinaryStorage,ControllerCharmDeployer,HTTPClient,LoggerFactory,CloudService,CloudServiceGetter,OperationApplier,Machine,MachineGetter,StateBackend,Application,Charm,Unit,Model,CharmUploader,ApplicationSaver
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination charm_mock_test.go github.com/juju/juju/core/charm Repository
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination downloader_mock_test.go github.com/juju/juju/apiserver/facades/client/charms/interfaces Downloader

func Test(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	storage          *MockAgentBinaryStorage
	deployer         *MockControllerCharmDeployer
	httpClient       *MockHTTPClient
	loggerFactory    *MockLoggerFactory
	objectStore      *MockObjectStore
	unit             *MockUnit
	model            *MockModel
	application      *MockApplication
	stateBackend     *MockStateBackend
	applicationSaver *MockApplicationSaver
	charmUploader    *MockCharmUploader
	charmDownloader  *MockDownloader
	charmRepo        *MockRepository
	charm            *MockCharm

	logger Logger
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.storage = NewMockAgentBinaryStorage(ctrl)
	s.deployer = NewMockControllerCharmDeployer(ctrl)
	s.httpClient = NewMockHTTPClient(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)
	s.loggerFactory = NewMockLoggerFactory(ctrl)

	s.unit = NewMockUnit(ctrl)
	s.model = NewMockModel(ctrl)
	s.application = NewMockApplication(ctrl)
	s.stateBackend = NewMockStateBackend(ctrl)
	s.applicationSaver = NewMockApplicationSaver(ctrl)
	s.charmUploader = NewMockCharmUploader(ctrl)
	s.charmDownloader = NewMockDownloader(ctrl)
	s.charmRepo = NewMockRepository(ctrl)
	s.charm = NewMockCharm(ctrl)

	s.logger = jujujujutesting.NewCheckLogger(c)

	s.loggerFactory = NewMockLoggerFactory(ctrl)
	s.loggerFactory.EXPECT().Child(gomock.Any()).Return(s.logger).AnyTimes()

	return ctrl
}

func (s *baseSuite) newConfig(c *gc.C) BaseDeployerConfig {
	controllerUUID := uuid.MustNewUUID()

	return BaseDeployerConfig{
		DataDir:          c.MkDir(),
		StateBackend:     s.stateBackend,
		CharmUploader:    s.charmUploader,
		ApplicationSaver: s.applicationSaver,
		ObjectStore:      s.objectStore,
		Constraints:      constraints.Value{},
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
		LoggerFactory:      s.loggerFactory,
	}
}

func ptr[T any](v T) *T {
	return &v
}
