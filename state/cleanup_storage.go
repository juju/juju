// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/status"
)

type dyingEntityStorageCleaner struct {
	sb      *storageBackend
	hostTag names.Tag
	manual  bool
	force   bool
}

func newDyingEntityStorageCleaner(sb *storageBackend, hostTag names.Tag, manual, force bool) *dyingEntityStorageCleaner {
	return &dyingEntityStorageCleaner{
		sb:      sb,
		hostTag: hostTag,
		manual:  manual,
		force:   force,
	}
}

func (c *dyingEntityStorageCleaner) cleanupStorage(
	filesystemAttachments []FilesystemAttachment,
	volumeAttachments []VolumeAttachment,
) error {
	filesystems, err := c.destroyNonDetachableFileSystems()
	if err != nil {
		return errors.Trace(err)
	}

	// Detach all filesystems from the machine/unit.
	for _, fsa := range filesystemAttachments {
		if err := c.detachFileSystem(fsa); err != nil {
			return errors.Trace(err)
		}
	}

	// For non-manual machines we immediately remove the non-detachable
	// filesystems, which should have been detached above. Short-circuiting
	// the removal of machine filesystems means we can avoid stuck
	// filesystems preventing any model-scoped backing volumes from being
	// detached and destroyed. For non-manual machines this is safe, because
	// the machine is about to be terminated. For manual machines, stuck
	// filesystems will have to be fixed manually.
	if !c.manual {
		for _, f := range filesystems {
			if err := c.sb.RemoveFilesystem(f.FilesystemTag()); err != nil && !errors.Is(err, errors.NotFound) {
				if !c.force {
					return errors.Trace(err)
				}
				logger.Warningf("could not remove filesystem %v for dying %v: %v", f.FilesystemTag().Id(), c.hostTag, err)
			}
		}
	}

	// Detach all remaining volumes from the machine.
	for _, va := range volumeAttachments {
		if detachable, err := isDetachableVolumeTag(c.sb.mb.db(), va.Volume()); err != nil && !errors.Is(err, errors.NotFound) {
			if !c.force {
				return errors.Trace(err)
			}
			logger.Warningf("could not determine if volume %v for dying %v is detachable: %v", va.Volume().Id(), c.hostTag, err)
		} else if !detachable {
			// Non-detachable volumes will be removed along with the machine.
			continue
		}
		if err := c.sb.DetachVolume(va.Host(), va.Volume(), c.force); err != nil && !errors.Is(err, errors.NotFound) {
			if IsContainsFilesystem(err) {
				// The volume will be destroyed when the
				// contained filesystem is removed, whose
				// destruction is initiated above.
				continue
			}
			if !c.force {
				return errors.Trace(err)
			}
			logger.Warningf("could not detach volume %v for dying %v: %v", va.Volume().Id(), c.hostTag, err)
		}
	}
	return nil
}

// destroyNonDetachableFileSystems marks file-systems and their attachments for
// future destruction. Such file-systems are indicated as non-detachable by the
// presence of a host ID; see `filesystemDoc`.
func (c *dyingEntityStorageCleaner) destroyNonDetachableFileSystems() ([]*filesystem, error) {
	filesystems, err := c.sb.filesystems(bson.D{{"hostid", c.hostTag.Id()}})
	if err != nil && !errors.Is(err, errors.NotFound) {
		err = errors.Annotate(err, "getting host filesystems")
		if !c.force {
			return nil, err
		}
		logger.Warningf("%v", err)
	}

	for _, f := range filesystems {
		if err := c.sb.DestroyFilesystem(f.FilesystemTag(), c.force); err != nil && !errors.Is(err, errors.NotFound) {
			if !c.force {
				return nil, errors.Trace(err)
			}
			logger.Warningf("could not destroy filesystem %v for %v: %v", f.FilesystemTag().Id(), c.hostTag, err)
		}
	}

	return filesystems, nil
}

func (c *dyingEntityStorageCleaner) detachFileSystem(fsa FilesystemAttachment) error {
	filesystem := fsa.Filesystem()

	detachable, err := isDetachableFilesystemTag(c.sb.mb.db(), filesystem)
	if err != nil && !errors.Is(err, errors.NotFound) {
		if !c.force {
			return errors.Trace(err)
		}
		logger.Warningf("could not determine if filesystem %v for %v is detachable: %v", filesystem.Id(), c.hostTag, err)
	}
	if detachable {
		if err := c.sb.DetachFilesystem(fsa.Host(), filesystem); err != nil && !errors.Is(err, errors.NotFound) {
			if !c.force {
				return errors.Trace(err)
			}
			logger.Warningf("could not detach filesystem %v for %v: %v", filesystem.Id(), c.hostTag, err)
		}
	}

	if c.manual {
		return nil
	}

	// For non-manual machines we immediately remove the attachments for
	// non-detachable or volume-backed filesystems, which should have been set
	// to Dying by the destruction of the machine filesystems,
	// or filesystem detachment, above.
	machineTag := fsa.Host()
	var volumeTag names.VolumeTag
	var updateStatus func() error
	if !detachable {
		updateStatus = func() error { return nil }
	} else {
		f, err := c.sb.Filesystem(filesystem)
		if err != nil {
			if !errors.Is(err, errors.NotFound) {
				if !c.force {
					return errors.Trace(err)
				}
				logger.Warningf("could not get filesystem %v for %v: %v", filesystem.Id(), c.hostTag, err)
			}
			return nil
		}

		if v, err := f.Volume(); err == nil && !errors.Is(err, errors.NotFound) {
			// Filesystem is volume-backed.
			volumeTag = v
		}
		updateStatus = func() error {
			return f.SetStatus(status.StatusInfo{
				Status: status.Detached,
			}, status.NoopStatusHistoryRecorder)
		}
	}

	if err := c.sb.RemoveFilesystemAttachment(fsa.Host(), filesystem, c.force); err != nil && !errors.Is(err, errors.NotFound) {
		if !c.force {
			return errors.Trace(err)
		}
		logger.Warningf("could not remove attachment for filesystem %v for %v: %v", filesystem.Id(), c.hostTag, err)
	}
	if volumeTag != (names.VolumeTag{}) {
		if err := c.sb.RemoveVolumeAttachmentPlan(machineTag, volumeTag, c.force); err != nil && !errors.Is(err, errors.NotFound) {
			if !c.force {
				return errors.Trace(err)
			}
			logger.Warningf("could not remove attachment plan for volume %v for %v: %v", volumeTag.Id(), c.hostTag, err)
		}
	}
	if err := updateStatus(); err != nil && !errors.Is(err, errors.NotFound) {
		if !c.force {
			return errors.Trace(err)
		}
		logger.Warningf("could not update status while cleaning up storage for dying %v: %v", c.hostTag, err)
	}

	return nil
}
