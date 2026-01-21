// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"
	time "time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// storageDetachSuite provides a suite of tests for asserting the functionality
// behind detaching storage from a unit.
type storageDetachSuite struct {
	baseStorageSuite
}

// TestStorageDetachSuite registers and runs all the tests from
// [storageDetachSuite].
func TestStorageDetachSuite(t *testing.T) {
	tc.Run(t, &storageDetachSuite{})
}

// TestWithModelWritePermission tests that a user with write permission on the
// model is allowed to detach storage.
func (s *storageDetachSuite) TestWithModelWritePermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasWriteTag: userTag,
		Tag:         userTag,
	}

	storageAttachmentUUID := tc.Must(c, domainstorageprovisioning.NewStorageAttachmentUUID)
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	appExp := s.applicationService.EXPECT()
	appExp.GetUnitUUID(gomock.Any(), coreunit.Name("myapp/0")).Return(
		unitUUID, nil,
	).AnyTimes()

	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(gomock.Any(), "data/1").Return(
		storageInstUUID, nil,
	).AnyTimes()
	storageExp.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		gomock.Any(), storageInstUUID, unitUUID,
	).Return(storageAttachmentUUID, nil).AnyTimes()

	removalEXP := s.removalService.EXPECT()
	removalEXP.RemoveStorageAttachment(
		gomock.Any(), storageAttachmentUUID, false, time.Duration(0),
	).Return("123", nil)

	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{
				{
					StorageTag: "storage-data/1",
					UnitTag:    "unit-myapp/0",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
}

// TestWithModelAdminPermission tests that a user with admin permission on the
// model is allowed to detach storage.
func (s *storageDetachSuite) TestWithModelAdminPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		AdminTag: userTag,
		Tag:      userTag,
	}

	storageAttachmentUUID := tc.Must(c, domainstorageprovisioning.NewStorageAttachmentUUID)
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	appExp := s.applicationService.EXPECT()
	appExp.GetUnitUUID(gomock.Any(), coreunit.Name("myapp/0")).Return(
		unitUUID, nil,
	).AnyTimes()

	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(gomock.Any(), "data/1").Return(
		storageInstUUID, nil,
	).AnyTimes()
	storageExp.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		gomock.Any(), storageInstUUID, unitUUID,
	).Return(storageAttachmentUUID, nil).AnyTimes()

	removalEXP := s.removalService.EXPECT()
	removalEXP.RemoveStorageAttachment(
		gomock.Any(), storageAttachmentUUID, false, time.Duration(0),
	).Return("123", nil)

	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{
				{
					StorageTag: "storage-data/1",
					UnitTag:    "unit-myapp/0",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
}

// TestWithReadPermissionFails tests that if the caller has model read
// permission they are unable to detach storage. The caller MUST get back an
// error with [params.CodeUnauthorized] set.
func (s *storageDetachSuite) TestWithReadPermissionFails(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{
				{
					StorageTag: "storage-data/1",
					UnitTag:    "unit-myapp/0",
				},
			},
		},
	})
	paramsErr, is := errors.AsType[*params.Error](err)
	c.Assert(is, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeUnauthorized)
	c.Check(result.Results, tc.HasLen, 0)
}

// TestWithNoPermissionFails tests that if the caller has no model permissions
// they are unable to detach storage. The caller MUST get back an error with
// [params.CodeUnauthorized] set.
func (s *storageDetachSuite) TestWithNoPermissionFails(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := tc.Must1(c, names.ParseUserTag, "user-tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: userTag,
	}

	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{
				{
					StorageTag: "storage-data/1",
					UnitTag:    "unit-myapp/0",
				},
			},
		},
	})
	paramsErr, is := errors.AsType[*params.Error](err)
	c.Assert(is, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeUnauthorized)
	c.Check(result.Results, tc.HasLen, 0)
}

