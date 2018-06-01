// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"
	"strings"
	"sync"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/storage"
)

const (
	storageProviderType = storage.ProviderType("gce")
)

// StorageProviderTypes implements storage.ProviderRegistry.
func (env *environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{storageProviderType}, nil
}

// StorageProvider implements storage.ProviderRegistry.
func (env *environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	if t == storageProviderType {
		return &storageProvider{env}, nil
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}

type storageProvider struct {
	env *environ
}

var _ storage.Provider = (*storageProvider)(nil)

func (g *storageProvider) ValidateConfig(cfg *storage.Config) error {
	return nil
}

func (g *storageProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindBlock
}

func (g *storageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

func (g *storageProvider) Dynamic() bool {
	return true
}

func (e *storageProvider) Releasable() bool {
	return true
}

func (g *storageProvider) DefaultPools() []*storage.Config {
	// TODO(perrito666) Add explicit pools.
	return nil
}

func (g *storageProvider) FilesystemSource(providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

type volumeSource struct {
	gce       gceConnection
	envName   string // non-unique, informational only
	modelUUID string
}

func (g *storageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	environConfig := g.env.Config()
	source := &volumeSource{
		gce:       g.env.gce,
		envName:   environConfig.Name(),
		modelUUID: environConfig.UUID(),
	}
	return source, nil
}

type instanceCache map[string]google.Instance

func (c instanceCache) update(gceClient gceConnection, ids ...string) error {
	if len(ids) == 1 {
		if _, ok := c[ids[0]]; ok {
			return nil
		}
	}
	idMap := make(map[string]int, len(ids))
	for _, id := range ids {
		idMap[id] = 0
	}
	instances, err := gceClient.Instances("", google.StatusRunning)
	if err != nil {
		return errors.Annotate(err, "querying instance details")
	}
	for _, instance := range instances {
		if _, ok := idMap[instance.ID]; !ok {
			continue
		}
		c[instance.ID] = instance
	}
	return nil
}

func (c instanceCache) get(id string) (google.Instance, error) {
	inst, ok := c[id]
	if !ok {
		return google.Instance{}, errors.Errorf("cannot attach to non-running instance %v", id)
	}
	return inst, nil
}

func (v *volumeSource) CreateVolumes(params []storage.VolumeParams) (_ []storage.CreateVolumesResult, err error) {
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
		if err := instances.update(v.gce, instanceIds.Values()...); err != nil {
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
		volume, attachment, err := v.createOneVolume(p, instances)
		if err != nil {
			results[i].Error = err
			logger.Errorf("could not create one volume (or attach it): %v", err)
			continue
		}
		results[i].Volume = volume
		results[i].VolumeAttachment = attachment
	}
	return results, nil
}

// mibToGib converts mebibytes to gibibytes.
// GCE expects GiB, we work in MiB; round up
// to nearest GiB.
func mibToGib(m uint64) uint64 {
	return (m + 1023) / 1024
}

func nameVolume(zone string) (string, error) {
	volumeUUID, err := utils.NewUUID()
	if err != nil {
		return "", errors.Annotate(err, "cannot generate uuid to name the volume")
	}
	// type-zone-uuid
	volumeName := fmt.Sprintf("%s--%s", zone, volumeUUID.String())
	return volumeName, nil
}

func (v *volumeSource) createOneVolume(p storage.VolumeParams, instances instanceCache) (volume *storage.Volume, volumeAttachment *storage.VolumeAttachment, err error) {
	var volumeName, zone string
	defer func() {
		if err == nil || volumeName == "" {
			return
		}
		if err := v.gce.RemoveDisk(zone, volumeName); err != nil {
			logger.Errorf("error cleaning up volume %v: %v", volumeName, err)
		}
	}()

	instId := string(p.Attachment.InstanceId)
	if err := instances.update(v.gce, instId); err != nil {
		return nil, nil, errors.Annotatef(err, "cannot add %q to instance cache", instId)
	}
	inst, err := instances.get(instId)
	if err != nil {
		// Can't create the volume without the instance,
		// because we need to know what its AZ is.
		return nil, nil, errors.Annotatef(err, "cannot obtain %q from instance cache", instId)
	}
	persistentType, ok := p.Attributes["type"].(google.DiskType)
	if !ok {
		persistentType = google.DiskPersistentStandard
	}

	zone = inst.ZoneName
	volumeName, err = nameVolume(zone)
	if err != nil {
		return nil, nil, errors.Annotate(err, "cannot create a new volume name")
	}
	// TODO(perrito666) the volumeName is arbitrary and it was crafted this
	// way to help solve the need to have zone all over the place.
	disk := google.DiskSpec{
		SizeHintGB:         mibToGib(p.Size),
		Name:               volumeName,
		PersistentDiskType: persistentType,
		Labels:             resourceTagsToDiskLabels(p.ResourceTags),
	}

	gceDisks, err := v.gce.CreateDisks(zone, []google.DiskSpec{disk})
	if err != nil {
		return nil, nil, errors.Annotate(err, "cannot create disk")
	}
	if len(gceDisks) != 1 {
		return nil, nil, errors.New(fmt.Sprintf("unexpected number of disks created: %d", len(gceDisks)))
	}
	gceDisk := gceDisks[0]

	attachedDisk, err := v.attachOneVolume(gceDisk.Name, google.ModeRW, inst.ID)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "attaching %q to %q", gceDisk.Name, instId)
	}

	volume = &storage.Volume{
		p.Tag,
		storage.VolumeInfo{
			VolumeId:   gceDisk.Name,
			Size:       gceDisk.Size,
			Persistent: true,
		},
	}

	volumeAttachment = &storage.VolumeAttachment{
		p.Tag,
		p.Attachment.Machine,
		storage.VolumeAttachmentInfo{
			DeviceLink: fmt.Sprintf(
				"/dev/disk/by-id/google-%s",
				attachedDisk.DeviceName,
			),
		},
	}

	return volume, volumeAttachment, nil
}

func (v *volumeSource) DestroyVolumes(volNames []string) ([]error, error) {
	return v.foreachVolume(volNames, v.destroyOneVolume), nil
}

func (v *volumeSource) ReleaseVolumes(volNames []string) ([]error, error) {
	return v.foreachVolume(volNames, v.releaseOneVolume), nil
}

func (v *volumeSource) foreachVolume(volNames []string, f func(string) error) []error {
	var wg sync.WaitGroup
	wg.Add(len(volNames))
	results := make([]error, len(volNames))
	for i, volumeName := range volNames {
		go func(i int, volumeName string) {
			defer wg.Done()
			results[i] = f(volumeName)
		}(i, volumeName)
	}
	wg.Wait()
	return results
}

func parseVolumeId(volName string) (string, string, error) {
	idRest := strings.SplitN(volName, "--", 2)
	if len(idRest) != 2 {
		return "", "", errors.New(fmt.Sprintf("malformed volume id %q", volName))
	}
	zone := idRest[0]
	volumeUUID := idRest[1]
	return zone, volumeUUID, nil
}

func isValidVolume(volumeName string) bool {
	_, _, err := parseVolumeId(volumeName)
	return err == nil
}

func (v *volumeSource) destroyOneVolume(volName string) error {
	zone, _, err := parseVolumeId(volName)
	if err != nil {
		return errors.Annotatef(err, "invalid volume id %q", volName)
	}
	if err := v.gce.RemoveDisk(zone, volName); err != nil {
		return errors.Annotatef(err, "cannot destroy volume %q", volName)
	}
	return nil
}

func (v *volumeSource) releaseOneVolume(volName string) error {
	zone, _, err := parseVolumeId(volName)
	if err != nil {
		return errors.Annotatef(err, "invalid volume id %q", volName)
	}
	disk, err := v.gce.Disk(zone, volName)
	if err != nil {
		return errors.Trace(err)
	}
	switch disk.Status {
	case google.StatusReady, google.StatusFailed:
	default:
		return errors.Errorf(
			"cannot release volume %q with status %q",
			volName, disk.Status,
		)
	}
	if len(disk.AttachedInstances) > 0 {
		return errors.Errorf(
			"cannot release volume %q, attached to instances %q",
			volName, disk.AttachedInstances,
		)
	}
	delete(disk.Labels, tags.JujuController)
	delete(disk.Labels, tags.JujuModel)
	if err := v.gce.SetDiskLabels(zone, volName, disk.LabelFingerprint, disk.Labels); err != nil {
		return errors.Annotatef(err, "cannot remove labels from volume %q", volName)
	}
	return nil
}

func (v *volumeSource) ListVolumes() ([]string, error) {
	var volumes []string
	disks, err := v.gce.Disks()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, disk := range disks {
		if !isValidVolume(disk.Name) {
			continue
		}
		if disk.Labels[tags.JujuModel] != v.modelUUID {
			continue
		}
		volumes = append(volumes, disk.Name)
	}
	return volumes, nil
}

// ImportVolume is specified on the storage.VolumeImporter interface.
func (v *volumeSource) ImportVolume(volName string, tags map[string]string) (storage.VolumeInfo, error) {
	zone, _, err := parseVolumeId(volName)
	if err != nil {
		return storage.VolumeInfo{}, errors.Annotatef(err, "cannot get volume %q", volName)
	}
	disk, err := v.gce.Disk(zone, volName)
	if err != nil {
		return storage.VolumeInfo{}, errors.Annotatef(err, "cannot get volume %q", volName)
	}
	if disk.Status != google.StatusReady {
		return storage.VolumeInfo{}, errors.Errorf(
			"cannot import volume %q with status %q",
			volName, disk.Status,
		)
	}
	if disk.Labels == nil {
		disk.Labels = make(map[string]string)
	}
	for k, v := range resourceTagsToDiskLabels(tags) {
		disk.Labels[k] = v
	}
	if err := v.gce.SetDiskLabels(zone, volName, disk.LabelFingerprint, disk.Labels); err != nil {
		return storage.VolumeInfo{}, errors.Annotatef(err, "cannot update labels on volume %q", volName)
	}
	return storage.VolumeInfo{
		VolumeId:   disk.Name,
		Size:       disk.Size,
		Persistent: true,
	}, nil
}

func (v *volumeSource) DescribeVolumes(volNames []string) ([]storage.DescribeVolumesResult, error) {
	results := make([]storage.DescribeVolumesResult, len(volNames))
	for i, vol := range volNames {
		res, err := v.describeOneVolume(vol)
		if err != nil {
			return nil, errors.Annotate(err, "cannot describe volumes")
		}
		results[i] = res
	}
	return results, nil
}

func (v *volumeSource) describeOneVolume(volName string) (storage.DescribeVolumesResult, error) {
	zone, _, err := parseVolumeId(volName)
	if err != nil {
		return storage.DescribeVolumesResult{}, errors.Annotatef(err, "cannot describe %q", volName)
	}
	disk, err := v.gce.Disk(zone, volName)
	if err != nil {
		return storage.DescribeVolumesResult{}, errors.Annotatef(err, "cannot get volume %q", volName)
	}
	desc := storage.DescribeVolumesResult{
		&storage.VolumeInfo{
			Size:     disk.Size,
			VolumeId: disk.Name,
		},
		nil,
	}
	return desc, nil
}

// TODO(perrito666) These rules are yet to be defined.
func (v *volumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	return nil
}

func (v *volumeSource) AttachVolumes(attachParams []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	results := make([]storage.AttachVolumesResult, len(attachParams))
	for i, attachment := range attachParams {
		volumeName := attachment.VolumeId
		mode := google.ModeRW
		if attachment.ReadOnly {
			mode = google.ModeRW
		}
		instanceId := attachment.InstanceId
		attached, err := v.attachOneVolume(volumeName, mode, string(instanceId))
		if err != nil {
			logger.Errorf("could not attach %q to %q: %v", volumeName, instanceId, err)
			results[i].Error = err
			continue
		}
		results[i].VolumeAttachment = &storage.VolumeAttachment{
			attachment.Volume,
			attachment.Machine,
			storage.VolumeAttachmentInfo{
				DeviceLink: fmt.Sprintf(
					"/dev/disk/by-id/google-%s",
					attached.DeviceName,
				),
			},
		}
	}
	return results, nil
}

func (v *volumeSource) attachOneVolume(volumeName string, mode google.DiskMode, instanceId string) (*google.AttachedDisk, error) {
	zone, _, err := parseVolumeId(volumeName)
	if err != nil {
		return nil, errors.Annotate(err, "invalid volume name")
	}
	instanceDisks, err := v.gce.InstanceDisks(zone, instanceId)
	if err != nil {
		return nil, errors.Annotate(err, "cannot verify if the disk is already in the instance")
	}
	// Is it already attached?
	for _, disk := range instanceDisks {
		if disk.VolumeName == volumeName {
			return disk, nil
		}
	}

	attachment, err := v.gce.AttachDisk(zone, volumeName, instanceId, mode)
	if err != nil {
		return nil, errors.Annotate(err, "cannot attach volume")
	}
	return attachment, nil
}

func (v *volumeSource) DetachVolumes(attachParams []storage.VolumeAttachmentParams) ([]error, error) {
	result := make([]error, len(attachParams))
	for i, volumeAttachment := range attachParams {
		result[i] = v.detachOneVolume(volumeAttachment)
	}
	return result, nil
}

func (v *volumeSource) detachOneVolume(attachParam storage.VolumeAttachmentParams) error {
	instId := attachParam.InstanceId
	volumeName := attachParam.VolumeId
	zone, _, err := parseVolumeId(volumeName)
	if err != nil {
		return errors.Annotatef(err, "%q is not a valid volume id", volumeName)
	}
	return v.gce.DetachDisk(zone, string(instId), volumeName)
}

// resourceTagsToDiskLabels translates a set of
// resource tags, provided by Juju, to disk labels.
func resourceTagsToDiskLabels(in map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range in {
		// Only the controller and model UUID tags are carried
		// over, as they're known not to conflict with GCE's
		// rules regarding label values.
		switch k {
		case tags.JujuController, tags.JujuModel:
			out[k] = v
		}
	}
	return out
}
