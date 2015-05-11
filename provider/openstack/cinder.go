// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"math"
	"net/url"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"gopkg.in/goose.v1/cinder"
	"gopkg.in/goose.v1/nova"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"
)

const (
	CinderProviderType = storage.ProviderType("cinder")
	// autoAssignedMountPoint specifies the value to pass in when
	// you'd like Cinder to automatically assign a mount point.
	autoAssignedMountPoint = ""
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
	return &cinderVolumeSource{storageAdapter}, nil
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
}

var _ storage.VolumeSource = (*cinderVolumeSource)(nil)

// CreateVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) CreateVolumes(args []storage.VolumeParams) (_ []storage.Volume, _ []storage.VolumeAttachment, resultErr error) {
	volumes := make([]storage.Volume, len(args))
	for i, arg := range args {
		volume, err := s.createVolume(arg)
		if err != nil {
			return nil, nil, err
		}
		volumes[i] = volume

		// If the method exits with an error, be sure to delete any
		// created volumes so that we're idempotent. We create several
		// clousures instead of one to take advantage of the loop
		// parameter.
		defer func(arg storage.VolumeParams, volume storage.Volume) {
			if resultErr == nil {
				return
			}
			attachments, err := s.storageAdapter.ListVolumeAttachments(string(arg.Attachment.InstanceId))
			if err != nil {
				logger.Warningf("could not list volumes while cleaning up: %v", err)
				return
			}
			if err := detachVolume(
				string(arg.Attachment.InstanceId),
				volume.VolumeId,
				attachments,
				s.storageAdapter,
			); err != nil {
				logger.Warningf("could not detach volumes while cleaning up: %v", err)
				return
			}
			if err := s.storageAdapter.DeleteVolume(volume.VolumeId); err != nil {
				logger.Warningf("could not delete volumes while cleaning up: %v", err)
			}
		}(arg, volume)
	}

	attachmentParams := make([]storage.VolumeAttachmentParams, len(volumes))
	for i, volume := range volumes {
		attachmentParams[i] = *args[i].Attachment
		attachmentParams[i].VolumeId = volume.VolumeId
		attachmentParams[i].Volume = volume.Tag
	}
	attachments, err := s.AttachVolumes(attachmentParams)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	return volumes, attachments, nil
}

func (s *cinderVolumeSource) createVolume(arg storage.VolumeParams) (storage.Volume, error) {
	cinderVolume, err := s.storageAdapter.CreateVolume(cinder.CreateVolumeVolumeParams{
		// The Cinder documentation incorrectly states the
		// size parameter is in GB. It is actually GiB.
		Size: int(math.Ceil(float64(arg.Size / 1024))),
		Name: arg.Tag.String(),
		// TODO(axw) use the AZ of the initially attached machine.
		AvailabilityZone: "",
	})
	if err != nil {
		return storage.Volume{}, errors.Trace(err)
	}

	// The response may (will?) come back before the volume transitions to,
	// "creating", in which case it will not have a size or status. Wait for
	// the volume to transition, so we can record its actual size.
	cinderVolume, err = s.waitVolume(cinderVolume.ID, func(v *cinder.Volume) (bool, error) {
		return v.Status != "", nil
	})
	if err != nil {
		if err := s.DestroyVolumes([]string{cinderVolume.ID}); err != nil {
			logger.Warningf("destroying volume %s: %s", cinderVolume.ID, err)
		}
		return storage.Volume{}, errors.Errorf("waiting for volume to be provisioned: %s", err)
	}
	logger.Debugf("created volume: %+v", cinderVolume)
	return cinderToJujuVolume(arg.Tag, cinderVolume), nil
}

// DescribeVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) DescribeVolumes(volumeIds []string) ([]storage.Volume, error) {
	// In most cases, it is quicker to get all volumes and loop
	// locally than to make several round-trips to the provider.
	cinderVolumes, err := s.storageAdapter.GetVolumesSimple()
	if err != nil {
		return nil, err
	}
	volumesById := make(map[string]*cinder.Volume)
	for i, volume := range cinderVolumes {
		volumesById[volume.ID] = &cinderVolumes[i]
	}
	volumes := make([]storage.Volume, len(volumeIds))
	for i, volumeId := range volumeIds {
		cinderVolume, ok := volumesById[volumeId]
		if !ok {
			return nil, errors.NotFoundf("volume %q", volumeId)
		}
		volumes[i] = cinderToJujuVolume(names.VolumeTag{}, cinderVolume)
	}
	return volumes, nil
}

// DestroyVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) DestroyVolumes(volumeIds []string) []error {
	errors := make([]error, len(volumeIds))
	for i, volumeId := range volumeIds {
		if err := s.storageAdapter.DeleteVolume(volumeId); err != nil {
			errors[i] = err
		}
	}
	return errors
}

// ValidateVolumeParams implements storage.VolumeSource.
func (s *cinderVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	// TODO(axw) this should move to the storageprovisioner.
	if params.Attachment == nil || params.Attachment.InstanceId == "" {
		return storage.ErrVolumeNeedsInstance
	}
	return nil
}

// AttachVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) AttachVolumes(args []storage.VolumeAttachmentParams) ([]storage.VolumeAttachment, error) {
	attachments := make([]storage.VolumeAttachment, len(args))
	for i, arg := range args {
		attachment, err := s.attachVolume(arg)
		if err != nil {
			return nil, errors.Trace(err)
		}
		attachments[i] = attachment
	}
	return attachments, nil
}

func (s *cinderVolumeSource) attachVolume(arg storage.VolumeAttachmentParams) (storage.VolumeAttachment, error) {
	// Check to see if the volume is already attached.
	existingAttachments, err := s.storageAdapter.ListVolumeAttachments(string(arg.InstanceId))
	if err != nil {
		return storage.VolumeAttachment{}, err
	}
	novaAttachment := findAttachment(arg.VolumeId, existingAttachments)
	if novaAttachment == nil {
		// A volume must be "available" before it can be attached.
		if _, err := s.waitVolume(arg.VolumeId, func(v *cinder.Volume) (bool, error) {
			return v.Status == "available", nil
		}); err != nil {
			return storage.VolumeAttachment{}, errors.Annotate(err, "waiting for volume to become available")
		}
		novaAttachment, err = s.storageAdapter.AttachVolume(
			string(arg.InstanceId),
			arg.VolumeId,
			autoAssignedMountPoint,
		)
		if err != nil {
			return storage.VolumeAttachment{}, err
		}
	}
	return storage.VolumeAttachment{
		Machine:    arg.Machine,
		Volume:     arg.Volume,
		DeviceName: novaAttachment.Device[len("/dev/"):],
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
func (s *cinderVolumeSource) DetachVolumes(args []storage.VolumeAttachmentParams) error {
	for _, arg := range args {
		// Check to see if the volume is already detached.
		attachments, err := s.storageAdapter.ListVolumeAttachments(string(arg.InstanceId))
		if err != nil {
			return err
		}
		if err := detachVolume(
			string(arg.InstanceId),
			arg.VolumeId,
			attachments,
			s.storageAdapter,
		); err != nil {
			return err
		}
	}
	return nil
}

func cinderToJujuVolume(tag names.VolumeTag, volume *cinder.Volume) storage.Volume {
	return storage.Volume{
		VolumeId: volume.ID,
		Size:     uint64(volume.Size * 1024),
		Tag:      tag,
		// TODO(axw) there is currently no way to mark a volume as
		// "delete on termination", so all volumes are persistent.
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
	GetVolumesSimple() ([]cinder.Volume, error)
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

	endpoint := authClient.EndpointsForRegion(ecfg.region())["volume"]
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

// GetVolumesSimple is part of the openstackStorage interface.
func (ga *openstackStorageAdapter) GetVolumesSimple() ([]cinder.Volume, error) {
	resp, err := ga.cinderClient.GetVolumesSimple()
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