// TestNegativeMaxWaitTime asserts that if the user supplies a negative max
// wait duration they get back an error satisfying [coreerrors.NotValid].
func (s *storageDetachSuite) TestNegativeMaxWaitTime(c *tc.C) {
	defer s.setupMocks(c).Finish()

	api := s.makeTestAPI(c)
	negativeMaxWait := time.Duration(-5)
	_, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		MaxWait: &negativeMaxWait,
	})
	perr, is := errors.AsType[*params.Error](err)
	c.Assert(is, tc.Equals, true)
	c.Check(perr.Code, tc.Equals, params.CodeNotValid)
}

// TestDetachStorageNoIDs assert calling DetachStorage with no storage ids
// performs no action and returns an empty result.
func (s *storageDetachSuite) TestDetachStorageNoIDs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 0)
}

// TestDetachStorageUnitNotFound asserts that if the application service reports
// that a unit is not found the callers gets back a [params.CodeNotFound] error.
func (s *storageDetachSuite) TestDetachStorageUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appExp := s.applicationService.EXPECT()
	appExp.GetUnitUUID(gomock.Any(), coreunit.Name("myapp/0")).Return(
		"", applicationerrors.UnitNotFound,
	)

	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{
				{
					StorageTag: "storage-data/1",
					UnitTag:    "unit-myapp/0",
				},
			},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

// TestDetachStorageInstanceNotFound asserts that if the application service
// reports that a unit is not found the callers gets back a
// [params.CodeNotFound] error.
func (s *storageDetachSuite) TestDetachStorageInstanceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	appExp := s.applicationService.EXPECT()
	appExp.GetUnitUUID(gomock.Any(), coreunit.Name("myapp/0")).Return(
		"", nil,
	).AnyTimes()
	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(gomock.Any(), "data/1").Return(
		"", storageerrors.StorageInstanceNotFound,
	)

	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{
				{
					StorageTag: "storage-data/1",
					UnitTag:    "unit-myapp/0",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

// TestDetachStorageAttachmentNotFound asserts that if the application service
// reports that a unit is not found the callers gets back a
// [params.CodeNotFound] error.
func (s *storageDetachSuite) TestDetachStorageAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	appExp := s.applicationService.EXPECT()
	appExp.GetUnitUUID(gomock.Any(), coreunit.Name("myapp/0")).Return(
		unitUUID, nil,
	).AnyTimes()
	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(gomock.Any(), "data/1").Return(
		storageInstUUID, nil,
	).AnyTimes()
	storageExp.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		gomock.Any(), storageInstUUID, unitUUID,
	).Return("", storageerrors.StorageAttachmentNotFound)

	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{
				{
					StorageTag: "storage-data/1",
					UnitTag:    "unit-myapp/0",
				},
			},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

// TestDetachStorageAttachmentUnitStorageViolation asserts that if a storage
// attachment being removed from a unit violates the minimum storage constraints
// of the unit the caller gets back a [params.CodeNotValid] error.
func (s *storageDetachSuite) TestDetachStorageAttachmentUnitStorageViolation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageAttachmentUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	appExp := s.applicationService.EXPECT()
	appExp.GetUnitUUID(gomock.Any(), coreunit.Name("myapp/0")).Return(
		unitUUID, nil,
	).AnyTimes()

	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(gomock.Any(), "data/1").Return(
		storageInstUUID, nil,
	).AnyTimes()
	storageExp.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		gomock.Any(), storageInstUUID, unitUUID,
	).Return(storageAttachmentUUID, nil).AnyTimes()

	removalEXP := s.removalService.EXPECT()
	removalEXP.RemoveStorageAttachment(
		gomock.Any(), storageAttachmentUUID, false, time.Duration(0),
	).Return("", applicationerrors.UnitStorageMinViolation{
		CharmStorageName: "data",
		RequiredMinimum:  1,
		UnitUUID:         unitUUID.String(),
	})

	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{
				{
					StorageTag: "storage-data/1",
					UnitTag:    "unit-myapp/0",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error.Code, tc.Equals, params.CodeNotValid)
}

