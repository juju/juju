package service

import (
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/application"
)

// setAddUnitNoopStorageExpects sets on the storage service mock a set of noop
// storage service calls for when a new unit is being added to an application.
// This exists as there is a range of tests that need to assert functionality
// other then storage which is tested in its own right. Having these expects and
// junk data pollute the tests encourages poor effort in testing.
func setAddUnitNoopStorageExpects(
	storageService *MockStorageService,
) {
	storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), gomock.Any()).Return(
		[]application.StorageDirective{}, nil,
	).AnyTimes()
	storageService.EXPECT().MakeUnitStorageArgs(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(application.CreateUnitStorageArg{}, nil).AnyTimes()
}

// setRegisterCAASunitNoopStorageExpects sets on the storage service mock a set
// of noop storage service calls for when a CAAS unit is being registered. This
// exists as there is a range of tests that need to assert functionality other
// then storage which is tested in its own right. Having these expects and junk
// data pollute the tests encourages poor effort in testing.
func setRegisterCAASunitNoopStorageExpects(
	storageService *MockStorageService,
) {
	storageService.EXPECT().GetRegisterCAASUnitStorageArg(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(application.RegisterUnitStorageArg{}, nil).AnyTimes()
}

// setCreateApplicationNoopStorageExpects sets on the storage service mock a set
// of noop storage service calls for when a new application is being created.
// This exists as there is a range of tests that need to assert functionality
// other then storage which is tested in its own right. Having these expects and
// junk data pollute the tests encourages poor effort in testing.
func setCreateApplicationNoopStorageExpects(
	storageService *MockStorageService,
) {
	storageService.EXPECT().GetApplicationStorageDirectives(gomock.Any(), gomock.Any()).Return(
		[]application.StorageDirective{}, nil,
	).AnyTimes()
	storageService.EXPECT().MakeApplicationStorageDirectiveArgs(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil, nil).AnyTimes()
	storageService.EXPECT().MakeUnitStorageArgs(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(application.CreateUnitStorageArg{}, nil).AnyTimes()
	storageService.EXPECT().ValidateApplicationStorageDirectiveOverrides(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil).AnyTimes()
}
