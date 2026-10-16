// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/application/charm"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// unitStorageArgsSuite is a suite of tests asserting the functionality on
// offer by the unit storage arguments making funcs of this package.
type unitStorageArgsSuite struct {
	poolProvider *MockStoragePoolProvider
}

func TestUnitStorageArgsSuite(t *testing.T) {
	tc.Run(t, &unitStorageArgsSuite{})
}

func (s *unitStorageArgsSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.poolProvider = NewMockStoragePoolProvider(ctrl)
	c.Cleanup(func() {
		s.poolProvider = nil
	})
	return ctrl
}

// TestMakeNewUnitStorageArgs tests the [MakeNewUnitStorageArgs] func. It
// wants to see that the storage arguments for a brand new unit are made
// from the given storage directives, with the storage instances attached
// to the net node of the machine hosting the unit, and that the machine
// scoped storage entities are owned by the machine.
func (s *unitStorageArgsSuite) TestMakeNewUnitStorageArgs(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	storageDirectives := []StorageDirective{
		{
			CharmMetadataName: "sub-charm",
			CharmStorageType:  charm.StorageFilesystem,
			Count:             1,
			MaxCount:          charm.StorageNoMaxCount,
			Name:              "data",
			PoolUUID:          poolUUID,
			Size:              1024,
		},
	}

	provider := NewMockInternalProvider(ctrl)
	provider.EXPECT().Scope().Return(internalstorage.ScopeMachine).AnyTimes()
	provider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true).AnyTimes()
	s.poolProvider.EXPECT().GetProviderForPool(gomock.Any(), poolUUID).Return(
		provider, nil,
	).AnyTimes()

	unitStorageArgs, iaasUnitStorageArgs, err := MakeNewUnitStorageArgs(
		c.Context(),
		s.poolProvider,
		machineNetNodeUUID,
		storageDirectives,
	)
	c.Assert(err, tc.ErrorIsNil)

	// The storage directive is passed through to the unit storage arguments.
	c.Check(unitStorageArgs.StorageDirectives, tc.DeepEquals, []domainstorage.DirectiveArg{
		{
			Count:    1,
			Name:     "data",
			PoolUUID: poolUUID,
			Size:     1024,
		},
	})

	// A single new storage instance is created for the new unit, and is
	// attached to the net node of the machine hosting the unit.
	c.Check(unitStorageArgs.StorageInstances, tc.HasLen, 1)
	c.Check(unitStorageArgs.StorageInstances[0].Name, tc.Equals, domainstorage.Name("data"))
	c.Check(unitStorageArgs.StorageInstances[0].CharmName, tc.Equals, "sub-charm")
	c.Check(unitStorageArgs.StorageInstances[0].Kind, tc.Equals, domainstorage.StorageKindFilesystem)
	c.Check(unitStorageArgs.StorageInstances[0].RequestSizeMiB, tc.Equals, uint64(1024))
	c.Check(unitStorageArgs.StorageInstances[0].StoragePoolUUID, tc.Equals, poolUUID)
	c.Check(unitStorageArgs.StorageToAttach, tc.HasLen, 1)
	c.Check(unitStorageArgs.StorageToAttach[0].StorageInstanceUUID, tc.Equals, unitStorageArgs.StorageInstances[0].UUID)
	c.Check(unitStorageArgs.StorageToAttach[0].FilesystemAttachment, tc.NotNil)
	if filesystemAttachment := unitStorageArgs.StorageToAttach[0].FilesystemAttachment; filesystemAttachment != nil {
		c.Check(filesystemAttachment.NetNodeUUID, tc.Equals, machineNetNodeUUID)
		c.Check(filesystemAttachment.ProvisionScope, tc.Equals, domainstorage.ProvisionScopeMachine)
	}
	c.Check(unitStorageArgs.StorageToOwn, tc.DeepEquals, []domainstorage.StorageInstanceUUID{
		unitStorageArgs.StorageInstances[0].UUID,
	})

	// The machine scoped filesystem is owned by the unit's machine.
	c.Check(iaasUnitStorageArgs.FilesystemsToOwn, tc.DeepEquals, []domainstorage.FilesystemUUID{
		unitStorageArgs.StorageInstances[0].Filesystem.UUID,
	})
	c.Check(iaasUnitStorageArgs.VolumesToOwn, tc.IsNil)
}

