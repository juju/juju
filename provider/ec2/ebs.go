// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"regexp"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"gopkg.in/amz.v3/ec2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
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
	volumeStatusAvailable = "available"
	volumeStatusInUse     = "in-use"
	volumeStatusCreating  = "creating"

	attachmentStatusAttaching = "attaching"
	attachmentStatusAttached  = "attached"
	attachmentStatusDetaching = "detaching"
	attachmentStatusDetached  = "detached"

	instanceStateShuttingDown = "shutting-down"
	instanceStateTerminated   = "terminated"
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
	EBS_IOPS:      schema.ForceInt(),
	EBS_Encrypted: schema.Bool(),
}

var ebsConfigChecker = schema.FieldMap(
	ebsConfigFields,
	schema.Defaults{
		storage.Persistent: false,
		EBS_VolumeType:     volumeTypeMagnetic,
		EBS_IOPS:           schema.Omit,
		EBS_Encrypted:      false,
	},
)

type ebsConfig struct {
	persistent bool
	volumeType string
	iops       int
	encrypted  bool
}

func newEbsConfig(attrs map[string]interface{}) (*ebsConfig, error) {
	out, err := ebsConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating EBS storage config")
	}
	coerced := out.(map[string]interface{})
	iops, _ := coerced[EBS_IOPS].(int)
	volumeType := coerced[EBS_VolumeType].(string)
	ebsConfig := &ebsConfig{
		persistent: coerced[storage.Persistent].(bool),
		volumeType: volumeType,
		iops:       iops,
		encrypted:  coerced[EBS_Encrypted].(bool),
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
	uuid, ok := environConfig.UUID()
	if !ok {
		return nil, errors.NotFoundf("environment UUID")
	}
	source := &ebsVolumeSource{
		ec2:     ec2,
		envName: environConfig.Name(),
		envUUID: uuid,
	}
	return source, nil
}

// FilesystemSource is defined on the Provider interface.
func (e *ebsProvider) FilesystemSource(environConfig *config.Config, providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

type ebsVolumeSource struct {
	ec2     *ec2.EC2
	envName string // non-unique, informational only
	envUUID string
}

var _ storage.VolumeSource = (*ebsVolumeSource)(nil)

// parseVolumeOptions uses storage volume parameters to make a struct used to create volumes.
func parseVolumeOptions(size uint64, attrs map[string]interface{}) (_ ec2.CreateVolume, _ error) {
	ebsConfig, err := newEbsConfig(attrs)
	if err != nil {
		return ec2.CreateVolume{}, errors.Trace(err)
	}
	vol := ec2.CreateVolume{
		// Juju size is MiB, AWS size is GiB.
		VolumeSize: int(mibToGib(size)),
		VolumeType: ebsConfig.volumeType,
		Encrypted:  ebsConfig.encrypted,
		IOPS:       int64(ebsConfig.iops),
	}
	return vol, nil
}

// CreateVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) CreateVolumes(params []storage.VolumeParams) (_ []storage.CreateVolumesResult, err error) {

	// First, validate the params before we use them.
	results := make([]storage.CreateVolumesResult, len(params))
	instanceIds := set.NewStrings()
	for i, p := range params {
		if err := v.ValidateVolumeParams(p); err != nil {
			results[i].Error = err
			continue
		}
		instanceIds.Add(string(p.Attachment.InstanceId))
	}

	instances := make(instanceCache)
	if instanceIds.Size() > 1 {
		if err := instances.update(v.ec2, instanceIds.Values()...); err != nil {
			logger.Debugf("querying running instances: %v", err)
			// We ignore the error, because we don't want an invalid
			// InstanceId reference from one VolumeParams to prevent
			// the creation of another volume.
		}
	}

	for i, p := range params {
		if results[i].Error != nil {
			continue
		}
		volume, attachment, err := v.createVolume(p, instances)
		if err != nil {
			results[i].Error = err
			continue
		}
		results[i].Volume = volume
		results[i].VolumeAttachment = attachment
	}
	return results, nil
}

