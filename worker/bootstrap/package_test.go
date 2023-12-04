// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	jujujujutesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination state_mock_test.go github.com/juju/juju/worker/state StateTracker
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -package bootstrap -destination lock_mock_test.go github.com/juju/juju/worker/gate Unlocker

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	jujutesting.IsolationSuite

	agent             *MockAgent
	stateTracker      *MockStateTracker
	objectStore       *MockObjectStore
	bootstrapUnlocker *MockUnlocker

	logger Logger
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agent = NewMockAgent(ctrl)
	s.stateTracker = NewMockStateTracker(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)
	s.bootstrapUnlocker = NewMockUnlocker(ctrl)

	s.logger = jujujujutesting.NewCheckLogger(c)

	return ctrl
}

func (s *baseSuite) expectGateUnlock() {
	s.bootstrapUnlocker.EXPECT().Unlock()
}
