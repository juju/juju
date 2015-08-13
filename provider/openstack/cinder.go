// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"math"
	"net/url"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/goose.v1/cinder"
	"gopkg.in/goose.v1/nova"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
)

const (
	CinderProviderType = storage.ProviderType("cinder")
	// autoAssignedMountPoint specifies the value to pass in when
	// you'd like Cinder to automatically assign a mount point.
	autoAssignedMountPoint = ""

	volumeStatusAvailable = "available"
	volumeStatusDeleting  = "deleting"
	volumeStatusError     = "error"
	volumeStatusInUse     = "in-use"
)

type cinderProvider struct {
	newStorageAdapter func(*config.Config) (openstackStorage, error)
}

var _ storage.Provider = (*cinderProvider)(nil)

var cinderAttempt = utils.AttemptStrategy{
	Total: 1 * time.Minute,
	Delay: 5 * time.Second,
}

// VolumeSource implements storage.Provider.
func (p *cinderProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	if err := p.ValidateConfig(providerConfig); err != nil {
		return nil, err
	}
	storageAdapter, err := p.newStorageAdapter(environConfig)
	if err != nil {
		return nil, err
	}
	uuid, ok := environConfig.UUID()
	if !ok {
		return nil, errors.NotFoundf("environment UUID")
	}
	source := &cinderVolumeSource{
		storageAdapter: storageAdapter,
		envName:        environConfig.Name(),
		envUUID:        uuid,
	}
	return source, nil
}

// FilesystemSource implements storage.Provider.
func (p *cinderProvider) FilesystemSource(environConfig *config.Config, providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

// Supports implements storage.Provider.
func (p *cinderProvider) Supports(kind storage.StorageKind) bool {
	switch kind {
	case storage.StorageKindBlock:
		return true
	}
	return false
}

// Scope implements storage.Provider.
func (s *cinderProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// ValidateConfig implements storage.Provider.
func (p *cinderProvider) ValidateConfig(cfg *storage.Config) error {
	// TODO(axw) 2015-05-01 #1450737
	// Reject attempts to create non-persistent volumes.
	return nil
}

// Dynamic implements storage.Provider.
func (p *cinderProvider) Dynamic() bool {
	return true
}

type cinderVolumeSource struct {
	storageAdapter openstackStorage
	envName        string // non unique, informational only
	envUUID        string
}

var _ storage.VolumeSource = (*cinderVolumeSource)(nil)

// CreateVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) CreateVolumes(args []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
	results := make([]storage.CreateVolumesResult, len(args))
	for i, arg := range args {
		volume, err := s.createVolume(arg)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}
		results[i].Volume = volume
	}
	return results, nil
}

func (s *cinderVolumeSource) createVolume(arg storage.VolumeParams) (*storage.Volume, error) {
	var metadata interface{}
	if len(arg.ResourceTags) > 0 {
		metadata = arg.ResourceTags
	}
	cinderVolume, err := s.storageAdapter.CreateVolume(cinder.CreateVolumeVolumeParams{
		// The Cinder documentation incorrectly states the
		// size parameter is in GB. It is actually GiB.
		Size: int(math.Ceil(float64(arg.Size / 1024))),
		Name: resourceName(arg.Tag, s.envName),
		// TODO(axw) use the AZ of the initially attached machine.
		AvailabilityZone: "",
		Metadata:         metadata,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// The response may (will?) come back before the volume transitions to
	// "creating", in which case it will not have a size or status. Wait for
	// the volume to transition, so we can record its actual size.
	volumeId := cinderVolume.ID
	cinderVolume, err = s.waitVolume(volumeId, func(v *cinder.Volume) (bool, error) {
		return v.Status != "", nil
	})
	if err != nil {
		if err := s.storageAdapter.DeleteVolume(volumeId); err != nil {
			logger.Warningf("destroying volume %s: %s", volumeId, err)
		}
		return nil, errors.Errorf("waiting for volume to be provisioned: %s", err)
	}
	logger.Debugf("created volume: %+v", cinderVolume)
	return &storage.Volume{arg.Tag, cinderToJujuVolumeInfo(cinderVolume)}, nil
}

// ListVolumes is specified on the storage.VolumeSource interface.
func (s *cinderVolumeSource) ListVolumes() ([]string, error) {
	cinderVolumes, err := s.storageAdapter.GetVolumesDetail()
	if err != nil {
		return nil, err
	}
	volumeIds := make([]string, 0, len(cinderVolumes))
	for _, volume := range cinderVolumes {
		envUUID, ok := volume.Metadata[tags.JujuEnv]
		if !ok || envUUID != s.envUUID {
			continue
		}
		volumeIds = append(volumeIds, cinderToJujuVolumeInfo(&volume).VolumeId)
	}
	return volumeIds, nil
}

// DescribeVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) DescribeVolumes(volumeIds []string) ([]storage.DescribeVolumesResult, error) {
	// In most cases, it is quicker to get all volumes and loop
	// locally than to make several round-trips to the provider.
	cinderVolumes, err := s.storageAdapter.GetVolumesDetail()
	if err != nil {
		return nil, errors.Trace(err)
	}
	volumesById := make(map[string]*cinder.Volume)
	for i, volume := range cinderVolumes {
		volumesById[volume.ID] = &cinderVolumes[i]
	}
	results := make([]storage.DescribeVolumesResult, len(volumeIds))
	for i, volumeId := range volumeIds {
		cinderVolume, ok := volumesById[volumeId]
		if !ok {
			results[i].Error = errors.NotFoundf("volume %q", volumeId)
			continue
		}
		info := cinderToJujuVolumeInfo(cinderVolume)
		results[i].VolumeInfo = &info
	}
	return results, nil
}

// DestroyVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) DestroyVolumes(volumeIds []string) ([]error, error) {
	var wg sync.WaitGroup
	wg.Add(len(volumeIds))
	results := make([]error, len(volumeIds))
	for i, volumeId := range volumeIds {
		go func(i int, volumeId string) {
			defer wg.Done()
			results[i] = s.destroyVolume(volumeId)
		}(i, volumeId)
	}
	wg.Wait()
	return results, nil
}

