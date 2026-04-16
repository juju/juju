// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"errors"
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
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
)

// importSuite is a set of tests to assert the interface and contracts
// importing storage into this state package.
type importSuite struct {
	testhelpers.IsolationSuite

	service *StorageImportService

	fsModelMigration        *MockFilesystemModelMigration
	registry                internalstorage.StaticProviderRegistry
	state                   *MockStorageImportState
	storageProvider         *MockProvider
	storageProviderRegistry *MockProviderRegistry
	storageRegistryGetter   *MockModelStorageRegistryGetter
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

	tmpfsProvider := NewMockProvider(ctrl)
	tmpfsProvider.EXPECT().Scope().Return(internalstorage.ScopeMachine).AnyTimes()
	tmpfsProvider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(false).AnyTimes()
	tmpfsProvider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true).AnyTimes()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("ebs")).Return(ebsProvider, nil).Times(2)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("tmpfs")).Return(tmpfsProvider, nil)

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
		Scope:               domainstorage.ProvisionScopeMachine,
	}, {
		ID:                  "test-1/0",
		Life:                life.Alive,
		SizeInMiB:           1024,
		ProviderID:          "provider-test-1/0",
		StorageInstanceUUID: "storageinstance-uuid-1",
		Scope:               domainstorage.ProvisionScopeMachine,
	}, {
		ID:         "test-3/2",
		Life:       life.Alive,
		SizeInMiB:  4096,
		ProviderID: "provider-test-3/2",
		Scope:      domainstorage.ProvisionScopeMachine,
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

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("ebs")).Return(ebsProvider, nil).Times(2)

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
		Scope:               domainstorage.ProvisionScopeMachine,
	}, {
		ID:                  "test-2/1",
		Life:                life.Alive,
		SizeInMiB:           2048,
		ProviderID:          "provider-test-2/1",
		StorageInstanceUUID: "storageinstance-uuid-2",
		Scope:               domainstorage.ProvisionScopeMachine,
	}}
	expectedAttachments := []internal.ImportFilesystemAttachmentArgs{{
		MountPoint:  "/mnt/test1-0",
		ReadOnly:    false,
		NetNodeUUID: "netnode-uuid-unit-0",
		Life:        life.Alive,
		Scope:       domainstorage.ProvisionScopeMachine,
	}, {
		MountPoint:  "/mnt/test1-0-ro",
		ReadOnly:    true,
		NetNodeUUID: "netnode-uuid-unit-1",
		Life:        life.Alive,
		Scope:       domainstorage.ProvisionScopeMachine,
	}, {
		MountPoint:  "/mnt/test2-1",
		ReadOnly:    false,
		NetNodeUUID: "netnode-uuid-0",
		ProviderID:  "provider-id",
		Life:        life.Alive,
		Scope:       domainstorage.ProvisionScopeMachine,
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

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)

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

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("ebs")).Return(ebsProvider, nil).Times(2)

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

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("ebs")).Return(ebsProvider, nil).Times(2)

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

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("ebs")).Return(ebsProvider, nil).Times(2)

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

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("kubernetes")).Return(kubernetesProvider, nil)

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
		Scope:               domainstorage.ProvisionScopeMachine,
	}, {
		ID:                  "test-2/1",
		Life:                life.Alive,
		SizeInMiB:           2048,
		ProviderID:          "pvc-deadbeef-6d0d-4d2c-b1e5-e2b3c02284f9",
		StorageInstanceUUID: "storageinstance-uuid-2",
		Scope:               domainstorage.ProvisionScopeMachine,
	}}
	expectedAttachments := []internal.ImportFilesystemAttachmentArgs{{
		MountPoint:  "/mnt/test1-0",
		ReadOnly:    false,
		NetNodeUUID: "netnode-uuid-unit-0",
		ProviderID:  "postgresql-k8s-pgdata-a6c8f4e1-postgresql-k8s-0",
		Life:        life.Alive,
		Scope:       domainstorage.ProvisionScopeMachine,
	}, {
		MountPoint:  "/mnt/test2-1",
		ReadOnly:    false,
		NetNodeUUID: "netnode-uuid-unit-1",
		ProviderID:  "mysql-k8s-data-a6c8f4e1-mysql-k8s-0",
		Life:        life.Alive,
		Scope:       domainstorage.ProvisionScopeMachine,
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

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("ebs")).Return(ebsProvider, nil)

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
			ProvisionScopeID:    domainstorage.ProvisionScopeModel,
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
					ProvisionScopeID: domainstorage.ProvisionScopeModel,
				},
			},
		}, {
			ID:                  "1",
			StorageInstanceID:   storageIDTwo,
			LifeID:              life.Alive,
			Persistent:          true,
			Provisioned:         true,
			ProvisionScopeID:    domainstorage.ProvisionScopeModel,
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
					ProvisionScopeID: domainstorage.ProvisionScopeModel,
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

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)

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

