// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// attachSuite provides a suite of tests for asserting the functionality
// behind attaching storage instances to a unit.
type attachSuite struct {
	baseStorageSuite
}

// TestAttachSuite registers and runs all the tests from [attachSuite].
func TestAttachSuite(t *testing.T) {
	tc.Run(t, &attachSuite{})
}

// attachOneStorageArgs returns a [params.StorageAttachmentIds] with a single
// attachment of "storage-data/0" to "unit-myapp/0", suitable for use as a
// standard test input.
func attachOneStorageArgs() params.StorageAttachmentIds {
	return params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-data/0",
				UnitTag:    "unit-myapp/0",
			},
		},
	}
}

// expectAttachPrereqs sets up the standard mock expectations for the block
// checker, GetUnitUUID, and GetStorageInstanceUUIDForID calls that precede
// AttachStorageToUnit, returning the UUIDs for use in further expectations.
func (s *attachSuite) expectAttachPrereqs(c *tc.C) (coreunit.UUID, domainstorage.StorageInstanceUUID) {
	unitUUID := tc.Must(c, coreunit.NewUUID)
	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.applicationService.EXPECT().GetUnitUUID(
		gomock.Any(), coreunit.Name("myapp/0"),
	).Return(unitUUID, nil)
	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "data/0",
	).Return(storageUUID, nil)
	return unitUUID, storageUUID
}

// expectAttachSuccess sets up the mock expectations for a successful attach of
// "data/0" to "myapp/0".
func (s *attachSuite) expectAttachSuccess(
	unitUUID coreunit.UUID,
	storageUUID domainstorage.StorageInstanceUUID,
) {
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.applicationService.EXPECT().GetUnitUUID(
		gomock.Any(), coreunit.Name("myapp/0"),
	).Return(unitUUID, nil)
	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "data/0",
	).Return(storageUUID, nil)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(nil)
}

// TestWithModelWritePermission tests that a user with write permission on the
// model is allowed to attach storage.
func (s *attachSuite) TestWithModelWritePermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasWriteTag: userTag,
		Tag:         userTag,
	}

	unitUUID := tc.Must(c, coreunit.NewUUID)
	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	s.expectAttachSuccess(unitUUID, storageUUID)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{Error: nil},
	})
}

// TestWithModelAdminPermission tests that a user with admin permission on the
// model is allowed to attach storage.
func (s *attachSuite) TestWithModelAdminPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		AdminTag: userTag,
		Tag:      userTag,
	}

	unitUUID := tc.Must(c, coreunit.NewUUID)
	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	s.expectAttachSuccess(unitUUID, storageUUID)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{Error: nil},
	})
}

// TestWithReadPermissionFails tests that a user with only read permission on
// the model is not allowed to attach storage. The caller MUST get back an error
// with [params.CodeUnauthorized] set.
func (s *attachSuite) TestWithReadPermissionFails(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	paramsErr, is := errors.AsType[*params.Error](err)
	c.Assert(is, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeUnauthorized)
	c.Check(result.Results, tc.HasLen, 0)
}

// TestWithNoPermissionFails tests that a user with no model permissions is not
// allowed to attach storage. The caller MUST get back an error with
// [params.CodeUnauthorized] set.
func (s *attachSuite) TestWithNoPermissionFails(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: userTag,
	}

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	paramsErr, is := errors.AsType[*params.Error](err)
	c.Assert(is, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeUnauthorized)
	c.Check(result.Results, tc.HasLen, 0)
}

// TestBlockCheckerDisallowsChanges asserts that if the block checker reports
// that changes are not allowed the caller gets back a top-level error with
// [params.CodeOperationBlocked] and no per-result errors.
func (s *attachSuite) TestBlockCheckerDisallowsChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(
		apiservererrors.OperationBlockedError("changes are blocked"),
	)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	paramsErr, is := errors.AsType[*params.Error](err)
	c.Assert(is, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeOperationBlocked)
	c.Check(result.Results, tc.HasLen, 0)
}

// TestAttachNoIDs asserts that calling Attach with no storage attachment IDs
// performs no action and returns an empty result.
func (s *attachSuite) TestAttachNoIDs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), params.StorageAttachmentIds{})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 0)
}

// TestAttachInvalidUnitTag asserts that if the unit tag supplied is not a valid
// tag the caller gets back a per-result error with [params.CodeNotValid].
func (s *attachSuite) TestAttachInvalidUnitTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "storage-data/0",
				UnitTag:    "not-a-unit-tag",
			},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: "invalid unit tag",
			},
		},
	})
}

// TestAttachInvalidStorageTag asserts that if the storage tag supplied is not
// a valid tag the caller gets back a per-result error with [params.CodeNotValid].
func (s *attachSuite) TestAttachInvalidStorageTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.applicationService.EXPECT().GetUnitUUID(
		gomock.Any(), coreunit.Name("myapp/0"),
	).Return(tc.Must(c, coreunit.NewUUID), nil)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{
				StorageTag: "not-a-storage-tag",
				UnitTag:    "unit-myapp/0",
			},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: "invalid storage tag",
			},
		},
	})
}

