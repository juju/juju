// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storagecommon_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/storagecommon"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

type StorageDetailsSuite struct {
	storageTag names.StorageTag
	unitTag    names.UnitTag
	instance   *fakeStorageInstance
	attachment *fakeStorageAttachment
}

var _ = gc.Suite(&StorageDetailsSuite{})

func (s *StorageDetailsSuite) SetUpTest(_ *gc.C) {
	s.storageTag = names.NewStorageTag("data/0")
	s.unitTag = names.NewUnitTag("mysql/0")
	s.instance = &fakeStorageInstance{
		tag:   s.storageTag,
		owner: s.unitTag,
		kind:  state.StorageKindBlock,
		life:  state.Alive,
	}
	s.attachment = &fakeStorageAttachment{
		storageTag: s.storageTag,
		unitTag:    s.unitTag,
		life:       state.Alive,
	}
}

// TestVolumeWithBackingEntityUsesStatusAndPersistent verifies the happy path
// where the backing volume exists and its status is returned.
func (s *StorageDetailsSuite) TestVolumeWithBackingEntityUsesStatusAndPersistent(c *gc.C) {
	volumeStatus := corestatus.StatusInfo{
		Status:  corestatus.Attached,
		Message: "volume attached",
	}
	st := &fakeStorage{
		storageInstanceVolume: func(tag names.StorageTag) (state.Volume, error) {
			return &fakeVolume{
				tag: names.NewVolumeTag("0"),
				info: &state.VolumeInfo{
					Persistent: true,
				},
				status: &volumeStatus,
			}, nil
		},
	}
	getUnit := func(name string) (storagecommon.Unit, error) {
		return fakeUnit{}, nil
	}

	details, err := storagecommon.StorageDetails(st, nil, s.instance, getUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Status.Status, gc.Equals, corestatus.Attached)
	c.Assert(details.Status.Info, gc.Equals, "volume attached")
	c.Assert(details.Persistent, gc.Equals, true)
	c.Assert(details.Attachments, gc.IsNil)
}

// TestFilesystemWithBackingEntityUsesStatus verifies the happy path where the
// backing filesystem exists and its status is returned.
func (s *StorageDetailsSuite) TestFilesystemWithBackingEntityUsesStatus(c *gc.C) {
	s.instance.kind = state.StorageKindFilesystem
	filesystemStatus := corestatus.StatusInfo{
		Status:  corestatus.Attached,
		Message: "filesystem attached",
	}
	st := &fakeStorage{
		storageInstanceFilesystem: func(tag names.StorageTag) (state.Filesystem, error) {
			return &fakeFilesystem{
				tag:    names.NewFilesystemTag("0"),
				status: &filesystemStatus,
			}, nil
		},
	}
	getUnit := func(name string) (storagecommon.Unit, error) {
		return fakeUnit{}, nil
	}

	details, err := storagecommon.StorageDetails(st, nil, s.instance, getUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Status.Status, gc.Equals, corestatus.Attached)
	c.Assert(details.Status.Info, gc.Equals, "filesystem attached")
	c.Assert(details.Attachments, gc.IsNil)
}

// TestVolumeNotFoundWithAssignedUnitsReturnsError verifies error is surfaced when
// the backing volume is missing despite the unit being assigned to a machine.
func (s *StorageDetailsSuite) TestVolumeNotFoundWithAssignedUnitsReturnsError(c *gc.C) {
	st := &fakeStorage{
		storageInstanceVolume: func(tag names.StorageTag) (state.Volume, error) {
			return nil, errors.NotFoundf("volume for storage %s", tag.Id())
		},
		storageAttachments: func(tag names.StorageTag) ([]state.StorageAttachment, error) {
			return []state.StorageAttachment{s.attachment}, nil
		},
		storageInstance: func(tag names.StorageTag) (state.StorageInstance, error) {
			return s.instance, nil
		},
	}
	unitToMachine := func(names.UnitTag) (names.MachineTag, error) {
		return names.NewMachineTag("0"), nil
	}
	unit := fakeUnit{}
	unit.shouldBeAssigned = true
	unit.machineId = "0"
	getUnit := func(name string) (storagecommon.Unit, error) {
		return unit, nil
	}
	details, err := storagecommon.StorageDetails(st, unitToMachine, s.instance, getUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Status.Status, gc.Equals, corestatus.Error)
	c.Assert(details.Status.Info, gc.Matches, "volume for storage data/0 not found")
}