// TestImportStoragePools tests the happy path where a single storage pool
// is validated and created successfully.
func (s *importSuite) TestImportStoragePools(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil).Times(2)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"storageprovider1"}, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("storageprovider1")).Return(s.storageProvider, nil).
		Times(2)
	s.storageProvider.EXPECT().ValidateConfig(gomock.Any())
	s.storageProvider.EXPECT().DefaultPools()
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)

	input := []domainstorage.UserStoragePoolParams{
		{
			Name:     "my-pool",
			Provider: "storageprovider1",
			Attributes: map[string]any{
				"key": "val",
			},
		},
	}

	s.state.EXPECT().
		CreateStoragePool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, pool internal.CreateStoragePool) error {
		c.Assert(pool.UUID, tc.IsUUID)
		c.Assert(pool.Name, tc.Equals, "my-pool")
		c.Assert(pool.ProviderType, tc.Equals, domainstorage.ProviderType("storageprovider1"))
		c.Assert(pool.Origin, tc.Equals, domainstorage.StoragePoolOriginUser)
		c.Assert(pool.Attrs, tc.DeepEquals, map[string]string{
			"key": "val",
		})
		return nil
	})
	s.state.EXPECT().SetModelStoragePools(gomock.Any(), []internal.RecommendedStoragePoolArg{})

	err := s.service.ImportStoragePools(
		c.Context(),
		input,
	)

	c.Check(err, tc.ErrorIsNil)
}

// TestImportStoragePoolsMultipleSuccess tests that multiple storage pools
// are validated and created successfully when no errors occur.
// One pool is user supplied and the other is provider default.
func (s *importSuite) TestImportStoragePoolsMultipleSuccess(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg, err := internalstorage.NewConfig(
		"lxd-btrfs",
		"lxd",
		internalstorage.Attrs{"b": "true"},
	)
	c.Assert(err, tc.ErrorIsNil)

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil).Times(3)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"lxd"}, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("lxd")).Return(s.storageProvider, nil).
		Times(3)
	s.storageProvider.EXPECT().ValidateConfig(gomock.Any()).Times(2)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{cfg})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)
	s.state.EXPECT().SetModelStoragePools(gomock.Any(), []internal.RecommendedStoragePoolArg{})

	gomock.InOrder(
		s.state.EXPECT().
			CreateStoragePool(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, pool internal.CreateStoragePool) error {
				c.Assert(pool.UUID, tc.IsUUID)
				c.Assert(pool.Name, tc.Equals, "lxd")
				c.Assert(pool.ProviderType, tc.Equals,
					domainstorage.ProviderType("lxd"))
				c.Assert(pool.Origin, tc.Equals, domainstorage.StoragePoolOriginUser)
				c.Assert(pool.Attrs, tc.DeepEquals, map[string]string{
					"a": "1",
				})
				return nil
			}),
		s.state.EXPECT().
			CreateStoragePool(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, pool internal.CreateStoragePool) error {
				// This is a provider default pool so we know the UUID.
				c.Assert(pool.UUID, tc.Equals,
					domainstorage.StoragePoolUUID("e1acb8b8-c978-5d53-bc22-2a0e7fd58734"))
				c.Assert(pool.Name, tc.Equals, "lxd-btrfs")
				c.Assert(pool.ProviderType, tc.Equals,
					domainstorage.ProviderType("lxd"))
				c.Assert(pool.Origin, tc.Equals,
					domainstorage.StoragePoolOriginProviderDefault)
				c.Assert(pool.Attrs, tc.DeepEquals, map[string]string{
					"b": "true",
				})
				return nil
			}),
	)
	input := []domainstorage.UserStoragePoolParams{
		{
			Name:       "lxd",
			Provider:   "lxd",
			Attributes: map[string]any{"a": 1},
		},
	}

	err = s.service.ImportStoragePools(
		c.Context(),
		input,
	)

	c.Check(err, tc.ErrorIsNil)
}

