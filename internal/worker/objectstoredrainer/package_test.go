// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	stdtesting "testing"

	"github.com/juju/testing"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package objectstoredrainer -destination service_mock_test.go github.com/juju/juju/internal/worker/objectstoreflag ObjectStoreService
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstoredrainer -destination fortress_mock_test.go github.com/juju/juju/internal/worker/fortress Guard

func TestPackage(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

type baseSuite struct {
	testing.IsolationSuite

	logger logger.Logger

	service *MockObjectStoreService
	guard   *MockGuard
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.service = NewMockObjectStoreService(ctrl)
	s.guard = NewMockGuard(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}
