// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	corelife "github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// storageDetailsSuite is a suite of tests for testing
// [StorageAPI.StorageDetails] facade method.
type storageDetailsSuite struct {
	baseStorageSuite
}

// TestStorageDetailsSuite runs all of the tests contained within
// [storageDetailsSuite].
func TestStorageDetailsSuite(t *testing.T) {
	tc.Run(t, &storageDetailsSuite{})
}

// TestWithReadPermission tests that when a caller has read permission on the
// model they are able to get storage details.
func (s *storageDetailsSuite) TestWithReadPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	currentTime := time.Now().UTC()
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageTag := names.NewStorageTag("pgdata/0")
	unitTag := names.NewUnitTag("foo/1")
	unitUUID := tc.Must(c, coreunit.NewUUID)

	s.storageService.EXPECT().GetStorageInstanceUUIDForID(gomock.Any(), "pgdata/0").Return(
		storageInstanceUUID, nil,
	)
	s.storageService.EXPECT().GetStorageInstanceInfo(
		gomock.Any(), storageInstanceUUID,
	).Return(domainstorage.StorageInstanceInfo{
		FilesystemStatus: &domainstorage.StorageInstanceFilesystemStatus{
			Data: map[string]any{
				"val": 1,
			},
			Message: "message",
			Since:   &currentTime,
			Status:  corestatus.Available,
		},
		ID:         "1",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindFilesystem,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/foo",
				MachineAttachment: &domainstorage.StorageInstanceMachineAttachment{
					MachineName: "1",
					MachineUUID: tc.Must(c, coremachine.NewUUID),
				},
				UnitName: "foo/1",
				UnitUUID: tc.Must(c, coreunit.NewUUID),
			},
		},
		UnitOwner: &domainstorage.StorageInstanceUnitOwner{
			Name: "foo/1",
			UUID: unitUUID,
		},
		UUID: storageInstanceUUID,
	}, nil)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("pgdata/0").String()},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.StorageDetailsResults{
		Results: []params.StorageDetailsResult{
			{
				Result: &params.StorageDetails{
					Attachments: map[string]params.StorageAttachmentDetails{
						unitTag.String(): {
							Life:       corelife.Alive,
							Location:   "/mnt/foo",
							MachineTag: names.NewMachineTag("1").String(),
							StorageTag: storageTag.String(),
							UnitTag:    unitTag.String(),
						},
					},
					Status: params.EntityStatus{
						Data: map[string]any{
							"val": 1,
						},
						Info:   "message",
						Since:  &currentTime,
						Status: corestatus.Available,
					},
					StorageTag: storageTag.String(),
					OwnerTag:   names.NewUnitTag("foo/1").String(),
					Life:       corelife.Alive,
					Kind:       params.StorageKindFilesystem,
					Persistent: false,
				},
			},
		},
	})
}

// TestWithReadPermission tests that when a caller has write permission on the
// model they are able to get storage details.
func (s *storageDetailsSuite) TestWithWritePermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasWriteTag: userTag,
		Tag:         userTag,
	}

	currentTime := time.Now().UTC()
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageTag := names.NewStorageTag("pgdata/0")
	unitTag := names.NewUnitTag("foo/1")
	unitUUID := tc.Must(c, coreunit.NewUUID)

	s.storageService.EXPECT().GetStorageInstanceUUIDForID(gomock.Any(), "pgdata/0").Return(
		storageInstanceUUID, nil,
	)
	s.storageService.EXPECT().GetStorageInstanceInfo(
		gomock.Any(), storageInstanceUUID,
	).Return(domainstorage.StorageInstanceInfo{
		FilesystemStatus: &domainstorage.StorageInstanceFilesystemStatus{
			Data: map[string]any{
				"val": 1,
			},
			Message: "message",
			Since:   &currentTime,
			Status:  corestatus.Available,
		},
		ID:         "1",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindFilesystem,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/foo",
				MachineAttachment: &domainstorage.StorageInstanceMachineAttachment{
					MachineName: "1",
					MachineUUID: tc.Must(c, coremachine.NewUUID),
				},
				UnitName: "foo/1",
				UnitUUID: tc.Must(c, coreunit.NewUUID),
			},
		},
		UnitOwner: &domainstorage.StorageInstanceUnitOwner{
			Name: "foo/1",
			UUID: unitUUID,
		},
		UUID: storageInstanceUUID,
	}, nil)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("pgdata/0").String()},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.StorageDetailsResults{
		Results: []params.StorageDetailsResult{
			{
				Result: &params.StorageDetails{
					Attachments: map[string]params.StorageAttachmentDetails{
						unitTag.String(): {
							Life:       corelife.Alive,
							Location:   "/mnt/foo",
							MachineTag: names.NewMachineTag("1").String(),
							StorageTag: storageTag.String(),
							UnitTag:    unitTag.String(),
						},
					},
					Status: params.EntityStatus{
						Data: map[string]any{
							"val": 1,
						},
						Info:   "message",
						Since:  &currentTime,
						Status: corestatus.Available,
					},
					StorageTag: storageTag.String(),
					OwnerTag:   names.NewUnitTag("foo/1").String(),
					Life:       corelife.Alive,
					Kind:       params.StorageKindFilesystem,
					Persistent: false,
				},
			},
		},
	})
}