func (v *ebsVolumeSource) createVolume(p storage.VolumeParams, instances instanceCache) (_ *storage.Volume, _ *storage.VolumeAttachment, err error) {
	var volumeId string
	defer func() {
		if err == nil || volumeId == "" {
			return
		}
		if _, err := v.ec2.DeleteVolume(volumeId); err != nil {
			logger.Warningf("error cleaning up volume %v: %v", volumeId, err)
		}
	}()

	// TODO(axw) if preference is to use ephemeral, use ephemeral
	// until the instance stores run out. We'll need to know how
	// many there are and how big each one is. We also need to
	// unmap ephemeral0 in cloud-init.

	// Create.
	instId := string(p.Attachment.InstanceId)
	if err := instances.update(v.ec2, instId); err != nil {
		return nil, nil, errors.Trace(err)
	}
	inst, err := instances.get(instId)
	if err != nil {
		// Can't create the volume without the instance,
		// because we need to know what its AZ is.
		return nil, nil, errors.Trace(err)
	}
	vol, _ := parseVolumeOptions(p.Size, p.Attributes)
	vol.AvailZone = inst.AvailZone
	resp, err := v.ec2.CreateVolume(vol)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	volumeId = resp.Id

	// Tag.
	resourceTags := make(map[string]string)
	for k, v := range p.ResourceTags {
		resourceTags[k] = v
	}
	resourceTags[tagName] = resourceName(p.Tag, v.envName)
	if err := tagResources(v.ec2, resourceTags, volumeId); err != nil {
		return nil, nil, errors.Annotate(err, "tagging volume")
	}

	volume := storage.Volume{
		p.Tag,
		storage.VolumeInfo{
			VolumeId:   volumeId,
			Size:       gibToMib(uint64(resp.Size)),
			Persistent: true,
		},
	}
	return &volume, nil, nil
}

// ListVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) ListVolumes() ([]string, error) {
	filter := ec2.NewFilter()
	filter.Add("tag:"+tags.JujuEnv, v.envUUID)
	resp, err := v.ec2.Volumes(nil, filter)
	if err != nil {
		return nil, err
	}
	volumeIds := make([]string, len(resp.Volumes))
	for i, vol := range resp.Volumes {
		volumeIds[i] = vol.Id
	}
	return volumeIds, nil
}

// DescribeVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) DescribeVolumes(volIds []string) ([]storage.DescribeVolumesResult, error) {
	// TODO(axw) invalid volIds here should not cause the whole
	// operation to fail. If we get an invalid volume ID response,
	// fall back to querying each volume individually. That should
	// be rare.
	resp, err := v.ec2.Volumes(volIds, nil)
	if err != nil {
		return nil, err
	}
	byId := make(map[string]ec2.Volume)
	for _, vol := range resp.Volumes {
		byId[vol.Id] = vol
	}
	results := make([]storage.DescribeVolumesResult, len(volIds))
	for i, volId := range volIds {
		vol, ok := byId[volId]
		if !ok {
			results[i].Error = errors.NotFoundf("%s", volId)
			continue
		}
		results[i].VolumeInfo = &storage.VolumeInfo{
			Size:       gibToMib(uint64(vol.Size)),
			VolumeId:   vol.Id,
			Persistent: true,
		}
		for _, attachment := range vol.Attachments {
			if attachment.DeleteOnTermination {
				results[i].VolumeInfo.Persistent = false
				break
			}
		}
	}
	return results, nil
}

// DestroyVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) DestroyVolumes(volIds []string) ([]error, error) {
	var wg sync.WaitGroup
	wg.Add(len(volIds))
	results := make([]error, len(volIds))
	for i, volumeId := range volIds {
		go func(i int, volumeId string) {
			defer wg.Done()
			results[i] = v.destroyVolume(volumeId)
		}(i, volumeId)
	}
	wg.Wait()
	return results, nil
}

var destroyVolumeAttempt = utils.AttemptStrategy{
	Total: 5 * time.Minute,
	Delay: 5 * time.Second,
}

