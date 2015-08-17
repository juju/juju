// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"path"
	"path/filepath"
	"unicode"

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
	devicePath := devicePath(blockDevice)
	if isDiskDevice(devicePath) {
		if err := destroyPartitions(s.run, devicePath); err != nil {
			return storage.Filesystem{}, errors.Trace(err)
		}
		if err := createPartition(s.run, devicePath); err != nil {
			return storage.Filesystem{}, errors.Trace(err)
		}
		devicePath = partitionDevicePath(devicePath)
	}
	if err := createFilesystem(s.run, devicePath); err != nil {
		return storage.Filesystem{}, errors.Trace(err)
	}
	return storage.Filesystem{
		arg.Tag,
		arg.Volume,
		storage.FilesystemInfo{
			arg.Tag.String(),
			blockDevice.Size,
		},
	}, nil
}

// DestroyFilesystems is defined on storage.FilesystemSource.
func (s *managedFilesystemSource) DestroyFilesystems(filesystemIds []string) []error {
	// DestroyFilesystems is a no-op; there is nothing to destroy,
	// since the filesystem is just data on a volume. The volume
	// is destroyed separately.
	return make([]error, len(filesystemIds))
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
	devicePath := devicePath(blockDevice)
	if isDiskDevice(devicePath) {
		devicePath = partitionDevicePath(devicePath)
	}
	if err := mountFilesystem(s.run, s.dirFuncs, devicePath, arg.Path, arg.ReadOnly); err != nil {
		return storage.FilesystemAttachment{}, errors.Trace(err)
	}
	return storage.FilesystemAttachment{
		arg.Filesystem,
		arg.Machine,
		storage.FilesystemAttachmentInfo{
			arg.Path,
			arg.ReadOnly,
		},
	}, nil
}

// DetachFilesystems is defined on storage.FilesystemSource.
func (s *managedFilesystemSource) DetachFilesystems(args []storage.FilesystemAttachmentParams) error {
	for _, arg := range args {
		if err := maybeUnmount(s.run, s.dirFuncs, arg.Path); err != nil {
			return errors.Annotatef(err, "detaching filesystem %s", arg.Filesystem.Id())
		}
	}
	return nil
}

func destroyPartitions(run runCommandFunc, devicePath string) error {
	logger.Debugf("destroying partitions on %q", devicePath)
	if _, err := run("sgdisk", "--zap-all", devicePath); err != nil {
		return errors.Annotate(err, "sgdisk failed")
	}
	return nil
}

// createPartition creates a single partition (1) on the disk with the
// specified device path.
func createPartition(run runCommandFunc, devicePath string) error {
	logger.Debugf("creating partition on %q", devicePath)
	if _, err := run("sgdisk", "-n", "1:0:-1", devicePath); err != nil {
		return errors.Annotate(err, "sgdisk failed")
	}
	return nil
}

func createFilesystem(run runCommandFunc, devicePath string) error {
	logger.Debugf("attempting to create filesystem on %q", devicePath)
	mkfscmd := "mkfs." + defaultFilesystemType
	_, err := run(mkfscmd, devicePath)
	if err != nil {
		return errors.Annotatef(err, "%s failed", mkfscmd)
	}
	logger.Infof("created filesystem on %q", devicePath)
	return nil
}

func mountFilesystem(run runCommandFunc, dirFuncs dirFuncs, devicePath, mountPoint string, readOnly bool) error {
	logger.Debugf("attempting to mount filesystem on %q at %q", devicePath, mountPoint)
	if err := dirFuncs.mkDirAll(mountPoint, 0755); err != nil {
		return errors.Annotate(err, "creating mount point")
	}
	mounted, mountSource, err := isMounted(dirFuncs, mountPoint)
	if err != nil {
		return errors.Trace(err)
	}
	if mounted {
		logger.Debugf("filesystem on %q already mounted at %q", mountSource, mountPoint)
		return nil
	}
	var args []string
	if readOnly {
		args = append(args, "-o", "ro")
	}
	args = append(args, devicePath, mountPoint)
	if _, err := run("mount", args...); err != nil {
		return errors.Annotate(err, "mount failed")
	}
	logger.Infof("mounted filesystem on %q at %q", devicePath, mountPoint)
	return nil
}

func maybeUnmount(run runCommandFunc, dirFuncs dirFuncs, mountPoint string) error {
	mounted, _, err := isMounted(dirFuncs, mountPoint)
	if err != nil {
		return errors.Trace(err)
	}
	if !mounted {
		return nil
	}
	logger.Debugf("attempting to unmount filesystem at %q", mountPoint)
	if _, err := run("umount", mountPoint); err != nil {
		return errors.Annotate(err, "umount failed")
	}
	logger.Infof("unmounted filesystem at %q", mountPoint)
	return nil
}

func isMounted(dirFuncs dirFuncs, mountPoint string) (bool, string, error) {
	mountPointParent := filepath.Dir(mountPoint)
	parentSource, err := dirFuncs.mountPointSource(mountPointParent)
	if err != nil {
		return false, "", errors.Trace(err)
	}
	source, err := dirFuncs.mountPointSource(mountPoint)
	if err != nil {
		return false, "", errors.Trace(err)
	}
	if source != parentSource {
		// Already mounted.
		return true, source, nil
	}
	return false, "", nil
}

// devicePath returns the device path for the given block device.
func devicePath(dev storage.BlockDevice) string {
	return path.Join("/dev", dev.DeviceName)
}

// partitionDevicePath returns the device path for the first (and only)
// partition of the disk with the specified device path.
func partitionDevicePath(devicePath string) string {
	return devicePath + "1"
}

// isDiskDevice reports whether or not the device is a full disk, as opposed
// to a partition or a loop device. We create a partition on disks to contain
// filesystems.
func isDiskDevice(devicePath string) bool {
	var last rune
	for _, r := range devicePath {
		last = r
	}
	return !unicode.IsDigit(last)
}