// TestMakeNewUnitStorageArgsNoStorageDirectives tests that making the
// storage arguments for a new unit of an application without storage
// directives returns zero-value arguments, proving that unit creation is
// not blocked for charms without storage.
func (s *unitStorageArgsSuite) TestMakeNewUnitStorageArgsNoStorageDirectives(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)

	unitStorageArgs, iaasUnitStorageArgs, err := MakeNewUnitStorageArgs(
		c.Context(),
		s.poolProvider,
		machineNetNodeUUID,
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	// No storage is provisioned for an application without storage
	// directives.
	c.Check(unitStorageArgs.StorageDirectives, tc.HasLen, 0)
	c.Check(unitStorageArgs.StorageInstances, tc.HasLen, 0)
	c.Check(unitStorageArgs.StorageToAttach, tc.HasLen, 0)
	c.Check(unitStorageArgs.StorageToOwn, tc.HasLen, 0)
	c.Check(iaasUnitStorageArgs, tc.DeepEquals, domainstorage.CreateIAASUnitStorageArg{})
}

// TestMakeNewUnitStorageArgsVolume tests that a block storage directive
// results in a block storage instance with its volume attached to the net
// node of the machine hosting the unit, and the machine owning the volume.
func (s *unitStorageArgsSuite) TestMakeNewUnitStorageArgsVolume(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	storageDirectives := []StorageDirective{
		{
			CharmMetadataName: "sub-charm",
			CharmStorageType:  charm.StorageBlock,
			Count:             1,
			MaxCount:          charm.StorageNoMaxCount,
			Name:              "data",
			PoolUUID:          poolUUID,
			Size:              1024,
		},
	}

	provider := NewMockInternalProvider(ctrl)
	provider.EXPECT().Scope().Return(internalstorage.ScopeMachine).AnyTimes()
	provider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true).AnyTimes()
	s.poolProvider.EXPECT().GetProviderForPool(gomock.Any(), poolUUID).Return(
		provider, nil,
	).AnyTimes()

	unitStorageArgs, iaasUnitStorageArgs, err := MakeNewUnitStorageArgs(
		c.Context(),
		s.poolProvider,
		machineNetNodeUUID,
		storageDirectives,
	)
	c.Assert(err, tc.ErrorIsNil)

	// A single new block storage instance is created for the new unit.
	c.Assert(unitStorageArgs.StorageInstances, tc.HasLen, 1)
	instance := unitStorageArgs.StorageInstances[0]
	c.Check(instance.CharmName, tc.Equals, "sub-charm")
	c.Check(instance.Kind, tc.Equals, domainstorage.StorageKindBlock)
	c.Assert(instance.Volume, tc.NotNil)
	c.Check(instance.Volume.ProvisionScope, tc.Equals, domainstorage.ProvisionScopeMachine)

	// The volume is attached to the net node of the machine hosting the
	// unit.
	c.Assert(unitStorageArgs.StorageToAttach, tc.HasLen, 1)
	volAttachment := unitStorageArgs.StorageToAttach[0].VolumeAttachment
	c.Assert(volAttachment, tc.NotNil)
	c.Check(volAttachment.VolumeUUID, tc.Equals, instance.Volume.UUID)
	c.Check(volAttachment.NetNodeUUID, tc.Equals, machineNetNodeUUID)
	c.Check(volAttachment.ProvisionScope, tc.Equals, domainstorage.ProvisionScopeMachine)

	// The unit owns the storage instance, and the machine owns the volume.
	c.Check(unitStorageArgs.StorageToOwn, tc.DeepEquals, []domainstorage.StorageInstanceUUID{
		instance.UUID,
	})
	c.Check(iaasUnitStorageArgs.VolumesToOwn, tc.DeepEquals, []domainstorage.VolumeUUID{
		instance.Volume.UUID,
	})
	c.Check(iaasUnitStorageArgs.FilesystemsToOwn, tc.IsNil)
}

