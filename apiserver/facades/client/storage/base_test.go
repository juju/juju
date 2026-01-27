// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremodel "github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

// baseStorageSuite provides a base [tc] testing suite that establishes the
// required dependencies of this facade for easier testing.
type baseStorageSuite struct {
	blockChecker       *MockBlockChecker
	applicationService *MockApplicationService
	removalService     *MockRemovalService
	storageService     *MockStorageService

	authorizer     apiservertesting.FakeAuthorizer
	controllerUUID string
	modelUUID      coremodel.UUID
}

// makeTestAPI constructs a new [StorageAPI] with the mock dependencies
// contained in [baseStorageSuite]. This func expects the caller to have setup
// mocks first with [baseStorageSuite.setupMocks]
func (s *baseStorageSuite) makeTestAPI(c *tc.C) *StorageAPI {
	return NewStorageAPI(
		s.controllerUUID,
		s.modelUUID,
		coremodel.IAAS,
		s.authorizer,
		loggertesting.WrapCheckLog(c),
		s.blockChecker,
		s.applicationService,
		s.removalService,
		s.storageService,
	)
}

// setupMocks establishes a go mock controller and creates the required
// dependency mocks for a [StorageAPI].
func (s *baseStorageSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin"), Controller: true}
	s.applicationService = NewMockApplicationService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)
	s.storageService = NewMockStorageService(ctrl)
	s.controllerUUID = uuid.MustNewUUID().String()
	s.modelUUID = tc.Must0(c, coremodel.NewUUID)

	c.Cleanup(func() {
		s.authorizer = apiservertesting.FakeAuthorizer{}
		s.applicationService = nil
		s.controllerUUID = ""
		s.modelUUID = ""
		s.removalService = nil
		s.storageService = nil
	})

	return ctrl
}
