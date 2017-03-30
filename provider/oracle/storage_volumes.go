package oracle

import (
	// oci "github.com/juju/go-oracle-cloud/api"
	ociResponse "github.com/juju/go-oracle-cloud/response"

	"github.com/juju/errors"

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

// CreateVolumes is specified on the storage.VolumeSource interface
// When you create a storage volume, you can specify
// the capacity that you need. The allowed range
// is from 1 GB to 2 TB, in increments of 1 GB
func (s *oracleVolumeSource) CreateVolumes(params []storage.VolumeParams) (result []storage.CreateVolumesResult, err error) {
	if params == nil {
		return []storage.CreateVolumesResult{}, nil
	}

	n := len(params)
	results := make([]storage.CreateVolumesResult, 0, n)
	for i, volume := range params {
		vol, err := s.createVolume(volume)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}
		results[i].Volume = vol
	}
	return result, nil
}

func (s *oracleVolumeSource) composeName(tag string) string {
	return s.env.namespace.Value(s.envName + "-" + tag)
}

func (s *oracleVolumeSource) createVolume(p storage.VolumeParams) (*storage.Volume, error) {
	if err := s.ValidateVolumeParams(p); err != nil {
		return nil, errors.Trace(err)
	}
	// name :=
	return nil, nil
}

// ListVolumes lists the provider volume IDs for every volume
// created by this volume source.
func (s *oracleVolumeSource) ListVolumes() ([]string, error) {
	volumes, err := s.api.AllStorageVolumes(nil)
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
		return nil, errors.Errorf("Empty slice of volIds passed")
	}

	result := make([]storage.DescribeVolumesResult, 0, n)
	if n == 1 {
		volume, err := s.api.StorageVolumeDetails(volIds[0])
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

	volumes, err := s.api.AllStorageVolumes(nil)
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
func (s *oracleVolumeSource) DestroyVolumes(volIds []string) ([]error, error) {
	return nil, nil
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
func (s *oracleVolumeSource) AttachVolumes(params []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	return nil, nil
}

// DetachVolumes detaches the volumes with the specified provider
// volume IDs from the instances with the corresponding index.
//
// TODO(axw) we need to record in state whether or not volumes
// are detachable, and reject attempts to attach/detach on
// that basis.
func (s *oracleVolumeSource) DetachVolumes(params []storage.VolumeAttachmentParams) ([]error, error) {
	return nil, nil
}