// TestMakeNewUnitStorageArgsPoolProviderError tests that an error looking
// up the storage provider for a pool is returned when making the storage
// arguments for a new unit.
func (s *unitStorageArgsSuite) TestMakeNewUnitStorageArgsPoolProviderError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineNetNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	storageDirectives := []StorageDirective{
		{
			CharmMetadataName: "sub-charm",
			CharmStorageType:  charm.StorageFilesystem,
			Count:             1,
			MaxCount:          charm.StorageNoMaxCount,
			Name:              "data",
			PoolUUID:          poolUUID,
			Size:              1024,
		},
	}

	providerErr := errors.New("boom")
	s.poolProvider.EXPECT().GetProviderForPool(gomock.Any(), poolUUID).
		Return(nil, providerErr)

	_, _, err := MakeNewUnitStorageArgs(
		c.Context(), s.poolProvider, machineNetNodeUUID, storageDirectives,
	)
	c.Assert(err, tc.ErrorIs, providerErr)
}

// TestMakeIAASUnitStorageArgs tests the [MakeIAASUnitStorageArgs] func. It
// wants to see that the machine scoped filesystems and volumes of the
// supplied storage instances are to be owned by the machine, and that no
// other storage entities are.
func (s *unitStorageArgsSuite) TestMakeIAASUnitStorageArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsUUID1 := tc.Must(c, domainstorage.NewFilesystemUUID)
	fsUUID2 := tc.Must(c, domainstorage.NewFilesystemUUID)
	volUUID1 := tc.Must(c, domainstorage.NewVolumeUUID)
	volUUID2 := tc.Must(c, domainstorage.NewVolumeUUID)

	expectedStorageInstances := []domainstorage.CreateUnitStorageInstanceArg{
		{
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				UUID:           tc.Must(c, domainstorage.NewFilesystemUUID),
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
			Volume: &domainstorage.CreateUnitStorageVolumeArg{
				UUID:           tc.Must(c, domainstorage.NewVolumeUUID),
				ProvisionScope: domainstorage.ProvisionScopeModel,
			},
		},
		{
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				UUID:           tc.Must(c, domainstorage.NewFilesystemUUID),
				ProvisionScope: domainstorage.ProvisionScopeModel,
			},
		},
		{
			Volume: &domainstorage.CreateUnitStorageVolumeArg{
				UUID:           tc.Must(c, domainstorage.NewVolumeUUID),
				ProvisionScope: domainstorage.ProvisionScopeModel,
			},
		},
		{
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				UUID:           fsUUID1,
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
		},
		{
			Volume: &domainstorage.CreateUnitStorageVolumeArg{
				UUID:           volUUID1,
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
		},
		{
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				UUID:           fsUUID2,
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
			Volume: &domainstorage.CreateUnitStorageVolumeArg{
				UUID:           volUUID2,
				ProvisionScope: domainstorage.ProvisionScopeMachine,
			},
		},
	}

	arg, err := MakeIAASUnitStorageArgs(c.Context(), expectedStorageInstances)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(arg.FilesystemsToOwn, tc.SameContents,
		[]domainstorage.FilesystemUUID{
			fsUUID1,
			fsUUID2,
		},
	)
	c.Check(arg.VolumesToOwn, tc.SameContents,
		[]domainstorage.VolumeUUID{
			volUUID1,
			volUUID2,
		},
	)
}
