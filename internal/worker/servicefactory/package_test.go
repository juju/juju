// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	context "context"
	"testing"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	domaintesting "github.com/juju/juju/domain/schema/testing"
	environs "github.com/juju/juju/environs"
)

//go:generate go run go.uber.org/mock/mockgen -package servicefactory -destination servicefactory_mock_test.go github.com/juju/juju/internal/servicefactory ControllerServiceFactory,ModelServiceFactory,ServiceFactory,ServiceFactoryGetter
//go:generate go run go.uber.org/mock/mockgen -package servicefactory -destination servicefactory_logger_mock_test.go github.com/juju/juju/internal/worker/servicefactory Logger,SystemState
//go:generate go run go.uber.org/mock/mockgen -package servicefactory -destination database_mock_test.go github.com/juju/juju/core/database DBDeleter
//go:generate go run go.uber.org/mock/mockgen -package servicefactory -destination changestream_mock_test.go github.com/juju/juju/core/changestream WatchableDBGetter
//go:generate go run go.uber.org/mock/mockgen -package servicefactory -destination domain_mock_test.go github.com/juju/juju/domain/servicefactory EnvironFactory
//go:generate go run go.uber.org/mock/mockgen -package servicefactory -destination environ_mock_test.go github.com/juju/juju/environs Environ
//go:generate go run go.uber.org/mock/mockgen -package servicefactory -destination state_mock_test.go github.com/juju/juju/internal/worker/state StateTracker

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type baseSuite struct {
	domaintesting.ControllerSuite

	logger    *MockLogger
	dbDeleter *MockDBDeleter
	dbGetter  *MockWatchableDBGetter

	serviceFactoryGetter     *MockServiceFactoryGetter
	controllerServiceFactory *MockControllerServiceFactory
	modelServiceFactory      *MockModelServiceFactory

	environFactory *MockEnvironFactory
	environConfig  EnvironConfig
	environ        *MockEnviron
	state          *MockSystemState
	stateTracker   *MockStateTracker
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = NewMockLogger(ctrl)
	s.dbDeleter = NewMockDBDeleter(ctrl)
	s.dbGetter = NewMockWatchableDBGetter(ctrl)

	s.serviceFactoryGetter = NewMockServiceFactoryGetter(ctrl)
	s.controllerServiceFactory = NewMockControllerServiceFactory(ctrl)
	s.modelServiceFactory = NewMockModelServiceFactory(ctrl)

	s.environFactory = NewMockEnvironFactory(ctrl)
	s.environ = NewMockEnviron(ctrl)
	s.state = NewMockSystemState(ctrl)
	s.stateTracker = NewMockStateTracker(ctrl)

	s.environConfig = NewEnvironConfig(func(ctx context.Context, op environs.OpenParams) (environs.Environ, error) {
		return s.environ, nil
	}, s.state)

	return ctrl
}
