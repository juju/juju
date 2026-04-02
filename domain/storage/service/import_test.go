// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/network"
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

	service *StorageImportService

	fsModelMigration *MockFilesystemModelMigration
	registry         internalstorage.StaticProviderRegistry
	state            *MockStorageImportState
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
	args := []domainstorage.ImportStorageInstanceParams{{}}

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
		Return(map[string]domainstorage.StorageInstanceUUID{
			"storageinstance/1": "storageinstance-uuid-1",
			"storageinstance/2": "storageinstance-uuid-2",
		}, nil)

	expectedFS := []internal.ImportFilesystemArgs{{
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
		tc.Bind(tc.UnorderedMatch[[]internal.ImportFilesystemArgs](mc), expectedFS),
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
			ProviderID:      "provider-id",
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
		Return(map[string]domainstorage.StorageInstanceUUID{
			"storageinstance/1": "storageinstance-uuid-1",
			"storageinstance/2": "storageinstance-uuid-2",
		}, nil)

	s.state.EXPECT().GetNetNodeUUIDsByMachineOrUnitName(gomock.Any(),
		tc.Bind(tc.SameContents, []machine.Name{"0"}),
		tc.Bind(tc.SameContents, []coreunit.Name{"unit/0", "unit/1"}),
	).Return(
		map[machine.Name]network.NetNodeUUID{"0": "netnode-uuid-0"},
		map[coreunit.Name]network.NetNodeUUID{"unit/0": "netnode-uuid-unit-0", "unit/1": "netnode-uuid-unit-1"},
		nil,
	)

	expectedFS := []internal.ImportFilesystemArgs{{
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
	expectedAttachments := []internal.ImportFilesystemAttachmentArgs{{
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
		ProviderID:  "provider-id",
		Life:        life.Alive,
		Scope:       domainstorageprovisioning.ProvisionScopeMachine,
	}}

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.UUID`, tc.IsNonZeroUUID)
	mc.AddExpr(`_.FilesystemUUID`, tc.IsNonZeroUUID)

	var (
		gotFS          []internal.ImportFilesystemArgs
		gotAttachments []internal.ImportFilesystemAttachmentArgs
	)
	s.state.EXPECT().ImportFilesystemsIAAS(gomock.Any(),
		tc.Bind(tc.UnorderedMatch[[]internal.ImportFilesystemArgs](mc), expectedFS),
		tc.Bind(tc.UnorderedMatch[[]internal.ImportFilesystemAttachmentArgs](mc), expectedAttachments),
	).DoAndReturn(func(_ context.Context, fsArgs []internal.ImportFilesystemArgs, attachmentArgs []internal.ImportFilesystemAttachmentArgs) error {
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
		Return(map[string]domainstorage.StorageInstanceUUID{
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
		Return(map[string]domainstorage.StorageInstanceUUID{
			"storageinstance/1": "storageinstance-uuid-1",
		}, nil)

	s.state.EXPECT().GetNetNodeUUIDsByMachineOrUnitName(gomock.Any(),
		tc.Bind(tc.SameContents, []machine.Name{"0"}),
		tc.Bind(tc.SameContents, []coreunit.Name{"unit/0", "unit/1"}),
	).Return(
		map[machine.Name]network.NetNodeUUID{"0": "netnode-uuid-0"},
		map[coreunit.Name]network.NetNodeUUID{"unit/0": "netnode-uuid-unit-0", "unit/1": "netnode-uuid-unit-1"},
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
		Return(map[string]domainstorage.StorageInstanceUUID{
			"storageinstance/1": "storageinstance-uuid-1",
			"storageinstance/2": "storageinstance-uuid-2",
		}, nil)

	// No uuid for "unit/1" is returned, which indicates it does not exist
	s.state.EXPECT().GetNetNodeUUIDsByMachineOrUnitName(gomock.Any(),
		tc.Bind(tc.SameContents, []machine.Name{"0"}),
		tc.Bind(tc.SameContents, []coreunit.Name{"unit/0", "unit/1"}),
	).Return(
		map[machine.Name]network.NetNodeUUID{"0": "netnode-uuid-0"},
		map[coreunit.Name]network.NetNodeUUID{"unit/0": "netnode-uuid-unit-0"},
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
		Return(map[string]domainstorage.StorageInstanceUUID{
			"storageinstance/1": "storageinstance-uuid-1",
			"storageinstance/2": "storageinstance-uuid-2",
		}, nil)

	// No uuid for "0" is returned, which indicates the machine does not exist
	s.state.EXPECT().GetNetNodeUUIDsByMachineOrUnitName(gomock.Any(),
		tc.Bind(tc.SameContents, []machine.Name{"0"}),
		tc.Bind(tc.SameContents, []coreunit.Name{"unit/0", "unit/1"}),
	).Return(
		map[machine.Name]network.NetNodeUUID{},
		map[coreunit.Name]network.NetNodeUUID{"unit/0": "netnode-uuid-unit-0", "unit/1": "netnode-uuid-unit-1"},
		nil,
	)

	// Act
	err := s.service.ImportFilesystemsIAAS(c.Context(), params)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *importSuite) TestImportFilesystemsCAAS(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange the storage providers
	kubernetesProvider := NewMockProvider(ctrl)
	kubernetesProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron).AnyTimes()
	kubernetesProvider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	kubernetesProvider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.registry.Providers["kubernetes"] = kubernetesProvider

	// Arrange: expected mocked calls
	s.state.EXPECT().GetStoragePoolProvidersByNames(
		gomock.Any(),
		tc.Bind(tc.SameContents, []string{"kubernetes", "kubernetes"}),
	).Return(map[string]string{
		"kubernetes": "kubernetes",
	}, nil)

	s.fsModelMigration.EXPECT().GetPersistentVolumeClaimIdentifiers(gomock.Any()).Return(
		[]internalstorage.PersistentVolumeClaimIdentifiers{{
			UID:  "753fff9e-6d0d-4d2c-b1e5-deadbeef94f9",
			Name: "postgresql-k8s-pgdata-a6c8f4e1-postgresql-k8s-1",
		}, {
			UID:  "753fff9e-6d0d-4d2c-b1e5-e2b3c02284f9",
			Name: "postgresql-k8s-pgdata-a6c8f4e1-postgresql-k8s-0",
		}, {
			UID:  "deadbeef-6d0d-4d2c-b1e5-e2b3c02284f9",
			Name: "mysql-k8s-data-a6c8f4e1-mysql-k8s-0",
		},
		}, nil)

	s.state.EXPECT().GetStorageInstanceUUIDsByIDs(gomock.Any(), []string{"storageinstance/1", "storageinstance/2"}).
		Return(map[string]domainstorage.StorageInstanceUUID{
			"storageinstance/1": "storageinstance-uuid-1",
			"storageinstance/2": "storageinstance-uuid-2",
		}, nil)

	s.state.EXPECT().GetNetNodeUUIDsByMachineOrUnitName(gomock.Any(),
		[]machine.Name{},
		tc.Bind(tc.SameContents, []coreunit.Name{"postgresql/0", "mysql-k8s/0"}),
	).Return(
		nil,
		map[coreunit.Name]network.NetNodeUUID{"postgresql/0": "netnode-uuid-unit-0", "mysql-k8s/0": "netnode-uuid-unit-1"},
		nil,
	)

	// Arrange: expected input for state method call.
	expectedFS := []internal.ImportFilesystemArgs{{
		ID:                  "test-1/0",
		Life:                life.Alive,
		SizeInMiB:           1024,
		ProviderID:          "pvc-753fff9e-6d0d-4d2c-b1e5-e2b3c02284f9",
		StorageInstanceUUID: "storageinstance-uuid-1",
		Scope:               domainstorageprovisioning.ProvisionScopeMachine,
	}, {
		ID:                  "test-2/1",
		Life:                life.Alive,
		SizeInMiB:           2048,
		ProviderID:          "pvc-deadbeef-6d0d-4d2c-b1e5-e2b3c02284f9",
		StorageInstanceUUID: "storageinstance-uuid-2",
		Scope:               domainstorageprovisioning.ProvisionScopeMachine,
	}}
	expectedAttachments := []internal.ImportFilesystemAttachmentArgs{{
		MountPoint:  "/mnt/test1-0",
		ReadOnly:    false,
		NetNodeUUID: "netnode-uuid-unit-0",
		ProviderID:  "postgresql-k8s-pgdata-a6c8f4e1-postgresql-k8s-0",
		Life:        life.Alive,
		Scope:       domainstorageprovisioning.ProvisionScopeMachine,
	}, {
		MountPoint:  "/mnt/test2-1",
		ReadOnly:    false,
		NetNodeUUID: "netnode-uuid-unit-1",
		ProviderID:  "mysql-k8s-data-a6c8f4e1-mysql-k8s-0",
		Life:        life.Alive,
		Scope:       domainstorageprovisioning.ProvisionScopeMachine,
	}}

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_.UUID`, tc.IsNonZeroUUID)
	mc.AddExpr(`_.FilesystemUUID`, tc.IsNonZeroUUID)

	var (
		gotFS          []internal.ImportFilesystemArgs
		gotAttachments []internal.ImportFilesystemAttachmentArgs
	)
	s.state.EXPECT().ImportFilesystemsIAAS(gomock.Any(),
		tc.Bind(tc.UnorderedMatch[[]internal.ImportFilesystemArgs](mc), expectedFS),
		tc.Bind(tc.UnorderedMatch[[]internal.ImportFilesystemAttachmentArgs](mc), expectedAttachments),
	).DoAndReturn(func(_ context.Context, fsArgs []internal.ImportFilesystemArgs, attachmentArgs []internal.ImportFilesystemAttachmentArgs) error {
		gotFS = fsArgs
		gotAttachments = attachmentArgs
		return nil
	})

	// Arrange: params for service method call
	params := []domainstorage.ImportFilesystemParams{{
		ID:                "test-1/0",
		PoolName:          "kubernetes",
		SizeInMiB:         1024,
		ProviderID:        "pvc-753fff9e-6d0d-4d2c-b1e5-e2b3c02284f9",
		StorageInstanceID: "storageinstance/1",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostUnitName: "postgresql/0",
			MountPoint:   "/mnt/test1-0",
			ReadOnly:     false,
			ProviderID:   "753fff9e-6d0d-4d2c-b1e5-e2b3c02284f9",
		}},
	}, {
		ID:                "test-2/1",
		PoolName:          "kubernetes",
		SizeInMiB:         2048,
		ProviderID:        "pvc-deadbeef-6d0d-4d2c-b1e5-e2b3c02284f9",
		StorageInstanceID: "storageinstance/2",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostUnitName: "mysql-k8s/0",
			MountPoint:   "/mnt/test2-1",
			ProviderID:   "deadbeef-6d0d-4d2c-b1e5-e2b3c02284f9",
			ReadOnly:     false,
		}},
	}}

	// Act
	err := s.service.ImportFilesystemsCAAS(c.Context(), params)

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

