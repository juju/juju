// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstorefacade

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package objectstorefacade -destination service_mock_test.go github.com/juju/juju/core/objectstore ObjectStoreGetter,ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstorefacade -destination fortress_mock_test.go github.com/juju/juju/internal/worker/fortress Guest

type baseSuite struct {
	testhelpers.IsolationSuite

	logger logger.Logger

	objectStoreGetter *MockObjectStoreGetter
	objectStore       *MockObjectStore
	guest             *MockGuest
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.objectStoreGetter = NewMockObjectStoreGetter(ctrl)
	s.objectStore = NewMockObjectStore(ctrl)
	s.guest = NewMockGuest(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	return ctrl
}
