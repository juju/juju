// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	oci "github.com/juju/go-oracle-cloud/api"
	ociCommon "github.com/juju/go-oracle-cloud/common"
	ociResponse "github.com/juju/go-oracle-cloud/response"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/storage"
)

// oracleVolumeSource implements the storage.VolumeSource interface
type oracleVolumeSource struct {
	env       *OracleEnviron
	envName   string // non-unique, informational only
	modelUUID string
	api       StorageAPI
	clock     clock.Clock
}

// newOracleVolumeSource returns a new volume source to provide an interface
// for creating, destroying, describing attaching and detaching volumes in the
// oracle cloud environment
func newOracleVolumeSource(env *OracleEnviron, name, uuid string, api StorageAPI, clock clock.Clock) (*oracleVolumeSource, error) {
	if env == nil {
		return nil, errors.NotFoundf("environ")
	}

	if api == nil {
		return nil, errors.NotFoundf("storage client")
	}

	return &oracleVolumeSource{
		env:       env,
		envName:   name,
		modelUUID: uuid,
		api:       api,
		clock:     clock,
	}, nil
}

var _ storage.VolumeSource = (*oracleVolumeSource)(nil)

// resourceName returns an oracle compatible resource name.
func (s *oracleVolumeSource) resourceName(tag string) string {
	return s.api.ComposeName(s.env.namespace.Value(s.envName + "-" + tag))
}

func (s *oracleVolumeSource) getStoragePool(attr map[string]interface{}) (ociCommon.StoragePool, error) {
	volumeType, ok := attr[oracleVolumeType]
	if !ok {
		return poolTypeMap[defaultPool], nil
	}
	switch volumeType.(type) {
	case poolType:
		if t, ok := poolTypeMap[volumeType.(poolType)]; ok {
			return t, nil
		}
		return poolTypeMap[defaultPool], errors.NotFoundf("storage pool %q not found", volumeType.(poolType))
	}
	return poolTypeMap[defaultPool], nil
}

