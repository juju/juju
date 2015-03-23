// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/amz.v3/ec2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

const (
	EBS_ProviderType = storage.ProviderType("ebs")

	// Config attributes
	// TODO(wallyworld) - use juju/schema for defining attributes

	// The volume type (default standard):
	//   "gp2" for General Purpose (SSD) volumes
	//   "io1" for Provisioned IOPS (SSD) volumes,
	//   "standard" for Magnetic volumes.
	EBS_VolumeType = "volume-type" // top level directory where loop devices are created.

	// The number of I/O operations per second (IOPS) to provision for the volume.
	// Only valid for Provisioned IOPS (SSD) volumes.
	EBS_IOPS = "iops" // optional subdirectory for loop devices.

	// Specifies whether the volume should be encrypted.
	EBS_Encrypted = "encrypted"

	// The availability zone in which the volume will be created.
	EBS_AvailabilityZone = "availability-zone"
)

// AWS error codes
const (
	volumeInUse        = "VolumeInUse"
	attachmentNotFound = "InvalidAttachment.NotFound"
)

const (
	// minRootDiskSizeMiB is the minimum/default size (in mebibytes) for ec2 root disks.
	minRootDiskSizeMiB uint64 = 8 * 1024

	// provisionedIopsvolumeSizeMinGiB is the minimum disk size (in gibibytes)
	// for provisioned IOPS EBS volumes.
	provisionedIopsvolumeSizeMinGiB = 10 // 10 GiB

	// volumeSizeMaxGiB is the maximum disk size (in gibibytes) for EBS volumes.
	volumeSizeMaxGiB = 1024 // 1024 GiB

	// maxProvisionedIopsSizeRatio is the maximum allowed ratio of IOPS to
	// size (in GiB), for provisioend IOPS volumes.
	maxProvisionedIopsSizeRatio = 30

	// devicePrefix is the prefix for device names specified when creating volumes.
	devicePrefix = "/dev/sd"

	// renamedDevicePrefix is the prefix for device names after they have
	// been renamed. This should replace "devicePrefix" in the device name
	// when recording the block device info in state.
	renamedDevicePrefix = "xvd"
)

func init() {
	ebsssdPool, _ := storage.NewConfig("ebs-ssd", EBS_ProviderType, map[string]interface{}{"volume-type": "gp2"})
	defaultPools := []*storage.Config{
		ebsssdPool,
	}
	poolmanager.RegisterDefaultStoragePools(defaultPools)
}

// ebsProvider creates volume sources which use AWS EBS volumes.
type ebsProvider struct{}

var _ storage.Provider = (*ebsProvider)(nil)

var validConfigOptions = set.NewStrings(
	storage.Persistent,
	EBS_VolumeType,
	EBS_IOPS,
	EBS_Encrypted,
	EBS_AvailabilityZone,
)

// ValidateConfig is defined on the Provider interface.
func (e *ebsProvider) ValidateConfig(providerConfig *storage.Config) error {
	// TODO - check valid values as well as attr names
	for attr := range providerConfig.Attrs() {
		if !validConfigOptions.Contains(attr) {
			return errors.Errorf("unknown provider config option %q", attr)
		}
	}
	return nil
}

// Supports is defined on the Provider interface.
func (e *ebsProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindBlock
}