// TestAttachGetUnitUUIDInvalidUnitName asserts that if the application service
// reports the unit name is invalid the caller gets back a per-result error with
// [params.CodeNotValid].
func (s *attachSuite) TestAttachGetUnitUUIDInvalidUnitName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.applicationService.EXPECT().GetUnitUUID(
		gomock.Any(), coreunit.Name("myapp/0"),
	).Return("", coreunit.InvalidUnitName)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `invalid unit name "myapp/0"`,
			},
		},
	})
}

// TestAttachGetUnitUUIDUnitNotFound asserts that if the application service
// reports the unit does not exist the caller gets back a per-result error with
// [params.CodeNotFound].
func (s *attachSuite) TestAttachGetUnitUUIDUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.applicationService.EXPECT().GetUnitUUID(
		gomock.Any(), coreunit.Name("myapp/0"),
	).Return("", applicationerrors.UnitNotFound)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: `unit "myapp/0" does not exist`,
			},
		},
	})
}

// TestAttachStorageInstanceNotFound asserts that if the storage service reports
// the storage instance does not exist the caller gets back a per-result error
// with [params.CodeNotFound].
func (s *attachSuite) TestAttachStorageInstanceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.applicationService.EXPECT().GetUnitUUID(
		gomock.Any(), coreunit.Name("myapp/0"),
	).Return(tc.Must(c, coreunit.NewUUID), nil)
	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "data/0",
	).Return("", storageerrors.StorageInstanceNotFound)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: `storage "data/0" does not exist`,
			},
		},
	})
}

// TestAttachUnitNotFound asserts that if AttachStorageToUnit reports the unit
// does not exist the caller gets back a per-result error with
// [params.CodeNotFound].
func (s *attachSuite) TestAttachUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.UnitNotFound)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: `unit "myapp/0" does not exist`,
			},
		},
	})
}

// TestAttachUnitNotAlive asserts that if AttachStorageToUnit reports the unit
// is not alive the caller gets back a per-result error with
// [params.CodeNotValid].
func (s *attachSuite) TestAttachUnitNotAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.UnitNotAlive)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `unit "myapp/0" is not alive`,
			},
		},
	})
}

// TestAttachStorageInstanceNotFoundFromService asserts that if
// AttachStorageToUnit reports the storage instance does not exist the caller
// gets back a per-result error with [params.CodeNotFound].
func (s *attachSuite) TestAttachStorageInstanceNotFoundFromService(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(storageerrors.StorageInstanceNotFound)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: `storage "data/0" not found`,
			},
		},
	})
}

// TestAttachStorageInstanceNotAlive asserts that if AttachStorageToUnit reports
// the storage instance is not alive the caller gets back a per-result error
// with [params.CodeNotValid].
func (s *attachSuite) TestAttachStorageInstanceNotAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(storageerrors.StorageInstanceNotAlive)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `storage "data/0" is not alive`,
			},
		},
	})
}

// TestAttachStorageNameNotSupported asserts that if AttachStorageToUnit reports
// the storage name is not supported by the unit's charm the caller gets back a
// per-result error with [params.CodeNotSupported].
func (s *attachSuite) TestAttachStorageNameNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.StorageNameNotSupported)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotSupported,
				Message: `storage "data/0" not supported by the charm of unit "myapp/0"`,
			},
		},
	})
}

// TestAttachStorageInstanceCharmNameMismatch asserts that if AttachStorageToUnit
// reports the storage instance was created for a different charm the caller gets
// back a per-result error with [params.CodeNotValid].
func (s *attachSuite) TestAttachStorageInstanceCharmNameMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.StorageInstanceCharmNameMismatch)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `storage "data/0" was created for a different charm than unit "myapp/0"`,
			},
		},
	})
}

// TestAttachStorageInstanceKindNotValid asserts that if AttachStorageToUnit
// reports the storage instance kind is not compatible with the charm storage
// definition the caller gets back a per-result error with [params.CodeNotValid].
func (s *attachSuite) TestAttachStorageInstanceKindNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.StorageInstanceKindNotValidForCharmStorageDefinition)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `storage "data/0" kind is not compatible with the charm storage definition of unit "myapp/0"`,
			},
		},
	})
}

// TestAttachStorageInstanceSizeNotValid asserts that if AttachStorageToUnit
// reports the storage instance size does not meet the charm minimum the caller
// gets back a per-result error with [params.CodeNotValid].
func (s *attachSuite) TestAttachStorageInstanceSizeNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `storage "data/0" size does not meet the charm storage minimum size requirement of unit "myapp/0"`,
			},
		},
	})
}

// TestAttachStorageCountLimitExceeded asserts that if AttachStorageToUnit
// reports the charm storage maximum count would be exceeded the caller gets
// back a per-result error with [params.CodeNotValid].
func (s *attachSuite) TestAttachStorageCountLimitExceeded(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	max := 3
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.StorageCountLimitExceeded{
		Maximum:     &max,
		Minimum:     1,
		Requested:   4,
		StorageName: "data",
	})

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `attaching storage "data/0" would exceed the maximum count of 3 for storage definition "data" of unit "myapp/0"`,
			},
		},
	})
}