// TestImportStoragePoolsNoUserPoolsWithProviderDefaults tests that when no
// user-defined storage pools are provided, provider default pools are still
// created and recommended pools are set.
func (s *importSuite) TestImportStoragePoolsNoUserPoolsWithProviderDefaults(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	defaultPoolCfg, err := internalstorage.NewConfig(
		"ebs-ssd",
		"ebs",
		internalstorage.Attrs{"volume-type": "ssd"},
	)
	c.Assert(err, tc.ErrorIsNil)

	recommendedFSCfg, err := internalstorage.NewConfig(
		"lxd",
		"lxd",
		internalstorage.Attrs{},
	)
	c.Assert(err, tc.ErrorIsNil)

	recommendedBlockCfg, err := internalstorage.NewConfig(
		"loop",
		"loop",
		internalstorage.Attrs{},
	)
	c.Assert(err, tc.ErrorIsNil)

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil).AnyTimes()
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"ebs"}, nil)

	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("ebs")).Return(s.storageProvider, nil).Times(2)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("lxd")).Return(s.storageProvider, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("loop")).Return(s.storageProvider, nil)

	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{defaultPoolCfg})
	s.storageProvider.EXPECT().ValidateConfig(gomock.Any()).AnyTimes()

	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem).Return(recommendedFSCfg)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock).Return(recommendedBlockCfg)

	gomock.InOrder(
		s.state.EXPECT().
			CreateStoragePool(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, pool internal.CreateStoragePool) error {
				c.Assert(pool.UUID, tc.IsUUID)
				c.Assert(pool.Name, tc.Equals, "ebs-ssd")
				c.Assert(pool.ProviderType, tc.Equals, domainstorage.ProviderType("ebs"))
				c.Assert(pool.Origin, tc.Equals, domainstorage.StoragePoolOriginProviderDefault)
				c.Assert(pool.Attrs, tc.DeepEquals, map[string]string{
					"volume-type": "ssd",
				})
				return nil
			}),
		s.state.EXPECT().
			CreateStoragePool(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, pool internal.CreateStoragePool) error {
				c.Assert(pool.UUID, tc.IsUUID)
				c.Assert(pool.Name, tc.Equals, "lxd")
				c.Assert(pool.ProviderType, tc.Equals, domainstorage.ProviderType("lxd"))
				c.Assert(pool.Origin, tc.Equals, domainstorage.StoragePoolOriginProviderDefault)
				c.Assert(pool.Attrs, tc.DeepEquals, map[string]string{})
				return nil
			}),
		s.state.EXPECT().
			CreateStoragePool(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, pool internal.CreateStoragePool) error {
				c.Assert(pool.UUID, tc.IsUUID)
				c.Assert(pool.Name, tc.Equals, "loop")
				c.Assert(pool.ProviderType, tc.Equals, domainstorage.ProviderType("loop"))
				c.Assert(pool.Origin, tc.Equals, domainstorage.StoragePoolOriginProviderDefault)
				c.Assert(pool.Attrs, tc.DeepEquals, map[string]string{})
				return nil
			}),
	)

	s.state.EXPECT().SetModelStoragePools(gomock.Any(), []internal.RecommendedStoragePoolArg{
		// lxd pool
		{
			StoragePoolUUID: "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
		// loop pool
		{
			StoragePoolUUID: "baa26e04-b1f0-50d9-9bf8-4d5a78ffe6ad",
			StorageKind:     domainstorage.StorageKindBlock,
		},
	})

	err = s.service.ImportStoragePools(c.Context(), nil)

	c.Check(err, tc.ErrorIsNil)
}