// Scope is defined on the Provider interface.
func (e *ebsProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// Dynamic is defined on the Provider interface.
func (e *ebsProvider) Dynamic() bool {
	// TODO(axw) this should be changed to true when support for dynamic
	// provisioning has been implemented for EBS. At that point, we need
	// to remove the block device mapping code.
	return false
}

// TranslateUserEBSOptions translates user friendly parameter values to the AWS values.
func TranslateUserEBSOptions(userOptions map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range userOptions {
		if k == EBS_VolumeType {
			switch v {
			case "magnetic":
				v = "standard"
			case "ssd":
				v = "gp2"
			case "provisioned-iops":
				v = "io1"
			}
		}
		result[k] = v
	}
	return result
}

// VolumeSource is defined on the Provider interface.
func (e *ebsProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	ec2, _, _, err := awsClients(environConfig)
	if err != nil {
		return nil, errors.Annotate(err, "creating AWS clients")
	}
	return &ebsVolumeSource{ec2}, nil
}

// FilesystemSource is defined on the Provider interface.
func (e *ebsProvider) FilesystemSource(environConfig *config.Config, providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

type ebsVolumeSource struct {
	ec2 *ec2.EC2
}

var _ storage.VolumeSource = (*ebsVolumeSource)(nil)

// parseVolumeOptions uses storage volume parameters to make a struct used to create volumes.
func parseVolumeOptions(size uint64, attr map[string]interface{}) (ec2.CreateVolume, error) {
	vol := ec2.CreateVolume{
		// Juju size is MiB, AWS size is GiB.
		VolumeSize: int(mibToGib(size)),
	}

	availabilityZone, _ := attr[EBS_AvailabilityZone].(string)
	if availabilityZone == "" {
		return vol, errors.New("missing availability zone")
	}
	vol.AvailZone = availabilityZone

	// TODO(wallyworld) - remove type assertions when juju/schema is used
	options := TranslateUserEBSOptions(attr)
	if v, ok := options[EBS_VolumeType]; ok && v != "" {
		vol.VolumeType = v.(string)
	}
	if v, ok := options[EBS_IOPS]; ok && v != "" {
		var err error
		vol.IOPS, err = strconv.ParseInt(v.(string), 10, 64)
		if err != nil {
			return vol, errors.Annotatef(err, "invalid iops value %v, expected integer", v)
		}
	}
	if v, ok := options[EBS_Encrypted].(bool); ok {
		vol.Encrypted = v
	}

	return vol, nil
}

// CreateVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) CreateVolumes(params []storage.VolumeParams) (vols []storage.Volume, _ []storage.VolumeAttachment, err error) {
	// If there's an error, we delete any ones that are created.
	defer func() {
		if err != nil && len(vols) > 0 {
			volIds := make([]string, len(vols))
			for i, v := range vols {
				volIds[i] = v.VolumeId
			}
			err2 := v.DestroyVolumes(volIds)
			for i, volErr := range err2 {
				if volErr == nil {
					continue
				}
				logger.Warningf("error cleaning up volume %v: %v", vols[i].Tag, volErr)
			}
		}
	}()

	// TODO(axw) if preference is to use ephemeral, use ephemeral
	// until the instance stores run out. We'll need to know how
	// many there are and how big each one is. We also need to
	// unmap ephemeral0 in cloud-init.

	// First, validate the params before we use them.
	for _, p := range params {
		if err := v.ValidateVolumeParams(p); err != nil {
			return vols, nil, errors.Trace(err)
		}
	}

	for _, p := range params {
		vol, _ := parseVolumeOptions(p.Size, p.Attributes)
		resp, err := v.ec2.CreateVolume(vol)
		if err != nil {
			return nil, nil, err
		}
		vols = append(vols, storage.Volume{
			Tag:        p.Tag,
			VolumeId:   resp.Id,
			Size:       gibToMib(uint64(resp.Size)),
			Persistent: p.IsPersistent(),
		})
	}
	return vols, nil, nil
}

// DescribeVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) DescribeVolumes(volIds []string) ([]storage.Volume, error) {
	resp, err := v.ec2.Volumes(volIds, nil)
	if err != nil {
		return nil, err
	}
	vols := make([]storage.Volume, len(resp.Volumes))
	for i, vol := range resp.Volumes {
		vols[i] = storage.Volume{
			// TODO(wallyworld) - fill in tag when interface is fixed
			Size:     gibToMib(uint64(vol.Size)),
			VolumeId: vol.Id,
		}
		for _, attachment := range vol.Attachments {
			if !attachment.DeleteOnTermination {
				vols[i].Persistent = true
				break
			}
		}
	}
	return vols, nil
}

// DestroyVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) DestroyVolumes(volIds []string) []error {
	results := make([]error, len(volIds))
	for i, volumeId := range volIds {
		if _, err := v.ec2.DeleteVolume(volumeId); err != nil {
			results[i] = errors.Annotatef(err, "destroying %q", volumeId)
		}
	}
	return results
}

