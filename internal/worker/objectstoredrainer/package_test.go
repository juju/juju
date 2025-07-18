// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package objectstoredrainer -destination service_mock_test.go github.com/juju/juju/internal/worker/objectstoredrainer ObjectStoreService,ObjectStoreServicesGetter,GuardService,ControllerService,ControllerConfigService,HashFileSystemAccessor
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstoredrainer -destination fortress_mock_test.go github.com/juju/juju/internal/worker/fortress Guard
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstoredrainer -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstoredrainer -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore Client,Session,ObjectStoreMetadata

type baseSuite struct {
	testhelpers.IsolationSuite

	logger logger.Logger

	agent                     *MockAgent
	agentConfig               *MockConfig
	guard                     *MockGuard
	guardService              *MockGuardService
	objectStoreService        *MockObjectStoreService
	objectStoreServicesGetter *MockObjectStoreServicesGetter
	objectStoreMetadata       *MockObjectStoreMetadata
	controllerService         *MockControllerService
	controllerConfigService   *MockControllerConfigService
	s3Client                  *MockClient
	s3Session                 *MockSession
	hashFileSystemAccessor    *MockHashFileSystemAccessor
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
	s.guard = NewMockGuard(ctrl)
	s.guardService = NewMockGuardService(ctrl)
	s.objectStoreService = NewMockObjectStoreService(ctrl)
	s.objectStoreServicesGetter = NewMockObjectStoreServicesGetter(ctrl)
	s.objectStoreMetadata = NewMockObjectStoreMetadata(ctrl)
	s.controllerService = NewMockControllerService(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.s3Client = NewMockClient(ctrl)
	s.s3Session = NewMockSession(ctrl)
	s.hashFileSystemAccessor = NewMockHashFileSystemAccessor(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	c.Cleanup(func() {
		s.agent = nil
		s.agentConfig = nil
		s.guard = nil
		s.guardService = nil
		s.objectStoreService = nil
		s.objectStoreServicesGetter = nil
		s.objectStoreMetadata = nil
		s.controllerService = nil
		s.controllerConfigService = nil
		s.s3Client = nil
		s.s3Session = nil
		s.hashFileSystemAccessor = nil
	})

	return ctrl
}