// TestImportStoragePoolsInvalidProviderType tests that an invalid provider type
// returns [domainstorageerrors.ProviderTypeInvalid].
func (s *importSuite) TestImportStoragePoolsInvalidProviderType(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"-invalid-provider-"}, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("-invalid-provider-")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)

	input := []domainstorage.UserStoragePoolParams{
		{
			Name:     "my-pool",
			Provider: "-invalid-provider-",
		},
	}

	err := s.service.ImportStoragePools(
		c.Context(),
		input,
	)

	c.Check(err, tc.ErrorIs, domainstorageerrors.ProviderTypeInvalid)
}

// TestImportStoragePoolsProviderTypeNotFound tests that importing a storage
// pool for a provider not present in the registry returns
// [domainstorageerrors.ProviderTypeNotFound].
func (s *importSuite) TestImportStoragePoolsProviderTypeNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil).Times(2)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"storagep1"}, nil)
	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("storagep1")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)
	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("storagep1")).
		Return(nil, coreerrors.NotFound)

	input := []domainstorage.UserStoragePoolParams{
		{
			Name:     "my-pool",
			Provider: "storagep1",
		},
	}

	err := s.service.ImportStoragePools(
		c.Context(),
		input,
	)

	c.Check(err, tc.ErrorIs, domainstorageerrors.ProviderTypeNotFound)
}

// TestImportStoragePoolsProviderRegistryError tests that unexpected
// errors returned by the provider registry are propagated.
func (s *importSuite) TestImportStoragePoolsProviderRegistryError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	registryErr := errors.New("registry failure")

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil).Times(2)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"storagep1"}, nil)
	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("storagep1")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)
	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("storageprovider1")).
		Return(nil, registryErr)

	input := []domainstorage.UserStoragePoolParams{
		{
			Name:     "my-pool",
			Provider: "storageprovider1",
		},
	}

	err := s.service.ImportStoragePools(
		c.Context(),
		input,
	)

	c.Check(err, tc.ErrorIs, registryErr)
}

// TestImportStoragePoolsInvalidName tests that an invalid legacy storage
// pool name returns [domainstorageerrors.StoragePoolNameInvalid].
func (s *importSuite) TestImportStoragePoolsInvalidName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"ebs"}, nil)
	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("ebs")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)

	input := []domainstorage.UserStoragePoolParams{
		{
			// Must start with a letter.
			Name:     "66invalid",
			Provider: "ebs",
		},
	}
	err := s.service.ImportStoragePools(
		c.Context(),
		input,
	)

	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNameInvalid)
}

// TestSetRecommendedStoragePools tests that the service correctly converts
// recommended storage pool parameters into model arguments and delegates
// persistence to the state layer without error.
func (s *importSuite) TestSetRecommendedStoragePools(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	uuid1 := tc.Must(c, domainstorage.NewStoragePoolUUID)
	uuid2 := tc.Must(c, domainstorage.NewStoragePoolUUID)

	input := []domainstorage.RecommendedStoragePoolParams{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: uuid1,
		},
		{
			StorageKind:     domainstorage.StorageKindBlock,
			StoragePoolUUID: uuid2,
		},
	}

	s.state.EXPECT().SetModelStoragePools(gomock.Any(), []internal.RecommendedStoragePoolArg{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: uuid1,
		},
		{
			StorageKind:     domainstorage.StorageKindBlock,
			StoragePoolUUID: uuid2,
		},
	})

	err := s.service.setRecommendedStoragePools(c.Context(), input)
	c.Assert(err, tc.ErrorIsNil)
}

