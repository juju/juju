// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/internal"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
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
	expected := []internal.ImportStorageInstanceArgs{
		{
			UUID:             tc.Must(c, uuid.NewUUID).String(),
			StorageName:      "test1",
			StorageKind:      "block",
			StorageID:        "test1/0",
			UnitName:         "unit/3",
			RequestedSizeMiB: 1024,
			PoolName:         "ebs",
		}, {
			UUID:             tc.Must(c, uuid.NewUUID).String(),
			StorageName:      "test1",
			StorageKind:      "block",
			StorageID:        "test1/2",
			UnitName:         "unit/2",
			RequestedSizeMiB: 1024,
			PoolName:         "ebs",
		},
	}

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].UUID`, tc.IsNonZeroUUID)
	s.state.EXPECT().ImportStorageInstances(gomock.Any(), tc.Bind(mc, expected)).Return(nil)

	args := []domainstorage.ImportStorageInstanceParams{
		{
			StorageName:      "test1",
			StorageKind:      "block",
			StorageID:        "test1/0",
			UnitName:         "unit/3",
			RequestedSizeMiB: 1024,
			PoolName:         "ebs",
		}, {
			StorageName:      "test1",
			StorageKind:      "block",
			StorageID:        "test1/2",
			UnitName:         "unit/2",
			RequestedSizeMiB: 1024,
			PoolName:         "ebs",
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

func (s *importSuite) TestImportFilesystemsValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := []domainstorage.ImportFilesystemParams{{}}
	err := s.service.ImportFilesystemsIAAS(c.Context(), args)

	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
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
		s.state = nil
		s.service = nil
	})

	return ctrl
}