// TestWithModelAdminPermission tests that when a caller has write permission on
// the model they are able to get storage details.
func (s *storageDetailsSuite) TestWithModelAdminPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		AdminTag: userTag,
		Tag:      userTag,
	}

	currentTime := time.Now().UTC()
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageTag := names.NewStorageTag("pgdata/0")
	unitTag := names.NewUnitTag("foo/1")
	unitUUID := tc.Must(c, coreunit.NewUUID)

	s.storageService.EXPECT().GetStorageInstanceUUIDForID(gomock.Any(), "pgdata/0").Return(
		storageInstanceUUID, nil,
	)
	s.storageService.EXPECT().GetStorageInstanceInfo(
		gomock.Any(), storageInstanceUUID,
	).Return(domainstorage.StorageInstanceInfo{
		FilesystemStatus: &domainstorage.StorageInstanceFilesystemStatus{
			Data: map[string]any{
				"val": 1,
			},
			Message: "message",
			Since:   &currentTime,
			Status:  corestatus.Available,
		},
		ID:         "1",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindFilesystem,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/foo",
				MachineAttachment: &domainstorage.StorageInstanceMachineAttachment{
					MachineName: "1",
					MachineUUID: tc.Must(c, coremachine.NewUUID),
				},
				UnitName: "foo/1",
				UnitUUID: tc.Must(c, coreunit.NewUUID),
			},
		},
		UnitOwner: &domainstorage.StorageInstanceUnitOwner{
			Name: "foo/1",
			UUID: unitUUID,
		},
		UUID: storageInstanceUUID,
	}, nil)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("pgdata/0").String()},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.StorageDetailsResults{
		Results: []params.StorageDetailsResult{
			{
				Result: &params.StorageDetails{
					Attachments: map[string]params.StorageAttachmentDetails{
						unitTag.String(): {
							Life:       corelife.Alive,
							Location:   "/mnt/foo",
							MachineTag: names.NewMachineTag("1").String(),
							StorageTag: storageTag.String(),
							UnitTag:    unitTag.String(),
						},
					},
					Status: params.EntityStatus{
						Data: map[string]any{
							"val": 1,
						},
						Info:   "message",
						Since:  &currentTime,
						Status: corestatus.Available,
					},
					StorageTag: storageTag.String(),
					OwnerTag:   names.NewUnitTag("foo/1").String(),
					Life:       corelife.Alive,
					Kind:       params.StorageKindFilesystem,
					Persistent: false,
				},
			},
		},
	})
}

// TestWithNoPermission tests that when the caller has no permissions on the
// model they get an unauthorized response back.
func (s *storageDetailsSuite) TestWithNoPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: userTag,
	}

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("pgdata/0").String()},
		},
	})

	// Assert that a params error with CodeUnauthorized and no further results
	// exist in [params.StorageDetailsResults].
	paramsErr, is := errors.AsType[*params.Error](err)
	c.Check(is, tc.IsTrue)
	c.Check(paramsErr.Code, tc.Equals, params.CodeUnauthorized)
	c.Check(result, tc.DeepEquals, params.StorageDetailsResults{})
}

