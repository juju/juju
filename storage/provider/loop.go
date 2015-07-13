// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

const (
	// Loop provider types.
	LoopProviderType     = storage.ProviderType("loop")
	HostLoopProviderType = storage.ProviderType("hostloop")
)

// loopProviders create volume sources which use loop devices.
type loopProvider struct {
	// run is a function used for running commands on the local machine.
	run runCommandFunc
	// runningInsideLXC is a function that determines whether or not
	// the code is running within an LXC container.
	runningInsideLXC func() (bool, error)
}

var _ storage.Provider = (*loopProvider)(nil)

// ValidateConfig is defined on the Provider interface.
func (*loopProvider) ValidateConfig(*storage.Config) error {
	// Loop provider has no configuration.
	return nil
}

// validateFullConfig validates a fully-constructed storage config,
// combining the user-specified config and any internally specified
// config.
func (lp *loopProvider) validateFullConfig(cfg *storage.Config) error {
	if err := lp.ValidateConfig(cfg); err != nil {
		return err
	}
	storageDir, ok := cfg.ValueString(storage.ConfigStorageDir)
	if !ok || storageDir == "" {
		return errors.New("storage directory not specified")
	}
	return nil
}

// VolumeSource is defined on the Provider interface.
func (lp *loopProvider) VolumeSource(
	environConfig *config.Config,
	sourceConfig *storage.Config,
) (storage.VolumeSource, error) {
	if err := lp.validateFullConfig(sourceConfig); err != nil {
		return nil, err
	}
	insideLXC, err := lp.runningInsideLXC()
	if err != nil {
		return nil, err
	}
	// storageDir is validated by validateFullConfig.
	storageDir, _ := sourceConfig.ValueString(storage.ConfigStorageDir)
	return &loopVolumeSource{
		&osDirFuncs{lp.run},
		lp.run,
		storageDir,
		insideLXC,
	}, nil
}

// FilesystemSource is defined on the Provider interface.
func (lp *loopProvider) FilesystemSource(
	environConfig *config.Config,
	providerConfig *storage.Config,
) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

// Supports is defined on the Provider interface.
func (*loopProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindBlock
}

// Scope is defined on the Provider interface.
func (*loopProvider) Scope() storage.Scope {
	return storage.ScopeMachine
}

// Dynamic is defined on the Provider interface.
func (*loopProvider) Dynamic() bool {
	return true
}

// loopVolumeSource provides common functionality to handle
// loop devices for rootfs and host loop volume sources.
type loopVolumeSource struct {
	dirFuncs         dirFuncs
	run              runCommandFunc
	storageDir       string
	runningInsideLXC bool
}

var _ storage.VolumeSource = (*loopVolumeSource)(nil)

// CreateVolumes is defined on the VolumeSource interface.
func (lvs *loopVolumeSource) CreateVolumes(args []storage.VolumeParams) ([]storage.Volume, []storage.VolumeAttachment, error) {
	volumes := make([]storage.Volume, len(args))
	for i, arg := range args {
		volume, err := lvs.createVolume(arg)
		if err != nil {
			return nil, nil, errors.Annotate(err, "creating volume")
		}
		volumes[i] = volume
	}
	return volumes, nil, nil
}

func (lvs *loopVolumeSource) createVolume(params storage.VolumeParams) (storage.Volume, error) {
	volumeId := params.Tag.String()
	loopFilePath := lvs.volumeFilePath(params.Tag)
	if err := ensureDir(lvs.dirFuncs, filepath.Dir(loopFilePath)); err != nil {
		return storage.Volume{}, errors.Trace(err)
	}
	if err := createBlockFile(lvs.run, loopFilePath, params.Size); err != nil {
		return storage.Volume{}, errors.Annotate(err, "could not create block file")
	}
	return storage.Volume{
		params.Tag,
		storage.VolumeInfo{
			VolumeId: volumeId,
			Size:     params.Size,
			// Loop devices may outlive LXC containers. If we're
			// running inside an LXC container, mark the volume as
			// persistent.
			Persistent: lvs.runningInsideLXC,
		},
	}, nil
}

func (lvs *loopVolumeSource) volumeFilePath(tag names.VolumeTag) string {
	return filepath.Join(lvs.storageDir, tag.String())
}

// DescribeVolumes is defined on the VolumeSource interface.
func (lvs *loopVolumeSource) DescribeVolumes(volumeIds []string) ([]storage.VolumeInfo, error) {
	// TODO(axw) implement this when we need it.
	return nil, errors.NotImplementedf("DescribeVolumes")
}

// DestroyVolumes is defined on the VolumeSource interface.
func (lvs *loopVolumeSource) DestroyVolumes(volumeIds []string) []error {
	results := make([]error, len(volumeIds))
	for i, volumeId := range volumeIds {
		if err := lvs.destroyVolume(volumeId); err != nil {
			results[i] = errors.Annotatef(err, "destroying %q", volumeId)
		}
	}
	return results
}

func (lvs *loopVolumeSource) destroyVolume(volumeId string) error {
	tag, err := names.ParseVolumeTag(volumeId)
	if err != nil {
		return errors.Errorf("invalid loop volume ID %q", volumeId)
	}
	loopFilePath := lvs.volumeFilePath(tag)
	err = os.Remove(loopFilePath)
	if err != nil && !os.IsNotExist(err) {
		return errors.Annotate(err, "removing loop backing file")
	}
	return nil
}