func (s *importSuite) TestImportFilesystemsCAASNameFail(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange the storage providers
	s.fsModelMigration.EXPECT().GetPersistentVolumeClaimIdentifiers(gomock.Any()).Return(
		[]internalstorage.PersistentVolumeClaimIdentifiers{{
			UID:  "753fff9e-6d0d-4d2c-b1e5-deadbeef94f9",
			Name: "postgresql-k8s-pgdata-a6c8f4e1-postgresql-k8s-1",
		}, {
			UID:  "753fff9e-6d0d-4d2c-b1e5-e2b3c02284f9",
			Name: "postgresql-k8s-pgdata-a6c8f4e1-postgresql-k8s-0",
		}, {
			UID:  "deadbeef-6d0d-4d2c-b1e5-e2b3c02284f9",
			Name: "mysql-k8s-data-a6c8f4e1-mysql-k8s-0",
		},
		}, nil)

	// Arrange: params for service method call
	params := []domainstorage.ImportFilesystemParams{{
		ID:                "test-1/0",
		PoolName:          "kubernetes",
		SizeInMiB:         1024,
		ProviderID:        "pvc-753fff9e-6d0d-4d2c-b1e5-e2b3c02284f9",
		StorageInstanceID: "storageinstance/1",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostUnitName: "postgresql/0",
			MountPoint:   "/mnt/test1-0",
			ReadOnly:     false,
			ProviderID:   "failme",
		}},
	}, {
		ID:                "test-2/1",
		PoolName:          "kubernetes",
		SizeInMiB:         2048,
		ProviderID:        "pvc-deadbeef-6d0d-4d2c-b1e5-e2b3c02284f9",
		StorageInstanceID: "storageinstance/2",
		Attachments: []domainstorage.ImportFilesystemAttachmentsParams{{
			HostUnitName: "mysql-k8s/0",
			MountPoint:   "/mnt/test2-1",
			ProviderID:   "deadbeef-6d0d-4d2c-b1e5-e2b3c02284f9",
			ReadOnly:     false,
		}},
	}}

	// Act
	err := s.service.ImportFilesystemsCAAS(c.Context(), params)

	// Assert
	c.Assert(err, tc.ErrorMatches, `persistent volume claim identifier "failme" not found`)
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

	// Arrange: CalculateStorageInstanceComposition
	ebsProvider := NewMockProvider(ctrl)
	ebsProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	ebsProvider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(false).AnyTimes()
	s.registry.Providers["ebs"] = ebsProvider
	s.state.EXPECT().GetStoragePoolProvidersByNames(gomock.Any(), []string{"ebs"}).Return(map[string]string{
		"ebs": "ebs",
	}, nil)

	// Arrange:
	netNodeUUIDOne := tc.Must(c, network.NewNetNodeUUID)
	netNodeUUIDTwo := tc.Must(c, network.NewNetNodeUUID)
	s.state.EXPECT().GetNetNodeUUIDsByMachineOrUnitName(gomock.Any(), gomock.InAnyOrder([]machine.Name{"0", "1"}), nil).Return(
		map[machine.Name]network.NetNodeUUID{
			"1": netNodeUUIDTwo,
			"0": netNodeUUIDOne,
		}, nil, nil)

	// Arrange: mocks for storage instances
	storageInstanceUUIDOne := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageInstanceUUIDTwo := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	storageIDOne := "multi-vol/0"
	storageIDTwo := "multi-fs/1"
	s.state.EXPECT().GetStorageInstanceUUIDsByIDs(gomock.Any(), []string{storageIDOne, storageIDTwo}).
		Return(map[string]domainstorage.StorageInstanceUUID{
			storageIDOne: storageInstanceUUIDOne,
			storageIDTwo: storageInstanceUUIDTwo,
		}, nil)

	// Arrange: found block devices for the volumes
	bdUUIDOne := tc.Must(c, blockdevice.NewBlockDeviceUUID)
	bdUUIDTwo := tc.Must(c, blockdevice.NewBlockDeviceUUID)
	s.state.EXPECT().GetBlockDevicesForMachinesByNetNodeUUIDs(gomock.Any(),
		gomock.InAnyOrder([]network.NetNodeUUID{netNodeUUIDOne, netNodeUUIDTwo})).Return(
		map[network.NetNodeUUID][]internal.BlockDevice{
			netNodeUUIDOne: {
				{
					UUID: bdUUIDOne,
					BlockDevice: coreblockdevice.BlockDevice{
						DeviceName:  "xvdf",
						DeviceLinks: []string{"/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol0e8b3aed0fbee6887"},
					},
				},
			},
			netNodeUUIDTwo: {
				{
					UUID: bdUUIDTwo,
					BlockDevice: coreblockdevice.BlockDevice{
						DeviceName:  "xvdf",
						DeviceLinks: []string{"/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol08195b158e8ce069d"},
					},
				},
			},
		}, nil)

	// Arrange: state call
	expected := []internal.ImportVolumeArgs{
		{
			ID:                  "0",
			StorageInstanceID:   storageIDOne,
			LifeID:              life.Alive,
			Provisioned:         true,
			Persistent:          true,
			ProvisionScopeID:    domainstorageprovisioning.ProvisionScopeModel,
			StorageInstanceUUID: storageInstanceUUIDOne,
			SizeMiB:             4048,
			ProviderID:          "vol-0f2829d7e5c4c0140",
			WWN:                 "uuid.c2f9e696-7b12-5368-b274-0510bf1feade",
			Attachments: []internal.ImportVolumeAttachmentArgs{
				{
					BlockDeviceUUID: bdUUIDOne,
					LifeID:          life.Alive,
					NetNodeUUID:     netNodeUUIDOne,
				},
			},
			AttachmentPlans: []internal.ImportVolumeAttachmentPlanArgs{
				{
					DeviceAttributes: map[string]string{
						"iqn":  "iqn.2015-12.com.oracleiaas:5349c1a7-36b4-4d7c-85f2-059c5cd6e344",
						"port": "3260",
					},
					LifeID:           life.Alive,
					NetNodeUUID:      netNodeUUIDOne,
					ProvisionScopeID: domainstorageprovisioning.ProvisionScopeModel,
				},
			},
		}, {
			ID:                  "1",
			StorageInstanceID:   storageIDTwo,
			LifeID:              life.Alive,
			Persistent:          true,
			Provisioned:         true,
			ProvisionScopeID:    domainstorageprovisioning.ProvisionScopeModel,
			StorageInstanceUUID: storageInstanceUUIDTwo,
			SizeMiB:             4048,
			HardwareID:          "hardware",
			ProviderID:          "vol-08195b158e8ce069d",
			WWN:                 "uuid.1c63bb59-9514-505d-8d85-275a629db6d9",
			Attachments: []internal.ImportVolumeAttachmentArgs{
				{
					BlockDeviceUUID: bdUUIDTwo,
					LifeID:          life.Alive,
					NetNodeUUID:     netNodeUUIDTwo,
				},
			},
			AttachmentPlans: []internal.ImportVolumeAttachmentPlanArgs{
				{
					DeviceTypeID:     new(domainstorage.VolumeDeviceTypeISCSI),
					LifeID:           life.Alive,
					NetNodeUUID:      netNodeUUIDTwo,
					ProvisionScopeID: domainstorageprovisioning.ProvisionScopeModel,
				},
			},
		},
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].UUID`, tc.IsNonZeroUUID)
	mc.AddExpr(`_[_].AttachmentPlans[_].UUID`, tc.IsNonZeroUUID)
	mc.AddExpr(`_[_].Attachments[_].UUID`, tc.IsNonZeroUUID)
	s.state.EXPECT().ImportVolumes(gomock.Any(), tc.Bind(mc, expected)).Return(nil)

	// Arrange: ImportVolumes params
	params := []domainstorage.ImportVolumeParams{
		{
			ID:                "0",
			StorageInstanceID: storageIDOne,
			Provisioned:       true,
			Persistent:        true,
			Pool:              "ebs",
			SizeMiB:           4048,
			ProviderID:        "vol-0f2829d7e5c4c0140",
			WWN:               "uuid.c2f9e696-7b12-5368-b274-0510bf1feade",
			Attachments: []domainstorage.ImportVolumeAttachmentParams{
				{
					HostMachineName: "0",
					Provisioned:     true,
					DeviceName:      "xvdf",
					DeviceLink:      "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol0e8b3aed0fbee6887",
				},
			},
			AttachmentPlans: []domainstorage.ImportVolumeAttachmentPlanParams{
				{
					HostMachineName: "0",
					DeviceType:      "local",
					DeviceAttributes: map[string]string{
						"iqn":  "iqn.2015-12.com.oracleiaas:5349c1a7-36b4-4d7c-85f2-059c5cd6e344",
						"port": "3260",
					},
				},
			},
		}, {
			ID:                "1",
			StorageInstanceID: storageIDTwo,
			Provisioned:       true,
			Persistent:        true,
			Pool:              "ebs",
			SizeMiB:           4048,
			ProviderID:        "vol-08195b158e8ce069d",
			WWN:               "uuid.1c63bb59-9514-505d-8d85-275a629db6d9",
			HardwareID:        "hardware",
			Attachments: []domainstorage.ImportVolumeAttachmentParams{
				{
					HostMachineName: "1",
					Provisioned:     true,
					DeviceName:      "xvdf",
					DeviceLink:      "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol08195b158e8ce069d",
				},
			},
			AttachmentPlans: []domainstorage.ImportVolumeAttachmentPlanParams{
				{
					HostMachineName: "1",
					DeviceType:      "iscsi",
				},
			},
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
	s.state.EXPECT().GetBlockDevicesForMachinesByNetNodeUUIDs(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.state.EXPECT().GetStorageInstanceUUIDsByIDs(gomock.Any(), []string{"multi-fs/0"}).
		Return(map[string]domainstorage.StorageInstanceUUID{
			"multi-fs/0": tc.Must(c, domainstorage.NewStorageInstanceUUID),
		}, nil)

	// No provider for "ebs" is returned, which indicates the provider does not exist
	s.state.EXPECT().GetStoragePoolProvidersByNames(gomock.Any(), []string{"ebs"}).Return(map[string]string{}, nil)

	// Arrange: input
	params := []domainstorage.ImportVolumeParams{
		{
			ID:                "multi-vol/0",
			Pool:              "ebs",
			StorageInstanceID: "multi-fs/0",
			SizeMiB:           4048,
			HardwareID:        "hardware",
			ProviderID:        "vol-0f2829d7e5c4c0140",
			WWN:               "uuid.06eba00f-72a0-5af0-9e94-891d7542e96c",
		},
	}

	// Act
	err := s.service.ImportVolumes(c.Context(), params)

	// Assert
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockStorageImportState(ctrl)
	s.registry = internalstorage.StaticProviderRegistry{
		Providers: map[internalstorage.ProviderType]internalstorage.Provider{},
	}
	s.fsModelMigration = NewMockFilesystemModelMigration(ctrl)

	s.service = NewImportService(
		s.state,
		loggertesting.WrapCheckLog(c),
		registryGetter{s.registry},
		func(_ context.Context, fn func(_ context.Context, fs internalstorage.FilesystemModelMigration) error) error {
			return fn(c.Context(), s.fsModelMigration)
		},
	)

	c.Cleanup(func() {
		s.fsModelMigration = nil
		s.registry = internalstorage.StaticProviderRegistry{}
		s.state = nil
		s.service = nil
	})

	return ctrl
}
