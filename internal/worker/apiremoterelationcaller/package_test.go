// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremoterelationcaller

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package apiremoterelationcaller -destination service_mock_test.go github.com/juju/juju/internal/worker/apiremoterelationcaller DomainServicesGetter,DomainServices,APIInfoGetter,ConnectionGetter,ExternalControllerService
//go:generate go run go.uber.org/mock/mockgen -typed -package apiremoterelationcaller -destination api_mock_test.go github.com/juju/juju/api Connection

type baseSuite struct {
	testhelpers.IsolationSuite

	domainServices       *MockDomainServices
	domainServicesGetter *MockDomainServicesGetter
	externalController   *MockExternalControllerService
	apiInfoGetter        *MockAPIInfoGetter
	connectionGetter     *MockConnectionGetter
	connection           *MockConnection

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.domainServices = NewMockDomainServices(ctrl)
	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)
	s.externalController = NewMockExternalControllerService(ctrl)
	s.apiInfoGetter = NewMockAPIInfoGetter(ctrl)
	s.connectionGetter = NewMockConnectionGetter(ctrl)
	s.connection = NewMockConnection(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}
