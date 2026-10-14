// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination leader_mock_test.go github.com/juju/juju/core/leadership Ensurer
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination package_mock_test.go github.com/juju/juju/domain/relation/service MigrationState,State,StatusHistory,WatcherFactory
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination poolprovider_mock_test.go github.com/juju/juju/domain/storageprovisioning StoragePoolProvider
//go:generate go run github.com/canonical/gomock/mockgen -package service -mock_names=Provider=MockStorageProvider -destination internal_storage_mock_test.go github.com/juju/juju/internal/storage Provider

type baseServiceSuite struct {
	testhelpers.IsolationSuite

	state         *MockState
	statusHistory *MockStatusHistory

	storagePoolProvider *MockStoragePoolProvider

	service *Service
}

func (s *baseServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.statusHistory = NewMockStatusHistory(ctrl)
	s.storagePoolProvider = NewMockStoragePoolProvider(ctrl)

	s.service = NewService(s.state, s.storagePoolProvider, s.statusHistory, loggertesting.WrapCheckLog(c))

	c.Cleanup(func() {
		s.state = nil
		s.statusHistory = nil
		s.storagePoolProvider = nil
	})

	return ctrl
}
