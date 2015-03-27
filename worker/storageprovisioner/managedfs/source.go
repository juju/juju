// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package managedfs provides an implementation of FilesystemSource that
// can be used to manage filesystems on volumes attached to the host machine.
package managedfs

import (
	"bytes"
	"os"
	"os/exec"
	"path"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.worker.storageprovisioner.managedfs")

const (
	// defaultFilesystemType is the default filesystem type
	// to create for volume-backed managed filesystems.
	defaultFilesystemType = "ext4"
)

// ManagedFilesystemSource is an implementation of storage.FilesystemSource
// that manages filesystems on volumes attached to the host machine.
//
// ManagedFilesystemSource is expected to be called from a single goroutine.
type ManagedFilesystemSource struct {
	VolumeBlockDevices map[names.VolumeTag]storage.BlockDevice
	Filesystems        map[names.FilesystemTag]storage.Filesystem
}

// ValidateFilesystemParams is defined on storage.FilesystemSource.
func (s *ManagedFilesystemSource) ValidateFilesystemParams(arg storage.FilesystemParams) error {
	if _, err := s.backingVolumeBlockDevice(arg.Volume); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (s *ManagedFilesystemSource) backingVolumeBlockDevice(v names.VolumeTag) (storage.BlockDevice, error) {
	blockDevice, ok := s.VolumeBlockDevices[v]
	if !ok {
		return storage.BlockDevice{}, errors.Errorf(
			"backing-volume %s is not yet attached", v.Id(),
		)
	}
	return blockDevice, nil
}

// CreateFilesystems is defined on storage.FilesystemSource.
func (s *ManagedFilesystemSource) CreateFilesystems(args []storage.FilesystemParams) ([]storage.Filesystem, error) {
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

func (s *ManagedFilesystemSource) createFilesystem(arg storage.FilesystemParams) (storage.Filesystem, error) {
	blockDevice, err := s.backingVolumeBlockDevice(arg.Volume)
	if err != nil {
		return storage.Filesystem{}, errors.Trace(err)
	}
	devicePath := s.devicePath(blockDevice)
	if err := createFilesystem(devicePath); err != nil {
		return storage.Filesystem{}, errors.Trace(err)
	}
	return storage.Filesystem{
		arg.Tag,
		arg.Volume,
		arg.Tag.String(),
		blockDevice.Size,
	}, nil
}

func (s *ManagedFilesystemSource) devicePath(dev storage.BlockDevice) string {
	if dev.DeviceName != "" {
		return path.Join("/dev", dev.DeviceName)
	}
	return path.Join("/dev/disk/by-id", dev.Serial)
}

// AttachFilesystems is defined on storage.FilesystemSource.
func (s *ManagedFilesystemSource) AttachFilesystems(args []storage.FilesystemAttachmentParams) ([]storage.FilesystemAttachment, error) {
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

func (s *ManagedFilesystemSource) attachFilesystem(arg storage.FilesystemAttachmentParams) (storage.FilesystemAttachment, error) {
	filesystem, ok := s.Filesystems[arg.Filesystem]
	if !ok {
		return storage.FilesystemAttachment{}, errors.Errorf("filesystem %v is not yet provisioned", arg.Filesystem.Id())
	}
	blockDevice, err := s.backingVolumeBlockDevice(filesystem.Volume)
	if err != nil {
		return storage.FilesystemAttachment{}, errors.Trace(err)
	}
	devicePath := s.devicePath(blockDevice)
	if err := mountFilesystem(devicePath, arg.Path); err != nil {
		return storage.FilesystemAttachment{}, errors.Trace(err)
	}
	return storage.FilesystemAttachment{
		arg.Filesystem,
		arg.Machine,
		arg.Path,
	}, nil
}

// DetachFilesystems is defined on storage.FilesystemSource.
func (s *ManagedFilesystemSource) DetachFilesystems(args []storage.FilesystemAttachmentParams) error {
	// TODO(axw)
	return errors.NotImplementedf("DetachFilesystems")
}

func createFilesystem(devicePath string) error {
	logger.Debugf("attempting to create filesystem on %q", devicePath)
	mkfscmd := "mkfs." + defaultFilesystemType
	output, err := exec.Command(mkfscmd, devicePath).CombinedOutput()
	if err != nil {
		return errors.Annotatef(err, "%s failed (%q)", mkfscmd, bytes.TrimSpace(output))
	}
	logger.Infof("created filesystem on %q", devicePath)
	return nil
}

func mountFilesystem(devicePath, mountPoint string) error {
	logger.Debugf("attempting to mount filesystem on %q at %q", devicePath, mountPoint)
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return errors.Annotate(err, "creating mount point")
	}
	// TODO(axw) check if the mount already exists, and do nothing if so.
	output, err := exec.Command("mount", devicePath, mountPoint).CombinedOutput()
	if err != nil {
		return errors.Annotatef(err, "mount failed (%q)", bytes.TrimSpace(output))
	}
	logger.Infof("mounted filesystem on %q at %q", devicePath, mountPoint)
	return nil
}
