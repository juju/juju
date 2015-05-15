// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"regexp"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils"
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

	// The volume type (default standard):
	//   "gp2" for General Purpose (SSD) volumes
	//   "io1" for Provisioned IOPS (SSD) volumes,
	//   "standard" for Magnetic volumes.
	EBS_VolumeType = "volume-type"

	// The number of I/O operations per second (IOPS) to provision for the volume.
	// Only valid for Provisioned IOPS (SSD) volumes.
	EBS_IOPS = "iops"

	// Specifies whether the volume should be encrypted.
	EBS_Encrypted = "encrypted"

	// The availability zone in which the volume will be created.
	//
	// Setting the availability-zone is an error for non-persistent
	// volumes, as the volume will be created in the zone of the
	// instance it is bound to.
	EBS_AvailabilityZone = "availability-zone"

	volumeTypeMagnetic        = "magnetic"         // standard
	volumeTypeSsd             = "ssd"              // gp2
	volumeTypeProvisionedIops = "provisioned-iops" // io1
	volumeTypeStandard        = "standard"
	volumeTypeGp2             = "gp2"
	volumeTypeIo1             = "io1"
)

// AWS error codes
const (
	deviceInUse        = "InvalidDevice.InUse"
	volumeInUse        = "VolumeInUse"
	attachmentNotFound = "InvalidAttachment.NotFound"
	incorrectState     = "IncorrectState"
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

var deviceInUseRegexp = regexp.MustCompile(".*Attachment point .* is already in use")

func init() {
	ebsssdPool, _ := storage.NewConfig("ebs-ssd", EBS_ProviderType, map[string]interface{}{
		EBS_VolumeType: volumeTypeSsd,
	})
	defaultPools := []*storage.Config{
		ebsssdPool,
	}
	poolmanager.RegisterDefaultStoragePools(defaultPools)
}

// ebsProvider creates volume sources which use AWS EBS volumes.
type ebsProvider struct{}

var _ storage.Provider = (*ebsProvider)(nil)

var ebsConfigFields = schema.Fields{
	storage.Persistent: schema.Bool(),
	EBS_VolumeType: schema.OneOf(
		schema.Const(volumeTypeMagnetic),
		schema.Const(volumeTypeSsd),
		schema.Const(volumeTypeProvisionedIops),
		schema.Const(volumeTypeStandard),
		schema.Const(volumeTypeGp2),
		schema.Const(volumeTypeIo1),
	),
	EBS_IOPS:             schema.ForceInt(),
	EBS_Encrypted:        schema.Bool(),
	EBS_AvailabilityZone: schema.String(),
}

var ebsConfigChecker = schema.FieldMap(
	ebsConfigFields,
	schema.Defaults{
		storage.Persistent:   false,
		EBS_VolumeType:       volumeTypeMagnetic,
		EBS_IOPS:             schema.Omit,
		EBS_Encrypted:        false,
		EBS_AvailabilityZone: schema.Omit,
	},
)

type ebsConfig struct {
	persistent       bool
	volumeType       string
	iops             int
	encrypted        bool
	availabilityZone string
}

func newEbsConfig(attrs map[string]interface{}) (*ebsConfig, error) {
	out, err := ebsConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating EBS storage config")
	}
	coerced := out.(map[string]interface{})
	iops, _ := coerced[EBS_IOPS].(int)
	availabilityZone, _ := coerced[EBS_AvailabilityZone].(string)
	volumeType := coerced[EBS_VolumeType].(string)
	ebsConfig := &ebsConfig{
		persistent:       coerced[storage.Persistent].(bool),
		volumeType:       volumeType,
		iops:             iops,
		encrypted:        coerced[EBS_Encrypted].(bool),
		availabilityZone: availabilityZone,
	}
	switch ebsConfig.volumeType {
	case volumeTypeMagnetic:
		ebsConfig.volumeType = volumeTypeStandard
	case volumeTypeSsd:
		ebsConfig.volumeType = volumeTypeGp2
	case volumeTypeProvisionedIops:
		ebsConfig.volumeType = volumeTypeIo1
	}
	if ebsConfig.iops > 0 && ebsConfig.volumeType != volumeTypeIo1 {
		return nil, errors.Errorf("IOPS specified, but volume type is %q", volumeType)
	} else if ebsConfig.iops == 0 && ebsConfig.volumeType == volumeTypeIo1 {
		return nil, errors.Errorf("volume type is %q, IOPS unspecified or zero", volumeTypeIo1)
	}
	return ebsConfig, nil
}

