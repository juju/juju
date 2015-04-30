// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"path"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/storage"
)

const (
	// defaultFilesystemType is the default filesystem type
	// to create for volume-backed managed filesystems.
	defaultFilesystemType = "ext4"
)

// managedFilesystemSource is an implementation of storage.FilesystemSource
// that manages filesystems on volumes attached to the host machine.
//
// managedFilesystemSource is expected to be called from a single goroutine.
type managedFilesystemSource struct {
	run                runCommandFunc
	dirFuncs           dirFuncs
	volumeBlockDevices map[names.VolumeTag]storage.BlockDevice
	filesystems        map[names.FilesystemTag]storage.Filesystem
}

// NewManagedFilesystemSource returns a storage.FilesystemSource that manages
// filesystems on block devices on the host machine.
//
// The parameters are maps that the caller will update with information about
// block devices and filesystems created by the source. The caller must not
// update the maps during calls to the source's methods.
func NewManagedFilesystemSource(
	volumeBlockDevices map[names.VolumeTag]storage.BlockDevice,
	filesystems map[names.FilesystemTag]storage.Filesystem,
) storage.FilesystemSource {
	return &managedFilesystemSource{
		logAndExec,
		&osDirFuncs{logAndExec},
		volumeBlockDevices, filesystems,
	}
}

// ValidateFilesystemParams is defined on storage.FilesystemSource.
func (s *managedFilesystemSource) ValidateFilesystemParams(arg storage.FilesystemParams) error {
	if _, err := s.backingVolumeBlockDevice(arg.Volume); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (s *managedFilesystemSource) backingVolumeBlockDevice(v names.VolumeTag) (storage.BlockDevice, error) {
	blockDevice, ok := s.volumeBlockDevices[v]
	if !ok {
		return storage.BlockDevice{}, errors.Errorf(
			"backing-volume %s is not yet attached", v.Id(),
		)
	}
	return blockDevice, nil
}

// CreateFilesystems is defined on storage.FilesystemSource.
func (s *managedFilesystemSource) CreateFilesystems(args []storage.FilesystemParams) ([]storage.Filesystem, error) {
	filesystems := make([]storage.Filesystem, len(args))
	for i, arg := range args {
		filesystem, err := s.createFilesystem(arg)
		if err != nil {
			return nil, errors.Annotatef(err, "creating filesystem %s", arg.Tag.Id())
		}
		filesystems[i] = filesystem
	}
	return filesystems, nil
}

func (s *managedFilesystemSource) createFilesystem(arg storage.FilesystemParams) (storage.Filesystem, error) {
	blockDevice, err := s.backingVolumeBlockDevice(arg.Volume)
	if err != nil {
		return storage.Filesystem{}, errors.Trace(err)
	}
	devicePath := s.devicePath(blockDevice)
	if err := createFilesystem(s.run, devicePath); err != nil {
		return storage.Filesystem{}, errors.Trace(err)
	}
	return storage.Filesystem{
		arg.Tag,
		arg.Volume,
		arg.Tag.String(),
		blockDevice.Size,
	}, nil
}

func (s *managedFilesystemSource) devicePath(dev storage.BlockDevice) string {
	if dev.DeviceName != "" {
		return path.Join("/dev", dev.DeviceName)
	}
	return path.Join("/dev/disk/by-id", dev.HardwareId)
}

// AttachFilesystems is defined on storage.FilesystemSource.
func (s *managedFilesystemSource) AttachFilesystems(args []storage.FilesystemAttachmentParams) ([]storage.FilesystemAttachment, error) {
	attachments := make([]storage.FilesystemAttachment, len(args))
	for i, arg := range args {
		attachment, err := s.attachFilesystem(arg)
		if err != nil {
			return nil, errors.Annotatef(err, "attaching filesystem %s", arg.Filesystem.Id())
		}
		attachments[i] = attachment
	}
	return attachments, nil
}

func (s *managedFilesystemSource) attachFilesystem(arg storage.FilesystemAttachmentParams) (storage.FilesystemAttachment, error) {
	filesystem, ok := s.filesystems[arg.Filesystem]
	if !ok {
		return storage.FilesystemAttachment{}, errors.Errorf("filesystem %v is not yet provisioned", arg.Filesystem.Id())
	}
	blockDevice, err := s.backingVolumeBlockDevice(filesystem.Volume)
	if err != nil {
		return storage.FilesystemAttachment{}, errors.Trace(err)
	}
	devicePath := s.devicePath(blockDevice)
	if err := mountFilesystem(s.run, s.dirFuncs, devicePath, arg.Path); err != nil {
		return storage.FilesystemAttachment{}, errors.Trace(err)
	}
	return storage.FilesystemAttachment{
		arg.Filesystem,
		arg.Machine,
		arg.Path,
	}, nil
}

// DetachFilesystems is defined on storage.FilesystemSource.
func (s *managedFilesystemSource) DetachFilesystems(args []storage.FilesystemAttachmentParams) error {
	// TODO(axw)
	return errors.NotImplementedf("DetachFilesystems")
}

func createFilesystem(run runCommandFunc, devicePath string) error {
	logger.Debugf("attempting to create filesystem on %q", devicePath)
	mkfscmd := "mkfs." + defaultFilesystemType
	_, err := run(mkfscmd, devicePath)
	if err != nil {
		return errors.Annotatef(err, "%s failed (%q)", mkfscmd)
	}
	logger.Infof("created filesystem on %q", devicePath)
	return nil
}

func mountFilesystem(run runCommandFunc, dirFuncs dirFuncs, devicePath, mountPoint string) error {
	logger.Debugf("attempting to mount filesystem on %q at %q", devicePath, mountPoint)
	if err := dirFuncs.mkDirAll(mountPoint, 0755); err != nil {
		return errors.Annotate(err, "creating mount point")
	}
	// TODO(axw) check if the mount already exists, and do nothing if so.
	_, err := run("mount", devicePath, mountPoint)
	if err != nil {
		return errors.Annotate(err, "mount failed")
	}
	logger.Infof("mounted filesystem on %q at %q", devicePath, mountPoint)
	return nil
}