// createVolume will create a storage volume given the storage volume parameters
// under the oracle cloud endpoint
func (s *oracleVolumeSource) createVolume(p storage.VolumeParams) (_ *storage.Volume, err error) {
	var details ociResponse.StorageVolume
	defer func() {
		// gsamfira: not really sure if this is needed. The only relevant error
		// on which we act is the one returned by the oracle API when creating
		// the volume. If the API returned an error, there is little chance, that
		// a volume was created. But for the sake of thoroughness, let's leave this
		// here
		if err != nil && details.Name != "" {
			_ = s.api.DeleteStorageVolume(details.Name)
		}
	}()

	// validate the parameters
	if err := s.ValidateVolumeParams(p); err != nil {
		return nil, errors.Trace(err)
	}
	name := s.resourceName(p.Tag.String())
	size := mibToGib(p.Size)
	if size > maxVolumeSizeInGB || size < minVolumeSizeInGB {
		return nil, errors.Errorf("invalid size for volume: %d", size)
	}

	poolType, err := s.getStoragePool(p.Attributes)
	if err != nil {
		return nil, errors.Trace(err)
	}
	volumeTags := []string{p.Tag.String()}
	for k, v := range p.ResourceTags {
		volumeTags = append(volumeTags, fmt.Sprintf("%s=%s", k, v))
	}

	params := oci.StorageVolumeParams{
		Bootable:    false,
		Description: fmt.Sprintf("Juju created volume for %q", p.Tag.String()),
		Name:        name,
		Properties: []ociCommon.StoragePool{
			poolType,
		},
		Size: ociCommon.NewStorageSize(size, ociCommon.G),
		Tags: volumeTags,
	}
	logger.Infof("creating volume: %v", params)
	details, err = s.api.CreateStorageVolume(params)
	if oci.IsStatusConflict(err) {
		// Volume already exists, so return its details.
		conflictErr := err
		details, err = s.api.StorageVolumeDetails(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		var modelTagValue string
		for _, tag := range details.Tags {
			prefix := tags.JujuModel + "="
			if !strings.HasPrefix(tag, prefix) {
				continue
			}
			modelTagValue = tag[len(prefix):]
		}
		if modelTagValue != s.modelUUID {
			return nil, errors.Trace(conflictErr)
		}
		return &storage.Volume{p.Tag, makeVolumeInfo(details)}, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	// wait for the newly created volume to reach "Online" status
	logger.Debugf("waiting for resource %v", details.Name)
	if err := s.waitForResourceStatus(
		s.fetchVolumeStatus,
		details.Name,
		string(ociCommon.VolumeOnline), 5*time.Minute); err != nil {
		return nil, errors.Trace(err)
	}
	volume := &storage.Volume{p.Tag, makeVolumeInfo(details)}
	logger.Debugf("volume details: %v", volume)
	return volume, nil
}

// CreateVolumes is specified on the storage.VolumeSource interface
func (s *oracleVolumeSource) CreateVolumes(ctx context.ProviderCallContext, params []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
	if params == nil {
		return []storage.CreateVolumesResult{}, nil
	}
	results := make([]storage.CreateVolumesResult, len(params))
	for i, volume := range params {
		vol, err := s.createVolume(volume)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}
		results[i].Volume = vol
	}
	return results, nil
}

// fetchVolumeStatus polls the status of a volume and returns true if the current status
// coincides with the desired status
func (s *oracleVolumeSource) fetchVolumeStatus(name, desiredStatus string) (complete bool, err error) {
	details, err := s.api.StorageVolumeDetails(name)
	if err != nil {
		return false, errors.Trace(err)
	}

	if details.Status == ociCommon.VolumeError {
		return false, errors.Errorf("volume entered error state: %q", details.Status_detail)
	}
	return string(details.Status) == desiredStatus, nil
}

// fetchVolumeAttachmentStatus polls the status of a volume attachment and returns true if the current status
// coincides with the desired status
func (s *oracleVolumeSource) fetchVolumeAttachmentStatus(name, desiredStatus string) (bool, error) {
	details, err := s.api.StorageAttachmentDetails(name)
	if err != nil {
		return false, errors.Trace(err)
	}
	return string(details.State) == desiredStatus, nil
}

// waitForResourceStatus will ping the resource until the fetch function returns true,
// the timeout is reached, or an error occurs.
func (o *oracleVolumeSource) waitForResourceStatus(
	fetch func(name string, desiredStatus string) (complete bool, err error),
	name, state string,
	timeout time.Duration,
) error {

	timeoutTimer := o.clock.NewTimer(timeout)
	defer timeoutTimer.Stop()

	retryTimer := o.clock.NewTimer(0)
	defer retryTimer.Stop()

	for {
		select {
		case <-retryTimer.Chan():
			done, err := fetch(name, state)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			retryTimer.Reset(2 * time.Second)
		case <-timeoutTimer.Chan():
			return errors.Errorf(
				"timed out waiting for resource %q to transition to %v",
				name, state,
			)
		}
	}
}

// ListVolumes is specified on the storage.VolumeSource interface.
func (s *oracleVolumeSource) ListVolumes(ctx context.ProviderCallContext) ([]string, error) {
	tag := fmt.Sprintf("%s=%s", tags.JujuModel, s.modelUUID)
	filter := []oci.Filter{{
		Arg:   "tags",
		Value: tag,
	}}
	volumes, err := s.api.AllStorageVolumes(filter)
	if err != nil {
		return nil, errors.Annotate(err, "listing volumes")
	}

	ids := make([]string, len(volumes.Result))
	for i, volume := range volumes.Result {
		ids[i] = volume.Name
	}

	return ids, nil
}

// DescribeVolumes is specified on the storage.VolumeSource interface.
func (s *oracleVolumeSource) DescribeVolumes(ctx context.ProviderCallContext, volIds []string) ([]storage.DescribeVolumesResult, error) {
	if volIds == nil || len(volIds) == 0 {
		return []storage.DescribeVolumesResult{}, nil
	}

	tag := fmt.Sprintf("%s=%s", tags.JujuModel, s.modelUUID)
	filter := []oci.Filter{{
		Arg:   "tags",
		Value: tag,
	}}

	result := make([]storage.DescribeVolumesResult, len(volIds), len(volIds))
	volumes, err := s.api.AllStorageVolumes(filter)
	if err != nil {
		return nil, errors.Annotatef(err, "describe volumes")
	}
	asMap := map[string]ociResponse.StorageVolume{}
	for _, val := range volumes.Result {
		asMap[val.Name] = val
	}
	for i, volume := range volIds {
		if vol, ok := asMap[volume]; ok {
			volumeInfo := makeVolumeInfo(vol)
			result[i].VolumeInfo = &volumeInfo
		} else {
			result[i].Error = errors.NotFoundf("%s", volume)
		}
	}
	return result, nil
}

func makeVolumeInfo(vol ociResponse.StorageVolume) storage.VolumeInfo {
	return storage.VolumeInfo{
		VolumeId: vol.Name,
		// Oracle returns the size of the volume
		// in bytes, VolumeInfo expects MiB.
		Size:       uint64(vol.Size) / (1024 * 1024),
		Persistent: true,
	}
}

// DestroyVolumes is specified on the storage.VolumeSource interface.
func (s *oracleVolumeSource) DestroyVolumes(ctx context.ProviderCallContext, volIds []string) ([]error, error) {
	return foreachVolume(volIds, s.api.DeleteStorageVolume), nil
}

// ReleaseVolumes is specified on the storage.VolumeSource interface.
func (s *oracleVolumeSource) ReleaseVolumes(ctx context.ProviderCallContext, volIds []string) ([]error, error) {
	releaseStorageVolume := func(volumeId string) error {
		details, err := s.api.StorageVolumeDetails(volumeId)
		if err != nil {
			return errors.Trace(err)
		}
		var newTags []string
		for _, tag := range details.Tags {
			fields := strings.Split(tag, "=")
			if len(fields) != 2 {
				newTags = append(newTags, tag)
				continue
			}
			switch fields[0] {
			case tags.JujuController, tags.JujuModel:
			default:
				newTags = append(newTags, tag)
			}
		}
		if len(newTags) == len(details.Tags) {
			return nil
		}
		details.Tags = newTags
		return errors.Trace(s.updateVolume(volumeId, details))
	}
	return foreachVolume(volIds, releaseStorageVolume), nil
}

func foreachVolume(volIds []string, f func(string) error) []error {
	results := make([]error, len(volIds))
	wg := sync.WaitGroup{}
	wg.Add(len(volIds))
	for i, val := range volIds {
		go func(volId string, idx int) {
			defer wg.Done()
			results[idx] = f(volId)
		}(val, i)
	}
	wg.Wait()
	return results
}

// ImportVolume is specified on the storage.VolumeImporter interface.
func (s *oracleVolumeSource) ImportVolume(ctx context.ProviderCallContext, volumeId string, tags map[string]string) (storage.VolumeInfo, error) {
	details, err := s.api.StorageVolumeDetails(volumeId)
	if err != nil {
		return storage.VolumeInfo{}, errors.Trace(err)
	}
	var newTags []string
	for _, tag := range details.Tags {
		fields := strings.Split(tag, "=")
		if len(fields) != 2 {
			newTags = append(newTags, tag)
			continue
		}
		key, value := fields[0], fields[1]
		if newValue, ok := tags[key]; !ok || newValue == value {
			delete(tags, key)
			newTags = append(newTags, tag)
			continue
		}
		// The tag has changed; we'll add it in the loop below.
	}
	if len(tags) != 0 {
		for key, value := range tags {
			newTags = append(newTags, fmt.Sprintf("%s=%s", key, value))
		}
		details.Tags = newTags
		if err := s.updateVolume(volumeId, details); err != nil {
			return storage.VolumeInfo{}, errors.Trace(err)
		}
	}
	return makeVolumeInfo(details), nil
}

func (s *oracleVolumeSource) updateVolume(volumeId string, details ociResponse.StorageVolume) error {
	derefString := func(s *string) string {
		if s != nil {
			return *s
		}
		return ""
	}
	_, err := s.api.UpdateStorageVolume(
		oci.StorageVolumeParams{
			Bootable:         details.Bootable,
			Description:      derefString(details.Description),
			Imagelist:        details.Imagelist,
			Imagelist_entry:  details.Imagelist_entry,
			Name:             details.Name,
			Properties:       details.Properties,
			Size:             ociCommon.StorageSize(fmt.Sprint(details.Size)),
			Snapshot:         derefString(details.Snapshot),
			Snapshot_account: details.Snapshot_account,
			Snapshot_id:      details.Snapshot_id,
			Tags:             details.Tags,
		},
		volumeId,
	)
	return errors.Annotatef(err, "updating volume %q", volumeId)
}

// ValidateVolumeParams is specified on the storage.VolumeSource interface.
func (s *oracleVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	size := mibToGib(params.Size)
	if size > maxVolumeSizeInGB || size < minVolumeSizeInGB {
		return errors.Errorf("invalid size for volume in GiB %d", size)
	}
	return nil
}

func (s *oracleVolumeSource) getStorageAttachments() (map[string][]ociResponse.StorageAttachment, error) {
	allAttachments, err := s.api.AllStorageAttachments(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	asMap := map[string][]ociResponse.StorageAttachment{}
	for _, val := range allAttachments.Result {
		hostname, err := extractInstanceIDFromMachineName(val.Instance_name)
		if err != nil {
			return nil, err
		}
		if _, ok := asMap[string(hostname)]; !ok {
			asMap[string(hostname)] = []ociResponse.StorageAttachment{
				val,
			}
		} else {
			asMap[string(hostname)] = append(asMap[string(hostname)], val)
		}
	}
	return asMap, nil
}

// AttachVolumes is specified on the storage.VolumeSource interface.
func (s *oracleVolumeSource) AttachVolumes(ctx context.ProviderCallContext, params []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	instanceIds := []instance.Id{}
	for _, val := range params {
		instanceIds = append(instanceIds, val.InstanceId)
	}
	if len(instanceIds) == 0 {
		return []storage.AttachVolumesResult{}, nil
	}
	instancesAsMap, err := s.env.getOracleInstancesAsMap(instanceIds...)
	if err != nil {
		return []storage.AttachVolumesResult{}, errors.Trace(err)
	}
	attachmentsAsMap, err := s.getStorageAttachments()
	if err != nil {
		return []storage.AttachVolumesResult{}, errors.Trace(err)
	}

	ret := make([]storage.AttachVolumesResult, len(params))

	for i, val := range params {
		instance, ok := instancesAsMap[string(val.InstanceId)]
		if !ok {
			ret[i].Error = errors.NotFoundf("instance %q was not found", string(val.InstanceId))
			continue
		}

		result, err := s.attachVolume(instance, attachmentsAsMap, val)
		if err != nil {
			ret[i].Error = errors.Trace(err)
			continue
		}
		ret[i] = result

	}
	logger.Infof("returning attachments: %v", ret)
	return ret, nil
}

// getFreeIndexNumber returns the first unused consecutive value in a sorted array of ints
// this is used to find an available index number for attaching a volume to an instance
func (s *oracleVolumeSource) getFreeIndexNumber(existing []int, max int) (int, error) {
	if len(existing) == 0 {
		return 1, nil
	}
	sort.Ints(existing)
	for i := 0; i <= len(existing)-1; i++ {
		if i+1 >= max {
			break
		}
		if i+1 == len(existing) {
			return existing[i] + 1, nil
		}
		if existing[0] > 1 {
			return existing[0] - 1, nil
		}
		diff := existing[i+1] - existing[i]
		if diff > 1 {
			return existing[i] + 1, nil
		}
	}
	return 0, errors.Errorf("no free index")
}

func (s *oracleVolumeSource) getDeviceNameForIndex(idx int) string {
	// We use an ephemeral disk when booting instances, so we get
	// the full range of 10 disks we can attach to an instance.
	// Alternatively, we can create a volume from an image and attach
	// it to the launchplan, and set it as a boot device.
	// NOTE(gsamfira): if we ever decide to boot from volume, this
	// needs to be addressed to return the proper device name
	return fmt.Sprintf("%s%s", blockDevicePrefix, string([]byte{blockDeviceStartIndex + byte(idx)}))
}

func (s *oracleVolumeSource) attachVolume(
	instance *oracleInstance,
	currentAttachments map[string][]ociResponse.StorageAttachment,
	params storage.VolumeAttachmentParams) (storage.AttachVolumesResult, error) {

	// keep track of all indexes of volumes attached to the instance
	existingIndexes := []int{}
	instanceStorage := instance.StorageAttachments()
	// append index numbers of volumes that were attached when creating the
	// launchpan. Not the case in the current implementation of the provider
	// but should this change in the future, this function will still work as
	// expected.
	// For information about attaching volumes at instance creation time, please
	// see: https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/op-launchplan--post.html
	for _, val := range instanceStorage {
		existingIndexes = append(existingIndexes, int(val.Index))
	}

	for _, val := range currentAttachments[string(instance.Id())] {
		// index numbers range from 1 to 10. Ignore 0 valued indexes
		// see: https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/op-storage-attachment--post.html
		if val.Index == 0 {
			continue
		}
		if val.Storage_volume_name == params.VolumeId && val.Instance_name == string(params.InstanceId) {
			// volume is already attached to this instance. Simply return it.
			return storage.AttachVolumesResult{
				VolumeAttachment: &storage.VolumeAttachment{
					params.Volume,
					params.Machine,
					storage.VolumeAttachmentInfo{
						DeviceName: s.getDeviceNameForIndex(int(val.Index)),
					},
				},
			}, nil
		}
		// append any indexes for volumes that were attached dynamically (after instance creation)
		existingIndexes = append(existingIndexes, int(val.Index))
	}

	logger.Infof("fetching free index. Existing: %v, Max: %v", existingIndexes, maxDevices)
	// gsamfira: fetch a free index number for this disk. There is a limit of 10 disks that can be attached to any
	// instance. The index number dictates the order in which the operating system will see the disks
	// Essentially an index for an attachment can be equated to the bus number that the disk will be made
	// available on inside the guest. This way, an index number of 1 will be (on a linux host) xvda, index 2
	// will be xvdb, and so on. One exception to this rule; if you boot an instance using an ephemeral disk
	// (which we currently do), then inside the guest, that disk will be xvda. Index 1 will be xvdb, index 2
	// will be xvdc and so on. Booting from ephemeral disks also has the added advantage that you get one
	// extra disk attachment on the instance, and it saves us the trouble of running another operation to
	// create the root disk from an image.
	idx, err := s.getFreeIndexNumber(existingIndexes, maxDevices)
	if err != nil {
		return storage.AttachVolumesResult{Error: errors.Trace(err)}, nil
	}

	p := oci.StorageAttachmentParams{
		Index:               ociCommon.Index(idx),
		Instance_name:       instance.machine.Name,
		Storage_volume_name: params.VolumeId,
	}
	details, err := s.api.CreateStorageAttachment(p)
	if err != nil {
		return storage.AttachVolumesResult{Error: errors.Trace(err)}, nil
	}
	if err := s.waitForResourceStatus(
		s.fetchVolumeAttachmentStatus,
		details.Name,
		string(ociCommon.StateAttached), 5*time.Minute); err != nil {

		currentAttachments[string(instance.Id())] = append(currentAttachments[string(instance.Id())], details)
		return storage.AttachVolumesResult{Error: errors.Trace(err)}, nil
	}
	currentAttachments[string(instance.Id())] = append(currentAttachments[string(instance.Id())], details)

	// TODO (gsamfira): make this more OS agnostic. In Windows you get disk indexes
	// instead of device names; however storage is not supported on windows instances (yet).
	result := storage.AttachVolumesResult{
		VolumeAttachment: &storage.VolumeAttachment{
			params.Volume,
			params.Machine,
			storage.VolumeAttachmentInfo{
				DeviceName: s.getDeviceNameForIndex(idx),
			},
		},
	}
	return result, nil
}

// DetachVolumes is specified on the storage.VolumeSource interface.
func (s *oracleVolumeSource) DetachVolumes(ctx context.ProviderCallContext, params []storage.VolumeAttachmentParams) ([]error, error) {
	attachAsMap, err := s.getStorageAttachments()
	if err != nil {
		return nil, errors.Trace(err)
	}
	toDelete := make([]string, len(params))
	ret := make([]error, len(params))
	for i, val := range params {
		found := false
		for _, attach := range attachAsMap[string(val.InstanceId)] {
			if val.VolumeId == attach.Storage_volume_name {
				toDelete[i] = attach.Name
				found = true
			}
		}
		if !found {
			toDelete[i] = ""
			ret[i] = errors.NotFoundf(
				"volume attachment for instance %v and volumeID %v not found",
				val.InstanceId, val.VolumeId)
		}
	}
	for i, val := range toDelete {
		if val == "" {
			continue
		}
		ret[i] = s.api.DeleteStorageAttachment(val)
	}
	return ret, nil
}