func (s *storageDetachSuite) TestDetachStorageAttachment(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageAttachmentUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	appExp := s.applicationService.EXPECT()
	appExp.GetUnitUUID(gomock.Any(), coreunit.Name("myapp/0")).Return(
		unitUUID, nil,
	).AnyTimes()

	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(gomock.Any(), "data/1").Return(
		storageInstUUID, nil,
	).AnyTimes()
	storageExp.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		gomock.Any(), storageInstUUID, unitUUID,
	).Return(storageAttachmentUUID, nil).AnyTimes()

	removalEXP := s.removalService.EXPECT()
	removalEXP.RemoveStorageAttachment(
		gomock.Any(), storageAttachmentUUID, false, time.Duration(0),
	).Return("123", nil)

	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{
				{
					StorageTag: "storage-data/1",
					UnitTag:    "unit-myapp/0",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
}

func (s *storageDetachSuite) TestDetachStorageAttachmentWithForceAndWait(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageAttachmentUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	appExp := s.applicationService.EXPECT()
	appExp.GetUnitUUID(gomock.Any(), coreunit.Name("myapp/0")).Return(
		unitUUID, nil,
	).AnyTimes()

	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(gomock.Any(), "data/1").Return(
		storageInstUUID, nil,
	).AnyTimes()
	storageExp.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		gomock.Any(), storageInstUUID, unitUUID,
	).Return(storageAttachmentUUID, nil).AnyTimes()

	removalEXP := s.removalService.EXPECT()
	removalEXP.RemoveStorageAttachment(
		gomock.Any(), storageAttachmentUUID, true, time.Minute,
	).Return("123", nil)

	var (
		force = true
		wait  = time.Minute
	)
	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		Force:   &force,
		MaxWait: &wait,
		StorageIds: params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{
				{
					StorageTag: "storage-data/1",
					UnitTag:    "unit-myapp/0",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
}

// TestDetachStorageAllAttachments asserts that if the caller only supplies a
// storage id to remove the storage is detached from all units.
func (s *storageDetachSuite) TestDetachStorageAllAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageAttachmentUUID1 := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	storageAttachmentUUID2 := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(gomock.Any(), "data/1").Return(
		storageInstUUID, nil,
	).AnyTimes()
	storageExp.GetStorageInstanceAttachments(
		gomock.Any(), storageInstUUID,
	).Return([]domainstorage.StorageAttachmentUUID{
		storageAttachmentUUID1, storageAttachmentUUID2,
	}, nil)

	// We want to see two removals occur
	removalEXP := s.removalService.EXPECT()
	removalEXP.RemoveStorageAttachment(
		gomock.Any(), storageAttachmentUUID1, false, time.Duration(0),
	).Return("123", nil)
	removalEXP.RemoveStorageAttachment(
		gomock.Any(), storageAttachmentUUID2, false, time.Duration(0),
	).Return("124", nil)

	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{
				{
					StorageTag: "storage-data/1",
				},
			},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
}

// TestDetachStorageAllAttachmentsEmpty asserts that if the caller only supplies
// a storage id to detached and the storage is not attached to anything the
// operation results in a noop.
func (s *storageDetachSuite) TestDetachStorageAllAttachmentsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	storageExp := s.storageService.EXPECT()
	storageExp.GetStorageInstanceUUIDForID(gomock.Any(), "data/1").Return(
		storageInstUUID, nil,
	).AnyTimes()
	storageExp.GetStorageInstanceAttachments(
		gomock.Any(), storageInstUUID,
	).Return([]domainstorage.StorageAttachmentUUID{}, nil)

	api := s.makeTestAPI(c)
	result, err := api.DetachStorage(c.Context(), params.StorageDetachmentParams{
		StorageIds: params.StorageAttachmentIds{
			Ids: []params.StorageAttachmentId{
				{
					StorageTag: "storage-data/1",
				},
			},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
}