func (s *cinderVolumeSource) destroyVolume(volumeId string) error {
	logger.Debugf("destroying volume %q", volumeId)
	// Volumes must not be in-use when destroying. A volume may
	// still be in-use when the instance it is attached to is
	// in the process of being terminated.
	var issuedDetach bool
	volume, err := s.waitVolume(volumeId, func(v *cinder.Volume) (bool, error) {
		switch v.Status {
		default:
			// Not ready for deletion; keep waiting.
			return false, nil
		case volumeStatusAvailable, volumeStatusDeleting, volumeStatusError:
			return true, nil
		case volumeStatusInUse:
			// Detach below.
			break
		}
		// Volume is still attached, so detach it.
		if !issuedDetach {
			args := make([]storage.VolumeAttachmentParams, len(v.Attachments))
			for i, a := range v.Attachments {
				args[i].VolumeId = volumeId
				args[i].InstanceId = instance.Id(a.ServerId)
			}
			if len(args) > 0 {
				results, err := s.DetachVolumes(args)
				if err != nil {
					return false, errors.Trace(err)
				}
				for _, err := range results {
					if err != nil {
						return false, errors.Trace(err)
					}
				}
			}
			issuedDetach = true
		}
		return false, nil
	})
	if err != nil {
		return errors.Trace(err)
	}
	if volume.Status == volumeStatusDeleting {
		// Already being deleted, nothing to do.
		return nil
	}
	if err := s.storageAdapter.DeleteVolume(volumeId); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ValidateVolumeParams implements storage.VolumeSource.
func (s *cinderVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	return nil
}

// AttachVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) AttachVolumes(args []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	results := make([]storage.AttachVolumesResult, len(args))
	for i, arg := range args {
		attachment, err := s.attachVolume(arg)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}
		results[i].VolumeAttachment = attachment
	}
	return results, nil
}

func (s *cinderVolumeSource) attachVolume(arg storage.VolumeAttachmentParams) (*storage.VolumeAttachment, error) {
	// Check to see if the volume is already attached.
	existingAttachments, err := s.storageAdapter.ListVolumeAttachments(string(arg.InstanceId))
	if err != nil {
		return nil, err
	}
	novaAttachment := findAttachment(arg.VolumeId, existingAttachments)
	if novaAttachment == nil {
		// A volume must be "available" before it can be attached.
		if _, err := s.waitVolume(arg.VolumeId, func(v *cinder.Volume) (bool, error) {
			return v.Status == "available", nil
		}); err != nil {
			return nil, errors.Annotate(err, "waiting for volume to become available")
		}
		novaAttachment, err = s.storageAdapter.AttachVolume(
			string(arg.InstanceId),
			arg.VolumeId,
			autoAssignedMountPoint,
		)
		if err != nil {
			return nil, err
		}
	}
	return &storage.VolumeAttachment{
		arg.Volume,
		arg.Machine,
		storage.VolumeAttachmentInfo{
			DeviceName: novaAttachment.Device[len("/dev/"):],
		},
	}, nil
}