// TestFilesystemNotFoundWithAssignedUnitsReturnsError verifies error is surfaced when
// the backing filesystem is missing despite the unit being assigned to a machine.
func (s *StorageDetailsSuite) TestFilesystemNotFoundWithAssignedUnitsReturnsError(c *gc.C) {
	s.instance.kind = state.StorageKindFilesystem

	st := &fakeStorage{
		storageInstanceFilesystem: func(tag names.StorageTag) (state.Filesystem, error) {
			return nil, errors.NotFoundf("filesystem for storage %s", tag.Id())
		},
		storageAttachments: func(tag names.StorageTag) ([]state.StorageAttachment, error) {
			return []state.StorageAttachment{s.attachment}, nil
		},
		storageInstance: func(tag names.StorageTag) (state.StorageInstance, error) {
			return s.instance, nil
		},
	}
	unitToMachine := func(names.UnitTag) (names.MachineTag, error) {
		return names.NewMachineTag("0"), nil
	}
	unit := fakeUnit{}
	unit.shouldBeAssigned = true
	unit.machineId = "0"
	getUnit := func(name string) (storagecommon.Unit, error) {
		return unit, nil
	}

	details, err := storagecommon.StorageDetails(st, unitToMachine, s.instance, getUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Status.Status, gc.Equals, corestatus.Error)
	c.Assert(details.Status.Info, gc.Matches, "filesystem for storage data/0 not found")
}

// TestVolumeNotFoundWithNotAssignedUnitsReturnsPending verifies status is surfaced
// as pending when the backing volume is not found due to the unit not being assigned
// to a machine yet.
func (s *StorageDetailsSuite) TestVolumeNotFoundWithNotAssignedUnitsReturnsPending(c *gc.C) {
	st := &fakeStorage{
		storageInstanceVolume: func(tag names.StorageTag) (state.Volume, error) {
			return nil, errors.NotFoundf("volume for storage %s", tag.Id())
		},
		storageAttachments: func(tag names.StorageTag) ([]state.StorageAttachment, error) {
			return []state.StorageAttachment{s.attachment}, nil
		},
	}
	unitToMachine := func(names.UnitTag) (names.MachineTag, error) {
		return names.MachineTag{}, errors.NewNotAssigned(nil, "unit not assigned")
	}
	unit := fakeUnit{}
	unit.shouldBeAssigned = true
	unit.err = errors.NewNotAssigned(nil, "unit not assigned")
	getUnit := func(name string) (storagecommon.Unit, error) {
		return unit, nil
	}

	details, err := storagecommon.StorageDetails(st, unitToMachine, s.instance, getUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Status.Status, gc.Equals, corestatus.Pending)
	c.Assert(details.Status.Info, gc.Equals, "waiting for volume to be provisioned")
	c.Assert(details.Attachments, gc.HasLen, 1)
	c.Assert(details.Attachments[s.unitTag.String()].MachineTag, gc.Equals, "")
}

// TestFilesystemNotFoundWithNotAssignedUnitsReturnsPending verifies status is surfaced
// as pending when the backing filesystem is not found due to the unit not being assigned
// to a machine yet.
func (s *StorageDetailsSuite) TestFilesystemNotFoundWithNotAssignedUnitsReturnsPending(c *gc.C) {
	s.instance.kind = state.StorageKindFilesystem
	st := &fakeStorage{
		storageInstanceFilesystem: func(tag names.StorageTag) (state.Filesystem, error) {
			return nil, errors.NotFoundf("filesystem for storage %s", tag.Id())
		},
		storageAttachments: func(tag names.StorageTag) ([]state.StorageAttachment, error) {
			return []state.StorageAttachment{s.attachment}, nil
		},
	}
	unitToMachine := func(names.UnitTag) (names.MachineTag, error) {
		return names.MachineTag{}, errors.NewNotAssigned(nil, "unit not assigned")
	}
	unit := fakeUnit{}
	unit.shouldBeAssigned = true
	unit.err = errors.NewNotAssigned(nil, "unit not assigned")
	getUnit := func(name string) (storagecommon.Unit, error) {
		return unit, nil
	}

	details, err := storagecommon.StorageDetails(st, unitToMachine, s.instance, getUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Status.Status, gc.Equals, corestatus.Pending)
	c.Assert(details.Status.Info, gc.Equals, "waiting for filesystem to be provisioned")
	c.Assert(details.Status.Since, gc.NotNil)
	c.Assert(details.Attachments, gc.HasLen, 1)
	c.Assert(details.Attachments[s.unitTag.String()].MachineTag, gc.Equals, "")
}

