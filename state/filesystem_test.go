// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
)

type FilesystemStateSuite struct {
	ConnSuite
}

var _ = gc.Suite(&FilesystemStateSuite{})

func (s *FilesystemStateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	// This suite is all about storage, so enable the feature by default.
	s.PatchEnvironment(osenv.JujuFeatureFlagEnvKey, feature.Storage)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	registry.RegisterEnvironStorageProviders(
		"someprovider", provider.LoopProviderType, provider.RootfsProviderType,
	)
}

func (s *FilesystemStateSuite) TestAddServiceInvalidPool(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("invalid-pool", 1024, 1),
	}
	_, err := s.State.AddService("storage-filesystem", s.Owner.String(), ch, nil, storage)
	c.Assert(err, gc.ErrorMatches, `.* pool "invalid-pool" not found`)
}

func (s *FilesystemStateSuite) TestAddServiceNoPool(c *gc.C) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons("", 1024, 1),
	}
	_, err := s.State.AddService("storage-filesystem", s.Owner.String(), ch, nil, storage)
	// TODO(axw) implement support for default filesystem pool.
	c.Assert(err, gc.ErrorMatches, `cannot add service "storage-filesystem": finding default stoage pool: no storage pool specifed and no default available`)
}

func (s *FilesystemStateSuite) TestAddFilesystemWithoutBackingVolume(c *gc.C) {
	s.addUnitWithFilesystem(c, "rootfs", false)
}

func (s *FilesystemStateSuite) TestAddFilesystemWithBackingVolume(c *gc.C) {
	s.addUnitWithFilesystem(c, "loop", true)
}

func (s *FilesystemStateSuite) addUnitWithFilesystem(c *gc.C, pool string, withVolume bool) {
	ch := s.AddTestingCharm(c, "storage-filesystem")
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-filesystem", ch, storage)
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)

	storageAttachments, err := s.State.StorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)
	storageInstance, err := s.State.StorageInstance(storageAttachments[0].StorageInstance())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageInstance.Kind(), gc.Equals, state.StorageKindFilesystem)

	filesystem, err := s.State.StorageInstanceFilesystem(storageInstance.StorageTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystem.FilesystemTag(), gc.Equals, names.NewFilesystemTag("0"))
	filesystemStorageTag, err := filesystem.Storage()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemStorageTag, gc.Equals, storageInstance.StorageTag())
	_, err = filesystem.Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok := filesystem.Params()
	c.Assert(ok, jc.IsTrue)

	volume, err := s.State.StorageInstanceVolume(storageInstance.StorageTag())
	if withVolume {
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(volume.VolumeTag(), gc.Equals, names.NewVolumeTag("0"))
		volumeStorageTag, err := volume.StorageInstance()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(volumeStorageTag, gc.Equals, storageInstance.StorageTag())
		filesystemVolume, err := filesystem.Volume()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(filesystemVolume, gc.Equals, volume.VolumeTag())
	} else {
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
		_, err = filesystem.Volume()
		c.Assert(errors.Cause(err), gc.Equals, state.ErrNoBackingVolume)
	}

	machine, err := s.State.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	filesystemAttachments, err := s.State.MachineFilesystemAttachments(names.NewMachineTag(assignedMachineId))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystemAttachments, gc.HasLen, 1)
	c.Assert(filesystemAttachments[0].Filesystem(), gc.Equals, filesystem.FilesystemTag())
	c.Assert(filesystemAttachments[0].Machine(), gc.Equals, machine.MachineTag())
	_, err = filesystemAttachments[0].Info()
	c.Assert(err, jc.Satisfies, errors.IsNotProvisioned)
	_, ok = filesystemAttachments[0].Params()
	c.Assert(ok, jc.IsTrue)

	_, err = s.State.FilesystemAttachment(machine.MachineTag(), filesystem.FilesystemTag())
	c.Assert(err, jc.ErrorIsNil)
}
