// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils"
	"gopkg.in/amz.v3/ec2"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
)

const (
	EBS_ProviderType = storage.ProviderType("ebs")

	// Config attributes

	// The volume type (default standard):
	//   "gp2" for General Purpose (SSD) volumes
	//   "io1" for Provisioned IOPS (SSD) volumes,
	//   "standard" for Magnetic volumes.
	EBS_VolumeType = "volume-type"

	// The number of I/O operations per second (IOPS) per GiB
	// to provision for the volume. Only valid for Provisioned
	// IOPS (SSD) volumes.
	EBS_IOPS = "iops"

	// Specifies whether the volume should be encrypted.
	EBS_Encrypted = "encrypted"

	// Volume Aliases
	volumeAliasMagnetic        = "magnetic"         // standard
	volumeAliasOptimizedHDD    = "optimized-hdd"    // sc1
	volumeAliasColdStorage     = "cold-storage"     // sc1
	volumeAliasSSD             = "ssd"              // gp2
	volumeAliasProvisionedIops = "provisioned-iops" // io1

	// Volume types
	volumeTypeStandard = "standard"
	volumeTypeGP2      = "gp2"
	volumeTypeIO1      = "io1"
	volumeTypeST1      = "st1"
	volumeTypeSC1      = "sc1"

	rootDiskDeviceName = "/dev/sda1"

	nvmeDeviceLinkPrefix = "/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_"

	// defaultControllerDiskSizeMiB is the default size for the
	// root disk of controller machines, if no root-disk constraint
	// is specified.
	defaultControllerDiskSizeMiB = 32 * 1024
)

// AWS error codes
const (
	deviceInUse        = "InvalidDevice.InUse"
	attachmentNotFound = "InvalidAttachment.NotFound"
	volumeNotFound     = "InvalidVolume.NotFound"
	incorrectState     = "IncorrectState"
)

const (
	volumeStatusAvailable = "available"
	volumeStatusInUse     = "in-use"
	volumeStatusCreating  = "creating"

	attachmentStatusAttaching = "attaching"
	attachmentStatusAttached  = "attached"

	instanceStateShuttingDown = "shutting-down"
	instanceStateTerminated   = "terminated"
)

// Limits for volume parameters. See:
//   http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/EBSVolumeTypes.html
const (
	// minMagneticVolumeSizeGiB is the minimum size for magnetic volumes in GiB.
	minMagneticVolumeSizeGiB = 1

	// maxMagneticVolumeSizeGiB is the maximum size for magnetic volumes in GiB.
	maxMagneticVolumeSizeGiB = 1024

	// minSSDVolumeSizeGiB is the minimum size for SSD volumes in GiB.
	minSSDVolumeSizeGiB = 1

	// maxSSDVolumeSizeGiB is the maximum size for SSD volumes in GiB.
	maxSSDVolumeSizeGiB = 16 * 1024

	// minProvisionedIopsVolumeSizeGiB is the minimum size of provisioned IOPS
	// volumes in GiB.
	minProvisionedIopsVolumeSizeGiB = 4

	// maxProvisionedIopsVolumeSizeGiB is the maximum size of provisioned IOPS
	// volumes in GiB.
	maxProvisionedIopsVolumeSizeGiB = 16 * 1024

	// maxProvisionedIopsSizeRatio is the maximum allowed ratio of IOPS to
	// size (in GiB), for provisioend IOPS volumes.
	maxProvisionedIopsSizeRatio = 30

	// maxProvisionedIops is the maximum allowed IOPS in total for provisioned IOPS
	// volumes. We take the minimum of volumeSize*maxProvisionedIopsSizeRatio and
	// maxProvisionedIops.
	maxProvisionedIops = 20000

	// minSt1VolumeSizeGiB is the minimum volume size for st1 volume instances.
	minSt1VolumeSizeGiB = 500

	// maxSt1VolumeSizeGiB is the maximum volume size for st1 volume instances.
	maxSt1VolumeSizeGiB = 16 * 1024

	// minSc1VolumeSizeGiB is the minimum volume size for sc1 volume instances.
	minSc1VolumeSizeGiB = 500

	// maxSc1VolumeSizeGiB is the maximum volume size for sc1 volume instances.
	maxSc1VolumeSizeGiB = 16 * 1024
)

