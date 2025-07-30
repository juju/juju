// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination objectstore_mock_test.go github.com/juju/juju/internal/worker/objectstore TrackedObjectStore,MetadataServiceGetter,MetadataService,ModelClaimGetter,ControllerConfigService,ModelService,ModelServiceGetter,ModelServices
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination claimer_mock_test.go github.com/juju/juju/internal/objectstore Claimer
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination lease_mock_test.go github.com/juju/juju/core/lease Manager
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination client_mock_test.go github.com/juju/juju/core/objectstore Client,Session
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination apiremotecaller_mock_test.go github.com/juju/juju/internal/worker/apiremotecaller APIRemoteCallers

type baseSuite struct {
	testhelpers.IsolationSuite

	logger logger.Logger

	clock           *MockClock
	agent           *MockAgent
	agentConfig     *MockConfig
	leaseManager    *MockManager
	claimer         *MockClaimer
	s3Client        *MockClient
	apiRemoteCaller *MockAPIRemoteCallers

	controllerConfigService *MockControllerConfigService
	metadataService         *MockMetadataService
	modelService            *MockModelService

	trackedObjectStore *MockTrackedObjectStore
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
	s.leaseManager = NewMockManager(ctrl)
	s.claimer = NewMockClaimer(ctrl)
	s.s3Client = NewMockClient(ctrl)
	s.apiRemoteCaller = NewMockAPIRemoteCallers(ctrl)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.metadataService = NewMockMetadataService(ctrl)
	s.modelService = NewMockModelService(ctrl)

	s.trackedObjectStore = NewMockTrackedObjectStore(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	c.Cleanup(func() {
		s.clock = nil
		s.agent = nil
		s.agentConfig = nil
		s.leaseManager = nil
		s.claimer = nil
		s.s3Client = nil
		s.apiRemoteCaller = nil
		s.controllerConfigService = nil
		s.metadataService = nil
		s.modelService = nil
		s.trackedObjectStore = nil
	})

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).AnyTimes()
}