func (s *cinderVolumeSource) waitVolume(
	volumeId string,
	pred func(*cinder.Volume) (bool, error),
) (*cinder.Volume, error) {
	for a := cinderAttempt.Start(); a.Next(); {
		volume, err := s.storageAdapter.GetVolume(volumeId)
		if err != nil {
			return nil, errors.Annotate(err, "getting volume")
		}
		ok, err := pred(volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if ok {
			return volume, nil
		}
	}
	return nil, errors.New("timed out")
}

// DetachVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) DetachVolumes(args []storage.VolumeAttachmentParams) ([]error, error) {
	results := make([]error, len(args))
	for i, arg := range args {
		// Check to see if the volume is already detached.
		attachments, err := s.storageAdapter.ListVolumeAttachments(string(arg.InstanceId))
		if err != nil {
			results[i] = errors.Annotate(err, "listing volume attachments")
			continue
		}
		if err := detachVolume(
			string(arg.InstanceId),
			arg.VolumeId,
			attachments,
			s.storageAdapter,
		); err != nil {
			results[i] = errors.Annotatef(
				err, "detaching volume %s from server %s",
				arg.VolumeId, arg.InstanceId,
			)
			continue
		}
	}
	return results, nil
}

func cinderToJujuVolumeInfo(volume *cinder.Volume) storage.VolumeInfo {
	return storage.VolumeInfo{
		VolumeId:   volume.ID,
		Size:       uint64(volume.Size * 1024),
		Persistent: true,
	}
}

func detachVolume(instanceId, volumeId string, attachments []nova.VolumeAttachment, storageAdapter openstackStorage) error {
	// TODO(axw) verify whether we need to do this find step. From looking at the example
	// responses in the OpenStack docs, the "attachment ID" is always the same as the
	// volume ID. So we should just be able to issue a blind detach request, and then
	// ignore errors that indicate the volume is already detached.
	if findAttachment(volumeId, attachments) == nil {
		return nil
	}
	return storageAdapter.DetachVolume(instanceId, volumeId)
}

func findAttachment(volId string, attachments []nova.VolumeAttachment) *nova.VolumeAttachment {
	for _, attachment := range attachments {
		if attachment.VolumeId == volId {
			return &attachment
		}
	}
	return nil
}

type openstackStorage interface {
	GetVolume(volumeId string) (*cinder.Volume, error)
	GetVolumesDetail() ([]cinder.Volume, error)
	DeleteVolume(volumeId string) error
	CreateVolume(cinder.CreateVolumeVolumeParams) (*cinder.Volume, error)
	AttachVolume(serverId, volumeId, mountPoint string) (*nova.VolumeAttachment, error)
	DetachVolume(serverId, attachmentId string) error
	ListVolumeAttachments(serverId string) ([]nova.VolumeAttachment, error)
}

func newOpenstackStorageAdapter(environConfig *config.Config) (openstackStorage, error) {
	ecfg, err := providerInstance.newConfig(environConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	authClient := authClient(ecfg)
	if err := authClient.Authenticate(); err != nil {
		return nil, errors.Trace(err)
	}

	endpoint, ok := authClient.EndpointsForRegion(ecfg.region())["volume"]
	if !ok {
		return nil, errors.Errorf("volume endpoint not found for %q endpoint", ecfg.region())
	}
	endpointUrl, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.Annotate(err, "error parsing endpoint")
	}

	return &openstackStorageAdapter{
		cinderClient{cinder.Basic(endpointUrl, authClient.TenantId(), authClient.Token)},
		novaClient{nova.New(authClient)},
	}, nil
}

type openstackStorageAdapter struct {
	cinderClient
	novaClient
}

type cinderClient struct {
	*cinder.Client
}

type novaClient struct {
	*nova.Client
}

// CreateVolume is part of the openstackStorage interface.
func (ga *openstackStorageAdapter) CreateVolume(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
	resp, err := ga.cinderClient.CreateVolume(args)
	if err != nil {
		return nil, err
	}
	return &resp.Volume, nil
}

// GetVolumesDetail is part of the openstackStorage interface.
func (ga *openstackStorageAdapter) GetVolumesDetail() ([]cinder.Volume, error) {
	resp, err := ga.cinderClient.GetVolumesDetail()
	if err != nil {
		return nil, err
	}
	return resp.Volumes, nil
}

// GetVolume is part of the openstackStorage interface.
func (ga *openstackStorageAdapter) GetVolume(volumeId string) (*cinder.Volume, error) {
	resp, err := ga.cinderClient.GetVolume(volumeId)
	if err != nil {
		return nil, err
	}
	return &resp.Volume, nil
}