const (
	// devicePrefix is the prefix for device names specified when creating volumes.
	devicePrefix = "/dev/sd"

	// renamedDevicePrefix is the prefix for device names after they have
	// been renamed. This should replace "devicePrefix" in the device name
	// when recording the block device info in state.
	renamedDevicePrefix = "xvd"
)

var deviceInUseRegexp = regexp.MustCompile(".*Attachment point .* is already in use")

// StorageProviderTypes implements storage.ProviderRegistry.
func (env *environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{EBS_ProviderType}, nil
}

// StorageProvider implements storage.ProviderRegistry.
func (env *environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	if t == EBS_ProviderType {
		return &ebsProvider{env}, nil
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}

// ebsProvider creates volume sources which use AWS EBS volumes.
type ebsProvider struct {
	env *environ
}

var _ storage.Provider = (*ebsProvider)(nil)

var ebsConfigFields = schema.Fields{
	EBS_VolumeType: schema.OneOf(
		schema.Const(volumeAliasMagnetic),
		schema.Const(volumeAliasOptimizedHDD),
		schema.Const(volumeAliasColdStorage),
		schema.Const(volumeAliasSSD),
		schema.Const(volumeAliasProvisionedIops),
		schema.Const(volumeTypeStandard),
		schema.Const(volumeTypeGP2),
		schema.Const(volumeTypeIO1),
		schema.Const(volumeTypeST1),
		schema.Const(volumeTypeSC1),
	),
	EBS_IOPS:      schema.ForceInt(),
	EBS_Encrypted: schema.Bool(),
}

var ebsConfigChecker = schema.FieldMap(
	ebsConfigFields,
	schema.Defaults{
		EBS_VolumeType: volumeAliasSSD,
		EBS_IOPS:       schema.Omit,
		EBS_Encrypted:  false,
	},
)

type ebsConfig struct {
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
		volumeType: volumeType,
		iops:       iops,
		encrypted:  coerced[EBS_Encrypted].(bool),
	}
	switch ebsConfig.volumeType {
	case volumeAliasMagnetic:
		ebsConfig.volumeType = volumeTypeStandard
	case volumeAliasColdStorage:
		ebsConfig.volumeType = volumeTypeSC1
	case volumeAliasOptimizedHDD:
		ebsConfig.volumeType = volumeTypeST1
	case volumeAliasSSD:
		ebsConfig.volumeType = volumeTypeGP2
	case volumeAliasProvisionedIops:
		ebsConfig.volumeType = volumeTypeIO1
	}
	if ebsConfig.iops > 0 && ebsConfig.volumeType != volumeTypeIO1 {
		return nil, errors.Errorf("IOPS specified, but volume type is %q", volumeType)
	} else if ebsConfig.iops == 0 && ebsConfig.volumeType == volumeTypeIO1 {
		return nil, errors.Errorf("volume type is %q, IOPS unspecified or zero", volumeTypeIO1)
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

// Releasable is defined on the Provider interface.
func (*ebsProvider) Releasable() bool {
	return true
}

// DefaultPools is defined on the Provider interface.
func (e *ebsProvider) DefaultPools() []*storage.Config {
	ssdPool, _ := storage.NewConfig("ebs-ssd", EBS_ProviderType, map[string]interface{}{
		EBS_VolumeType: volumeAliasSSD,
	})
	return []*storage.Config{ssdPool}
}

// VolumeSource is defined on the Provider interface.
func (e *ebsProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	environConfig := e.env.Config()
	source := &ebsVolumeSource{
		env:       e.env,
		envName:   environConfig.Name(),
		modelUUID: environConfig.UUID(),
	}
	return source, nil
}

// FilesystemSource is defined on the Provider interface.
func (e *ebsProvider) FilesystemSource(providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

type ebsVolumeSource struct {
	env       *environ
	envName   string // non-unique, informational only
	modelUUID string
}

var _ storage.VolumeSource = (*ebsVolumeSource)(nil)

// parseVolumeOptions uses storage volume parameters to make a struct used to create volumes.
func parseVolumeOptions(size uint64, attrs map[string]interface{}) (_ ec2.CreateVolume, _ error) {
	ebsConfig, err := newEbsConfig(attrs)
	if err != nil {
		return ec2.CreateVolume{}, errors.Trace(err)
	}
	if ebsConfig.iops > maxProvisionedIopsSizeRatio {
		return ec2.CreateVolume{}, errors.Errorf(
			"specified IOPS ratio is %d/GiB, maximum is %d/GiB",
			ebsConfig.iops, maxProvisionedIopsSizeRatio,
		)
	}

	sizeInGib := mibToGib(size)
	iops := uint64(ebsConfig.iops) * sizeInGib
	if iops > maxProvisionedIops {
		iops = maxProvisionedIops
	}
	vol := ec2.CreateVolume{
		// Juju size is MiB, AWS size is GiB.
		VolumeSize: int(sizeInGib),
		VolumeType: ebsConfig.volumeType,
		Encrypted:  ebsConfig.encrypted,
		IOPS:       int64(iops),
	}
	return vol, nil
}

// CreateVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) CreateVolumes(ctx context.ProviderCallContext, params []storage.VolumeParams) (_ []storage.CreateVolumesResult, err error) {

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
		if err := instances.update(v.env.ec2, ctx, instanceIds.Values()...); err != nil {
			err := maybeConvertCredentialError(err, ctx)
			logger.Debugf("querying running instances: %v", err)
			// We ignore the error, because we don't want an invalid
			// InstanceId reference from one VolumeParams to prevent
			// the creation of another volume.
			// Except if it is a credential error...
			if common.IsCredentialNotValid(err) {
				return nil, errors.Trace(err)
			}
		}
	}

	for i, p := range params {
		if results[i].Error != nil {
			continue
		}
		volume, attachment, err := v.createVolume(ctx, p, instances)
		if err != nil {
			results[i].Error = err
			continue
		}
		results[i].Volume = volume
		results[i].VolumeAttachment = attachment
	}
	return results, nil
}

func (v *ebsVolumeSource) createVolume(ctx context.ProviderCallContext, p storage.VolumeParams, instances instanceCache) (_ *storage.Volume, _ *storage.VolumeAttachment, err error) {
	var volumeId string
	defer func() {
		if err == nil || volumeId == "" {
			return
		}
		if _, err := v.env.ec2.DeleteVolume(volumeId); err != nil {
			logger.Errorf("error cleaning up volume %v: %v", volumeId, maybeConvertCredentialError(err, ctx))
		}
	}()

	// TODO(axw) if preference is to use ephemeral, use ephemeral
	// until the instance stores run out. We'll need to know how
	// many there are and how big each one is. We also need to
	// unmap ephemeral0 in cloud-init.

	// Create.
	instId := string(p.Attachment.InstanceId)
	if err := instances.update(v.env.ec2, ctx, instId); err != nil {
		return nil, nil, errors.Trace(maybeConvertCredentialError(err, ctx))
	}
	inst, err := instances.get(instId)
	if err != nil {
		// Can't create the volume without the instance,
		// because we need to know what its AZ is.
		return nil, nil, errors.Trace(maybeConvertCredentialError(err, ctx))
	}
	vol, _ := parseVolumeOptions(p.Size, p.Attributes)
	vol.AvailZone = inst.AvailZone
	resp, err := v.env.ec2.CreateVolume(vol)
	if err != nil {
		return nil, nil, errors.Trace(maybeConvertCredentialError(err, ctx))
	}
	volumeId = resp.Id

	// Tag.
	resourceTags := make(map[string]string)
	for k, v := range p.ResourceTags {
		resourceTags[k] = v
	}
	resourceTags[tagName] = resourceName(p.Tag, v.envName)
	if err := tagResources(v.env.ec2, ctx, resourceTags, volumeId); err != nil {
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
func (v *ebsVolumeSource) ListVolumes(ctx context.ProviderCallContext) ([]string, error) {
	filter := ec2.NewFilter()
	filter.Add("tag:"+tags.JujuModel, v.modelUUID)
	return listVolumes(v.env.ec2, ctx, filter, false)
}

func listVolumes(client *ec2.EC2, ctx context.ProviderCallContext, filter *ec2.Filter, includeRootDisks bool) ([]string, error) {
	resp, err := client.Volumes(nil, filter)
	if err != nil {
		return nil, maybeConvertCredentialError(err, ctx)
	}
	volumeIds := make([]string, 0, len(resp.Volumes))
	for _, vol := range resp.Volumes {
		var isRootDisk bool
		for _, att := range vol.Attachments {
			if att.Device == rootDiskDeviceName {
				isRootDisk = true
				break
			}
		}
		if isRootDisk && !includeRootDisks {
			// We don't want to list root disks in the output.
			// These are managed by the instance provisioning
			// code; they will be created and destroyed with
			// instances.
			continue
		}
		volumeIds = append(volumeIds, vol.Id)
	}
	return volumeIds, nil
}

// DescribeVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) DescribeVolumes(ctx context.ProviderCallContext, volIds []string) ([]storage.DescribeVolumesResult, error) {
	// TODO(axw) invalid volIds here should not cause the whole
	// operation to fail. If we get an invalid volume ID response,
	// fall back to querying each volume individually. That should
	// be rare.
	resp, err := v.env.ec2.Volumes(volIds, nil)
	if err != nil {
		return nil, maybeConvertCredentialError(err, ctx)
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
func (v *ebsVolumeSource) DestroyVolumes(ctx context.ProviderCallContext, volIds []string) ([]error, error) {
	return foreachVolume(v.env.ec2, ctx, volIds, destroyVolume), nil
}

// ReleaseVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) ReleaseVolumes(ctx context.ProviderCallContext, volIds []string) ([]error, error) {
	return foreachVolume(v.env.ec2, ctx, volIds, releaseVolume), nil
}

func foreachVolume(client *ec2.EC2, ctx context.ProviderCallContext, volIds []string, f func(*ec2.EC2, context.ProviderCallContext, string) error) []error {
	var wg sync.WaitGroup
	wg.Add(len(volIds))
	results := make([]error, len(volIds))
	for i, volumeId := range volIds {
		go func(i int, volumeId string) {
			defer wg.Done()
			results[i] = f(client, ctx, volumeId)
		}(i, volumeId)
	}
	wg.Wait()
	return results
}

var destroyVolumeAttempt = utils.AttemptStrategy{
	Total: 5 * time.Minute,
	Delay: 5 * time.Second,
}

func destroyVolume(client *ec2.EC2, ctx context.ProviderCallContext, volumeId string) (err error) {
	defer func() {
		if err != nil {
			if ec2ErrCode(err) == volumeNotFound || errors.IsNotFound(err) {
				// Either the volume isn't found, or we queried the
				// instance corresponding to a DeleteOnTermination
				// attachment; in either case, the volume is or will
				// be destroyed.
				logger.Tracef("Ignoring error destroying volume %q: %v", volumeId, err)
				err = nil
			} else {
				err = maybeConvertCredentialError(err, ctx)
			}
		}
	}()

	logger.Debugf("destroying %q", volumeId)

	// Volumes must not be in-use when destroying. A volume may
	// still be in-use when the instance it is attached to is
	// in the process of being terminated.
	volume, err := waitVolume(client, ctx, volumeId, destroyVolumeAttempt, func(volume *ec2.Volume) (bool, error) {
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
			result, err := client.Instances(deleteOnTermination, nil)
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
		results, err := detachVolumes(client, ctx, args)
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
		if err == errWaitVolumeTimeout {
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
	_, err = client.DeleteVolume(volumeId)
	return errors.Annotatef(maybeConvertCredentialError(err, ctx), "destroying %q", volumeId)
}

func releaseVolume(client *ec2.EC2, ctx context.ProviderCallContext, volumeId string) error {
	logger.Debugf("releasing %q", volumeId)
	_, err := waitVolume(client, ctx, volumeId, destroyVolumeAttempt, func(volume *ec2.Volume) (bool, error) {
		if volume.Status == volumeStatusAvailable {
			return true, nil
		}
		for _, a := range volume.Attachments {
			if a.DeleteOnTermination {
				return false, errors.New("delete-on-termination flag is set")
			}
			switch a.Status {
			case attachmentStatusAttaching, attachmentStatusAttached:
				return false, errors.New("attachments still active")
			}
		}
		return false, nil
	})
	if err != nil {
		if err == errWaitVolumeTimeout {
			return errors.Errorf("timed out waiting for volume %v to become available", volumeId)
		}
		return errors.Annotatef(maybeConvertCredentialError(err, ctx), "cannot release volume %q", volumeId)
	}
	// Releasing the volume just means dropping the
	// tags that associate it with the model and
	// controller.
	tags := map[string]string{
		tags.JujuModel:      "",
		tags.JujuController: "",
	}
	return errors.Annotate(tagResources(client, ctx, tags, volumeId), "tagging volume")
}

// ValidateVolumeParams is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	vol, err := parseVolumeOptions(params.Size, params.Attributes)
	if err != nil {
		return err
	}
	var minVolumeSize, maxVolumeSize int
	switch vol.VolumeType {
	case volumeTypeStandard:
		minVolumeSize = minMagneticVolumeSizeGiB
		maxVolumeSize = maxMagneticVolumeSizeGiB
	case volumeTypeGP2:
		minVolumeSize = minSSDVolumeSizeGiB
		maxVolumeSize = maxSSDVolumeSizeGiB
	case volumeTypeIO1:
		minVolumeSize = minProvisionedIopsVolumeSizeGiB
		maxVolumeSize = maxProvisionedIopsVolumeSizeGiB
	case volumeTypeST1:
		minVolumeSize = minSt1VolumeSizeGiB
		maxVolumeSize = maxSt1VolumeSizeGiB
	case volumeTypeSC1:
		minVolumeSize = minSc1VolumeSizeGiB
		maxVolumeSize = maxSc1VolumeSizeGiB
	}
	if vol.VolumeSize < minVolumeSize {
		return errors.Errorf(
			"volume size is %d GiB, must be at least %d GiB",
			vol.VolumeSize, minVolumeSize,
		)
	}
	if vol.VolumeSize > maxVolumeSize {
		return errors.Errorf(
			"volume size %d GiB exceeds the maximum of %d GiB",
			vol.VolumeSize, maxVolumeSize,
		)
	}
	return nil
}

// AttachVolumes is specified on the storage.VolumeSource interface.
func (v *ebsVolumeSource) AttachVolumes(ctx context.ProviderCallContext, attachParams []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	// We need the instance type for each instance we are
	// attaching to so we can determine how to identify the
	// volume attachment
	instIds := set.NewStrings()
	for _, p := range attachParams {
		instIds.Add(string(p.InstanceId))
	}
	instances := make(instanceCache)
	if err := instances.update(v.env.ec2, ctx, instIds.Values()...); err != nil {
		err := maybeConvertCredentialError(err, ctx)
		logger.Debugf("querying running instances: %v", err)
		// We ignore the error, because we don't want an invalid
		// InstanceId reference from one VolumeParams to prevent
		// the creation of another volume.
		// Except if it is a credential error...
		if common.IsCredentialNotValid(err) {
			return nil, errors.Trace(err)
		}
	}

	results := make([]storage.AttachVolumesResult, len(attachParams))
	for i, params := range attachParams {
		instId := string(params.InstanceId)

		// By default we should allocate device names without the
		// trailing number. Block devices with a trailing number are
		// not liked by some applications, e.g. Ceph, which want full
		// disks.
		//
		// TODO(axw) introduce a configuration option if and when
		// someone asks for it to enable use of numbers. This option
		// must error if used with an "hvm" instance type.
		const numbers = false
		nextDeviceName := blockDeviceNamer(numbers)
		_, deviceName, err := v.attachOneVolume(ctx, nextDeviceName, params.VolumeId, instId)
		if err != nil {
			results[i].Error = maybeConvertCredentialError(err, ctx)
			continue
		}

		var attachmentInfo storage.VolumeAttachmentInfo
		// The newer hypervisor attaches EBS volumes
		// as NVMe devices, and the block device names
		// are unpredictable from here.
		//
		// Instead of using device name, we fill in
		// the device link, based on the statically
		// defined model name ("Amazon Elastic Block Store",
		// and serial (vol123456abcdef...); the serial
		// is the same as the volume ID without the "-".
		//
		// NOTE(axw) inst.Hypervisor still says "xen" for
		// affected instance types, which would seem to
		// be a lie. There's no way to tell how a volume will
		// be exposed so we have to assume an nvme link - the
		// subsequent matching code will correctly skip the link
		// and match against device name for non-nvme volumes.
		sn := strings.Replace(params.VolumeId, "-", "", 1)
		attachmentInfo.DeviceLink = nvmeDeviceLinkPrefix + sn
		attachmentInfo.DeviceName = deviceName

		results[i].VolumeAttachment = &storage.VolumeAttachment{
			params.Volume,
			params.Machine,
			attachmentInfo,
		}
	}
	return results, nil
}

func (v *ebsVolumeSource) attachOneVolume(
	ctx context.ProviderCallContext,
	nextDeviceName func() (string, string, error),
	volumeId, instId string,
) (string, string, error) {
	// Wait for the volume to move out of "creating".
	volume, err := v.waitVolumeCreated(ctx, volumeId)
	if err != nil {
		return "", "", errors.Trace(maybeConvertCredentialError(err, ctx))
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
			return "", "", errors.Errorf("volume %v has unexpected attachment count: %v", volumeId, len(attachments))
		}
		if attachments[0].InstanceId != instId {
			return "", "", errors.Errorf("volume %v is attached to %v", volumeId, attachments[0].InstanceId)
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
		_, err = v.env.ec2.AttachVolume(volumeId, instId, requestDeviceName)
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
			return "", "", errors.Annotate(maybeConvertCredentialError(err, ctx), "attaching volume")
		}
		return requestDeviceName, actualDeviceName, nil
	}
}

func (v *ebsVolumeSource) waitVolumeCreated(ctx context.ProviderCallContext, volumeId string) (*ec2.Volume, error) {
	var attempt = utils.AttemptStrategy{
		Total: 5 * time.Second,
		Delay: 200 * time.Millisecond,
	}
	var lastStatus string
	volume, err := waitVolume(v.env.ec2, ctx, volumeId, attempt, func(volume *ec2.Volume) (bool, error) {
		lastStatus = volume.Status
		return volume.Status != volumeStatusCreating, nil
	})
	if err == errWaitVolumeTimeout {
		return nil, errors.Errorf(
			"timed out waiting for volume %v to become available (%v)",
			volumeId, lastStatus,
		)
	} else if err != nil {
		return nil, errors.Trace(maybeConvertCredentialError(err, ctx))
	}
	return volume, nil
}

var errWaitVolumeTimeout = errors.New("timed out")

func waitVolume(
	client *ec2.EC2,
	ctx context.ProviderCallContext,
	volumeId string,
	attempt utils.AttemptStrategy,
	pred func(v *ec2.Volume) (bool, error),
) (*ec2.Volume, error) {
	for a := attempt.Start(); a.Next(); {
		volume, err := describeVolume(client, ctx, volumeId)
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

func describeVolume(client *ec2.EC2, ctx context.ProviderCallContext, volumeId string) (*ec2.Volume, error) {
	resp, err := client.Volumes([]string{volumeId}, nil)
	if err != nil {
		return nil, errors.Annotate(maybeConvertCredentialError(err, ctx), "querying volume")
	}
	if len(resp.Volumes) == 0 {
		return nil, errors.NotFoundf("%v", volumeId)
	} else if len(resp.Volumes) != 1 {
		return nil, errors.Errorf("expected one volume, got %d", len(resp.Volumes))
	}
	return &resp.Volumes[0], nil
}

type instanceCache map[string]ec2.Instance

func (c instanceCache) update(ec2client *ec2.EC2, ctx context.ProviderCallContext, ids ...string) error {
	if len(ids) == 1 {
		if _, ok := c[ids[0]]; ok {
			return nil
		}
	}
	filter := ec2.NewFilter()
	filter.Add("instance-state-name", "running")
	resp, err := ec2client.Instances(ids, filter)
	if err != nil {
		return errors.Annotate(maybeConvertCredentialError(err, ctx), "querying instance details")
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
func (v *ebsVolumeSource) DetachVolumes(ctx context.ProviderCallContext, attachParams []storage.VolumeAttachmentParams) ([]error, error) {
	return detachVolumes(v.env.ec2, ctx, attachParams)
}

func detachVolumes(client *ec2.EC2, ctx context.ProviderCallContext, attachParams []storage.VolumeAttachmentParams) ([]error, error) {
	results := make([]error, len(attachParams))
	for i, params := range attachParams {
		_, err := client.DetachVolume(params.VolumeId, string(params.InstanceId), "", false)
		// Process aws specific error information.
		if err != nil {
			if ec2Err, ok := err.(*ec2.Error); ok {
				switch ec2Err.Code {
				case incorrectState:
					// incorrect state means this volume is "available",
					// i.e. is not attached to any machine.
					err = nil
				case attachmentNotFound:
					// attachment not found means this volume is already detached.
					err = nil
				}
			}
		}
		if err != nil {
			results[i] = errors.Annotatef(
				maybeConvertCredentialError(err, ctx), "detaching %s from %s",
				names.ReadableString(params.Volume),
				names.ReadableString(params.Machine),
			)
		}
	}
	return results, nil
}

// ImportVolume is specified on the storage.VolumeImporter interface.
func (v *ebsVolumeSource) ImportVolume(ctx context.ProviderCallContext, volumeId string, tags map[string]string) (storage.VolumeInfo, error) {
	resp, err := v.env.ec2.Volumes([]string{volumeId}, nil)
	if err != nil {
		// TODO(axw) check for "not found" response, massage error message?
		return storage.VolumeInfo{}, maybeConvertCredentialError(err, ctx)
	}
	if len(resp.Volumes) != 1 {
		return storage.VolumeInfo{}, errors.Errorf("expected 1 volume result, got %d", len(resp.Volumes))
	}
	vol := resp.Volumes[0]
	if vol.Status != volumeStatusAvailable {
		return storage.VolumeInfo{}, errors.Errorf("cannot import volume with status %q", vol.Status)
	}
	if err := tagResources(v.env.ec2, ctx, tags, volumeId); err != nil {
		return storage.VolumeInfo{}, errors.Annotate(err, "tagging volume")
	}
	return storage.VolumeInfo{
		VolumeId:   volumeId,
		Size:       gibToMib(uint64(vol.Size)),
		Persistent: true,
	}, nil
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

func minRootDiskSizeMiB(series string) uint64 {
	return gibToMib(common.MinRootDiskSizeGiB(series))
}

// getBlockDeviceMappings translates constraints into BlockDeviceMappings.
//
// The first entry is always the root disk mapping, followed by instance
// stores (ephemeral disks).
func getBlockDeviceMappings(
	cons constraints.Value,
	series string,
	controller bool,
) []ec2.BlockDeviceMapping {
	minRootDiskSizeMiB := minRootDiskSizeMiB(series)
	rootDiskSizeMiB := minRootDiskSizeMiB
	if controller {
		rootDiskSizeMiB = defaultControllerDiskSizeMiB
	}
	if cons.RootDisk != nil {
		if *cons.RootDisk >= minRootDiskSizeMiB {
			rootDiskSizeMiB = *cons.RootDisk
		} else {
			logger.Infof(
				"Ignoring root-disk constraint of %dM because it is smaller than the minimum size %dM",
				*cons.RootDisk,
				minRootDiskSizeMiB,
			)
		}
	}
	// The first block device is for the root disk.
	blockDeviceMappings := []ec2.BlockDeviceMapping{{
		DeviceName: rootDiskDeviceName,
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

	return blockDeviceMappings
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