// TestVolumeNotFoundPropagatesUnitAssignmentError verifies that unexpected getUnit errors
// are propagated to the caller.
func (s *StorageDetailsSuite) TestVolumeNotFoundPropagatesUnitAssignmentError(c *gc.C) {
	st := &fakeStorage{
		storageInstanceVolume: func(tag names.StorageTag) (state.Volume, error) {
			return nil, errors.NotFoundf("volume for storage %s", tag.Id())
		},
	}
	unitToMachine := func(names.UnitTag) (names.MachineTag, error) {
		return names.MachineTag{}, nil
	}
	unit := fakeUnit{}
	unit.shouldBeAssigned = true
	unit.err = errors.New("cannot determine unit machine")
	getUnit := func(name string) (storagecommon.Unit, error) {
		return unit, nil
	}

	_, err := storagecommon.StorageDetails(st, unitToMachine, s.instance, getUnit)
	c.Assert(err, gc.ErrorMatches, ".*cannot determine unit machine.*")
}

// TestFilesystemNotFoundPropagatesUnitAssignmentError verifies that unexpected
// unitToMachine errors are propagated to the caller.
func (s *StorageDetailsSuite) TestFilesystemNotFoundPropagatesUnitAssignmentError(c *gc.C) {
	s.instance.kind = state.StorageKindFilesystem
	st := &fakeStorage{
		storageInstanceFilesystem: func(tag names.StorageTag) (state.Filesystem, error) {
			return nil, errors.NotFoundf("filesystem for storage %s", tag.Id())
		},
	}
	unitToMachine := func(names.UnitTag) (names.MachineTag, error) {
		return names.MachineTag{}, nil
	}
	unit := fakeUnit{}
	unit.shouldBeAssigned = true
	unit.err = errors.New("cannot determine unit machine")
	getUnit := func(name string) (storagecommon.Unit, error) {
		return unit, nil
	}

	_, err := storagecommon.StorageDetails(st, unitToMachine, s.instance, getUnit)
	c.Assert(err, gc.ErrorMatches, ".*cannot determine unit machine.*")
}

// TestVolumeNotFoundPropagatesErrorWhenMissingOwner verifies error is returned
// when a storage instance does not have an owner.
func (s *StorageDetailsSuite) TestVolumeNotFoundPropagatesErrorWhenMissingOwner(c *gc.C) {
	st := &fakeStorage{
		storageInstanceVolume: func(tag names.StorageTag) (state.Volume, error) {
			return nil, errors.NotFoundf("volume for storage %s", tag.Id())
		},
	}
	unitToMachine := func(names.UnitTag) (names.MachineTag, error) {
		return names.MachineTag{}, nil
	}
	// Set owner to nil to simulate missing owner.
	s.instance.owner = nil
	getUnit := func(name string) (storagecommon.Unit, error) {
		return fakeUnit{}, nil
	}
	details, err := storagecommon.StorageDetails(st, unitToMachine, s.instance, getUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Status.Status, gc.Equals, corestatus.Error)
	c.Assert(details.Status.Info, gc.Equals, "volume for storage data/0 not found")
}

// TestVolumeNotFoundPropagatesErrorWhenOwnerIsNonUnit verifies error is returned
// when a storage instance owner is not a UnitTagKind.
func (s *StorageDetailsSuite) TestVolumeNotFoundPropagatesErrorWhenOwnerIsNonUnit(c *gc.C) {
	st := &fakeStorage{
		storageInstanceVolume: func(tag names.StorageTag) (state.Volume, error) {
			return nil, errors.NotFoundf("volume for storage %s", tag.Id())
		},
	}
	unitToMachine := func(names.UnitTag) (names.MachineTag, error) {
		return names.MachineTag{}, nil
	}
	// Set owner to a non-unit tag.
	s.instance.owner = names.NewApplicationTag("someapp")
	getUnit := func(name string) (storagecommon.Unit, error) {
		return fakeUnit{}, nil
	}
	details, err := storagecommon.StorageDetails(st, unitToMachine, s.instance, getUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Status.Status, gc.Equals, corestatus.Error)
	c.Assert(details.Status.Info, gc.Equals, "volume for storage data/0 not found")
}

