// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/internal/storage"
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
	volumeBlockDevices map[names.VolumeTag]blockdevice.BlockDevice
	filesystems        map[names.FilesystemTag]storage.Filesystem
}

// NewManagedFilesystemSource returns a storage.FilesystemSource that manages
// filesystems on block devices on the host machine.
//
// The parameters are maps that the caller will update with information about
// block devices and filesystems created by the source. The caller must not
// update the maps during calls to the source's methods.
func NewManagedFilesystemSource(
	volumeBlockDevices map[names.VolumeTag]blockdevice.BlockDevice,
	filesystems map[names.FilesystemTag]storage.Filesystem,
) storage.FilesystemSource {
	return &managedFilesystemSource{
		logAndExec,
		&osDirFuncs{run: logAndExec},
		volumeBlockDevices, filesystems,
	}
}

// ValidateFilesystemParams is defined on storage.FilesystemSource.
func (s *managedFilesystemSource) ValidateFilesystemParams(arg storage.FilesystemParams) error {
	// NOTE(axw) the parameters may be for destroying a filesystem, which
	// may be called when the backing volume is detached from the machine.
	// We must not perform any validation here that would fail if the
	// volume is detached.
	return nil
}

func (s *managedFilesystemSource) backingVolumeBlockDevice(v names.VolumeTag) (blockdevice.BlockDevice, error) {
	blockDevice, ok := s.volumeBlockDevices[v]
	if !ok {
		return blockdevice.BlockDevice{}, errors.Errorf(
			"backing-volume %s is not yet attached", v.Id(),
		)
	}
	return blockDevice, nil
}

// CreateFilesystems is defined on storage.FilesystemSource.
func (s *managedFilesystemSource) CreateFilesystems(ctx context.Context, args []storage.FilesystemParams) ([]storage.CreateFilesystemsResult, error) {
	results := make([]storage.CreateFilesystemsResult, len(args))
	for i, arg := range args {
		filesystem, err := s.createFilesystem(arg)
		if err != nil {
			results[i].Error = err
			continue
		}
		results[i].Filesystem = filesystem
	}
	return results, nil
}

func (s *managedFilesystemSource) createFilesystem(arg storage.FilesystemParams) (*storage.Filesystem, error) {
	blockDevice, err := s.backingVolumeBlockDevice(arg.Volume)
	if err != nil {
		return nil, errors.Trace(err)
	}
	devicePath := devicePath(blockDevice)
	if isDiskDevice(devicePath) {
		if err := destroyPartitions(s.run, devicePath); err != nil {
			return nil, errors.Trace(err)
		}
		if err := createPartition(s.run, devicePath); err != nil {
			return nil, errors.Trace(err)
		}
		devicePath = partitionDevicePath(devicePath)
	}
	if err := createFilesystem(s.run, devicePath); err != nil {
		return nil, errors.Trace(err)
	}
	return &storage.Filesystem{
		arg.Tag,
		arg.Volume,
		storage.FilesystemInfo{
			arg.Tag.String(),
			blockDevice.SizeMiB,
		},
	}, nil
}

// DestroyFilesystems is defined on storage.FilesystemSource.
func (s *managedFilesystemSource) DestroyFilesystems(ctx context.Context, filesystemIds []string) ([]error, error) {
	// DestroyFilesystems is a no-op; there is nothing to destroy,
	// since the filesystem is just data on a volume. The volume
	// is destroyed separately.
	return make([]error, len(filesystemIds)), nil
}

// ReleaseFilesystems is defined on storage.FilesystemSource.
func (s *managedFilesystemSource) ReleaseFilesystems(ctx context.Context, filesystemIds []string) ([]error, error) {
	return make([]error, len(filesystemIds)), nil
}

// AttachFilesystems is defined on storage.FilesystemSource.
func (s *managedFilesystemSource) AttachFilesystems(ctx context.Context, args []storage.FilesystemAttachmentParams) ([]storage.AttachFilesystemsResult, error) {
	results := make([]storage.AttachFilesystemsResult, len(args))
	for i, arg := range args {
		attachment, err := s.attachFilesystem(arg)
		if err != nil {
			results[i].Error = err
			continue
		}
		results[i].FilesystemAttachment = attachment
	}
	return results, nil
}

