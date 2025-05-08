// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreflag

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package objectstoreflag -destination service_mock_test.go github.com/juju/juju/internal/worker/objectstoreflag ObjectStoreService

func TestPackage(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}

type baseSuite struct {
	testing.IsolationSuite

	logger logger.Logger

	service *MockObjectStoreService
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.service = NewMockObjectStoreService(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}
