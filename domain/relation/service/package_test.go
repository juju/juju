// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"
	"github.com/canonical/gomock/gomock"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination leader_mock_test.go github.com/juju/juju/core/leadership Ensurer
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination package_mock_test.go github.com/juju/juju/domain/relation/service MigrationState,State,StatusHistory,WatcherFactory

type baseServiceSuite struct {
	testhelpers.IsolationSuite

	state         *MockState
	statusHistory *MockStatusHistory

	service *Service
}

func (s *baseServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.statusHistory = NewMockStatusHistory(ctrl)

	s.service = NewService(s.state, s.statusHistory, loggertesting.WrapCheckLog(c))

	c.Cleanup(func() {
		s.state = nil
		s.statusHistory = nil
	})

	return ctrl
}
