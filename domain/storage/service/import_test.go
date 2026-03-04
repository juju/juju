// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storage/internal"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
)

// importSuite is a set of tests to assert the interface and contracts
// importing storage into this state package.
type importSuite struct {
	testhelpers.IsolationSuite

	service *Service

	registry internalstorage.StaticProviderRegistry
	state    *MockState
}

// TestImportSuite runs all of the tests contained in
// [importSuite].
func TestImportSuite(t *stdtesting.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImportStorageInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unit2UUID := tc.Must(c, coreunit.NewUUID).String()
	unit3UUID := tc.Must(c, coreunit.NewUUID).String()

	expectedInstances := []internal.ImportStorageInstanceArgs{
		{
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/0",
			UnitUUID:          unit3UUID,
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
		}, {
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/2",
			UnitUUID:          unit2UUID,
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
		},
	}

	s.state.EXPECT().GetUnitUUIDsByNames(gomock.Any(), tc.Bind(tc.SameContents, []string{"unit/2", "unit/3"})).Return(map[string]string{
		"unit/2": unit2UUID,
		"unit/3": unit3UUID,
	}, nil)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].UUID`, tc.IsNonZeroUUID)
	s.state.EXPECT().ImportStorageInstances(
		gomock.Any(),
		tc.Bind(mc, expectedInstances),
		tc.Bind(tc.HasLen, 0),
	).Return(nil)

	args := []domainstorage.ImportStorageInstanceParams{
		{
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/0",
			UnitName:          "unit/3",
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
		}, {
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/2",
			UnitName:          "unit/2",
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
		},
	}

	// Act
	err := s.service.ImportStorageInstances(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportStorageInstancesWithNoUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	expectedInstances := []internal.ImportStorageInstanceArgs{
		{
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/0",
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
		},
	}

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].UUID`, tc.IsNonZeroUUID)
	s.state.EXPECT().ImportStorageInstances(
		gomock.Any(),
		tc.Bind(mc, expectedInstances),
		tc.Bind(tc.HasLen, 0),
	).Return(nil)

	args := []domainstorage.ImportStorageInstanceParams{
		{
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/0",
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
		},
	}

	// Act
	err := s.service.ImportStorageInstances(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportStorageInstancesMissingUnitOwner(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unit0UUID := tc.Must(c, coreunit.NewUUID).String()

	// No "unit/1" in the returned map. This indicates that "unit/1" does not exist
	s.state.EXPECT().GetUnitUUIDsByNames(gomock.Any(), tc.Bind(tc.SameContents, []string{"unit/0", "unit/1"})).Return(map[string]string{
		"unit/0": unit0UUID,
	}, nil)

	args := []domainstorage.ImportStorageInstanceParams{
		{
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/0",
			UnitName:          "unit/0",
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
		}, {
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/2",
			UnitName:          "unit/1",
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
		},
	}

	// Act
	err := s.service.ImportStorageInstances(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *importSuite) TestImportStorageInstancesMissingUnitAttachment(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unit0UUID := tc.Must(c, coreunit.NewUUID).String()

	// No "unit/1" in the returned map. This indicates that "unit/1" does not exist
	s.state.EXPECT().GetUnitUUIDsByNames(gomock.Any(), tc.Bind(tc.SameContents, []string{"unit/0", "unit/1"})).Return(map[string]string{
		"unit/0": unit0UUID,
	}, nil)

	args := []domainstorage.ImportStorageInstanceParams{
		{
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/0",
			UnitName:          "unit/0",
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
			AttachedUnitNames: []string{"unit/1"},
		},
	}

	// Act
	err := s.service.ImportStorageInstances(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *importSuite) TestImportStorageInstancesWithAttachments(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unit0UUID := tc.Must(c, coreunit.NewUUID).String()
	unit1UUID := tc.Must(c, coreunit.NewUUID).String()
	unit2UUID := tc.Must(c, coreunit.NewUUID).String()
	unit3UUID := tc.Must(c, coreunit.NewUUID).String()

	// Arrange
	expectedInstances := []internal.ImportStorageInstanceArgs{
		{
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/0",
			UnitUUID:          unit3UUID,
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
		}, {
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/2",
			UnitUUID:          unit2UUID,
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
		},
	}
	expectedAttachments := []internal.ImportStorageInstanceAttachmentArgs{
		{
			UnitUUID: unit0UUID,
			Life:     life.Alive,
		}, {
			UnitUUID: unit1UUID,
			Life:     life.Alive,
		}, {
			UnitUUID: unit2UUID,
			Life:     life.Alive,
		},
	}

	unitNames := []string{"unit/0", "unit/1", "unit/2", "unit/3"}
	s.state.EXPECT().GetUnitUUIDsByNames(gomock.Any(), tc.Bind(tc.SameContents, unitNames)).Return(map[string]string{
		"unit/0": unit0UUID,
		"unit/1": unit1UUID,
		"unit/2": unit2UUID,
		"unit/3": unit3UUID,
	}, nil)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].UUID`, tc.IsNonZeroUUID)
	mc.AddExpr(`_[_].StorageInstanceUUID`, tc.IsNonZeroUUID)
	s.state.EXPECT().ImportStorageInstances(
		gomock.Any(),
		tc.Bind(mc, expectedInstances),
		tc.Bind(mc, expectedAttachments),
	).DoAndReturn(func(_ context.Context, gotInstances []internal.ImportStorageInstanceArgs, gotAttachments []internal.ImportStorageInstanceAttachmentArgs) error {
		c.Assert(gotInstances, tc.HasLen, 2)
		c.Assert(gotAttachments, tc.HasLen, 3)
		c.Check(gotAttachments[0].StorageInstanceUUID, tc.Equals, gotInstances[0].UUID)
		c.Check(gotAttachments[1].StorageInstanceUUID, tc.Equals, gotInstances[0].UUID)
		c.Check(gotAttachments[2].StorageInstanceUUID, tc.Equals, gotInstances[1].UUID)
		return nil
	})

	args := []domainstorage.ImportStorageInstanceParams{
		{
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/0",
			UnitName:          "unit/3",
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
			AttachedUnitNames: []string{"unit/0", "unit/1"},
		}, {
			StorageName:       "test1",
			StorageKind:       "block",
			StorageInstanceID: "test1/2",
			UnitName:          "unit/2",
			RequestedSizeMiB:  1024,
			PoolName:          "ebs",
			AttachedUnitNames: []string{"unit/2"},
		},
	}

	// Act
	err := s.service.ImportStorageInstances(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportStorageInstancesValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	args := []domainstorage.ImportStorageInstanceParams{
		{
			// There is not StorageID.
			StorageName:      "test1",
			StorageKind:      "block",
			UnitName:         "unit/2",
			RequestedSizeMiB: uint64(1024),
			PoolName:         "ebs",
		},
	}

	// Act
	err := s.service.ImportStorageInstances(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportFilesystemsIAAS(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	params := []domainstorage.ImportFilesystemParams{{
		ID:                "test-1/0",
		PoolName:          "ebs",
		SizeInMiB:         1024,
		ProviderID:        "provider-test-1/0",
		StorageInstanceID: "storageinstance/1",
	}, {
		ID:                "test-2/1",
		PoolName:          "ebs-ssd",
		SizeInMiB:         2048,
		ProviderID:        "provider-test-2/1",
		StorageInstanceID: "storageinstance/2",
	}, {
		ID:         "test-3/2",
		PoolName:   "tmpfs",
		SizeInMiB:  4096,
		ProviderID: "provider-test-3/2",
		// sometimes filesystems are not associated with a storage instance
		StorageInstanceID: "",
	}}

	s.state.EXPECT().GetStoragePoolProvidersByNames(gomock.Any(), tc.Bind(tc.SameContents, []string{"ebs", "ebs-ssd", "tmpfs"})).Return(map[string]string{
		"ebs":     "ebs",
		"ebs-ssd": "ebs",
		"tmpfs":   "tmpfs",
	}, nil)

	ebsProvider := NewMockProvider(ctrl)
	ebsProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.registry.Providers["ebs"] = ebsProvider

	tmpfsProvider := NewMockProvider(ctrl)
	tmpfsProvider.EXPECT().Scope().Return(internalstorage.ScopeMachine).AnyTimes()
	tmpfsProvider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(false).AnyTimes()
	tmpfsProvider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	s.registry.Providers["tmpfs"] = tmpfsProvider

	s.state.EXPECT().GetStorageInstanceUUIDsByIDs(gomock.Any(), []string{"storageinstance/1", "storageinstance/2"}).
		Return(map[string]string{
			"storageinstance/1": "storageinstance-uuid-1",
			"storageinstance/2": "storageinstance-uuid-2",
		}, nil)

	expectedFS := []internal.ImportFilesystemIAASArgs{{
		ID:                  "test-2/1",
		Life:                life.Alive,
		SizeInMiB:           2048,
		ProviderID:          "provider-test-2/1",
		StorageInstanceUUID: "storageinstance-uuid-2",
		Scope:               domainstorageprovisioning.ProvisionScopeMachine,
	}, {
		ID:                  "test-1/0",
		Life:                life.Alive,
		SizeInMiB:           1024,
		ProviderID:          "provider-test-1/0",
		StorageInstanceUUID: "storageinstance-uuid-1",
		Scope:               domainstorageprovisioning.ProvisionScopeMachine,
	}, {
		ID:         "test-3/2",
		Life:       life.Alive,
		SizeInMiB:  4096,
		ProviderID: "provider-test-3/2",
		Scope:      domainstorageprovisioning.ProvisionScopeMachine,
	}}

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.UUID`, tc.IsNonZeroUUID)

	s.state.EXPECT().ImportFilesystemsIAAS(gomock.Any(),
		tc.Bind(tc.UnorderedMatch[[]internal.ImportFilesystemIAASArgs](mc), expectedFS),
		tc.Bind(tc.HasLen, 0),
	).Return(nil)

	err := s.service.ImportFilesystemsIAAS(c.Context(), params)

	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportFilesystemsIAASWithAttachments(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	params := []domainstorage.ImportFilesystemParams{{
		ID:                "test-1/0",
		PoolName:          "ebs",
		SizeInMiB:         1024,
		ProviderID:        "provider-test-1/0",
		StorageInstanceID: "storageinstance/1",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostUnitName: "unit/0",
			MountPoint:   "/mnt/test1-0",
			ReadOnly:     false,
		}, {
			HostUnitName: "unit/1",
			MountPoint:   "/mnt/test1-0-ro",
			ReadOnly:     true,
		}},
	}, {
		ID:                "test-2/1",
		PoolName:          "ebs-ssd",
		SizeInMiB:         2048,
		ProviderID:        "provider-test-2/1",
		StorageInstanceID: "storageinstance/2",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostMachineName: "0",
			MountPoint:      "/mnt/test2-1",
			ReadOnly:        false,
		}},
	}}

	s.state.EXPECT().GetStoragePoolProvidersByNames(gomock.Any(), tc.Bind(tc.SameContents, []string{"ebs", "ebs-ssd"})).Return(map[string]string{
		"ebs":     "ebs",
		"ebs-ssd": "ebs",
	}, nil)

	ebsProvider := NewMockProvider(ctrl)
	ebsProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.registry.Providers["ebs"] = ebsProvider

	s.state.EXPECT().GetStorageInstanceUUIDsByIDs(gomock.Any(), []string{"storageinstance/1", "storageinstance/2"}).
		Return(map[string]string{
			"storageinstance/1": "storageinstance-uuid-1",
			"storageinstance/2": "storageinstance-uuid-2",
		}, nil)

	s.state.EXPECT().GetNetNodeUUIDsByMachineOrUnitName(gomock.Any(),
		tc.Bind(tc.SameContents, []string{"0"}),
		tc.Bind(tc.SameContents, []string{"unit/0", "unit/1"}),
	).Return(
		map[string]string{"0": "netnode-uuid-0"},
		map[string]string{"unit/0": "netnode-uuid-unit-0", "unit/1": "netnode-uuid-unit-1"},
		nil,
	)

	expectedFS := []internal.ImportFilesystemIAASArgs{{
		ID:                  "test-1/0",
		Life:                life.Alive,
		SizeInMiB:           1024,
		ProviderID:          "provider-test-1/0",
		StorageInstanceUUID: "storageinstance-uuid-1",
		Scope:               domainstorageprovisioning.ProvisionScopeMachine,
	}, {
		ID:                  "test-2/1",
		Life:                life.Alive,
		SizeInMiB:           2048,
		ProviderID:          "provider-test-2/1",
		StorageInstanceUUID: "storageinstance-uuid-2",
		Scope:               domainstorageprovisioning.ProvisionScopeMachine,
	}}
	expectedAttachments := []internal.ImportFilesystemAttachmentIAASArgs{{
		MountPoint:  "/mnt/test1-0",
		ReadOnly:    false,
		NetNodeUUID: "netnode-uuid-unit-0",
		Life:        life.Alive,
		Scope:       domainstorageprovisioning.ProvisionScopeMachine,
	}, {
		MountPoint:  "/mnt/test1-0-ro",
		ReadOnly:    true,
		NetNodeUUID: "netnode-uuid-unit-1",
		Life:        life.Alive,
		Scope:       domainstorageprovisioning.ProvisionScopeMachine,
	}, {
		MountPoint:  "/mnt/test2-1",
		ReadOnly:    false,
		NetNodeUUID: "netnode-uuid-0",
		Life:        life.Alive,
		Scope:       domainstorageprovisioning.ProvisionScopeMachine,
	}}

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.UUID`, tc.IsNonZeroUUID)
	mc.AddExpr(`_.FilesystemUUID`, tc.IsNonZeroUUID)

	var (
		gotFS          []internal.ImportFilesystemIAASArgs
		gotAttachments []internal.ImportFilesystemAttachmentIAASArgs
	)
	s.state.EXPECT().ImportFilesystemsIAAS(gomock.Any(),
		tc.Bind(tc.UnorderedMatch[[]internal.ImportFilesystemIAASArgs](mc), expectedFS),
		tc.Bind(tc.UnorderedMatch[[]internal.ImportFilesystemAttachmentIAASArgs](mc), expectedAttachments),
	).DoAndReturn(func(_ context.Context, fsArgs []internal.ImportFilesystemIAASArgs, attachmentArgs []internal.ImportFilesystemAttachmentIAASArgs) error {
		gotFS = fsArgs
		gotAttachments = attachmentArgs
		return nil
	})

	// Act
	err := s.service.ImportFilesystemsIAAS(c.Context(), params)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	var (
		fs1UUID, fs2UUID string
	)
	for _, fs := range gotFS {
		switch fs.ID {
		case "test-1/0":
			fs1UUID = fs.UUID
		case "test-2/1":
			fs2UUID = fs.UUID
		default:
			c.Fatalf("unexpected filesystem ID %q", fs.ID)
		}
	}
	for _, a := range gotAttachments {
		switch a.MountPoint {
		case "/mnt/test1-0":
			c.Assert(a.FilesystemUUID, tc.Equals, fs1UUID)
		case "/mnt/test1-0-ro":
			c.Assert(a.FilesystemUUID, tc.Equals, fs1UUID)
		case "/mnt/test2-1":
			c.Assert(a.FilesystemUUID, tc.Equals, fs2UUID)
		default:
			c.Fatalf("unexpected attachment mount point %q", a.MountPoint)
		}
	}
}

func (s *importSuite) TestImportFilesystemsIAASMissingProvider(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	params := []domainstorage.ImportFilesystemParams{{
		ID:                "test-1/0",
		PoolName:          "ebs",
		SizeInMiB:         1024,
		ProviderID:        "provider-test-1/0",
		StorageInstanceID: "storageinstance/1",
	}}

	// No provider for "ebs" is returned, which indicates the provider does not exist
	s.state.EXPECT().GetStoragePoolProvidersByNames(gomock.Any(), tc.Bind(tc.SameContents, []string{"ebs"})).Return(map[string]string{}, nil)

	s.state.EXPECT().GetStorageInstanceUUIDsByIDs(gomock.Any(), []string{"storageinstance/1"}).
		Return(map[string]string{
			"storageinstance/1": "storageinstance-uuid-1",
		}, nil)

	// Act
	err := s.service.ImportFilesystemsIAAS(c.Context(), params)

	// Assert
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

func (s *importSuite) TestImportFilesystemsIAASMissingStorageInstance(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	params := []domainstorage.ImportFilesystemParams{{
		ID:                "test-1/0",
		PoolName:          "ebs",
		SizeInMiB:         1024,
		ProviderID:        "provider-test-1/0",
		StorageInstanceID: "storageinstance/1",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostUnitName: "unit/0",
			MountPoint:   "/mnt/test1-0",
			ReadOnly:     false,
		}, {
			HostUnitName: "unit/1",
			MountPoint:   "/mnt/test1-0-ro",
			ReadOnly:     true,
		}},
	}, {
		ID:                "test-2/1",
		PoolName:          "ebs-ssd",
		SizeInMiB:         2048,
		ProviderID:        "provider-test-2/1",
		StorageInstanceID: "storageinstance/2",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostMachineName: "0",
			MountPoint:      "/mnt/test2-1",
			ReadOnly:        false,
		}},
	}}

	s.state.EXPECT().GetStoragePoolProvidersByNames(gomock.Any(), tc.Bind(tc.SameContents, []string{"ebs", "ebs-ssd"})).Return(map[string]string{
		"ebs":     "ebs",
		"ebs-ssd": "ebs",
	}, nil)

	ebsProvider := NewMockProvider(ctrl)
	ebsProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.registry.Providers["ebs"] = ebsProvider

	// No uuid for storageinstance/2 is returned, which indicates it does not exist
	s.state.EXPECT().GetStorageInstanceUUIDsByIDs(gomock.Any(), []string{"storageinstance/1", "storageinstance/2"}).
		Return(map[string]string{
			"storageinstance/1": "storageinstance-uuid-1",
		}, nil)

	s.state.EXPECT().GetNetNodeUUIDsByMachineOrUnitName(gomock.Any(),
		tc.Bind(tc.SameContents, []string{"0"}),
		tc.Bind(tc.SameContents, []string{"unit/0", "unit/1"}),
	).Return(
		map[string]string{"0": "netnode-uuid-0"},
		map[string]string{"unit/0": "netnode-uuid-unit-0", "unit/1": "netnode-uuid-unit-1"},
		nil,
	)

	// Act
	err := s.service.ImportFilesystemsIAAS(c.Context(), params)

	// Assert
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

func (s *importSuite) TestImportFilesystemsIAASMissingUnit(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	params := []domainstorage.ImportFilesystemParams{{
		ID:                "test-1/0",
		PoolName:          "ebs",
		SizeInMiB:         1024,
		ProviderID:        "provider-test-1/0",
		StorageInstanceID: "storageinstance/1",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostUnitName: "unit/0",
			MountPoint:   "/mnt/test1-0",
			ReadOnly:     false,
		}, {
			HostUnitName: "unit/1",
			MountPoint:   "/mnt/test1-0-ro",
			ReadOnly:     true,
		}},
	}, {
		ID:                "test-2/1",
		PoolName:          "ebs-ssd",
		SizeInMiB:         2048,
		ProviderID:        "provider-test-2/1",
		StorageInstanceID: "storageinstance/2",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostMachineName: "0",
			MountPoint:      "/mnt/test2-1",
			ReadOnly:        false,
		}},
	}}

	s.state.EXPECT().GetStoragePoolProvidersByNames(gomock.Any(), tc.Bind(tc.SameContents, []string{"ebs", "ebs-ssd"})).Return(map[string]string{
		"ebs":     "ebs",
		"ebs-ssd": "ebs",
	}, nil)

	ebsProvider := NewMockProvider(ctrl)
	ebsProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.registry.Providers["ebs"] = ebsProvider

	s.state.EXPECT().GetStorageInstanceUUIDsByIDs(gomock.Any(), []string{"storageinstance/1", "storageinstance/2"}).
		Return(map[string]string{
			"storageinstance/1": "storageinstance-uuid-1",
			"storageinstance/2": "storageinstance-uuid-2",
		}, nil)

	// No uuid for "unit/1" is returned, which indicates it does not exist
	s.state.EXPECT().GetNetNodeUUIDsByMachineOrUnitName(gomock.Any(),
		tc.Bind(tc.SameContents, []string{"0"}),
		tc.Bind(tc.SameContents, []string{"unit/0", "unit/1"}),
	).Return(
		map[string]string{"0": "netnode-uuid-0"},
		map[string]string{"unit/0": "netnode-uuid-unit-0"},
		nil,
	)

	// Act
	err := s.service.ImportFilesystemsIAAS(c.Context(), params)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *importSuite) TestImportFilesystemsIAASMissingMachine(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	params := []domainstorage.ImportFilesystemParams{{
		ID:                "test-1/0",
		PoolName:          "ebs",
		SizeInMiB:         1024,
		ProviderID:        "provider-test-1/0",
		StorageInstanceID: "storageinstance/1",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostUnitName: "unit/0",
			MountPoint:   "/mnt/test1-0",
			ReadOnly:     false,
		}, {
			HostUnitName: "unit/1",
			MountPoint:   "/mnt/test1-0-ro",
			ReadOnly:     true,
		}},
	}, {
		ID:                "test-2/1",
		PoolName:          "ebs-ssd",
		SizeInMiB:         2048,
		ProviderID:        "provider-test-2/1",
		StorageInstanceID: "storageinstance/2",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostMachineName: "0",
			MountPoint:      "/mnt/test2-1",
			ReadOnly:        false,
		}},
	}}

	s.state.EXPECT().GetStoragePoolProvidersByNames(gomock.Any(), tc.Bind(tc.SameContents, []string{"ebs", "ebs-ssd"})).Return(map[string]string{
		"ebs":     "ebs",
		"ebs-ssd": "ebs",
	}, nil)

	ebsProvider := NewMockProvider(ctrl)
	ebsProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.registry.Providers["ebs"] = ebsProvider

	s.state.EXPECT().GetStorageInstanceUUIDsByIDs(gomock.Any(), []string{"storageinstance/1", "storageinstance/2"}).
		Return(map[string]string{
			"storageinstance/1": "storageinstance-uuid-1",
			"storageinstance/2": "storageinstance-uuid-2",
		}, nil)

	// No uuid for "0" is returned, which indicates the machine does not exist
	s.state.EXPECT().GetNetNodeUUIDsByMachineOrUnitName(gomock.Any(),
		tc.Bind(tc.SameContents, []string{"0"}),
		tc.Bind(tc.SameContents, []string{"unit/0", "unit/1"}),
	).Return(
		map[string]string{},
		map[string]string{"unit/0": "netnode-uuid-unit-0", "unit/1": "netnode-uuid-unit-1"},
		nil,
	)

	// Act
	err := s.service.ImportFilesystemsIAAS(c.Context(), params)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *importSuite) TestImportFilesystemsValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := []domainstorage.ImportFilesystemParams{{}}
	err := s.service.ImportFilesystemsIAAS(c.Context(), args)

	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *importSuite) TestImportVolumes(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	storageInstanceUUID := tc.Must(c, domainstorage.NewStoragePoolUUID).String()
	s.state.EXPECT().GetStorageInstanceUUIDsByIDs(gomock.Any(), []string{"multi-fs/0"}).
		Return(map[string]string{
			"multi-fs/0": storageInstanceUUID,
		}, nil)

	s.state.EXPECT().GetStoragePoolProvidersByNames(gomock.Any(), []string{"ebs"}).Return(map[string]string{
		"ebs": "ebs",
	}, nil)

	// Arrange: CalculateStorageInstanceComposition
	ebsProvider := NewMockProvider(ctrl)
	ebsProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.registry.Providers["ebs"] = ebsProvider

	// Arrange: state call
	expected := []internal.ImportVolumeArgs{
		{
			ID:                  "multi-vol/0",
			LifeID:              life.Alive,
			ProvisionScopeID:    domainstorageprovisioning.ProvisionScopeModel,
			StorageInstanceUUID: storageInstanceUUID,
			SizeMiB:             4048,
			HardwareID:          "hardware",
			ProviderID:          "vol-0f2829d7e5c4c0140",
			WWN:                 "uuid.06eba00f-72a0-5af0-9e94-891d7542e96c",
		},
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].UUID`, tc.IsNonZeroUUID)
	s.state.EXPECT().ImportVolumes(gomock.Any(), tc.Bind(mc, expected)).Return(nil)

	// Arrange: input
	params := []domainstorage.ImportVolumeParams{
		{
			ID:         "multi-vol/0",
			Pool:       "ebs",
			StorageID:  "multi-fs/0",
			SizeMiB:    4048,
			HardwareID: "hardware",
			ProviderID: "vol-0f2829d7e5c4c0140",
			WWN:        "uuid.06eba00f-72a0-5af0-9e94-891d7542e96c",
		},
	}

	// Act
	err := s.service.ImportVolumes(c.Context(), params)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportVolumesMissingStorageProvider(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID).String()
	s.state.EXPECT().GetStorageInstanceUUIDsByIDs(gomock.Any(), []string{"multi-fs/0"}).
		Return(map[string]string{
			"multi-fs/0": storageInstanceUUID,
		}, nil)

	// No provider for "ebs" is returned, which indicates the provider does not exist
	s.state.EXPECT().GetStoragePoolProvidersByNames(gomock.Any(), []string{"ebs"}).Return(map[string]string{}, nil)

	// Arrange: input
	params := []domainstorage.ImportVolumeParams{
		{
			ID:         "multi-vol/0",
			Pool:       "ebs",
			StorageID:  "multi-fs/0",
			SizeMiB:    4048,
			HardwareID: "hardware",
			ProviderID: "vol-0f2829d7e5c4c0140",
			WWN:        "uuid.06eba00f-72a0-5af0-9e94-891d7542e96c",
		},
	}

	// Act
	err := s.service.ImportVolumes(c.Context(), params)

	// Assert
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.registry = internalstorage.StaticProviderRegistry{
		Providers: map[internalstorage.ProviderType]internalstorage.Provider{},
	}

	s.service = NewService(
		s.state, loggertesting.WrapCheckLog(c), registryGetter{s.registry},
	)

	c.Cleanup(func() {
		s.registry = internalstorage.StaticProviderRegistry{}
		s.state = nil
		s.service = nil
	})

	return ctrl
}