func (s *managedFilesystemSource) attachFilesystem(arg storage.FilesystemAttachmentParams) (*storage.FilesystemAttachment, error) {
	filesystem, ok := s.filesystems[arg.Filesystem]
	if !ok {
		return nil, errors.Errorf("filesystem %v is not yet provisioned", arg.Filesystem.Id())
	}
	blockDevice, err := s.backingVolumeBlockDevice(filesystem.Volume)
	if err != nil {
		return nil, errors.Trace(err)
	}
	devicePath := devicePath(blockDevice)
	if isDiskDevice(devicePath) {
		devicePath = partitionDevicePath(devicePath)
	}
	if err := mountFilesystem(s.run, s.dirFuncs, devicePath, blockDevice.UUID, arg.Path, arg.ReadOnly); err != nil {
		return nil, errors.Trace(err)
	}
	return &storage.FilesystemAttachment{
		Filesystem: arg.Filesystem,
		Machine:    arg.Machine,
		FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
			Path:     arg.Path,
			ReadOnly: arg.ReadOnly,
		},
	}, nil
}

// DetachFilesystems is defined on storage.FilesystemSource.
func (s *managedFilesystemSource) DetachFilesystems(ctx context.Context, args []storage.FilesystemAttachmentParams) ([]error, error) {
	results := make([]error, len(args))
	for i, arg := range args {
		if err := maybeUnmount(s.run, s.dirFuncs, arg.Path); err != nil {
			results[i] = err
		}
	}
	return results, nil
}

func destroyPartitions(run runCommandFunc, devicePath string) error {
	logger.Debugf(context.TODO(), "destroying partitions on %q", devicePath)
	if _, err := run("sgdisk", "--zap-all", devicePath); err != nil {
		return errors.Annotate(err, "sgdisk failed")
	}
	return nil
}

// createPartition creates a single partition (1) on the disk with the
// specified device path.
func createPartition(run runCommandFunc, devicePath string) error {
	logger.Debugf(context.TODO(), "creating partition on %q", devicePath)
	if _, err := run("sgdisk", "-n", "1:0:-1", devicePath); err != nil {
		return errors.Annotate(err, "sgdisk failed")
	}
	return nil
}

func createFilesystem(run runCommandFunc, devicePath string) error {
	logger.Debugf(context.TODO(), "attempting to create filesystem on %q", devicePath)
	mkfscmd := "mkfs." + defaultFilesystemType
	_, err := run(mkfscmd, devicePath)
	if err != nil {
		return errors.Annotatef(err, "%s failed", mkfscmd)
	}
	logger.Infof(context.TODO(), "created filesystem on %q", devicePath)
	return nil
}

func mountFilesystem(run runCommandFunc, dirFuncs dirFuncs, devicePath, uuid, mountPoint string, readOnly bool) error {
	logger.Debugf(context.TODO(), "attempting to mount filesystem on %q at %q", devicePath, mountPoint)
	if err := dirFuncs.mkDirAll(mountPoint, 0755); err != nil {
		return errors.Annotate(err, "creating mount point")
	}
	mounted, mountSource, err := isMounted(dirFuncs, mountPoint)
	if err != nil {
		return errors.Trace(err)
	}
	if mounted {
		logger.Debugf(context.TODO(), "filesystem on %q already mounted at %q", mountSource, mountPoint)
	} else {
		var args []string
		if readOnly {
			args = append(args, "-o", "ro")
		}
		args = append(args, devicePath, mountPoint)
		if _, err := run("mount", args...); err != nil {
			return errors.Annotate(err, "mount failed")
		}
		logger.Debugf(context.TODO(), "mounted filesystem on %q at %q", devicePath, mountPoint)
	}
	// Look for the mtab entry resulting from the mount and copy it to fstab.
	// This ensures the mount is available available after a reboot.
	etcDir := dirFuncs.etcDir()
	mtabEntry, err := extractMtabEntry(etcDir, devicePath, mountPoint)
	if err != nil {
		return errors.Annotate(err, "parsing /etc/mtab")
	}
	if mtabEntry == "" {
		return nil
	}
	return ensureFstabEntry(etcDir, devicePath, uuid, mountPoint, mtabEntry)
}