// TestStorageDetailsWithTheSameIDReuse is making sure that if we receive the
// same storage entity id from a caller multiple times we reuse the first
// result instead of going back to the database for the same information
// again. This test also makes sure that results are returned in the order
// requested.
func (s *storageDetailsSuite) TestStorageDetailsWithTheSameIDReuse(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	currentTime := time.Now().UTC()
	storageInstanceUUID1 := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageInstanceUUID2 := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageTag1 := names.NewStorageTag("pgdata/0")
	storageTag2 := names.NewStorageTag("pgdata/1")
	unitTag1 := names.NewUnitTag("foo/1")
	unitTag2 := names.NewUnitTag("foo/2")
	unitUUID1 := tc.Must(c, coreunit.NewUUID)
	unitUUID2 := tc.Must(c, coreunit.NewUUID)

	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "pgdata/0").Return(
		storageInstanceUUID1, nil,
	)
	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "pgdata/1").Return(
		storageInstanceUUID2, nil,
	)

	// Storage Instance 1
	s.storageService.EXPECT().GetStorageInstanceInfo(
		gomock.Any(), storageInstanceUUID1,
	).Return(domainstorage.StorageInstanceInfo{
		FilesystemStatus: &domainstorage.StorageInstanceFilesystemStatus{
			Data: map[string]any{
				"val": 1,
			},
			Message: "message",
			Since:   &currentTime,
			Status:  corestatus.Available,
		},
		ID:         "1",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindFilesystem,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/foo",
				MachineAttachment: &domainstorage.StorageInstanceMachineAttachment{
					MachineName: "1",
					MachineUUID: tc.Must(c, coremachine.NewUUID),
				},
				UnitName: "foo/1",
				UnitUUID: tc.Must(c, coreunit.NewUUID),
			},
		},
		UnitOwner: &domainstorage.StorageInstanceUnitOwner{
			Name: "foo/1",
			UUID: unitUUID1,
		},
		UUID: storageInstanceUUID1,
	}, nil)

	// Storage Instance 2
	s.storageService.EXPECT().GetStorageInstanceInfo(
		gomock.Any(), storageInstanceUUID2,
	).Return(domainstorage.StorageInstanceInfo{
		FilesystemStatus: &domainstorage.StorageInstanceFilesystemStatus{
			Data: map[string]any{
				"val": 1,
			},
			Message: "message",
			Since:   &currentTime,
			Status:  corestatus.Available,
		},
		ID:         "2",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindFilesystem,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/foo",
				MachineAttachment: &domainstorage.StorageInstanceMachineAttachment{
					MachineName: "2",
					MachineUUID: tc.Must(c, coremachine.NewUUID),
				},
				UnitName: "foo/2",
				UnitUUID: tc.Must(c, coreunit.NewUUID),
			},
		},
		UnitOwner: &domainstorage.StorageInstanceUnitOwner{
			Name: "foo/2",
			UUID: unitUUID2,
		},
		UUID: storageInstanceUUID2,
	}, nil)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("pgdata/0").String()},
			{Tag: names.NewStorageTag("pgdata/1").String()},
			{Tag: names.NewStorageTag("pgdata/0").String()}, // Repeat
		},
	})

	c.Check(err, tc.ErrorIsNil)
	// We are also looking to see here that results are returned in the same
	// order requested.
	c.Check(result, tc.DeepEquals, params.StorageDetailsResults{
		Results: []params.StorageDetailsResult{
			// Storage Instance 1
			{
				Result: &params.StorageDetails{
					Attachments: map[string]params.StorageAttachmentDetails{
						unitTag1.String(): {
							Life:       corelife.Alive,
							Location:   "/mnt/foo",
							MachineTag: names.NewMachineTag("1").String(),
							StorageTag: storageTag1.String(),
							UnitTag:    unitTag1.String(),
						},
					},
					Status: params.EntityStatus{
						Data: map[string]any{
							"val": 1,
						},
						Info:   "message",
						Since:  &currentTime,
						Status: corestatus.Available,
					},
					StorageTag: storageTag1.String(),
					OwnerTag:   names.NewUnitTag("foo/1").String(),
					Life:       corelife.Alive,
					Kind:       params.StorageKindFilesystem,
					Persistent: false,
				},
			},
			// Storage Instance 2
			{
				Result: &params.StorageDetails{
					Attachments: map[string]params.StorageAttachmentDetails{
						unitTag2.String(): {
							Life:       corelife.Alive,
							Location:   "/mnt/foo",
							MachineTag: names.NewMachineTag("2").String(),
							StorageTag: storageTag2.String(),
							UnitTag:    unitTag2.String(),
						},
					},
					Status: params.EntityStatus{
						Data: map[string]any{
							"val": 1,
						},
						Info:   "message",
						Since:  &currentTime,
						Status: corestatus.Available,
					},
					StorageTag: storageTag2.String(),
					OwnerTag:   names.NewUnitTag("foo/2").String(),
					Life:       corelife.Alive,
					Kind:       params.StorageKindFilesystem,
					Persistent: false,
				},
			},
			// Storage Instance 1
			{
				Result: &params.StorageDetails{
					Attachments: map[string]params.StorageAttachmentDetails{
						unitTag1.String(): {
							Life:       corelife.Alive,
							Location:   "/mnt/foo",
							MachineTag: names.NewMachineTag("1").String(),
							StorageTag: storageTag1.String(),
							UnitTag:    unitTag1.String(),
						},
					},
					Status: params.EntityStatus{
						Data: map[string]any{
							"val": 1,
						},
						Info:   "message",
						Since:  &currentTime,
						Status: corestatus.Available,
					},
					StorageTag: storageTag1.String(),
					OwnerTag:   names.NewUnitTag("foo/1").String(),
					Life:       corelife.Alive,
					Kind:       params.StorageKindFilesystem,
					Persistent: false,
				},
			},
		},
	})
}

