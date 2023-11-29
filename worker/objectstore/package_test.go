// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"testing"
	"time"

	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jujujujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package objectstore -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -package objectstore -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -package objectstore -destination objectstore_mock_test.go github.com/juju/juju/worker/objectstore TrackedObjectStore,StatePool,MongoSession,MetadataService
//go:generate go run go.uber.org/mock/mockgen -package objectstore -destination state_mock_test.go github.com/juju/juju/worker/state StateTracker

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	logger Logger

	clock       *MockClock
	agent       *MockAgent
	agentConfig *MockConfig

	// Deprecated: These are only here for backwards compatibility.
	stateTracker *MockStateTracker
	statePool    *MockStatePool
	mongoSession *MockMongoSession
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
	s.stateTracker = NewMockStateTracker(ctrl)
	s.statePool = NewMockStatePool(ctrl)
	s.mongoSession = NewMockMongoSession(ctrl)

	s.logger = jujujujutesting.NewCheckLogger(c)

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).AnyTimes()
}