// extractMtabEntry returns any /etc/mtab entry for the specified
// device path and mount point, or "" if none exists.
func extractMtabEntry(etcDir string, devicePath, mountPoint string) (string, error) {
	f, err := os.Open(filepath.Join(etcDir, "mtab"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == devicePath && fields[1] == mountPoint {
			return line, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", errors.Trace(err)
	}
	return "", nil
}

// ensureFstabEntry creates an entry in /etc/fstab for the specified
// device path and mount point so long as there's no existing entry already.
func ensureFstabEntry(etcDir, devicePath, uuid, mountPoint, entry string) error {
	f, err := os.Open(filepath.Join(etcDir, "fstab"))
	if err != nil && !os.IsNotExist(err) {
		return errors.Annotate(err, "opening /etc/fstab")
	}
	if err == nil {
		defer f.Close()
	}

	newFsTab, err := os.CreateTemp(etcDir, "juju-fstab-")
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		_ = newFsTab.Close()
		_ = os.Remove(newFsTab.Name())
	}()
	if err := os.Chmod(newFsTab.Name(), 0644); err != nil {
		return errors.Trace(err)
	}

	// Add nofail if not there already
	resultFields := strings.Fields(entry)
	options := set.NewStrings()
	if len(resultFields) >= 4 {
		options = set.NewStrings(strings.Split(resultFields[3], ",")...)
	}
	if !options.Contains("nofail") {
		options.Add("nofail")
		opts := strings.Join(options.SortedValues(), ",")
		if len(resultFields) >= 4 {
			resultFields[3] = opts
		} else {
			resultFields = append(resultFields, opts)
		}
	}

	uuidField := "UUID=" + uuid
	addNewEntry := true
	// Scan all the fstab lines, searching for one
	// which describes the entry we want to create.
	scanner := bufio.NewScanner(f)
	for f != nil && scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != mountPoint {
			goto writeLine
		}
		// Is the line the UUID based mount entry we want.
		if fields[0] == uuidField {
			addNewEntry = false
			goto writeLine
		}
		// Is the line for some other entry.
		if fields[0] != devicePath {
			goto writeLine
		}
		// We have a match, if UUID is not yet known, retain the line.
		if uuid == "" {
			addNewEntry = false
			goto writeLine
		}
		continue
	writeLine:
		_, err := newFsTab.WriteString(line + "\n")
		if err != nil {
			return errors.Trace(err)
		}
	}
	if err := scanner.Err(); err != nil {
		return errors.Trace(err)
	}

	if addNewEntry {
		if uuid != "" {
			if len(resultFields) >= 2 { // just being defensive, check should never fail.
				_, err := newFsTab.WriteString(fmt.Sprintf("# %s was on %s during installation\n", resultFields[1], resultFields[0]))
				if err != nil {
					return errors.Trace(err)
				}
			}
			resultFields[0] = uuidField
		}
		_, err := newFsTab.WriteString(strings.Join(resultFields, " ") + "\n")
		if err != nil {
			return errors.Trace(err)
		}

	}
	return os.Rename(newFsTab.Name(), filepath.Join(etcDir, "fstab"))
}

func maybeUnmount(run runCommandFunc, dirFuncs dirFuncs, mountPoint string) error {
	mounted, _, err := isMounted(dirFuncs, mountPoint)
	if err != nil {
		return errors.Trace(err)
	}
	if !mounted {
		return nil
	}
	logger.Debugf(context.TODO(), "attempting to unmount filesystem at %q", mountPoint)
	if err := removeFstabEntry(dirFuncs.etcDir(), mountPoint); err != nil {
		return errors.Annotate(err, "updating /etc/fstab failed")
	}
	if _, err := run("umount", mountPoint); err != nil {
		return errors.Annotate(err, "umount failed")
	}
	logger.Infof(context.TODO(), "unmounted filesystem at %q", mountPoint)
	return nil
}

// removeFstabEntry removes any existing /etc/fstab entry for
// the specified mount point.
func removeFstabEntry(etcDir string, mountPoint string) error {
	fstab := filepath.Join(etcDir, "fstab")
	f, err := os.Open(fstab)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)

	// Use a tempfile in /etc and rename when done.
	newFsTab, err := os.CreateTemp(etcDir, "juju-fstab-")
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		_ = newFsTab.Close()
		_ = os.Remove(newFsTab.Name())
	}()
	if err := os.Chmod(newFsTab.Name(), 0644); err != nil {
		return errors.Trace(err)
	}

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != mountPoint {
			_, err := newFsTab.WriteString(line + "\n")
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return errors.Trace(err)
	}

	return os.Rename(newFsTab.Name(), fstab)
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
func devicePath(dev blockdevice.BlockDevice) string {
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