// TestStorageDetailsFilesystemNoStatusSet is testing that when no filesystem
// status is provided the returned params to the caller as at least the status
// since time filled out to unix 0 time. This must happen to avoid panics in the
// Juju client.
func (s *storageDetailsSuite) TestStorageDetailsFilesystemNoStatusSet(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	zeroTime := time.UnixMicro(0).UTC()
	storageInstanceUUID1 := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageTag1 := names.NewStorageTag("pgdata/0")
	unitTag1 := names.NewUnitTag("foo/1")
	unitUUID1 := tc.Must(c, coreunit.NewUUID)

	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "pgdata/0").Return(
		storageInstanceUUID1, nil,
	)

	// Storage Instance 1
	s.storageService.EXPECT().GetStorageInstanceInfo(
		gomock.Any(), storageInstanceUUID1,
	).Return(domainstorage.StorageInstanceInfo{
		ID:         "1",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindFilesystem,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/foo",
				MachineAttachment: &domainstorage.StorageInstanceMachineAttachment{
					MachineName: "1",
					MachineUUID: tc.Must(c, coremachine.NewUUID),
				},
				UnitName: "foo/1",
				UnitUUID: tc.Must(c, coreunit.NewUUID),
			},
		},
		UnitOwner: &domainstorage.StorageInstanceUnitOwner{
			Name: "foo/1",
			UUID: unitUUID1,
		},
		UUID: storageInstanceUUID1,
	}, nil)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("pgdata/0").String()},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.StorageDetailsResults{
		Results: []params.StorageDetailsResult{
			// Storage Instance 1
			{
				Result: &params.StorageDetails{
					Attachments: map[string]params.StorageAttachmentDetails{
						unitTag1.String(): {
							Life:       corelife.Alive,
							Location:   "/mnt/foo",
							MachineTag: names.NewMachineTag("1").String(),
							StorageTag: storageTag1.String(),
							UnitTag:    unitTag1.String(),
						},
					},
					Status: params.EntityStatus{
						Since: &zeroTime,
					},
					StorageTag: storageTag1.String(),
					OwnerTag:   names.NewUnitTag("foo/1").String(),
					Life:       corelife.Alive,
					Kind:       params.StorageKindFilesystem,
					Persistent: false,
				},
			},
		},
	})
}

