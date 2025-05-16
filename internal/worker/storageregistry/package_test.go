// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageregistry

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

//go:generate go run go.uber.org/mock/mockgen -typed -package storageregistry -destination clock_mock_test.go github.com/juju/clock Clock,Timer
//go:generate go run go.uber.org/mock/mockgen -typed -package storageregistry -destination storage_mock_test.go github.com/juju/juju/internal/storage ProviderRegistry,Provider
//go:generate go run go.uber.org/mock/mockgen -typed -package storageregistry -destination provider_mock_test.go github.com/juju/juju/core/providertracker ProviderFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package storageregistry -destination storageregistry_mock_test.go github.com/juju/juju/internal/worker/storageregistry StorageRegistryWorker

func TestPackage(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}

type baseSuite struct {
	testhelpers.IsolationSuite

	logger logger.Logger

	clock           *MockClock
	providerFactory *MockProviderFactory
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)
	s.providerFactory = NewMockProviderFactory(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}

func (s *baseSuite) expectClock() {
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	s.clock.EXPECT().After(gomock.Any()).AnyTimes()
}