// ValidateConfig is defined on the Provider interface.
func (e *ebsProvider) ValidateConfig(cfg *storage.Config) error {
	_, err := newEbsConfig(cfg.Attrs())
	return errors.Trace(err)
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
	return true
}

// VolumeSource is defined on the Provider interface.
func (e *ebsProvider) VolumeSource(environConfig *config.Config, cfg *storage.Config) (storage.VolumeSource, error) {
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
func parseVolumeOptions(size uint64, attrs map[string]interface{}) (_ ec2.CreateVolume, persistent bool, _ error) {
	ebsConfig, err := newEbsConfig(attrs)
	if err != nil {
		return ec2.CreateVolume{}, false, errors.Trace(err)
	}
	vol := ec2.CreateVolume{
		// Juju size is MiB, AWS size is GiB.
		VolumeSize: int(mibToGib(size)),
		VolumeType: ebsConfig.volumeType,
		AvailZone:  ebsConfig.availabilityZone,
		Encrypted:  ebsConfig.encrypted,
		IOPS:       int64(ebsConfig.iops),
	}
	return vol, ebsConfig.persistent, nil
}

// CreateVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) CreateVolumes(params []storage.VolumeParams) (_ []storage.Volume, _ []storage.VolumeAttachment, err error) {
	volumes := make([]storage.Volume, 0, len(params))
	volumeAttachments := make([]storage.VolumeAttachment, 0, len(params))

	// If there's an error, we delete any ones that are created.
	defer func() {
		if err != nil && len(volumes) > 0 {
			volIds := make([]string, len(volumes))
			for i, v := range volumes {
				volIds[i] = v.VolumeId
			}
			err2 := v.DestroyVolumes(volIds)
			for i, volErr := range err2 {
				if volErr == nil {
					continue
				}
				logger.Warningf("error cleaning up volume %v: %v", volumes[i].Tag, volErr)
			}
		}
	}()

	// TODO(axw) if preference is to use ephemeral, use ephemeral
	// until the instance stores run out. We'll need to know how
	// many there are and how big each one is. We also need to
	// unmap ephemeral0 in cloud-init.

	// First, validate the params before we use them.
	instanceIds := set.NewStrings()
	for _, p := range params {
		if err := v.ValidateVolumeParams(p); err != nil {
			return nil, nil, errors.Trace(err)
		}
		if p.Attachment != nil && p.Attachment.InstanceId != "" {
			instanceIds.Add(string(p.Attachment.InstanceId))
		}
	}
	instances, err := v.instances(instanceIds.Values())
	if err != nil {
		return nil, nil, errors.Annotate(err, "querying instance details")
	}

	for _, p := range params {
		var instId string
		vol, persistent, _ := parseVolumeOptions(p.Size, p.Attributes)
		if !persistent {
			instId = string(p.Attachment.InstanceId)
			vol.AvailZone = instances[instId].AvailZone
		}
		resp, err := v.ec2.CreateVolume(vol)
		if err != nil {
			return nil, nil, err
		}
		volumeId := resp.Id
		volumes = append(volumes, storage.Volume{
			Tag:        p.Tag,
			VolumeId:   volumeId,
			Size:       gibToMib(uint64(resp.Size)),
			Persistent: persistent,
		})

		// Persistent volumes' attachments are created independently.
		// We must create the attachments for non-persistent volumes
		// immediately, as the "non-persistence" is a property of the
		// attachment.
		if persistent {
			continue
		}
		nextDeviceName := blockDeviceNamer(instances[instId])
		requestDeviceName, actualDeviceName, err := v.attachOneVolume(nextDeviceName, resp.Volume.Id, instId, false)
		if err != nil {
			return nil, nil, errors.Annotatef(err, "attaching %v to %v", resp.Volume.Id, instId)
		}
		if !persistent {
			_, err := v.ec2.ModifyInstanceAttribute(&ec2.ModifyInstanceAttribute{
				InstanceId: instId,
				BlockDeviceMappings: []ec2.InstanceBlockDeviceMapping{{
					DeviceName:          requestDeviceName,
					VolumeId:            volumeId,
					DeleteOnTermination: true,
				}},
			}, nil)
			if err != nil {
				return nil, nil, errors.Annotatef(err, "binding termination of %v to %v", resp.Volume.Id, instId)
			}
		}
		volumeAttachments = append(volumeAttachments, storage.VolumeAttachment{
			Volume:     p.Tag,
			Machine:    p.Attachment.Machine,
			DeviceName: actualDeviceName,
		})
	}
	return volumes, volumeAttachments, nil
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
	vol, persistent, err := parseVolumeOptions(params.Size, params.Attributes)
	if err != nil {
		return err
	}
	if !persistent && (params.Attachment == nil || params.Attachment.InstanceId == "") {
		// Non-persistent volumes require an instance before they can
		// be created, in order to set the appropriate availability
		// zone and to bind the volume to the instance's lifetime.
		return storage.ErrVolumeNeedsInstance
	}
	if vol.VolumeSize > volumeSizeMaxGiB {
		return errors.Errorf("%d GiB exceeds the maximum of %d GiB", vol.VolumeSize, volumeSizeMaxGiB)
	}
	if vol.VolumeType == volumeTypeIo1 {
		if vol.VolumeSize < provisionedIopsvolumeSizeMinGiB {
			return errors.Errorf(
				"volume size is %d GiB, must be at least %d GiB for provisioned IOPS",
				vol.VolumeSize,
				provisionedIopsvolumeSizeMinGiB,
			)
		}
	}
	if vol.IOPS > 0 {
		minSize := int(vol.IOPS / maxProvisionedIopsSizeRatio)
		if vol.VolumeSize < minSize {
			return errors.Errorf(
				"volume size is %d GiB, must be at least %d GiB to support %d IOPS",
				vol.VolumeSize, minSize, vol.IOPS,
			)
		}
	}
	// TODO(axw) we should always attach volumes to a machine initially, so the user should not
	// have an option to specify the AZ.
	if persistent && vol.AvailZone == "" {
		return errors.New("missing availability zone for persistent volume")
	} else if !persistent && vol.AvailZone != "" {
		return errors.New("cannot specify availability zone for non-persistent volume")
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
	instances, err := v.instances(instIds.Values())
	if err != nil {
		return nil, errors.Trace(err)
	}

	for _, params := range attachParams {
		instId := string(params.InstanceId)
		nextDeviceName := blockDeviceNamer(instances[instId])
		_, deviceName, err := v.attachOneVolume(nextDeviceName, params.VolumeId, instId, false)
		if err != nil {
			return nil, errors.Annotatef(err, "attaching %v to %v", params.VolumeId, params.InstanceId)
		}
		attached = append(attached, params)
		attachments = append(attachments, storage.VolumeAttachment{
			Volume:     params.Volume,
			Machine:    params.Machine,
			DeviceName: deviceName,
		})
	}
	return attachments, nil
}

func (v *ebsVolumeSource) attachOneVolume(
	nextDeviceName func() (string, string, error),
	volumeId, instId string,
	deleteOnTermination bool,
) (string, string, error) {
	// Wait for the volume to be "available".
	if err := v.waitVolumeAvailable(volumeId); err != nil {
		return "", "", errors.Trace(err)
	}
	for {
		requestDeviceName, actualDeviceName, err := nextDeviceName()
		if err != nil {
			// Can't attach any more volumes.
			return "", "", err
		}
		_, err = v.ec2.AttachVolume(volumeId, instId, requestDeviceName)
		if ec2Err, ok := err.(*ec2.Error); ok {
			switch ec2Err.Code {
			case invalidParameterValue:
				// InvalidParameterValue is returned by AttachVolume
				// rather than InvalidDevice.InUse as the docs would
				// suggest.
				if !deviceInUseRegexp.MatchString(ec2Err.Message) {
					break
				}
				fallthrough

			case deviceInUse:
				// deviceInUse means that the requested device name
				// is in use already. Try again with the next name.
				continue

			case volumeInUse:
				// volumeInUse means this volume is already attached.
				// query the volume and verify that the attachment is
				// for this machine.
				volume, err := v.describeVolume(volumeId)
				if err != nil {
					return "", "", errors.Trace(err)
				}
				attachments := volume.Attachments
				if len(attachments) != 1 {
					return "", "", errors.Annotatef(err, "volume %v has unexpected attachment count: %v", volumeId, len(attachments))
				}
				if attachments[0].InstanceId != instId {
					return "", "", errors.Annotatef(err, "volume %v is attached to %v", volumeId, attachments[0].InstanceId)
				}
				if requestDeviceName != attachments[0].Device {
					requestDeviceName = attachments[0].Device
					actualDeviceName = renamedDevicePrefix + requestDeviceName[len(devicePrefix):]
				}
				return requestDeviceName, actualDeviceName, nil
			}
		}
		if err != nil {
			return "", "", errors.Annotate(err, "attaching volume")
		}
		return requestDeviceName, actualDeviceName, nil
	}
}

func (v *ebsVolumeSource) waitVolumeAvailable(volumeId string) error {
	var attempt = utils.AttemptStrategy{
		Total: 5 * time.Second,
		Delay: 200 * time.Millisecond,
	}
	for a := attempt.Start(); a.Next(); {
		volume, err := v.describeVolume(volumeId)
		if err != nil {
			return errors.Trace(err)
		}
		if volume.Status == "available" {
			return nil
		}
	}
	return errors.Errorf("timed out waiting for volume %v to become available", volumeId)
}

func (v *ebsVolumeSource) describeVolume(volumeId string) (*ec2.Volume, error) {
	resp, err := v.ec2.Volumes([]string{volumeId}, nil)
	if err != nil {
		return nil, errors.Annotate(err, "querying volume")
	}
	if len(resp.Volumes) != 1 {
		return nil, errors.Errorf("expected one volume, got %d", len(resp.Volumes))
	}
	return &resp.Volumes[0], nil
}

// instances returns a mapping from the specified instance IDs to ec2.Instance
// structures. If any of the specified IDs does not refer to a running instance,
// it will cause an error to be returned.
func (v *ebsVolumeSource) instances(instIds []string) (map[string]ec2.Instance, error) {
	instances := make(map[string]ec2.Instance)
	// Can only attach to running instances.
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "running")
	resp, err := v.ec2.Instances(instIds, filter)
	if err != nil {
		return nil, err
	}
	for j := range resp.Reservations {
		r := &resp.Reservations[j]
		for _, inst := range r.Instances {
			instances[inst.InstanceId] = inst
		}
	}
	// TODO(wallyworld) - retry to allow instances to get to running state.
	if len(instances) < len(instIds) {
		notRunning := set.NewStrings(instIds...)
		for id, _ := range instances {
			notRunning.Remove(id)
		}
		return nil, errors.Errorf(
			"volumes can only be attached to running instances, these instances are not running: %v",
			strings.Join(notRunning.Values(), ","),
		)
	}
	return instances, nil
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
func blockDeviceNamer(inst ec2.Instance) func() (requestName, actualName string, err error) {
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
	numbers := inst.VirtType == "paravirtual"
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