func (v *ebsVolumeSource) destroyVolume(volumeId string) error {
	logger.Debugf("destroying %q", volumeId)
	// Volumes must not be in-use when destroying. A volume may
	// still be in-use when the instance it is attached to is
	// in the process of being terminated.
	volume, err := v.waitVolume(volumeId, destroyVolumeAttempt, func(volume *ec2.Volume) (bool, error) {
		if volume.Status != volumeStatusInUse {
			// Volume is not in use, it should be OK to destroy now.
			return true, nil
		}
		if len(volume.Attachments) == 0 {
			// There are no attachments remaining now; keep querying
			// until volume transitions out of in-use.
			return false, nil
		}
		var deleteOnTermination []string
		var args []storage.VolumeAttachmentParams
		for _, a := range volume.Attachments {
			switch a.Status {
			case attachmentStatusAttaching, attachmentStatusAttached:
				// The volume is attaching or attached to an
				// instance, we need for it to be detached
				// before we can destroy it.
				args = append(args, storage.VolumeAttachmentParams{
					AttachmentParams: storage.AttachmentParams{
						InstanceId: instance.Id(a.InstanceId),
					},
					VolumeId: volumeId,
				})
				if a.DeleteOnTermination {
					// The volume is still attached, and the
					// attachment is "delete on termination";
					// check if the related instance is being
					// terminated, in which case we can stop
					// waiting and skip destroying the volume.
					//
					// Note: we still accrue in "args" above
					// in case the instance is not terminating;
					// in that case we detach and destroy as
					// usual.
					deleteOnTermination = append(
						deleteOnTermination, a.InstanceId,
					)
				}
			}
		}
		if len(deleteOnTermination) > 0 {
			result, err := v.ec2.Instances(deleteOnTermination, nil)
			if err != nil {
				return false, errors.Trace(err)
			}
			for _, reservation := range result.Reservations {
				for _, instance := range reservation.Instances {
					switch instance.State.Name {
					case instanceStateShuttingDown, instanceStateTerminated:
						// The instance is or will be terminated,
						// and so the volume will be deleted by
						// virtue of delete-on-termination.
						return true, nil
					}
				}
			}
		}
		if len(args) == 0 {
			return false, nil
		}
		results, err := v.DetachVolumes(args)
		if err != nil {
			return false, errors.Trace(err)
		}
		for _, err := range results {
			if err != nil {
				return false, errors.Trace(err)
			}
		}
		return false, nil
	})
	if err != nil {
		if errors.IsNotFound(err) {
			// Either the volume isn't found, or we queried the
			// instance corresponding to a DeleteOnTermination
			// attachment; in either case, the volume is or will
			// be destroyed.
			return nil
		} else if err == errWaitVolumeTimeout {
			return errors.Errorf("timed out waiting for volume %v to not be in-use", volumeId)
		}
		return errors.Trace(err)
	}
	if volume.Status == volumeStatusInUse {
		// If the volume is in-use, that means it will be
		// handled by delete-on-termination and we have
		// nothing more to do.
		return nil
	}
	if _, err := v.ec2.DeleteVolume(volumeId); err != nil {
		return errors.Annotatef(err, "destroying %q", volumeId)
	}
	return nil
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
	return nil
}

// AttachVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) AttachVolumes(attachParams []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	// We need the virtualisation types for each instance we are
	// attaching to so we can determine the device name.
	instIds := set.NewStrings()
	for _, p := range attachParams {
		instIds.Add(string(p.InstanceId))
	}
	instances := make(instanceCache)
	if instIds.Size() > 1 {
		if err := instances.update(v.ec2, instIds.Values()...); err != nil {
			logger.Debugf("querying running instances: %v", err)
			// We ignore the error, because we don't want an invalid
			// InstanceId reference from one VolumeParams to prevent
			// the creation of another volume.
		}
	}

	results := make([]storage.AttachVolumesResult, len(attachParams))
	for i, params := range attachParams {
		instId := string(params.InstanceId)
		if err := instances.update(v.ec2, instId); err != nil {
			results[i].Error = err
			continue
		}
		inst, err := instances.get(instId)
		if err != nil {
			results[i].Error = err
			continue
		}
		nextDeviceName := blockDeviceNamer(inst)
		_, deviceName, err := v.attachOneVolume(nextDeviceName, params.VolumeId, instId)
		if err != nil {
			results[i].Error = err
			continue
		}
		results[i].VolumeAttachment = &storage.VolumeAttachment{
			params.Volume,
			params.Machine,
			storage.VolumeAttachmentInfo{
				DeviceName: deviceName,
			},
		}
	}
	return results, nil
}

