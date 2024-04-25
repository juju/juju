// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	stdtesting "testing"
	"time"

	"github.com/juju/testing"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package providertracker -destination providertracker_mock_test.go github.com/juju/juju/internal/worker/providertracker ServiceFactoryGetter,ServiceFactory,ModelService,CloudService,ConfigService,CredentialService
//go:generate go run go.uber.org/mock/mockgen -typed -package providertracker -destination environs_mock_test.go github.com/juju/juju/environs Environ,CloudDestroyer,CloudSpecSetter
//go:generate go run go.uber.org/mock/mockgen -typed -package providertracker -destination storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry
//go:generate go run go.uber.org/mock/mockgen -typed -package providertracker -destination caas_mock_test.go github.com/juju/juju/caas Broker

func TestPackage(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

type baseSuite struct {
	testing.IsolationSuite

	states chan string

	serviceFactoryGetter *MockServiceFactoryGetter
	serviceFactory       *MockServiceFactory
	modelService         *MockModelService
	cloudService         *MockCloudService
	configService        *MockConfigService
	credentialService    *MockCredentialService

	environ          *MockEnviron
	broker           *MockBroker
	cloudDestroyer   *MockCloudDestroyer
	providerRegistry *MockProviderRegistry
	cloudSpecSetter  *MockCloudSpecSetter

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	ctrl := gomock.NewController(c)

	s.serviceFactoryGetter = NewMockServiceFactoryGetter(ctrl)
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

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

func (s *baseSuite) ensureStartup(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *baseSuite) expectServiceFactory(namespace string) {
	s.serviceFactoryGetter.EXPECT().FactoryForModel(namespace).Return(s.serviceFactory)
	s.serviceFactory.EXPECT().Cloud().Return(s.cloudService)
	s.serviceFactory.EXPECT().Config().Return(s.configService)
	s.serviceFactory.EXPECT().Credential().Return(s.credentialService)
	s.serviceFactory.EXPECT().Model().Return(s.modelService)
}
