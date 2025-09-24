// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	domainstorage "github.com/juju/juju/domain/storage"
	internalstorage "github.com/juju/juju/internal/storage"
)

// provisioningSuite is a test suite asserting storage provisioning business
// logic that is offered up in this package.
type provisioningSuite struct {
	storageProvider *MockStorageProvider
}

// TestProvisioningSuite runs all of the tests contained within
// [provisioningSuite].
func TestProvisioningSuite(t *testing.T) {
	tc.Run(t, &provisioningSuite{})
}

func (s *provisioningSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.storageProvider = NewMockStorageProvider(ctrl)

	c.Cleanup(func() {
		s.storageProvider = nil
	})

	return ctrl
}

// TestBlockCompositionVolumeBackedModelScoped asserts the composition
// when a block device is requested and the provider supports model provisioned
// volumes.
func (s *provisioningSuite) TestBlockCompositionVolumeBackedModelScoped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem,
	).Return(false).AnyTimes()
	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindBlock,
	).Return(true).AnyTimes()
	s.storageProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)

	comp, err := CalculateStorageInstanceComposition(
		domainstorage.StorageKindBlock, s.storageProvider,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(comp, tc.Equals, StorageInstanceComposition{
		VolumeProvisionScope: ProvisionScopeModel,
		VolumeRequired:       true,
	})
}

// TestBlockCompositionVolumeBackedMachineScoped asserts the composition
// when a block device is requested and the provider supports machine
// provisioned volumes.
func (s *provisioningSuite) TestBlockCompositionVolumeBackedMachineScoped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem,
	).Return(false).AnyTimes()
	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindBlock,
	).Return(true).AnyTimes()
	s.storageProvider.EXPECT().Scope().Return(internalstorage.ScopeMachine)

	comp, err := CalculateStorageInstanceComposition(
		domainstorage.StorageKindBlock, s.storageProvider,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(comp, tc.Equals, StorageInstanceComposition{
		VolumeProvisionScope: ProvisionScopeMachine,
		VolumeRequired:       true,
	})
}

// TestBlockCompositionVolumesNotSupported asserts that if the provider does not
// support a volume source than the caller gets back an error as the provider
// cannot be used.
func (s *provisioningSuite) TestBlockCompositionVolumesNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem,
	).Return(false).AnyTimes()
	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindBlock,
	).Return(false).AnyTimes()
	s.storageProvider.EXPECT().Scope().Return(internalstorage.ScopeMachine)

	_, err := CalculateStorageInstanceComposition(
		domainstorage.StorageKindBlock, s.storageProvider,
	)
	c.Check(err, tc.NotNil)
}

// TestFilesystemCompositionFilesystemBackedMachineScoped asserts the
// composition when a filesystem is requested and the provider supports machine
// provisioned filesystems.
func (s *provisioningSuite) TestFilesystemCompositionFilesystemBackedMachineScoped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem,
	).Return(true).AnyTimes()
	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindBlock,
	).Return(false).AnyTimes()
	s.storageProvider.EXPECT().Scope().Return(internalstorage.ScopeMachine)

	comp, err := CalculateStorageInstanceComposition(
		domainstorage.StorageKindFilesystem, s.storageProvider,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(comp, tc.Equals, StorageInstanceComposition{
		FilesystemProvisionScope: ProvisionScopeMachine,
		FilesystemRequired:       true,
	})
}

// TestFilesystemCompositionFilesystemBackedModelScoped asserts the
// composition when a filesystem is requested and the provider supports model
// provisioned filesystems.
func (s *provisioningSuite) TestFilesystemCompositionFilesystemBackedModelScoped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem,
	).Return(true).AnyTimes()
	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindBlock,
	).Return(false).AnyTimes()
	s.storageProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)

	comp, err := CalculateStorageInstanceComposition(
		domainstorage.StorageKindFilesystem, s.storageProvider,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(comp, tc.Equals, StorageInstanceComposition{
		FilesystemProvisionScope: ProvisionScopeModel,
		FilesystemRequired:       true,
	})
}

// TestFilesystemCompositionSupportsFilesystemAndVolume asserts the composition
// of a filesystem when the provider supports both filesystems and volume
// sources.
//
// This test is important because we will try and create filesystems on volumes
// if the composition thinks a filesystem source is not available. This test
// ensures that if a filesystem source is on offer from the provider it is
// always chosen.
//
// If this test fails for you it means you have most likely re-arranged logic
// internally and this is your chance to make the change conform to the
// contract.
func (s *provisioningSuite) TestFilesystemCompositionSupportsFilesystemAndVolume(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem,
	).Return(true).AnyTimes()
	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindBlock,
	).Return(true).AnyTimes()
	s.storageProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)

	comp, err := CalculateStorageInstanceComposition(
		domainstorage.StorageKindFilesystem, s.storageProvider,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(comp, tc.Equals, StorageInstanceComposition{
		FilesystemProvisionScope: ProvisionScopeModel,
		FilesystemRequired:       true,
	})
}

// TestFilesystemCompositionSupportsVolumeMachineScoped asserts that filesystems
// are composed on top of volumes when the provider does not support filesystems.
// We must see that the volume provision scope matches the provider and that the
// filesystem provision scope is set to machine.
func (s *provisioningSuite) TestFilesystemCompositionSupportsVolumeMachineScoped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem,
	).Return(false).AnyTimes()
	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindBlock,
	).Return(true).AnyTimes()
	s.storageProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)

	comp, err := CalculateStorageInstanceComposition(
		domainstorage.StorageKindFilesystem, s.storageProvider,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(comp, tc.Equals, StorageInstanceComposition{
		FilesystemProvisionScope: ProvisionScopeMachine,
		FilesystemRequired:       true,
		VolumeProvisionScope:     ProvisionScopeModel,
		VolumeRequired:           true,
	})
}

// TestFilesystemCompositionSupportsVolumeModelScoped asserts that filesystems
// are composed on top of volumes when the provider does not support filesystems.
// We must see that the volume provision scope matches the provider and that the
// filesystem provision scope is set to machine.
func (s *provisioningSuite) TestFilesystemCompositionSupportsVolumeModelScoped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindFilesystem,
	).Return(false).AnyTimes()
	s.storageProvider.EXPECT().Supports(
		internalstorage.StorageKindBlock,
	).Return(true).AnyTimes()
	s.storageProvider.EXPECT().Scope().Return(internalstorage.ScopeEnviron)

	comp, err := CalculateStorageInstanceComposition(
		domainstorage.StorageKindFilesystem, s.storageProvider,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(comp, tc.Equals, StorageInstanceComposition{
		FilesystemProvisionScope: ProvisionScopeMachine,
		FilesystemRequired:       true,
		VolumeProvisionScope:     ProvisionScopeModel,
		VolumeRequired:           true,
	})
}