// TestSetRecommendedStoragePoolsPoolNotFound tests that the service propagates
// a [domainstorageerrors.StoragePoolNotFound] error returned by the state layer
// when a referenced storage pool does not exist.
func (s *importSuite) TestSetRecommendedStoragePoolsPropagatesError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	expectedErr := domainstorageerrors.StoragePoolNotFound

	s.state.EXPECT().SetModelStoragePools(gomock.Any(), gomock.Any()).Return(
		domainstorageerrors.StoragePoolNotFound)

	err := s.service.setRecommendedStoragePools(c.Context(), []domainstorage.RecommendedStoragePoolParams{
		{
			StorageKind:     domainstorage.StorageKindFilesystem,
			StoragePoolUUID: tc.Must(c, domainstorage.NewStoragePoolUUID),
		},
	})

	c.Check(err, tc.ErrorIs, expectedErr)
}

// TestGetStoragePoolsToImport tests that both user-defined storage pools
// and provider default pools are returned and no recommended storage pools are returned.
func (s *importSuite) TestGetStoragePoolsToImport(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg, err := internalstorage.NewConfig(
		"lxd",
		"lxd",
		internalstorage.Attrs{"foo": "bar"},
	)
	c.Assert(err, tc.ErrorIsNil)

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"lxd"}, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("lxd")).Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{cfg})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)

	input := []domainstorage.UserStoragePoolParams{
		{
			Name:       "custom-pool",
			Provider:   "lxd",
			Attributes: nil,
		},
	}

	pools, recommended, err := s.service.getStoragePoolsToImport(
		c.Context(),
		input,
	)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(recommended, tc.HasLen, 0)
	c.Assert(pools, tc.HasLen, 2)
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:  "custom-pool",
		Type:  "lxd",
		Attrs: nil,
	}))
	c.Assert(pools[1], tc.DeepEquals, domainstorage.ImportStoragePoolParams{
		UUID:   "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
		Name:   "lxd",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginProviderDefault,
		Attrs:  map[string]any{"foo": "bar"},
	})
}

// TestGetStoragePoolsToImportUserPoolsOnly verifies that when only user-defined
// storage pools are present and that there are no provider default pools, the
// service returns user-defined pools, generates UUIDs, and does not include provider default or
// recommended pools.
func (s *importSuite) TestGetStoragePoolsToImportUserPoolsOnly(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"lxd"}, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("lxd")).Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools()
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)

	input := []domainstorage.UserStoragePoolParams{{
		Name:       "user-pool",
		Provider:   "lxd",
		Attributes: map[string]any{"foo": "bar"},
	}}
	pools, recommended, err := s.service.getStoragePoolsToImport(
		c.Context(),
		input,
	)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(recommended, tc.HasLen, 0)
	c.Assert(pools, tc.HasLen, 1)
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "user-pool",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  map[string]any{"foo": "bar"},
	}))
}

