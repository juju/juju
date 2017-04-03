package oracle

import (
	"fmt"
	"sort"
	"sync"
	"time"

	oci "github.com/juju/go-oracle-cloud/api"
	ociCommon "github.com/juju/go-oracle-cloud/common"
	ociResponse "github.com/juju/go-oracle-cloud/response"

	"github.com/juju/errors"

	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
)

type oracleVolumeSource struct {
	env       *oracleEnviron
	envName   string // non-unique, informational only
	modelUUID string
	api       StorageAPI
}

var _ storage.VolumeSource = (*oracleVolumeSource)(nil)

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
	if err := s.ValidateVolumeParams(p); err != nil {
		return nil, errors.Trace(err)
	}
	name := s.resourceName(p.Tag.String())
	size := mibToGib(p.Size)

	details, err = s.api.StorageVolumeDetails(name)
	if err != nil {
		if !oci.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
	} else {
		if uint64(details.Size) != size {
			// a disk with the same name but different characteristics
			// exists on the cloud. Error out?
			return nil, errors.Errorf("found duplicate disk: %q", name)
		}
		volume := storage.Volume{
			p.Tag,
			storage.VolumeInfo{
				VolumeId:   details.Name,
				Size:       uint64(details.Size) / 1024 / 1024 / 1024,
				Persistent: true,
			},
		}
		return &volume, nil
	}

	attr := p.Attributes
	poolType, err := s.getStoragePool(attr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	tags := []string{
		p.Tag.String(),
	}
	for k, v := range p.ResourceTags {
		tags = append(tags, fmt.Sprintf("%s=%s", k, v))
	}
	if size > maxVolumeSizeInGB || size < minVolumeSizeInGB {
		return nil, errors.Errorf("invalid size for volume: %d", size)
	}
	params := oci.StorageVolumeParams{
		Bootable:    false,
		Description: fmt.Sprintf("Juju created volume for %q", p.Tag.String()),
		Name:        name,
		Properties: []ociCommon.StoragePool{
			poolType,
		},
		Size: ociCommon.NewStorageSize(size, ociCommon.G),
		Tags: tags,
	}
	logger.Infof("creating volume: %v", params)
	details, err = s.api.CreateStorageVolume(params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Infof("waiting for resource %v", details.Name)
	// TODO: add sane constant with default timeout
	if err := s.waitForResourceStatus(
		s.fetchVolumeStatus,
		string(details.Name),
		string(ociCommon.VolumeOnline), 5*time.Minute); err != nil {
		return nil, errors.Trace(err)
	}
	volume := storage.Volume{
		p.Tag,
		storage.VolumeInfo{
			VolumeId:   details.Name,
			Size:       uint64(details.Size) / 1024 / 1024 / 1024,
			Persistent: true,
		},
	}
	logger.Infof("returning volume details: %v", volume)
	return &volume, nil
}

// CreateVolumes is specified on the storage.VolumeSource interface
// When you create a storage volume, you can specify
// the capacity that you need. The allowed range
// is from 1 GB to 2 TB, in increments of 1 GB
func (s *oracleVolumeSource) CreateVolumes(params []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
	if params == nil {
		return []storage.CreateVolumesResult{}, nil
	}

	n := len(params)
	results := make([]storage.CreateVolumesResult, n)
	for i, volume := range params {
		logger.Infof("running createVolume for %v", volume)
		vol, err := s.createVolume(volume)
		logger.Infof("got result: %v -> %v", vol, err)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}
		results[i].Volume = vol
	}
	return results, nil
}

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

func (s *oracleVolumeSource) fetchVolumeAttachmentStatus(name, desiredStatus string) (bool, error) {
	details, err := s.api.StorageAttachmentDetails(name)
	if err != nil {
		return false, errors.Trace(err)
	}
	return string(details.State) == desiredStatus, nil
}

// waitForMachineStatus will ping the volume until the timeout duration is reached or an error appeared
func (o *oracleVolumeSource) waitForResourceStatus(
	fetch func(name string, desiredStatus string) (complete bool, err error),
	name, state string, timeout time.Duration) error {

	// chan user for errors
	errChan := make(chan error)
	// chan used for timeout
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-done:
				return
			default:
				done, err := fetch(name, state)
				if err != nil {
					errChan <- err
					return
				}
				if done {
					errChan <- nil
					return
				}
				time.Sleep(2 * time.Second)
			}
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-time.After(timeout):
		done <- true
		return errors.Errorf(
			"timed out waiting for resource %q to transition to %v",
			name, state,
		)
	}

	return nil
}

// ListVolumes lists the provider volume IDs for every volume
// created by this volume source.
func (s *oracleVolumeSource) ListVolumes() ([]string, error) {
	tag := fmt.Sprintf("%s=%s", tags.JujuModel, s.modelUUID)
	filter := []oci.Filter{
		oci.Filter{
			Arg:   "tags",
			Value: tag,
		},
	}
	volumes, err := s.api.AllStorageVolumes(filter)
	if err != nil {
		return nil, errors.Annotate(err, "listing volumes")
	}

	ids := make([]string, 0, len(volumes.Result))
	for _, volume := range volumes.Result {
		ids = append(ids, volume.Name)
	}

	return ids, nil
}