// TestStorageDetailsFilesystemNoStatusSet is testing that when no filesystem
// status is provided the returned params to the caller as at least the status
// since time filled out to unix 0 time. This must happen to avoid panics in the
// Juju client.
func (s *storageDetailsSuite) TestStorageDetailsVolumeNoStatusSet(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	zeroTime := time.UnixMicro(0).UTC()
	storageInstanceUUID1 := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageTag1 := names.NewStorageTag("pgdata/0")
	unitTag1 := names.NewUnitTag("foo/1")
	unitUUID1 := tc.Must(c, coreunit.NewUUID)

	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "pgdata/0").Return(
		storageInstanceUUID1, nil,
	)

	// Storage Instance 1
	s.storageService.EXPECT().GetStorageInstanceInfo(
		gomock.Any(), storageInstanceUUID1,
	).Return(domainstorage.StorageInstanceInfo{
		ID:         "1",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindBlock,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/foo",
				MachineAttachment: &domainstorage.StorageInstanceMachineAttachment{
					MachineName: "1",
					MachineUUID: tc.Must(c, coremachine.NewUUID),
				},
				UnitName: "foo/1",
				UnitUUID: tc.Must(c, coreunit.NewUUID),
			},
		},
		UnitOwner: &domainstorage.StorageInstanceUnitOwner{
			Name: "foo/1",
			UUID: unitUUID1,
		},
		UUID: storageInstanceUUID1,
	}, nil)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("pgdata/0").String()},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.StorageDetailsResults{
		Results: []params.StorageDetailsResult{
			// Storage Instance 1
			{
				Result: &params.StorageDetails{
					Attachments: map[string]params.StorageAttachmentDetails{
						unitTag1.String(): {
							Life:       corelife.Alive,
							Location:   "/mnt/foo",
							MachineTag: names.NewMachineTag("1").String(),
							StorageTag: storageTag1.String(),
							UnitTag:    unitTag1.String(),
						},
					},
					Status: params.EntityStatus{
						Since: &zeroTime,
					},
					StorageTag: storageTag1.String(),
					OwnerTag:   names.NewUnitTag("foo/1").String(),
					Life:       corelife.Alive,
					Kind:       params.StorageKindBlock,
					Persistent: false,
				},
			},
		},
	})
}

// TestStorageDetailsInvalidTag tests that when supplied an invalid tag the
// caller gets back an error with a params code set to [params.CodeNotValid].
func (s *storageDetailsSuite) TestStorageDetailsInvalidTag(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("pgdata/0").String()}, // Valid tag
			{Tag: "invalid-tag-rubbish"},                    // Invalid tag
		},
	})

	paramsError, is := errors.AsType[*params.Error](err)
	c.Check(is, tc.IsTrue)
	c.Check(paramsError.Code, tc.Equals, params.CodeNotValid)
	c.Check(result, tc.DeepEquals, params.StorageDetailsResults{})
}

// TestStorageDetailsInvalidTagKind tests that when supplied an invalid tag kind
// the caller gets back an error with a params code set to [params.CodeNotValid].
func (s *storageDetailsSuite) TestStorageDetailsInvalidTagKind(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("pgdata/0").String()}, // Valid tag
			{Tag: names.NewFilesystemTag("1").String()},     // Invalid tag kind
		},
	})

	paramsError, is := errors.AsType[*params.Error](err)
	c.Check(is, tc.IsTrue)
	c.Check(paramsError.Code, tc.Equals, params.CodeNotValid)
	c.Check(result, tc.DeepEquals, params.StorageDetailsResults{})
}

// TestStorageDetailsStorageIDNotFound tests that when the supplied storage id
// is not found by the service layer a valid [params.CodeNotFound] error is
// returned in the result.
func (s *storageDetailsSuite) TestStorageDetailsStorageIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "pgdata/0",
	).Return("", domainstorageerrors.StorageInstanceNotFound)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("pgdata/0").String()}, // Valid tag
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