// TestGetStoragePoolsToImportPickUserDefinedOnDuplicate ensures that when a user-defined storage
// pool conflicts by name and provider with a provider default pool, the user-defined
// pool is preferred and the conflicting default pool is skipped.
func (s *importSuite) TestGetStoragePoolsToImportPickUserDefinedOnDuplicate(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg1, err := internalstorage.NewConfig(
		"lxd-btrfs",
		"lxd",
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)
	cfg2, err := internalstorage.NewConfig(
		"lxd-zfs",
		"lxd",
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().
		Return([]internalstorage.ProviderType{"lxd"}, nil)
	s.storageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("lxd")).Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return([]*internalstorage.Config{
		cfg1,
		cfg2,
	})
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindFilesystem)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(
		internalstorage.StorageKindBlock)

	input := []domainstorage.UserStoragePoolParams{
		// This pool conflicts with a provider default pool.
		{
			Name:       "lxd-btrfs",
			Provider:   "lxd",
			Attributes: nil,
		},
		{
			Name:       "custom-pool",
			Provider:   "lxd",
			Attributes: nil,
		},
	}

	pools, _, err := s.service.getStoragePoolsToImport(
		c.Context(),
		input,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pools, tc.HasLen, 3)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	// User pool: lxd-btrfs.
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "lxd-btrfs",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  nil,
	}))

	// User pool: custom-pool.
	c.Assert(pools[1], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "custom-pool",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  nil,
	}))

	// Provider default (non-conflicting): lxd-zfs.
	c.Assert(pools[2], tc.DeepEquals, domainstorage.ImportStoragePoolParams{
		// Use the hardcoded UUID for this pool.
		UUID:   "635f1873-be0b-5f07-b841-9fa02466a9f6",
		Name:   "lxd-zfs",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginProviderDefault,
		Attrs:  nil,
	})
}

// TestGetStoragePoolsToImportReturnsRecommendedPools verifies that provider default
// pools are added and recommended storage pools are returned when the registry
// supplies recommendations for specific storage kinds.
func (s *importSuite) TestGetStoragePoolsToImportReturnsRecommendedPools(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	defaultPool, _ := internalstorage.NewConfig("lxd", "lxd", internalstorage.Attrs{})
	zfsPool, _ := internalstorage.NewConfig("lxd-zfs", "lxd", internalstorage.Attrs{
		"driver":        "zfs",
		"lxd-pool":      "juju-zfs",
		"zfs.pool_name": "juju-lxd",
	})
	btrfsPool, _ := internalstorage.NewConfig("lxd-btrfs", "lxd", internalstorage.Attrs{
		"driver":   "btrfs",
		"lxd-pool": "juju-btrfs",
	})
	recommendedPoolForBlock, _ := internalstorage.NewConfig("loop", "loop",
		internalstorage.Attrs{})
	lxdDefaultPools := []*internalstorage.Config{defaultPool, zfsPool, btrfsPool}

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).Return(
		s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]internalstorage.ProviderType{"lxd"}, nil,
	)
	s.storageProviderRegistry.EXPECT().StorageProvider(internalstorage.ProviderType("lxd")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return(lxdDefaultPools)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(internalstorage.StorageKindFilesystem).
		Return(defaultPool)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(internalstorage.StorageKindBlock).
		Return(recommendedPoolForBlock)

	input := []domainstorage.UserStoragePoolParams{
		{
			Name:       "custom-pool",
			Provider:   "lxd",
			Attributes: map[string]any{"foo": "bar"},
		},
	}
	pools, recommended, err := s.service.getStoragePoolsToImport(
		c.Context(),
		input,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(recommended, tc.SameContents, []domainstorage.RecommendedStoragePoolParams{
		// This is a pool with name: lxd and provider: lxd.
		{
			StoragePoolUUID: "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
		// This is a pool with name: loop and provider: loop.
		{
			StoragePoolUUID: "baa26e04-b1f0-50d9-9bf8-4d5a78ffe6ad",
			StorageKind:     domainstorage.StorageKindBlock,
		},
	})

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	c.Assert(pools, tc.HasLen, 5)
	// User pool. The UUID is generated by the service so we don't know the exact
	// value, but we assert that it is a UUID.
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "custom-pool",
		Type:   "lxd",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  map[string]any{"foo": "bar"},
	}))
	// The rest are provider default pools.
	c.Assert(pools[1:], tc.SameContents, []domainstorage.ImportStoragePoolParams{
		{
			UUID:   "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			Name:   "lxd",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs:  map[string]any{},
		},
		{
			UUID:   "635f1873-be0b-5f07-b841-9fa02466a9f6",
			Name:   "lxd-zfs",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs: map[string]any{
				"driver":        "zfs",
				"lxd-pool":      "juju-zfs",
				"zfs.pool_name": "juju-lxd",
			},
		},
		{
			UUID:   "e1acb8b8-c978-5d53-bc22-2a0e7fd58734",
			Name:   "lxd-btrfs",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs: map[string]any{
				"driver":   "btrfs",
				"lxd-pool": "juju-btrfs",
			},
		},
		{
			UUID:   "baa26e04-b1f0-50d9-9bf8-4d5a78ffe6ad",
			Name:   "loop",
			Type:   "loop",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs:  map[string]any{},
		},
	})
}

