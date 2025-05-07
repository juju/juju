// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"testing"
	"time"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination objectstore_mock_test.go github.com/juju/juju/internal/worker/objectstore TrackedObjectStore,MetadataServiceGetter,MetadataService,ModelClaimGetter,ControllerConfigService
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination claimer_mock_test.go github.com/juju/juju/internal/objectstore Claimer
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination lease_mock_test.go github.com/juju/juju/core/lease Manager
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination client_mock_test.go github.com/juju/juju/core/objectstore Client,Session

func TestPackage(t *testing.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	logger logger.Logger

	clock        *MockClock
	agent        *MockAgent
	agentConfig  *MockConfig
	leaseManager *MockManager
	claimer      *MockClaimer
	s3Client     *MockClient

	controllerConfigService *MockControllerConfigService
	metadataService         *MockMetadataService
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
	s.leaseManager = NewMockManager(ctrl)
	s.claimer = NewMockClaimer(ctrl)
	s.s3Client = NewMockClient(ctrl)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.metadataService = NewMockMetadataService(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).AnyTimes()
}