// TestFilesystemNotFoundPropagatesErrorWhenMissingOwner verifies error is returned
// when a storage instance does not have an owner.
func (s *StorageDetailsSuite) TestFilesystemNotFoundPropagatesErrorWhenMissingOwner(c *gc.C) {
	s.instance.kind = state.StorageKindFilesystem
	st := &fakeStorage{
		storageInstanceFilesystem: func(tag names.StorageTag) (state.Filesystem, error) {
			return nil, errors.NotFoundf("filesystem for storage %s", tag.Id())
		},
	}
	unitToMachine := func(names.UnitTag) (names.MachineTag, error) {
		return names.MachineTag{}, nil
	}
	// Set owner to nil to simulate missing owner.
	s.instance.owner = nil
	getUnit := func(name string) (storagecommon.Unit, error) {
		return fakeUnit{}, nil
	}
	details, err := storagecommon.StorageDetails(st, unitToMachine, s.instance, getUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Status.Status, gc.Equals, corestatus.Error)
	c.Assert(details.Status.Info, gc.Equals, "filesystem for storage data/0 not found")
}

// TestFilesystemNotFoundPropagatesErrorWhenOwnerIsNonUnit verifies error is returned
// when a storage instance owner is not a UnitTagKind.
func (s *StorageDetailsSuite) TestFilesystemNotFoundPropagatesErrorWhenOwnerIsNonUnit(c *gc.C) {
	s.instance.kind = state.StorageKindFilesystem
	st := &fakeStorage{
		storageInstanceFilesystem: func(tag names.StorageTag) (state.Filesystem, error) {
			return nil, errors.NotFoundf("filesystem for storage %s", tag.Id())
		},
	}
	unitToMachine := func(names.UnitTag) (names.MachineTag, error) {
		return names.MachineTag{}, nil
	}
	// Set owner to a non-unit tag.
	s.instance.owner = names.NewApplicationTag("someapp")
	getUnit := func(name string) (storagecommon.Unit, error) {
		return fakeUnit{}, nil
	}
	details, err := storagecommon.StorageDetails(st, unitToMachine, s.instance, getUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Status.Status, gc.Equals, corestatus.Error)
	c.Assert(details.Status.Info, gc.Equals, "filesystem for storage data/0 not found")
}

// TestVolumeNotFoundPropagatesErrorWhenUnitShouldNotBeAssigned verifies error is returned
// when a volume storage backing is missing and the unit should not be assigned,
// which means it's a CAAS deployment.
func (s *StorageDetailsSuite) TestVolumeNotFoundPropagatesErrorWhenUnitShouldNotBeAssigned(c *gc.C) {
	st := &fakeStorage{
		storageInstanceVolume: func(tag names.StorageTag) (state.Volume, error) {
			return nil, errors.NotFoundf("volume for storage %s", tag.Id())
		},
	}
	unitToMachine := func(names.UnitTag) (names.MachineTag, error) {
		return names.MachineTag{}, nil
	}
	getUnit := func(name string) (storagecommon.Unit, error) {
		// fakeUnit has shouldBeAssigned=false to simulate a CAAS unit.
		return fakeUnit{}, nil
	}
	details, err := storagecommon.StorageDetails(st, unitToMachine, s.instance, getUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Status.Status, gc.Equals, corestatus.Error)
	c.Assert(details.Status.Info, gc.Equals, "volume for storage data/0 not found")
}

// TestFilesystemNotFoundPropagatesErrorWhenUnitShouldNotBeAssigned verifies error is returned
// when a filesystem storage backing is missing and the unit should not be assigned,
// which means it's a CAAS deployment.
func (s *StorageDetailsSuite) TestFilesystemNotFoundPropagatesErrorWhenUnitShouldNotBeAssigned(c *gc.C) {
	s.instance.kind = state.StorageKindFilesystem
	st := &fakeStorage{
		storageInstanceFilesystem: func(tag names.StorageTag) (state.Filesystem, error) {
			return nil, errors.NotFoundf("filesystem for storage %s", tag.Id())
		},
	}
	unitToMachine := func(names.UnitTag) (names.MachineTag, error) {
		return names.MachineTag{}, nil
	}
	getUnit := func(name string) (storagecommon.Unit, error) {
		// fakeUnit has shouldBeAssigned=false to simulate a CAAS unit.
		return fakeUnit{}, nil
	}
	details, err := storagecommon.StorageDetails(st, unitToMachine, s.instance, getUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(details.Status.Status, gc.Equals, corestatus.Error)
	c.Assert(details.Status.Info, gc.Equals, "filesystem for storage data/0 not found")
}
