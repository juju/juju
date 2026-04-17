// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/application/internal"
	domainstorage "github.com/juju/juju/domain/storage"
)

// setAddUnitNoopStorageExpects sets on the storage service mock a set of noop
// storage service calls for when a new unit is being added to an application.
// This exists as there is a range of tests that need to assert functionality
// other then storage which is tested in its own right. Having these expects and
// junk data pollute the tests encourages poor effort in testing.
func setAddUnitNoopStorageExpects(
	c *tc.C,
	st *MockState,
	storageService *MockStorageService,
) {
	st.EXPECT().GetStorageAttachInfoForStorageInstances(
		gomock.Any(), tc.Bind(tc.HasLen, 0),
	).AnyTimes()
	storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), gomock.Any()).Return(
		[]internal.StorageDirective{}, nil,
	).AnyTimes()
	storageService.EXPECT().MakeUnitStorageArgs(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(domainstorage.CreateUnitStorageArg{}, nil).AnyTimes()
	storageService.EXPECT().MakeIAASUnitStorageArgs(
		gomock.Any(), gomock.Any(),
	).Return(domainstorage.CreateIAASUnitStorageArg{}, nil).AnyTimes()
}

// setCreateApplicationNoopStorageExpects sets on the storage service mock a set
// of noop storage service calls for when a new application is being created.
// This exists as there is a range of tests that need to assert functionality
// other then storage which is tested in its own right. Having these expects and
// junk data pollute the tests encourages poor effort in testing.
func setCreateApplicationNoopStorageExpects(
	c *tc.C,
	st *MockState,
	storageService *MockStorageService,
) {
	st.EXPECT().GetStorageAttachInfoForStorageInstances(
		gomock.Any(), tc.Bind(tc.HasLen, 0),
	).AnyTimes()
	storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), gomock.Any()).Return(
		[]internal.StorageDirective{}, nil,
	).AnyTimes()
	storageService.EXPECT().MakeApplicationStorageDirectiveArgs(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil, nil).AnyTimes()
	storageService.EXPECT().MakeUnitStorageArgs(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(domainstorage.CreateUnitStorageArg{}, nil).AnyTimes()
	storageService.EXPECT().MakeIAASUnitStorageArgs(
		gomock.Any(), gomock.Any(),
	).Return(domainstorage.CreateIAASUnitStorageArg{}, nil).AnyTimes()
	storageService.EXPECT().ValidateCharmStorage(
		gomock.Any(), gomock.Any(),
	).Return(nil).AnyTimes()
	storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil).AnyTimes()
}