// TestGetStoragePoolsToImportExcludeConflictingUserPool tests that if there is a recommended provider default
// pool with conflicting name with a user-defined pool, then that pool will not be included
// in the recommended pools because we cannot guarantee they refer to the same pool.
func (s *importSuite) TestGetStoragePoolsToImportExcludeConflictingUserPool(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	defaultPool, _ := internalstorage.NewConfig("lxd", "lxd", internalstorage.Attrs{})
	zfsPool, _ := internalstorage.NewConfig("lxd-zfs", "lxd", internalstorage.Attrs{
		"driver":        "zfs",
		"lxd-pool":      "juju-zfs",
		"zfs.pool_name": "juju-lxd",
	})
	btrfsPool, _ := internalstorage.NewConfig("lxd-btrfs", "lxd", internalstorage.Attrs{
		"driver":   "btrfs",
		"lxd-pool": "juju-btrfs",
	})
	// This is the conflicting pool.
	recommendedPoolForBlock, _ := internalstorage.NewConfig("loop", "loop",
		internalstorage.Attrs{})
	lxdDefaultPools := []*internalstorage.Config{defaultPool, zfsPool, btrfsPool}

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).Return(
		s.storageProviderRegistry, nil)
	s.storageProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]internalstorage.ProviderType{"lxd"}, nil,
	)
	s.storageProviderRegistry.EXPECT().StorageProvider(internalstorage.ProviderType("lxd")).
		Return(s.storageProvider, nil)
	s.storageProvider.EXPECT().DefaultPools().Return(lxdDefaultPools)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(internalstorage.StorageKindFilesystem).
		Return(defaultPool)
	s.storageProviderRegistry.EXPECT().RecommendedPoolForKind(internalstorage.StorageKindBlock).
		Return(recommendedPoolForBlock)

	// This has the same name as the provider loop pool but we cannot guarantee
	// they are refer to the same instance.
	input := []domainstorage.UserStoragePoolParams{
		{
			Name:       "loop",
			Provider:   "loop",
			Attributes: map[string]any{"foo": "bar"},
		},
	}
	pools, recommended, err := s.service.getStoragePoolsToImport(
		c.Context(),
		input,
	)

	c.Assert(err, tc.ErrorIsNil)
	// We assert that only the lxd pool is recommended.
	c.Assert(recommended, tc.DeepEquals, []domainstorage.RecommendedStoragePoolParams{
		// This is a pool name: lxd and provider: lxd.
		{
			StoragePoolUUID: "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			StorageKind:     domainstorage.StorageKindFilesystem,
		},
	})

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UUID", tc.IsUUID)

	c.Assert(pools, tc.HasLen, 4)
	// User pool. The UUID is generated by the service so we don't know the exact
	// value, but we assert that it is a UUID.
	c.Assert(pools[0], tc.Bind(mc, domainstorage.ImportStoragePoolParams{
		Name:   "loop",
		Type:   "loop",
		Origin: domainstorage.StoragePoolOriginUser,
		Attrs:  map[string]any{"foo": "bar"},
	}))
	// The rest are provider default pools.
	c.Assert(pools[1:], tc.SameContents, []domainstorage.ImportStoragePoolParams{
		{
			UUID:   "16d8c090-8ef4-59b4-8e88-0bc64a0598a3",
			Name:   "lxd",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs:  map[string]any{},
		},
		{
			UUID:   "635f1873-be0b-5f07-b841-9fa02466a9f6",
			Name:   "lxd-zfs",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs: map[string]any{
				"driver":        "zfs",
				"lxd-pool":      "juju-zfs",
				"zfs.pool_name": "juju-lxd",
			},
		},
		{
			UUID:   "e1acb8b8-c978-5d53-bc22-2a0e7fd58734",
			Name:   "lxd-btrfs",
			Type:   "lxd",
			Origin: domainstorage.StoragePoolOriginProviderDefault,
			Attrs: map[string]any{
				"driver":   "btrfs",
				"lxd-pool": "juju-btrfs",
			},
		},
	})
}