func (v *ebsVolumeSource) attachOneVolume(
	nextDeviceName func() (string, string, error),
	volumeId, instId string,
) (string, string, error) {
	// Wait for the volume to move out of "creating".
	volume, err := v.waitVolumeCreated(volumeId)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	// Possible statuses:
	//    creating | available | in-use | deleting | deleted | error
	switch volume.Status {
	default:
		return "", "", errors.Errorf("cannot attach to volume with status %q", volume.Status)

	case volumeStatusInUse:
		// Volume is already attached; see if it's attached to the
		// instance requested.
		attachments := volume.Attachments
		if len(attachments) != 1 {
			return "", "", errors.Annotatef(err, "volume %v has unexpected attachment count: %v", volumeId, len(attachments))
		}
		if attachments[0].InstanceId != instId {
			return "", "", errors.Annotatef(err, "volume %v is attached to %v", volumeId, attachments[0].InstanceId)
		}
		requestDeviceName := attachments[0].Device
		actualDeviceName := renamedDevicePrefix + requestDeviceName[len(devicePrefix):]
		return requestDeviceName, actualDeviceName, nil

	case volumeStatusAvailable:
		// Attempt to attach below.
		break
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
			}
		}
		if err != nil {
			return "", "", errors.Annotate(err, "attaching volume")
		}
		return requestDeviceName, actualDeviceName, nil
	}
}

func (v *ebsVolumeSource) waitVolumeCreated(volumeId string) (*ec2.Volume, error) {
	var attempt = utils.AttemptStrategy{
		Total: 5 * time.Second,
		Delay: 200 * time.Millisecond,
	}
	var lastStatus string
	volume, err := v.waitVolume(volumeId, attempt, func(volume *ec2.Volume) (bool, error) {
		lastStatus = volume.Status
		return volume.Status != volumeStatusCreating, nil
	})
	if err == errWaitVolumeTimeout {
		return nil, errors.Errorf(
			"timed out waiting for volume %v to become available (%v)",
			volumeId, lastStatus,
		)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return volume, nil
}

var errWaitVolumeTimeout = errors.New("timed out")

func (v *ebsVolumeSource) waitVolume(
	volumeId string,
	attempt utils.AttemptStrategy,
	pred func(v *ec2.Volume) (bool, error),
) (*ec2.Volume, error) {
	for a := attempt.Start(); a.Next(); {
		volume, err := v.describeVolume(volumeId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ok, err := pred(volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if ok {
			return volume, nil
		}
	}
	return nil, errWaitVolumeTimeout
}

func (v *ebsVolumeSource) describeVolume(volumeId string) (*ec2.Volume, error) {
	resp, err := v.ec2.Volumes([]string{volumeId}, nil)
	if err != nil {
		return nil, errors.Annotate(err, "querying volume")
	}
	if len(resp.Volumes) == 0 {
		return nil, errors.NotFoundf("%v", volumeId)
	} else if len(resp.Volumes) != 1 {
		return nil, errors.Errorf("expected one volume, got %d", len(resp.Volumes))
	}
	return &resp.Volumes[0], nil
}

type instanceCache map[string]ec2.Instance

func (c instanceCache) update(ec2client *ec2.EC2, ids ...string) error {
	if len(ids) == 1 {
		if _, ok := c[ids[0]]; ok {
			return nil
		}
	}
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "running")
	resp, err := ec2client.Instances(ids, filter)
	if err != nil {
		return errors.Annotate(err, "querying instance details")
	}
	for j := range resp.Reservations {
		r := &resp.Reservations[j]
		for _, inst := range r.Instances {
			c[inst.InstanceId] = inst
		}
	}
	return nil
}

func (c instanceCache) get(id string) (ec2.Instance, error) {
	inst, ok := c[id]
	if !ok {
		return ec2.Instance{}, errors.Errorf("cannot attach to non-running instance %v", id)
	}
	return inst, nil
}

// DetachVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) DetachVolumes(attachParams []storage.VolumeAttachmentParams) ([]error, error) {
	results := make([]error, len(attachParams))
	for i, params := range attachParams {
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
			results[i] = errors.Annotatef(
				err, "detaching %v from %v", params.Volume, params.Machine,
			)
		}
	}
	return results, nil
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