// DescribeVolumes returns the properties of the volumes with the
// specified provider volume IDs.
func (s *oracleVolumeSource) DescribeVolumes(volIds []string) ([]storage.DescribeVolumesResult, error) {
	n := len(volIds)
	if volIds == nil || n == 0 {
		return []storage.DescribeVolumesResult{}, nil
	}

	tag := fmt.Sprintf("%s=%s", tags.JujuModel, s.modelUUID)
	filter := []oci.Filter{
		oci.Filter{
			Arg:   "tags",
			Value: tag,
		},
	}

	result := make([]storage.DescribeVolumesResult, 0, n)
	volumes, err := s.api.AllStorageVolumes(filter)
	if err != nil {
		return nil, errors.Annotatef(err, "descrie volumes")
	}
	asMap := map[string]ociResponse.StorageVolume{}
	for _, val := range volumes.Result {
		asMap[val.Name] = val
	}
	for i, volume := range volIds {
		if vol, ok := asMap[volume]; ok {
			volumeInfo := &storage.VolumeInfo{
				VolumeId:   vol.Name,
				Size:       uint64(vol.Size) / 1024 / 1024 / 1024,
				Persistent: true,
			}
			v := storage.DescribeVolumesResult{
				VolumeInfo: volumeInfo,
			}
			result[i] = v
		} else {
			result[i].Error = errors.NotFoundf("%s", volume)
		}

	}

	return result, nil
}

// DestroyVolumes destroys the volumes with the specified provider
// volume IDs.
func (s *oracleVolumeSource) DestroyVolumes(volIds []string) ([]error, error) {
	results := make([]error, len(volIds))
	wg := sync.WaitGroup{}
	wg.Add(len(volIds))
	for i, val := range volIds {
		go func(volId string) {
			err := s.api.DeleteStorageVolume(volId)
			results[i] = err
			wg.Done()
		}(val)
	}
	wg.Wait()
	return results, nil
}

// ValidateVolumeParams validates the provided volume creation
// parameters, returning an error if they are invalid.
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
		if _, ok := asMap[val.Instance_name]; !ok {
			asMap[val.Instance_name] = []ociResponse.StorageAttachment{
				val,
			}
		} else {
			asMap[val.Instance_name] = append(asMap[val.Instance_name], val)
		}
	}
	return asMap, nil
}

// AttachVolumes attaches volumes to machines.
//
// AttachVolumes must be idempotent; it may be called even if the
// attachment already exists, to ensure that it exists, e.g. over
// machine restarts.
func (s *oracleVolumeSource) AttachVolumes(params []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
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
	// start from 97. xvda will always be the root disk
	// we use an ephemeral disk when booting instances, so we get
	// the full range of 10 disks we can attach to an instance.
	// Alternatively, we can create a volume from an image and attach
	// it to the launchplan, and set it as a boot device.
	return fmt.Sprintf("%s%s", blockDevicePrefix, string([]byte{97 + byte(idx)}))
}

func (s *oracleVolumeSource) attachVolume(
	instance *oracleInstance,
	currentAttachments map[string][]ociResponse.StorageAttachment,
	params storage.VolumeAttachmentParams) (storage.AttachVolumesResult, error) {

	existingIndexes := []int{}
	instanceStorage := instance.StorageAttachments()
	for _, val := range instanceStorage {
		existingIndexes = append(existingIndexes, int(val.Index))
	}

	for _, val := range currentAttachments[string(instance.Id())] {
		if val.Index == 0 {
			continue
		}
		if val.Storage_volume_name == string(params.VolumeId) && val.Instance_name == string(params.InstanceId) {
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
		existingIndexes = append(existingIndexes, int(val.Index))
	}

	logger.Infof("fetching free index. Existing: %v, Max: %v", existingIndexes, maxDevices)
	idx, err := s.getFreeIndexNumber(existingIndexes, maxDevices)
	if err != nil {
		return storage.AttachVolumesResult{Error: errors.Trace(err)}, nil
	}
	p := oci.StorageAttachmentParams{
		Index:               ociCommon.Index(idx),
		Instance_name:       string(instance.Id()),
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
	// however storage is not supported on windows instances (yet).
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

// DetachVolumes detaches the volumes with the specified provider
// volume IDs from the instances with the corresponding index.
func (s *oracleVolumeSource) DetachVolumes(params []storage.VolumeAttachmentParams) ([]error, error) {
	attachAsMap, err := s.getStorageAttachments()
	if err != nil {
		return nil, errors.Trace(err)
	}
	toDelete := make([]string, len(params))
	ret := make([]error, len(params))
	for i, val := range params {
		found := false
		for _, attach := range attachAsMap[string(val.InstanceId)] {
			if string(val.VolumeId) == attach.Storage_volume_name {
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