// TestStorageDetailsStorageUUIDNotFound tests that when the supplied storage id
// is translated to the storage instance uuid but the Storage Instance uuid is
// no longer found a valid [params.CodeNotFound] error is returned in the result.
func (s *storageDetailsSuite) TestStorageDetailsStorageUUIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "pgdata/0",
	).Return(storageInstanceUUID, nil)
	s.storageService.EXPECT().GetStorageInstanceInfo(
		gomock.Any(), storageInstanceUUID,
	).Return(
		domainstorage.StorageInstanceInfo{},
		domainstorageerrors.StorageInstanceNotFound,
	)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("pgdata/0").String()}, // Valid tag
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

// TestStorageDetailsMixedResults is a happy path test to show that when
// multiple Storage Instance details are requested containing duplicates, not
// found Storage Instances and valid Storage Instances the result set correctly
// contains the valid composition.
//
// This is a happy path test of many scenarios playing out together.
func (s *storageDetailsSuite) TestStorageDetailsMixedResults(c *tc.C) {
	defer s.setupMocks(c).Finish()
	userTag := names.NewUserTag("tlm")
	s.authorizer = apiservertesting.FakeAuthorizer{
		HasReadTag: userTag,
		Tag:        userTag,
	}

	currentTime := time.Now().UTC()
	storageInstanceUUID1 := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageInstanceUUID2 := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	// Not found Storage Instance
	storageInstanceUUID3 := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageTag1 := names.NewStorageTag("pgdata/0")
	storageTag2 := names.NewStorageTag("pgdata/1")
	unitTag1 := names.NewUnitTag("foo/1")
	unitTag2 := names.NewUnitTag("foo/2")
	unitUUID1 := tc.Must(c, coreunit.NewUUID)
	unitUUID2 := tc.Must(c, coreunit.NewUUID)

	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "pgdata/0").Return(
		storageInstanceUUID1, nil,
	)
	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "pgdata/1").Return(
		storageInstanceUUID2, nil,
	)
	s.storageService.EXPECT().GetStorageInstanceUUIDForID(
		gomock.Any(), "pgdata/2").Return(
		storageInstanceUUID3, nil,
	)

	// Storage Instance 1
	s.storageService.EXPECT().GetStorageInstanceInfo(
		gomock.Any(), storageInstanceUUID1,
	).Return(domainstorage.StorageInstanceInfo{
		FilesystemStatus: &domainstorage.StorageInstanceFilesystemStatus{
			Data: map[string]any{
				"val": 1,
			},
			Message: "message",
			Since:   &currentTime,
			Status:  corestatus.Available,
		},
		ID:         "1",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindFilesystem,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/foo",
				MachineAttachment: &domainstorage.StorageInstanceMachineAttachment{
					MachineName: "1",
					MachineUUID: tc.Must(c, coremachine.NewUUID),
				},
				UnitName: "foo/1",
				UnitUUID: tc.Must(c, coreunit.NewUUID),
			},
		},
		UnitOwner: &domainstorage.StorageInstanceUnitOwner{
			Name: "foo/1",
			UUID: unitUUID1,
		},
		UUID: storageInstanceUUID1,
	}, nil)

	// Storage Instance 2
	s.storageService.EXPECT().GetStorageInstanceInfo(
		gomock.Any(), storageInstanceUUID2,
	).Return(domainstorage.StorageInstanceInfo{
		FilesystemStatus: &domainstorage.StorageInstanceFilesystemStatus{
			Data: map[string]any{
				"val": 1,
			},
			Message: "message",
			Since:   &currentTime,
			Status:  corestatus.Available,
		},
		ID:         "2",
		Life:       domainlife.Alive,
		Kind:       domainstorage.StorageKindFilesystem,
		Persistent: false,
		UnitAttachments: []domainstorage.StorageInstanceUnitAttachment{
			{
				Life:     domainlife.Alive,
				Location: "/mnt/foo",
				MachineAttachment: &domainstorage.StorageInstanceMachineAttachment{
					MachineName: "2",
					MachineUUID: tc.Must(c, coremachine.NewUUID),
				},
				UnitName: "foo/2",
				UnitUUID: tc.Must(c, coreunit.NewUUID),
			},
		},
		UnitOwner: &domainstorage.StorageInstanceUnitOwner{
			Name: "foo/2",
			UUID: unitUUID2,
		},
		UUID: storageInstanceUUID2,
	}, nil)

	// Storage Instance 3
	s.storageService.EXPECT().GetStorageInstanceInfo(
		gomock.Any(), storageInstanceUUID3,
	).Return(
		domainstorage.StorageInstanceInfo{},
		domainstorageerrors.StorageInstanceNotFound,
	)

	api := s.makeTestAPIForIAASModel(c)
	result, err := api.StorageDetails(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewStorageTag("pgdata/0").String()},
			{Tag: names.NewStorageTag("pgdata/2").String()}, // Not found
			{Tag: names.NewStorageTag("pgdata/1").String()},
			{Tag: names.NewStorageTag("pgdata/0").String()}, // Repeat
		},
	})

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.Results[1].Error.Message", tc.Ignore)

	c.Check(err, tc.ErrorIsNil)
	// We are also looking to see here that results are returned in the same
	// order requested.
	c.Check(result, mc, params.StorageDetailsResults{
		Results: []params.StorageDetailsResult{
			// Storage Instance 1
			{
				Result: &params.StorageDetails{
					Attachments: map[string]params.StorageAttachmentDetails{
						unitTag1.String(): {
							Life:       corelife.Alive,
							Location:   "/mnt/foo",
							MachineTag: names.NewMachineTag("1").String(),
							StorageTag: storageTag1.String(),
							UnitTag:    unitTag1.String(),
						},
					},
					Status: params.EntityStatus{
						Data: map[string]any{
							"val": 1,
						},
						Info:   "message",
						Since:  &currentTime,
						Status: corestatus.Available,
					},
					StorageTag: storageTag1.String(),
					OwnerTag:   names.NewUnitTag("foo/1").String(),
					Life:       corelife.Alive,
					Kind:       params.StorageKindFilesystem,
					Persistent: false,
				},
			},
			// Storage Instance 3 Not Found
			{
				Error: &params.Error{
					Code: params.CodeNotFound,
				},
			},
			// Storage Instance 2
			{
				Result: &params.StorageDetails{
					Attachments: map[string]params.StorageAttachmentDetails{
						unitTag2.String(): {
							Life:       corelife.Alive,
							Location:   "/mnt/foo",
							MachineTag: names.NewMachineTag("2").String(),
							StorageTag: storageTag2.String(),
							UnitTag:    unitTag2.String(),
						},
					},
					Status: params.EntityStatus{
						Data: map[string]any{
							"val": 1,
						},
						Info:   "message",
						Since:  &currentTime,
						Status: corestatus.Available,
					},
					StorageTag: storageTag2.String(),
					OwnerTag:   names.NewUnitTag("foo/2").String(),
					Life:       corelife.Alive,
					Kind:       params.StorageKindFilesystem,
					Persistent: false,
				},
			},
			// Storage Instance 1
			{
				Result: &params.StorageDetails{
					Attachments: map[string]params.StorageAttachmentDetails{
						unitTag1.String(): {
							Life:       corelife.Alive,
							Location:   "/mnt/foo",
							MachineTag: names.NewMachineTag("1").String(),
							StorageTag: storageTag1.String(),
							UnitTag:    unitTag1.String(),
						},
					},
					Status: params.EntityStatus{
						Data: map[string]any{
							"val": 1,
						},
						Info:   "message",
						Since:  &currentTime,
						Status: corestatus.Available,
					},
					StorageTag: storageTag1.String(),
					OwnerTag:   names.NewUnitTag("foo/1").String(),
					Life:       corelife.Alive,
					Kind:       params.StorageKindFilesystem,
					Persistent: false,
				},
			},
		},
	})
}
