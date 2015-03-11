// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"math"
	"net/url"
	"time"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/storage"

	"github.com/juju/errors"
	"github.com/juju/names"

	"gopkg.in/goose.v1/cinder"
	"gopkg.in/goose.v1/client"
	"gopkg.in/goose.v1/identity"
	"gopkg.in/goose.v1/nova"
)

const (
	CinderProviderType = storage.ProviderType("cinder")
	// autoAssignedMountPoint specifies the value to pass in when
	// you'd like Cinder to automatically assign a mount point.
	autoAssignedMountPoint = ""
)

type OpenstackProvider struct {
	ClientFactory func(*storage.Config) (OpenstackAdapter, error)
}

var _ storage.Provider = (*OpenstackProvider)(nil)

// VolumeSource implements storage.Provider.
func (p *OpenstackProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {

	if err := p.ValidateConfig(providerConfig); err != nil {
		return nil, err
	}

	openstackClient, err := p.ClientFactory(providerConfig)
	if err != nil {
		return nil, err
	}

	return &openstackVolumeSource{openstackClient}, nil
}

// FilesystemSource implements storage.Provider.
func (p *OpenstackProvider) FilesystemSource(environConfig *config.Config, providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

// Supports implements storage.Provider.
func (p *OpenstackProvider) Supports(kind storage.StorageKind) bool {
	switch kind {
	case storage.StorageKindBlock:
		return true
	}
	return false
}

// Scope implements storage.Provider.
func (s *OpenstackProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// ValidateConfig implements storage.Provider.
func (p *OpenstackProvider) ValidateConfig(cfg *storage.Config) error {

	for _, cfgKey := range []string{
		openstackCfgKeyUrl,
		openstackCfgKeyUser,
		openstackCfgKeySecrets,
		openstackCfgKeyRegion,
		openstackCfgKeyTenantName} {
		if _, ok := cfg.Attrs()[cfgKey]; !ok {
			return errors.NotAssignedf("requisite configuration was not set: %v", cfgKey)
		}
	}

	return nil
}

// Dynamic implements storage.Provider.
func (p *OpenstackProvider) Dynamic() bool {
	return true
}

type openstackVolumeSource struct {
	openstackClient OpenstackAdapter
}

var _ storage.VolumeSource = (*openstackVolumeSource)(nil)

// CreateVolumes implements storage.VolumeSource.
func (s *openstackVolumeSource) CreateVolumes(args []storage.VolumeParams) (vols []storage.Volume, volAtt []storage.VolumeAttachment, returnErr error) {

	vols = make([]storage.Volume, 0, len(args))
	volAtt = make([]storage.VolumeAttachment, 0, len(args))

	for _, a := range args {

		createdVol, err := s.openstackClient.CreateVolume(a.Size, a.Tag)
		if err != nil {
			return nil, nil, err
		}
		vols = append(vols, createdVol)

		// If the method exits with an error, be sure to delete any
		// created volumes so that we're idempotent. We create several
		// clousures instead of one to take advantage of the loop
		// parameter.
		defer func(arg storage.VolumeParams, createdVol storage.Volume) {
			if returnErr == nil {
				return
			}

			if arg.Attachment != nil {
				volAttachments, err := s.openstackClient.ListVolumeAttachments(string(arg.Attachment.InstanceId))
				if err != nil {
					logger.Warningf("could not list volumes while cleaning up: %v", err)
				}

				if err := detachVolume(
					string(arg.Attachment.InstanceId),
					arg.Attachment.VolumeId,
					volAttachments,
					s.openstackClient,
				); err != nil {
					logger.Warningf("could not detach volumes while cleaning up: %v", err)
				}
			}

			if err := s.openstackClient.DeleteVolume(createdVol.VolumeId); err != nil {
				logger.Warningf("could not delete volumes while cleaning up: %v", err)
			}
		}(a, createdVol)

		if a.Attachment != nil {

			a.Attachment.VolumeId = createdVol.VolumeId

			attachments, err := s.AttachVolumes([]storage.VolumeAttachmentParams{*a.Attachment})
			if err != nil {
				return nil, nil, err
			}

			volAtt = append(volAtt, attachments...)
		}
	}

	return vols, volAtt, nil
}

// DescribeVolumes implements storage.VolumeSource.
func (s *openstackVolumeSource) DescribeVolumes(volIds []string) ([]storage.Volume, error) {

	// In most cases, it is quicker to get all volumes and loop
	// locally than to make several round-trips to the provider.
	cinderVols, err := s.openstackClient.GetVolumesSimple()
	if err != nil {
		return nil, err
	}

	// Map volume ID to volume info for faster lookup.
	idToCinderVol := make(map[string]storage.Volume)
	for _, volInfo := range cinderVols {
		idToCinderVol[volInfo.VolumeId] = volInfo
	}

	volumes := make([]storage.Volume, len(volIds))
	for volIdx, volId := range volIds {
		cinderVol, ok := idToCinderVol[volId]
		if !ok {
			return nil, errors.Errorf("could not find volume: %s", volId)
		}
		volumes[volIdx] = cinderVol
	}

	return volumes, nil
}

// DestroyVolumes implements storage.VolumeSource.
func (s *openstackVolumeSource) DestroyVolumes(volIds []string) []error {

	errors := make([]error, len(volIds))
	for volIdx, volId := range volIds {
		if err := s.openstackClient.DeleteVolume(volId); err != nil {
			errors[volIdx] = err
		}
	}
	return errors
}

// ValidateVolumeParams implements storage.VolumeSource.
func (s *openstackVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	return nil
}

// AttachVolumes implements storage.VolumeSource.
func (s *openstackVolumeSource) AttachVolumes(args []storage.VolumeAttachmentParams) ([]storage.VolumeAttachment, error) {

	volAttachments := make([]storage.VolumeAttachment, len(args))
	for argIdx, attachArg := range args {

		// Check to see if the volume is already attached.
		volAttachments, err := s.openstackClient.ListVolumeAttachments(string(attachArg.InstanceId))
		if err != nil {
			return nil, err
		}
		if findAttachment(attachArg.VolumeId, volAttachments) != nil {
			continue
		}

		// A volume's status must be available before it can be
		// attached.
		const numAttempts = 10
		const retryInterval = 5 * time.Second
		if err := <-s.openstackClient.VolumeStatusNotifier(
			attachArg.VolumeId,
			"available",
			numAttempts,
			retryInterval,
		); err != nil {
			return nil, err
		}

		attachment, err := s.openstackClient.AttachVolume(
			string(attachArg.InstanceId),
			attachArg.VolumeId,
			autoAssignedMountPoint,
		)
		if err != nil {
			return nil, err
		}

		volAttachments[argIdx] = attachment
	}

	return volAttachments, nil
}

// DetachVolumes implements storage.VolumeSource.
func (s *openstackVolumeSource) DetachVolumes(args []storage.VolumeAttachmentParams) error {

	for _, attachArg := range args {
		// Check to see if the volume is already detached.
		volAttachments, err := s.openstackClient.ListVolumeAttachments(string(attachArg.InstanceId))
		if err != nil {
			return err
		}

		if err := detachVolume(
			string(attachArg.InstanceId),
			attachArg.VolumeId,
			volAttachments,
			s.openstackClient,
		); err != nil {
			return err
		}
	}

	return nil
}

type OpenstackAdapter interface {
	GetVolumesSimple() ([]storage.Volume, error)
	DeleteVolume(string) error
	CreateVolume(size uint64, tag names.VolumeTag) (storage.Volume, error)
	AttachVolume(serverId, volumeId, mountPoint string) (storage.VolumeAttachment, error)
	VolumeStatusNotifier(volId, status string, numAttempts int, waitDur time.Duration) <-chan error
	DetachVolume(serverId, attachmentId string) error
	ListVolumeAttachments(serverId string) ([]storage.VolumeAttachment, error)
}

func NewGooseAdapter(cfg *storage.Config) (OpenstackAdapter, error) {

	region, _ := cfg.ValueString("region")

	authClient := client.NewClient(&identity.Credentials{
		URL:        cfg.Attrs()[openstackCfgKeyUrl].(string),
		User:       cfg.Attrs()[openstackCfgKeyUser].(string),
		Secrets:    cfg.Attrs()[openstackCfgKeySecrets].(string),
		Region:     region,
		TenantName: cfg.Attrs()[openstackCfgKeyTenantName].(string),
	}, identity.AuthUserPass, nil)

	endpoint := authClient.EndpointsForRegion(region)["volume"]
	endpointUrl, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.Annotate(err, "error parsing endpoint")
	}
	cinderClient := cinder.Basic(endpointUrl, authClient.TenantId(), authClient.Token)

	novaClient := nova.New(authClient)

	return &gooseAdapter{cinderClient, novaClient}, nil
}

type gooseAdapter struct {
	cinder *cinder.Client
	nova   *nova.Client
}

func (ga *gooseAdapter) CreateVolume(size uint64, tag names.VolumeTag) (storage.Volume, error) {

	resp, err := ga.cinder.CreateVolume(cinder.CreateVolumeVolumeParams{
		// NOTE: The Cinder documentation incorrectly states the
		// size parameter is in GB. It is actually GiB.
		Size: int(math.Ceil(float64(size / 1024))),
		Name: tag.String(),
	})
	if err != nil {
		return storage.Volume{}, err
	}
	return cinderToJujuVolume(tag, &resp.Volume), err
}

func (ga *gooseAdapter) DeleteVolume(volId string) error {
	return ga.cinder.DeleteVolume(volId)
}

func (ga *gooseAdapter) GetVolumesSimple() ([]storage.Volume, error) {

	resp, err := ga.cinder.GetVolumesSimple()
	if err != nil {
		return nil, err
	}

	vols := make([]storage.Volume, len(resp.Volumes))
	for idx, vol := range resp.Volumes {
		vols[idx] = cinderToJujuVolume(names.VolumeTag{}, &vol)
	}

	return vols, nil
}

func (ga *gooseAdapter) AttachVolume(serverId, volumeId, mountPoint string) (storage.VolumeAttachment, error) {
	attachment, err := ga.nova.AttachVolume(serverId, volumeId, mountPoint)
	if err != nil {
		return storage.VolumeAttachment{}, err
	}
	return novaToJujuVolumeAttachment(attachment), nil
}

func (ga *gooseAdapter) VolumeStatusNotifier(volId, status string, numAttempts int, waitDur time.Duration) <-chan error {
	return ga.cinder.VolumeStatusNotifier(volId, status, numAttempts, waitDur)
}

func (ga *gooseAdapter) ListVolumeAttachments(serverId string) ([]storage.VolumeAttachment, error) {
	novaVolAttachments, err := ga.nova.ListVolumeAttachments(serverId)
	if err != nil {
		return nil, err
	}

	volumes := make([]storage.VolumeAttachment, len(novaVolAttachments))
	for volIdx, volAttach := range novaVolAttachments {
		volumes[volIdx] = novaToJujuVolumeAttachment(&volAttach)
	}

	return volumes, nil
}

func (ga *gooseAdapter) DetachVolume(serverId, attachmentId string) error {
	return ga.nova.DetachVolume(serverId, attachmentId)
}

const (
	openstackCfgKeyUrl        = "auth-url"
	openstackCfgKeyUser       = "user"
	openstackCfgKeySecrets    = "secrets"
	openstackCfgKeyRegion     = "region-name"
	openstackCfgKeyTenantName = "tenant-name"
)

func NewOpenstackStorageConfig(url, user, secrets, region, tenantName string) (*storage.Config, error) {

	return storage.NewConfig(
		"cinder",
		CinderProviderType,
		map[string]interface{}{
			openstackCfgKeyUrl:        url,
			openstackCfgKeyUser:       user,
			openstackCfgKeySecrets:    secrets,
			openstackCfgKeyRegion:     region,
			openstackCfgKeyTenantName: tenantName,
		},
	)
}

func cinderToJujuVolume(tag names.VolumeTag, volume *cinder.Volume) storage.Volume {
	var size uint64
	size = uint64(int64(volume.Size * 1024))
	return storage.Volume{
		VolumeId: volume.ID,
		Size:     size,
		Tag:      tag,
	}
}

func novaToJujuVolumeAttachment(volAttach *nova.VolumeAttachment) storage.VolumeAttachment {
	return storage.VolumeAttachment{
		Volume:     names.NewVolumeTag(volAttach.VolumeId),
		Machine:    names.NewMachineTag(volAttach.ServerId),
		DeviceName: volAttach.Device,
	}
}

func detachVolume(instanceId, volId string, volAttachments []storage.VolumeAttachment, openstackClient OpenstackAdapter) error {

	attachment := findAttachment(volId, volAttachments)
	if attachment == nil {
		return nil
	}

	return openstackClient.DetachVolume(instanceId, attachment.Volume.String())
}

func findAttachment(volId string, volAttachments []storage.VolumeAttachment) *storage.VolumeAttachment {
	for _, volAttach := range volAttachments {
		if volAttach.Volume.String() == volId {
			return &volAttach
		}
	}
	return nil
}
