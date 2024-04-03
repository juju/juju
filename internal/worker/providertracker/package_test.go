// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	stdtesting "testing"

	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package providertracker -destination providertracker_mock_test.go github.com/juju/juju/internal/worker/providertracker ServiceFactory,ModelService,CloudService,ConfigService,CredentialService
//go:generate go run go.uber.org/mock/mockgen -package providertracker -destination environs_mock_test.go github.com/juju/juju/environs Environ,CloudDestroyer,CloudSpecSetter
//go:generate go run go.uber.org/mock/mockgen -package providertracker -destination storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry
//go:generate go run go.uber.org/mock/mockgen -package providertracker -destination caas_mock_test.go github.com/juju/juju/caas Broker

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	testing.IsolationSuite

	serviceFactory    *MockServiceFactory
	modelService      *MockModelService
	cloudService      *MockCloudService
	configService     *MockConfigService
	credentialService *MockCredentialService

	environ          *MockEnviron
	broker           *MockBroker
	cloudDestroyer   *MockCloudDestroyer
	providerRegistry *MockProviderRegistry
	cloudSpecSetter  *MockCloudSpecSetter

	logger Logger
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.serviceFactory = NewMockServiceFactory(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.cloudService = NewMockCloudService(ctrl)
	s.configService = NewMockConfigService(ctrl)
	s.credentialService = NewMockCredentialService(ctrl)

	s.environ = NewMockEnviron(ctrl)
	s.broker = NewMockBroker(ctrl)
	s.cloudDestroyer = NewMockCloudDestroyer(ctrl)
	s.providerRegistry = NewMockProviderRegistry(ctrl)
	s.cloudSpecSetter = NewMockCloudSpecSetter(ctrl)

	s.logger = jujutesting.NewCheckLogger(c)

	return ctrl
}