// ValidateVolumeParams is defined on the VolumeSource interface.
func (lvs *loopVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	// ValdiateVolumeParams may be called on a machine other than the
	// machine where the loop device will be created, so we cannot check
	// available size until we get to CreateVolumes.
	return nil
}

// AttachVolumes is defined on the VolumeSource interface.
func (lvs *loopVolumeSource) AttachVolumes(args []storage.VolumeAttachmentParams) ([]storage.VolumeAttachment, error) {
	attachments := make([]storage.VolumeAttachment, len(args))
	for i, arg := range args {
		attachment, err := lvs.attachVolume(arg)
		if err != nil {
			return nil, errors.Annotatef(err, "attaching volume %v", arg.Volume.Id())
		}
		attachments[i] = attachment
	}
	return attachments, nil
}

func (lvs *loopVolumeSource) attachVolume(arg storage.VolumeAttachmentParams) (storage.VolumeAttachment, error) {
	loopFilePath := lvs.volumeFilePath(arg.Volume)
	deviceName, err := attachLoopDevice(lvs.run, loopFilePath, arg.ReadOnly)
	if err != nil {
		os.Remove(loopFilePath)
		return storage.VolumeAttachment{}, errors.Annotate(err, "attaching loop device")
	}
	return storage.VolumeAttachment{
		arg.Volume,
		arg.Machine,
		storage.VolumeAttachmentInfo{
			DeviceName: deviceName,
			ReadOnly:   arg.ReadOnly,
		},
	}, nil
}

// DetachVolumes is defined on the VolumeSource interface.
func (lvs *loopVolumeSource) DetachVolumes(args []storage.VolumeAttachmentParams) error {
	for _, arg := range args {
		if err := lvs.detachVolume(arg.Volume); err != nil {
			return errors.Annotatef(err, "detaching volume %s", arg.Volume.Id())
		}
	}
	return nil
}

func (lvs *loopVolumeSource) detachVolume(tag names.VolumeTag) error {
	loopFilePath := lvs.volumeFilePath(tag)
	deviceNames, err := associatedLoopDevices(lvs.run, loopFilePath)
	if err != nil {
		return errors.Annotate(err, "locating loop device")
	}
	if len(deviceNames) > 1 {
		logger.Warningf("expected 1 loop device, got %d", len(deviceNames))
	}
	for _, deviceName := range deviceNames {
		if err := detachLoopDevice(lvs.run, deviceName); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// createBlockFile creates a file at the specified path, with the
// given size in mebibytes.
func createBlockFile(run runCommandFunc, filePath string, sizeInMiB uint64) error {
	// fallocate will reserve the space without actually writing to it.
	_, err := run("fallocate", "-l", fmt.Sprintf("%dMiB", sizeInMiB), filePath)
	if err != nil {
		return errors.Annotatef(err, "allocating loop backing file %q", filePath)
	}
	return nil
}

// attachLoopDevice attaches a loop device to the file with the
// specified path, and returns the loop device's name (e.g. "loop0").
// losetup will create additional loop devices as necessary.
func attachLoopDevice(run runCommandFunc, filePath string, readOnly bool) (loopDeviceName string, _ error) {
	devices, err := associatedLoopDevices(run, filePath)
	if err != nil {
		return "", err
	}
	if len(devices) > 0 {
		// Already attached.
		logger.Debugf("%s already attached to %s", filePath, devices)
		return devices[0], nil
	}
	// -f automatically finds the first available loop-device.
	// -r sets up a read-only loop-device.
	// --show returns the loop device chosen on stdout.
	args := []string{"-f", "--show"}
	if readOnly {
		args = append(args, "-r")
	}
	args = append(args, filePath)
	stdout, err := run("losetup", args...)
	if err != nil {
		return "", errors.Annotatef(err, "attaching loop device to %q", filePath)
	}
	stdout = strings.TrimSpace(stdout)
	loopDeviceName = stdout[len("/dev/"):]
	return loopDeviceName, nil
}

// detachLoopDevice detaches the loop device with the specified name.
func detachLoopDevice(run runCommandFunc, deviceName string) error {
	_, err := run("losetup", "-d", path.Join("/dev", deviceName))
	if err != nil {
		return errors.Annotatef(err, "detaching loop device %q", deviceName)
	}
	return err
}

// associatedLoopDevices returns the device names of the loop devices
// associated with the specified file path.
func associatedLoopDevices(run runCommandFunc, filePath string) ([]string, error) {
	stdout, err := run("losetup", "-j", filePath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return nil, nil
	}
	// The output will be zero or more lines with the format:
	//    "/dev/loop0: [0021]:7504142 (/tmp/test.dat)"
	lines := strings.Split(stdout, "\n")
	deviceNames := make([]string, len(lines))
	for i, line := range lines {
		pos := strings.IndexRune(line, ':')
		if pos == -1 {
			return nil, errors.Errorf("unexpected output %q", line)
		}
		deviceName := line[:pos][len("/dev/"):]
		deviceNames[i] = deviceName
	}
	return deviceNames, nil
}