// TestGetStoragePoolsToImportRegistryGetterError asserts that an error propagated
// correctly when the storage provider registry returns an error.
func (s *importSuite) TestGetStoragePoolsToImportRegistryGetterError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	expectedErr := errors.New("registry down")

	s.storageRegistryGetter.EXPECT().
		GetStorageRegistry(gomock.Any()).
		Return(nil, expectedErr)

	_, _, err := s.service.getStoragePoolsToImport(c.Context(), nil)

	c.Assert(err, tc.ErrorMatches,
		`getting storage provider registry for model: registry down`)
}

// TestGetStoragePoolsToImportProviderTypesError asserts that an error is propagated
// correctly when fetching storage provider types returns an error.
func (s *importSuite) TestGetStoragePoolsToImportProviderTypesError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.storageRegistryGetter.EXPECT().
		GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)

	s.storageProviderRegistry.EXPECT().
		StorageProviderTypes().
		Return(nil, errors.New("types boom"))

	_, _, err := s.service.getStoragePoolsToImport(c.Context(), nil)

	c.Assert(err, tc.ErrorMatches,
		`getting storage provider types for model storage registry: types boom`)
}

// TestGetStoragePoolsToImportStorageProviderError asserts that an error is propagated
// correctly when fetching a specific storage provider returns an error.
func (s *importSuite) TestGetStoragePoolsToImportStorageProviderError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.storageRegistryGetter.EXPECT().
		GetStorageRegistry(gomock.Any()).
		Return(s.storageProviderRegistry, nil)

	s.storageProviderRegistry.EXPECT().
		StorageProviderTypes().
		Return([]internalstorage.ProviderType{"lxd"}, nil)

	s.storageProviderRegistry.EXPECT().
		StorageProvider(internalstorage.ProviderType("lxd")).
		Return(nil, errors.New("provider boom"))

	_, _, err := s.service.getStoragePoolsToImport(c.Context(), nil)

	c.Assert(err, tc.ErrorMatches,
		`getting storage provider "lxd" from registry: provider boom`)
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockStorageImportState(ctrl)
	s.fsModelMigration = NewMockFilesystemModelMigration(ctrl)
	s.storageProviderRegistry = NewMockProviderRegistry(ctrl)
	s.storageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)
	s.storageProvider = NewMockProvider(ctrl)

	s.service = NewImportService(
		s.state,
		loggertesting.WrapCheckLog(c),
		s.storageRegistryGetter,
		func(_ context.Context, fn func(_ context.Context, fs internalstorage.FilesystemModelMigration) error) error {
			return fn(c.Context(), s.fsModelMigration)
		},
	)

	c.Cleanup(func() {
		s.fsModelMigration = nil
		s.registry = internalstorage.StaticProviderRegistry{}
		s.state = nil
		s.service = nil
		s.storageProviderRegistry = nil
		s.storageRegistryGetter = nil
		s.storageProvider = nil
	})

	return ctrl
}