// ValidateVolumeParams is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	vol, err := parseVolumeOptions(params.Size, params.Attributes)
	if err != nil {
		return err
	}
	if vol.VolumeSize > volumeSizeMaxGiB {
		return errors.Errorf("%d GiB exceeds the maximum of %d GiB", vol.VolumeSize, volumeSizeMaxGiB)
	}
	if vol.VolumeType == "io1" {
		if vol.VolumeSize < provisionedIopsvolumeSizeMinGiB {
			return errors.Errorf(
				"volume size is %d GiB, must be at least %d GiB for provisioned IOPS",
				vol.VolumeSize,
				provisionedIopsvolumeSizeMinGiB,
			)
		}
	}
	if vol.IOPS > 0 {
		if vol.VolumeType != "io1" {
			return errors.Errorf("IOPS specified, but volume type is %q", vol.VolumeType)
		}
		minSize := int(vol.IOPS / maxProvisionedIopsSizeRatio)
		if vol.VolumeSize < minSize {
			return errors.Errorf(
				"volume size is %d GiB, must be at least %d GiB to support %d IOPS",
				vol.VolumeSize, minSize, vol.IOPS,
			)
		}
	}
	return nil
}

// AttachVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) AttachVolumes(attachParams []storage.VolumeAttachmentParams) (attachments []storage.VolumeAttachment, err error) {
	// If there's an error, we detach any ones that are attached.
	var attached []storage.VolumeAttachmentParams
	defer func() {
		if err != nil && len(attachments) > 0 {
			err2 := v.DetachVolumes(attached)
			if err2 != nil {
				logger.Warningf("error detaching volumes: %v", err2)
			}
		}
	}()

	// We need the virtualisation types for each instance we are
	// attaching to so we can determine the device name.
	instIds := set.NewStrings()
	for _, p := range attachParams {
		instIds.Add(string(p.InstanceId))
	}
	instVirtTypes, err := v.virtTypes(instIds.Values())
	if err != nil {
		return nil, errors.Annotate(err, "finding virtulisation types for instances")
	}

	for _, params := range attachParams {
		instId := string(params.InstanceId)
		nextDeviceName := blockDeviceNamer(instVirtTypes[instId] == paravirtual)
		requestDeviceName, _, err := nextDeviceName()
		if err != nil {
			// Can't attach any more volumes.
			return nil, err
		}
		device, err := v.attachOneVolume(params.VolumeId, instId, requestDeviceName)
		if err != nil {
			return nil, errors.Annotatef(err, "attaching %v to %v as %s", params.Volume, params.Machine, requestDeviceName)
		}
		attached = append(attached, params)
		attachments = append(attachments, storage.VolumeAttachment{
			Volume:     params.Volume,
			Machine:    params.Machine,
			DeviceName: device,
			// TODO(wallyworld) - read-only
		})
	}
	return attachments, nil
}

func (v *ebsVolumeSource) attachOneVolume(volumeId, instId, requestDeviceName string) (string, error) {
	resp, err := v.ec2.AttachVolume(volumeId, instId, requestDeviceName)
	// TODO(wallyworld) - retry on IncorrectState error (volume being created)
	// Process aws specific error information.
	var device string
	if err == nil {
		device = resp.Device
	} else {
		if ec2Err, ok := err.(*ec2.Error); ok {
			switch ec2Err.Code {
			// volumeInUse means this volume is already attached.
			// TODO(wallyworld) - check that the volume is attached to the expected machine.
			case volumeInUse:
				// We need to fetch the device as the response won't have it.
				var attachedVols *ec2.VolumesResp
				attachedVols, err = v.ec2.Volumes([]string{volumeId}, nil)
				if err == nil {
					attachments := attachedVols.Volumes[0].Attachments
					if len(attachments) != 1 {
						return "", errors.Annotatef(err, "volume %v has unexpected attachment count: %v", volumeId, len(attachments))
					}
					device = attachments[0].Device
				}
			}
		}
		if err != nil {
			return "", err
		}
	}
	return device, nil
}

// virtTypes determines a mapping from instance id to virtualisation type.
func (v *ebsVolumeSource) virtTypes(instIds []string) (map[string]string, error) {
	instVirtTypes := make(map[string]string)
	filter := ec2.NewFilter()
	// Can only attach to running instances.
	filter.Add("instance-state-name", "running")
	resp, err := v.ec2.Instances(instIds, filter)
	if err != nil {
		return nil, err
	}
	for j := range resp.Reservations {
		r := &resp.Reservations[j]
		for _, inst := range r.Instances {
			instVirtTypes[inst.InstanceId] = inst.VirtType
		}
	}
	// TODO(wallyworld) - retry to allow instances to get to running state.
	if len(instVirtTypes) < len(instIds) {
		notRunning := set.NewStrings(instIds...)
		for id, _ := range instVirtTypes {
			notRunning.Remove(id)
		}
		return nil, errors.Errorf(
			"volumes can only be attached to running instances, these instances are not running: %v",
			strings.Join(notRunning.Values(), ","),
		)
	}
	return instVirtTypes, nil
}

// DetachVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) DetachVolumes(attachParams []storage.VolumeAttachmentParams) error {
	for _, params := range attachParams {
		_, err := v.ec2.DetachVolume(params.VolumeId, string(params.InstanceId), "", false)
		// Process aws specific error information.
		if err != nil {
			if ec2Err, ok := err.(*ec2.Error); ok {
				switch ec2Err.Code {
				// attachment not found means this volume is already detached.
				case attachmentNotFound:
					err = nil
				}
			}
		}
		if err != nil {
			return errors.Annotatef(err, "detaching %v from %v", params.Volume, params.Machine)
		}
	}
	return nil
}

var errTooManyVolumes = errors.New("too many EBS volumes to attach")

// blockDeviceNamer returns a function that cycles through block device names.
//
// The returned function returns the device name that should be used in
// requests to the EC2 API, and and also the (kernel) device name as it
// will appear on the machine.
//
// See http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/block-device-mapping-concepts.html
func blockDeviceNamer(numbers bool) func() (requestName, actualName string, err error) {
	const (
		// deviceLetterMin is the first letter to use for EBS block device names.
		deviceLetterMin = 'f'
		// deviceLetterMax is the last letter to use for EBS block device names.
		deviceLetterMax = 'p'
		// deviceNumMax is the maximum value for trailing numbers on block device name.
		deviceNumMax = 6
	)
	var n int
	letterRepeats := 1
	if numbers {
		letterRepeats = deviceNumMax
	}
	return func() (string, string, error) {
		letter := deviceLetterMin + (n / letterRepeats)
		if letter > deviceLetterMax {
			return "", "", errTooManyVolumes
		}
		deviceName := devicePrefix + string(letter)
		if numbers {
			deviceName += string('1' + (n % deviceNumMax))
		}
		n++
		realDeviceName := renamedDevicePrefix + deviceName[len(devicePrefix):]
		return deviceName, realDeviceName, nil
	}
}

// getBlockDeviceMappings translates constraints into BlockDeviceMappings.
//
// The first entry is always the root disk mapping, followed by instance
// stores (ephemeral disks).
func getBlockDeviceMappings(cons constraints.Value) ([]ec2.BlockDeviceMapping, error) {
	rootDiskSizeMiB := minRootDiskSizeMiB
	if cons.RootDisk != nil {
		if *cons.RootDisk >= minRootDiskSizeMiB {
			rootDiskSizeMiB = *cons.RootDisk
		} else {
			logger.Infof(
				"Ignoring root-disk constraint of %dM because it is smaller than the EC2 image size of %dM",
				*cons.RootDisk,
				minRootDiskSizeMiB,
			)
		}
	}

	// The first block device is for the root disk.
	blockDeviceMappings := []ec2.BlockDeviceMapping{{
		DeviceName: "/dev/sda1",
		VolumeSize: int64(mibToGib(rootDiskSizeMiB)),
	}}

	// Not all machines have this many instance stores.
	// Instances will be started with as many of the
	// instance stores as they can support.
	blockDeviceMappings = append(blockDeviceMappings, []ec2.BlockDeviceMapping{{
		VirtualName: "ephemeral0",
		DeviceName:  "/dev/sdb",
	}, {
		VirtualName: "ephemeral1",
		DeviceName:  "/dev/sdc",
	}, {
		VirtualName: "ephemeral2",
		DeviceName:  "/dev/sdd",
	}, {
		VirtualName: "ephemeral3",
		DeviceName:  "/dev/sde",
	}}...)

	return blockDeviceMappings, nil
}

// mibToGib converts mebibytes to gibibytes.
// AWS expects GiB, we work in MiB; round up
// to nearest GiB.
func mibToGib(m uint64) uint64 {
	return (m + 1023) / 1024
}

// gibToMib converts gibibytes to mebibytes.
func gibToMib(g uint64) uint64 {
	return g * 1024
}