// TestAttachStorageCountLimitExceededFallback asserts that if
// AttachStorageToUnit reports a [applicationerrors.StorageCountLimitExceeded]
// without a maximum set the caller gets back a per-result error with
// [params.CodeNotValid] containing the error's own description.
func (s *attachSuite) TestAttachStorageCountLimitExceededFallback(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.StorageCountLimitExceeded{
		Minimum:     3,
		Requested:   2,
		StorageName: "data",
	})

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `attaching storage "data/0" to unit "myapp/0": storage "data" cannot have less than 3 storage instances`,
			},
		},
	})
}

// TestAttachStorageInstanceAlreadyAttached asserts that if AttachStorageToUnit
// reports the storage instance is already attached to the unit the caller gets
// back a per-result error with [params.CodeAlreadyExists].
func (s *attachSuite) TestAttachStorageInstanceAlreadyAttached(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.StorageInstanceAlreadyAttachedToUnit)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeAlreadyExists,
				Message: `storage "data/0" is already attached to unit "myapp/0"`,
			},
		},
	})
}

// TestAttachStorageSharedAccessNotSupported asserts that if AttachStorageToUnit
// reports the storage instance has existing attachments but the charm storage
// definition does not support shared access the caller gets back a per-result
// error with [params.CodeNotValid].
func (s *attachSuite) TestAttachStorageSharedAccessNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.StorageInstanceAttachSharedAccessNotSupported)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `storage "data/0" already has attachments but the charm storage definition of unit "myapp/0" does not support shared access`,
			},
		},
	})
}

// TestAttachStorageUnexpectedAttachments asserts that if AttachStorageToUnit
// reports the storage instance attachments changed concurrently the caller gets
// back a per-result error with [params.CodeNotValid].
func (s *attachSuite) TestAttachStorageUnexpectedAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.StorageInstanceUnexpectedAttachments)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `storage "data/0" attachments changed while attaching to unit "myapp/0", please retry`,
			},
		},
	})
}

// TestAttachStorageInstanceMachineMismatch asserts that if AttachStorageToUnit
// reports the storage instance is bound to a different machine than the unit
// the caller gets back a per-result error with [params.CodeNotValid].
func (s *attachSuite) TestAttachStorageInstanceMachineMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.StorageInstanceAttachMachineOwnerMismatch)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `storage "data/0" is bound to a different machine than unit "myapp/0"`,
			},
		},
	})
}

// TestAttachUnitCharmChanged asserts that if AttachStorageToUnit reports the
// unit's charm changed concurrently the caller gets back a per-result error
// with [params.CodeNotValid].
func (s *attachSuite) TestAttachUnitCharmChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.UnitCharmChanged)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `unit "myapp/0" charm changed during storage attachment, please retry`,
			},
		},
	})
}

// TestAttachUnitMachineChanged asserts that if AttachStorageToUnit reports the
// unit's machine changed concurrently the caller gets back a per-result error
// with [params.CodeNotValid].
func (s *attachSuite) TestAttachUnitMachineChanged(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID, storageUUID := s.expectAttachPrereqs(c)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID, unitUUID,
	).Return(applicationerrors.UnitMachineChanged)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{
			Error: &params.Error{
				Code:    params.CodeNotValid,
				Message: `unit "myapp/0" machine changed during storage attachment, please retry`,
			},
		},
	})
}

// TestAttachSuccess asserts that a single storage instance is successfully
// attached to a unit.
func (s *attachSuite) TestAttachSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := tc.Must(c, coreunit.NewUUID)
	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	s.expectAttachSuccess(unitUUID, storageUUID)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), attachOneStorageArgs())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{Error: nil},
	})
}

// TestAttachMultipleSuccess asserts that multiple storage instances are
// successfully attached to their respective units in a single call.
func (s *attachSuite) TestAttachMultipleSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID0 := tc.Must(c, coreunit.NewUUID)
	storageUUID0 := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID1 := tc.Must(c, coreunit.NewUUID)
	storageUUID1 := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)
	s.applicationService.EXPECT().GetUnitUUID(
		gomock.Any(), coreunit.Name("myapp/0"),
	).Return(unitUUID0, nil)
	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "data/0",
	).Return(storageUUID0, nil)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID0, unitUUID0,
	).Return(nil)
	s.applicationService.EXPECT().GetUnitUUID(
		gomock.Any(), coreunit.Name("myapp/1"),
	).Return(unitUUID1, nil)
	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "data/1",
	).Return(storageUUID1, nil)
	s.applicationService.EXPECT().AttachStorageToUnit(
		gomock.Any(), storageUUID1, unitUUID1,
	).Return(nil)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.Attach(c.Context(), params.StorageAttachmentIds{
		Ids: []params.StorageAttachmentId{
			{StorageTag: "storage-data/0", UnitTag: "unit-myapp/0"},
			{StorageTag: "storage-data/1", UnitTag: "unit-myapp/1"},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.DeepEquals, []params.ErrorResult{
		{Error: nil},
		{Error: nil},
	})
}
