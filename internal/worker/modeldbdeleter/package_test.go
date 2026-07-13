// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modeldbdeleter

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run github.com/canonical/gomock/mockgen -package modeldbdeleter -destination database_mock_test.go github.com/juju/juju/core/database DBDeleter
//go:generate go run github.com/canonical/gomock/mockgen -package modeldbdeleter -destination package_mock_test.go github.com/juju/juju/internal/worker/modeldbdeleter ModelDatabaseDeletionService

type baseSuite struct {
	testhelpers.IsolationSuite

	dbDeleter       *MockDBDeleter
	deletionService *MockModelDatabaseDeletionService

	logger logger.Logger
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.dbDeleter = NewMockDBDeleter(ctrl)
	s.deletionService = NewMockModelDatabaseDeletionService(ctrl)

	s.logger = loggertesting.WrapCheckLog(c)

	c.Cleanup(func() {
		s.dbDeleter = nil
		s.deletionService = nil
		s.logger = nil
	})

	return ctrl
}
