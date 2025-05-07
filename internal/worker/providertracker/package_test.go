// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package providertracker -destination providertracker_mock_test.go github.com/juju/juju/internal/worker/providertracker DomainServicesGetter,DomainServices,ModelService,CloudService,ConfigService,CredentialService
//go:generate go run go.uber.org/mock/mockgen -typed -package providertracker -destination environs_mock_test.go github.com/juju/juju/environs Environ,CloudDestroyer,CloudSpecSetter
//go:generate go run go.uber.org/mock/mockgen -typed -package providertracker -destination storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry
//go:generate go run go.uber.org/mock/mockgen -typed -package providertracker -destination caas_mock_test.go github.com/juju/juju/caas Broker

func TestPackage(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}

type baseSuite struct {
	testhelpers.IsolationSuite

	states chan string

	domainServicesGetter *MockDomainServicesGetter
	domainServices       *MockDomainServices
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

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	ctrl := gomock.NewController(c)

	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)
	s.domainServices = NewMockDomainServices(ctrl)
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

func (s *baseSuite) ensureStartup(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateStarted)
	case <-time.After(testhelpers.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *baseSuite) expectDomainServices(namespace string) {
	s.domainServicesGetter.EXPECT().ServicesForModel(namespace).Return(s.domainServices)
	s.domainServices.EXPECT().Cloud().Return(s.cloudService)
	s.domainServices.EXPECT().Config().Return(s.configService)
	s.domainServices.EXPECT().Credential().Return(s.credentialService)
	s.domainServices.EXPECT().Model().Return(s.modelService)
}
