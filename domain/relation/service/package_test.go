// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination leader_mock_test.go github.com/juju/juju/core/leadership Ensurer
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination package_mock_test.go github.com/juju/juju/domain/relation/service State,MigrationState,WatcherFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination relation_mock_test.go github.com/juju/juju/domain/relation SubordinateCreator

type baseServiceSuite struct {
	testhelpers.IsolationSuite

	state              *MockState
	subordinateCreator *MockSubordinateCreator

	service *Service
}

func (s *baseServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.subordinateCreator = NewMockSubordinateCreator(ctrl)

	s.service = NewService(s.state, loggertesting.WrapCheckLog(c))

	return ctrl
}
