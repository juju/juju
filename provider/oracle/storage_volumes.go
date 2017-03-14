package oracle

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
)

// CreateVolumes is specified on the storage.VolumeSource interface
// When you create a storage volume, you can specify
// the capacity that you need. The allowed range
// is from 1 GB to 2 TB, in increments of 1 GB
func (s storageProvider) CreateVolumes(
	params []storage.VolumeParams,
) (result []storage.CreateVolumesResult, err error) {

	n := len(params)

	if params == nil || n == 0 {
		return nil, nil
	}

	results := make([]storage.CreateVolumesResult, 0, n)
	ids := []instance.Id{}

	for i, volume := range params {
		if err := s.ValidateVolumeParams(volume); err != nil {
			results[i].Error = err
			continue
		}
		ids = append(ids, volume.Attachment.InstanceId)
	}

	if len(ids) == 0 {
		return result, nil
	}

	instances, err := s.env.getOracleInstances(ids...)
	if err != nil {
		return nil, errors.Annotatef(err, "getting oracle instances")
	}

	//TODO
	_ = instances
	_ = err

	return result, nil
}

// ListVolumes lists the provider volume IDs for every volume
// created by this volume source.
func (s storageProvider) ListVolumes() ([]string, error) {
	volumes, err := s.env.client.AllStorageVolumes(nil)
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
func (s storageProvider) DescribeVolumes(
	volIds []string,
) ([]storage.DescribeVolumesResult, error) {

	n := len(volIds)

	if volIds == nil || n == 0 {
		return nil, errors.Errorf("Empty slice of volIds passed")
	}

	result := make([]storage.DescribeVolumesResult, 0, n)
	if n == 1 {
		volume, err := s.env.client.StorageVolumeDetails(volIds[0])
		if err != nil {
			return nil, errors.Annotatef(err, "describe volumes")
		}
		v := storage.DescribeVolumesResult{
			VolumeInfo: &storage.VolumeInfo{
			// TODO(stip and extract the correct size)
			//Size: volume.Size,
			},
		}
		_ = volume
		return append(result, v), nil
	}

	volumes, err := s.env.client.AllStorageVolumes(nil)
	if err != nil {
		return nil, errors.Annotatef(err, "descrie volumes")
	}

	for _, volume := range volumes.Result {
		v := storage.DescribeVolumesResult{
			VolumeInfo: &storage.VolumeInfo{
			// TODO(stip and extract the correct size)
			// Size: volume.Size,
			},
		}
		_ = volume
		result = append(result, v)
	}

	return result, nil
}

// DestroyVolumes destroys the volumes with the specified provider
// volume IDs.
func (s storageProvider) DestroyVolumes(
	volIds []string,
) ([]error, error) {
	return nil, nil
}

// ValidateVolumeParams validates the provided volume creation
// parameters, returning an error if they are invalid.
func (s storageProvider) ValidateVolumeParams(params storage.VolumeParams) error {
	size := mibToGib(params.Size)
	if size > maxVolumeSizeInGB {
		return errors.Errorf(
			"%d Gib exceeds the maximum of %d GiB",
			size, maxVolumeSizeInGB,
		)
	}

	return nil
}

// AttachVolumes attaches volumes to machines.
//
// AttachVolumes must be idempotent; it may be called even if the
// attachment already exists, to ensure that it exists, e.g. over
// machine restarts.
//
// TODO(axw) we need to validate attachment requests prior to
// recording in state. For example, the ec2 provider must reject
// an attempt to attach a volume to an instance if they are in
// different availability zones.
func (s storageProvider) AttachVolumes(
	params []storage.VolumeAttachmentParams,
) ([]storage.AttachVolumesResult, error) {
	return nil, nil
}

// DetachVolumes detaches the volumes with the specified provider
// volume IDs from the instances with the corresponding index.
//
// TODO(axw) we need to record in state whether or not volumes
// are detachable, and reject attempts to attach/detach on
// that basis.
func (s storageProvider) DetachVolumes(
	params []storage.VolumeAttachmentParams,
) ([]error, error) {
	return nil, nil
}
