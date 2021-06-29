// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"math"
	"net/url"
	"sync"
	"time"

	"github.com/go-goose/goose/v4/cinder"
	gooseerrors "github.com/go-goose/goose/v4/errors"
	"github.com/go-goose/goose/v4/identity"
	"github.com/go-goose/goose/v4/nova"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils/v2"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
)

const (
	CinderProviderType = storage.ProviderType("cinder")

	cinderVolumeType = "volume-type"

	// autoAssignedMountPoint specifies the value to pass in when
	// you'd like Cinder to automatically assign a mount point.
	autoAssignedMountPoint = ""

	volumeStatusAvailable = "available"
	volumeStatusDeleting  = "deleting"
	volumeStatusError     = "error"
	volumeStatusInUse     = "in-use"
)

var cinderConfigFields = schema.Fields{
	cinderVolumeType: schema.String(),
}

var cinderConfigChecker = schema.FieldMap(
	cinderConfigFields,
	schema.Defaults{
		cinderVolumeType: schema.Omit,
	},
)

type cinderConfig struct {
	volumeType string
}

func newCinderConfig(attrs map[string]interface{}) (*cinderConfig, error) {
	out, err := cinderConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating Cinder storage config")
	}
	coerced := out.(map[string]interface{})
	volumeType, _ := coerced[cinderVolumeType].(string)
	cinderConfig := &cinderConfig{
		volumeType: volumeType,
	}
	return cinderConfig, nil
}

// StorageProviderTypes implements storage.ProviderRegistry.
func (e *Environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	var types []storage.ProviderType
	if _, err := e.cinderProvider(); err == nil {
		types = append(types, CinderProviderType)
	} else if !errors.IsNotSupported(err) {
		return nil, errors.Trace(err)
	}
	return types, nil
}

// StorageProvider implements storage.ProviderRegistry.
func (e *Environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	if t != CinderProviderType {
		return nil, errors.NotFoundf("storage provider %q", t)
	}
	return e.cinderProvider()
}

func (e *Environ) cinderProvider() (*cinderProvider, error) {
	storageAdapter, err := newOpenstackStorage(e)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &cinderProvider{
		storageAdapter: storageAdapter,
		envName:        e.name,
		modelUUID:      e.uuid,
		namespace:      e.namespace,
		zonedEnv:       e,
	}, nil
}

var newOpenstackStorage = func(env *Environ) (OpenstackStorage, error) {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()

	client := env.clientUnlocked
	if env.volumeURL == nil {
		url, err := getVolumeEndpointURL(client, env.cloudUnlocked.Region)
		if IsNotFoundError(err) {
			// No volume endpoint found; Cinder is not supported.
			return nil, errors.NotSupportedf("volumes")
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		env.volumeURL = url
		logger.Debugf("volume URL: %v", url)
	}

	// TODO (stickupkid): Move this to the ClientFactory.
	// We shouldn't have another wrapper around an existing client.
	cinderCl := cinderClient{cinder.Basic(env.volumeURL, client.TenantId(), client.Token)}

	cloudSpec := env.cloudUnlocked
	if len(cloudSpec.CACertificates) > 0 {
		cinderCl = cinderClient{cinder.BasicTLSConfig(
			env.volumeURL,
			client.TenantId(),
			client.Token,
			tlsConfig(cloudSpec.CACertificates)),
		}
	}

	return &openstackStorageAdapter{
		cinderCl,
		novaClient{env.novaUnlocked},
	}, nil
}

type cinderProvider struct {
	storageAdapter OpenstackStorage
	envName        string
	modelUUID      string
	namespace      instance.Namespace
	zonedEnv       common.ZonedEnviron
}

var _ storage.Provider = (*cinderProvider)(nil)

var cinderAttempt = utils.AttemptStrategy{
	Total: 1 * time.Minute,
	Delay: 5 * time.Second,
}

// VolumeSource implements storage.Provider.
func (p *cinderProvider) VolumeSource(providerConfig *storage.Config) (storage.VolumeSource, error) {
	if err := p.ValidateConfig(providerConfig); err != nil {
		return nil, err
	}
	source := &cinderVolumeSource{
		storageAdapter: p.storageAdapter,
		envName:        p.envName,
		modelUUID:      p.modelUUID,
		namespace:      p.namespace,
		zonedEnv:       p.zonedEnv,
	}
	return source, nil
}

// FilesystemSource implements storage.Provider.
func (p *cinderProvider) FilesystemSource(providerConfig *storage.Config) (storage.FilesystemSource, error) {
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
	_, err := newCinderConfig(cfg.Attrs())
	return errors.Trace(err)
}

// Dynamic implements storage.Provider.
func (p *cinderProvider) Dynamic() bool {
	return true
}

// Releasable is defined on the Provider interface.
func (*cinderProvider) Releasable() bool {
	return true
}

// DefaultPools implements storage.Provider.
func (p *cinderProvider) DefaultPools() []*storage.Config {
	return nil
}

type cinderVolumeSource struct {
	storageAdapter OpenstackStorage
	envName        string // non unique, informational only
	modelUUID      string
	namespace      instance.Namespace
	zonedEnv       common.ZonedEnviron
}

var _ storage.VolumeSource = (*cinderVolumeSource)(nil)

// CreateVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) CreateVolumes(
	ctx context.ProviderCallContext, args []storage.VolumeParams,
) ([]storage.CreateVolumesResult, error) {
	results := make([]storage.CreateVolumesResult, len(args))
	for i, arg := range args {
		volume, err := s.createVolume(ctx, arg)
		if err != nil {
			results[i].Error = errors.Trace(err)
			if denied := common.MaybeHandleCredentialError(IsAuthorisationFailure, err, ctx); denied {
				// If it is an unauthorised error, no need to continue since we will 100% fail...
				break
			}
			continue
		}
		results[i].Volume = volume
	}
	return results, nil
}

func (s *cinderVolumeSource) createVolume(
	ctx context.ProviderCallContext, arg storage.VolumeParams) (*storage.Volume, error) {
	cinderConfig, err := newCinderConfig(arg.Attributes)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var metadata interface{}
	if len(arg.ResourceTags) > 0 {
		metadata = arg.ResourceTags
	}

	az, err := s.availabilityZoneForVolume(ctx, arg.Tag.Id(), arg.Attachment)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cinderVolume, err := s.storageAdapter.CreateVolume(cinder.CreateVolumeVolumeParams{
		// The Cinder documentation incorrectly states the
		// size parameter is in GB. It is actually GiB.
		Size:             int(math.Ceil(float64(arg.Size / 1024))),
		Name:             resourceName(s.namespace, s.envName, arg.Tag.String()),
		VolumeType:       cinderConfig.volumeType,
		AvailabilityZone: az,
		Metadata:         metadata,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// The response may (will?) come back before the volume transitions to
	// "creating", in which case it will not have a size or status.
	// Wait for the volume to transition, so we can record its actual size.
	volumeId := cinderVolume.ID
	cinderVolume, err = waitVolume(s.storageAdapter, volumeId, func(v *cinder.Volume) (bool, error) {
		return v.Status != "", nil
	})
	if err != nil {
		if err := s.storageAdapter.DeleteVolume(volumeId); err != nil {
			logger.Warningf("destroying volume %s: %s", volumeId, err)
		}
		return nil, errors.Errorf("waiting for volume to be provisioned: %s", err)
	}
	logger.Debugf("created volume: %+v", cinderVolume)
	return &storage.Volume{Tag: arg.Tag, VolumeInfo: cinderToJujuVolumeInfo(cinderVolume)}, nil
}

func (s *cinderVolumeSource) availabilityZoneForVolume(
	ctx context.ProviderCallContext, volName string, attachment *storage.VolumeAttachmentParams,
) (string, error) {
	// If this volume is being attached to an instance, attempt to provision
	// the storage in the same availability zone.
	// This helps to avoid a situation with all storage residing in a single
	// AZ that upon failure would effectively take down attached instances
	// whatever zone they were in.
	// However, we first attempt to query the possible volume availability zones.
	// If the API is old and does not support explicit volume AZs, or volumes can
	// only be provisioned in say the default "nova" zone and the instance is in
	// a different zone, we won't attempt to use the instance zone because that
	// won't work and we'll get a 400 error back.
	if attachment == nil || attachment.InstanceId == "" {
		return "", nil
	}

	volumeZones, err := s.storageAdapter.ListVolumeAvailabilityZones()
	if err != nil && !gooseerrors.IsNotImplemented(err) {
		logger.Infof("block volume zones not supported, not using availability zone for volume %q", volName)
		return "", errors.Trace(err)
	}
	vZones := set.NewStrings()
	for _, vz := range volumeZones {
		if vz.State.Available {
			vZones.Add(vz.Name)
		}
	}
	if vZones.Size() == 0 {
		logger.Infof("no block volume zones defined, not using availability zone for volume %q", volName)
		return "", nil
	}
	logger.Debugf("possible block volume zones: %v", vZones.SortedValues())
	aZones, err := s.zonedEnv.InstanceAvailabilityZoneNames(ctx, []instance.Id{attachment.InstanceId})
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(aZones) == 0 {
		// All instances should have an availability zone.
		// The default is "nova" so something is wrong if nothing
		// is returned from this call.
		logger.Warningf("no availability zone detected for instance %q", attachment.InstanceId)
		return "", nil
	}
	// Only choose an AZ from the instance if there's a matching volume AZ.
	var az string
	for _, az = range aZones {
		break
	}
	if vZones.Contains(az) {
		logger.Debugf("using availability zone %q to create cinder volume %q", az, volName)
		return az, nil
	}
	logger.Warningf("no compatible availability zone detected for volume %q", volName)
	return "", nil
}

// ListVolumes is specified on the storage.VolumeSource interface.
func (s *cinderVolumeSource) ListVolumes(ctx context.ProviderCallContext) ([]string, error) {
	cinderVolumes, err := modelCinderVolumes(s.storageAdapter, s.modelUUID)
	if err != nil {
		handleCredentialError(err, ctx)
		return nil, errors.Trace(err)
	}
	return volumeInfoToVolumeIds(cinderToJujuVolumeInfos(cinderVolumes)), nil
}

// modelCinderVolumes returns all of the cinder volumes for the model.
func modelCinderVolumes(storageAdapter OpenstackStorage, modelUUID string) ([]cinder.Volume, error) {
	return cinderVolumes(storageAdapter, func(v *cinder.Volume) bool {
		return v.Metadata[tags.JujuModel] == modelUUID
	})
}

// controllerCinderVolumes returns all of the cinder volumes for the model.
func controllerCinderVolumes(storageAdapter OpenstackStorage, controllerUUID string) ([]cinder.Volume, error) {
	return cinderVolumes(storageAdapter, func(v *cinder.Volume) bool {
		return v.Metadata[tags.JujuController] == controllerUUID
	})
}

// cinderVolumes returns all of the cinder volumes matching the given predicate.
func cinderVolumes(storageAdapter OpenstackStorage, pred func(*cinder.Volume) bool) ([]cinder.Volume, error) {
	allCinderVolumes, err := storageAdapter.GetVolumesDetail()
	if err != nil {
		return nil, err
	}
	var matching []cinder.Volume
	for _, v := range allCinderVolumes {
		if pred(&v) {
			matching = append(matching, v)
		}
	}
	return matching, nil
}

func volumeInfoToVolumeIds(volumes []storage.VolumeInfo) []string {
	volumeIds := make([]string, len(volumes))
	for i, volume := range volumes {
		volumeIds[i] = volume.VolumeId
	}
	return volumeIds
}

// DescribeVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) DescribeVolumes(ctx context.ProviderCallContext, volumeIds []string) ([]storage.DescribeVolumesResult, error) {
	// In most cases, it is quicker to get all volumes and loop
	// locally than to make several round-trips to the provider.
	cinderVolumes, err := s.storageAdapter.GetVolumesDetail()
	if err != nil {
		handleCredentialError(err, ctx)
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
func (s *cinderVolumeSource) DestroyVolumes(ctx context.ProviderCallContext, volumeIds []string) ([]error, error) {
	return foreachVolume(ctx, s.storageAdapter, volumeIds, destroyVolume), nil
}

// ReleaseVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) ReleaseVolumes(ctx context.ProviderCallContext, volumeIds []string) ([]error, error) {
	return foreachVolume(ctx, s.storageAdapter, volumeIds, releaseVolume), nil
}

func foreachVolume(ctx context.ProviderCallContext, storageAdapter OpenstackStorage, volumeIds []string, f func(context.ProviderCallContext, OpenstackStorage, string) error) []error {
	var wg sync.WaitGroup
	wg.Add(len(volumeIds))
	results := make([]error, len(volumeIds))
	for i, volumeId := range volumeIds {
		go func(i int, volumeId string) {
			defer wg.Done()
			results[i] = f(ctx, storageAdapter, volumeId)
		}(i, volumeId)
	}
	wg.Wait()
	return results
}

func destroyVolume(ctx context.ProviderCallContext, storageAdapter OpenstackStorage, volumeId string) error {
	logger.Debugf("destroying volume %q", volumeId)
	// Volumes must not be in-use when destroying. A volume may
	// still be in-use when the instance it is attached to is
	// in the process of being terminated.
	var issuedDetach bool
	volume, err := waitVolume(storageAdapter, volumeId, func(v *cinder.Volume) (bool, error) {
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
				results := detachVolumes(ctx, storageAdapter, args)
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
		if IsNotFoundError(err) {
			// The volume wasn't found; nothing
			// to destroy, so we're done.
			return nil
		}
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	if volume.Status == volumeStatusDeleting {
		// Already being deleted, nothing to do.
		return nil
	}
	if err := storageAdapter.DeleteVolume(volumeId); err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	return nil
}

func releaseVolume(ctx context.ProviderCallContext, storageAdapter OpenstackStorage, volumeId string) error {
	logger.Debugf("releasing volume %q", volumeId)
	_, err := waitVolume(storageAdapter, volumeId, func(v *cinder.Volume) (bool, error) {
		switch v.Status {
		case volumeStatusAvailable, volumeStatusError:
			return true, nil
		case volumeStatusDeleting:
			return false, errors.New("volume is being deleted")
		case volumeStatusInUse:
			return false, errors.New("volume still in-use")
		}
		// Not ready for releasing; keep waiting.
		return false, nil
	})
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Annotatef(err, "cannot release volume %q", volumeId)
	}
	// Drop the model and controller tags from the volume.
	tags := map[string]string{
		tags.JujuModel:      "",
		tags.JujuController: "",
	}
	_, err = storageAdapter.SetVolumeMetadata(volumeId, tags)
	handleCredentialError(err, ctx)
	return errors.Annotate(err, "tagging volume")
}

// ValidateVolumeParams implements storage.VolumeSource.
func (s *cinderVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	_, err := newCinderConfig(params.Attributes)
	return errors.Trace(err)
}

// AttachVolumes implements storage.VolumeSource.
func (s *cinderVolumeSource) AttachVolumes(ctx context.ProviderCallContext, args []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	results := make([]storage.AttachVolumesResult, len(args))
	for i, arg := range args {
		attachment, err := s.attachVolume(arg)
		if err != nil {
			results[i].Error = errors.Trace(err)
			if denial := common.MaybeHandleCredentialError(IsAuthorisationFailure, err, ctx); denial {
				// We do not want to continue here as we'll 100% fail if we got unauthorised error.
				break
			}
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
		if _, err := waitVolume(s.storageAdapter, arg.VolumeId, func(v *cinder.Volume) (bool, error) {
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
	if novaAttachment.Device == nil {
		return nil, errors.Errorf("device not assigned to volume attachment")
	}
	return &storage.VolumeAttachment{
		Volume:  arg.Volume,
		Machine: arg.Machine,
		VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
			DeviceName: (*novaAttachment.Device)[len("/dev/"):],
		},
	}, nil
}

// ImportVolume is part of the storage.VolumeImporter interface.
func (s *cinderVolumeSource) ImportVolume(ctx context.ProviderCallContext, volumeId string, resourceTags map[string]string) (storage.VolumeInfo, error) {
	volume, err := s.storageAdapter.GetVolume(volumeId)
	if err != nil {
		handleCredentialError(err, ctx)
		return storage.VolumeInfo{}, errors.Annotate(err, "getting volume")
	}
	if volume.Status != volumeStatusAvailable {
		return storage.VolumeInfo{}, errors.Errorf(
			"cannot import volume %q with status %q", volumeId, volume.Status,
		)
	}
	if _, err := s.storageAdapter.SetVolumeMetadata(volumeId, resourceTags); err != nil {
		handleCredentialError(err, ctx)
		return storage.VolumeInfo{}, errors.Annotatef(err, "tagging volume %q", volumeId)
	}
	return cinderToJujuVolumeInfo(volume), nil
}

func waitVolume(
	storageAdapter OpenstackStorage,
	volumeId string,
	pred func(*cinder.Volume) (bool, error),
) (*cinder.Volume, error) {
	for a := cinderAttempt.Start(); a.Next(); {
		volume, err := storageAdapter.GetVolume(volumeId)
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
func (s *cinderVolumeSource) DetachVolumes(ctx context.ProviderCallContext, args []storage.VolumeAttachmentParams) ([]error, error) {
	return detachVolumes(ctx, s.storageAdapter, args), nil
}

func detachVolumes(ctx context.ProviderCallContext, storageAdapter OpenstackStorage, args []storage.VolumeAttachmentParams) []error {
	results := make([]error, len(args))
	for i, arg := range args {
		if err := detachVolume(
			string(arg.InstanceId),
			arg.VolumeId,
			storageAdapter,
		); err != nil {
			handleCredentialError(err, ctx)
			results[i] = errors.Annotatef(
				err, "detaching volume %s from server %s",
				arg.VolumeId, arg.InstanceId,
			)
			continue
		}
	}
	return results
}

func cinderToJujuVolumeInfos(volumes []cinder.Volume) []storage.VolumeInfo {
	out := make([]storage.VolumeInfo, len(volumes))
	for i, v := range volumes {
		out[i] = cinderToJujuVolumeInfo(&v)
	}
	return out
}

func cinderToJujuVolumeInfo(volume *cinder.Volume) storage.VolumeInfo {
	return storage.VolumeInfo{
		VolumeId:   volume.ID,
		Size:       uint64(volume.Size * 1024),
		Persistent: true,
	}
}

func detachVolume(instanceId, volumeId string, storageAdapter OpenstackStorage) error {
	err := storageAdapter.DetachVolume(instanceId, volumeId)
	if err != nil && !IsNotFoundError(err) {
		return errors.Trace(err)
	}
	// The volume was successfully detached, or was
	// already detached (i.e. NotFound error case).
	return nil
}

func IsNotFoundError(err error) bool {
	return errors.IsNotFound(err) || gooseerrors.IsNotFound(err)
}

func findAttachment(volId string, attachments []nova.VolumeAttachment) *nova.VolumeAttachment {
	for _, attachment := range attachments {
		if attachment.VolumeId == volId {
			return &attachment
		}
	}
	return nil
}

type OpenstackStorage interface {
	GetVolume(volumeId string) (*cinder.Volume, error)
	GetVolumesDetail() ([]cinder.Volume, error)
	DeleteVolume(volumeId string) error
	CreateVolume(cinder.CreateVolumeVolumeParams) (*cinder.Volume, error)
	AttachVolume(serverId, volumeId, mountPoint string) (*nova.VolumeAttachment, error)
	DetachVolume(serverId, attachmentId string) error
	ListVolumeAttachments(serverId string) ([]nova.VolumeAttachment, error)
	SetVolumeMetadata(volumeId string, metadata map[string]string) (map[string]string, error)
	ListVolumeAvailabilityZones() ([]cinder.AvailabilityZone, error)
}

type endpointResolver interface {
	Authenticate() error
	IsAuthenticated() bool
	EndpointsForRegion(region string) identity.ServiceURLs
}

func getVolumeEndpointURL(client endpointResolver, region string) (*url.URL, error) {
	if !client.IsAuthenticated() {
		if err := authenticateClient(client); err != nil {
			return nil, errors.Trace(err)
		}
	}
	endpointMap := client.EndpointsForRegion(region)

	// Different versions of block storage in OpenStack have different entries
	// in the service catalog.  Find the most recent version.  If it does exist,
	// fall back to older ones.
	endpoint, ok := endpointMap["volumev3"]
	if ok {
		return url.Parse(endpoint)
	}
	logger.Debugf(`endpoint "volumev3" not found for %q region, trying "volumev2"`, region)
	endpoint, ok = endpointMap["volumev2"]
	if ok {
		return url.Parse(endpoint)
	}
	logger.Debugf(`endpoint "volumev2" not found for %q region, trying "volume"`, region)
	endpoint, ok = endpointMap["volume"]
	if ok {
		return url.Parse(endpoint)
	}
	return nil, errors.NotFoundf(`endpoint "volume" in region %q`, region)
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

// CreateVolume is part of the OpenstackStorage interface.
func (ga *openstackStorageAdapter) CreateVolume(args cinder.CreateVolumeVolumeParams) (*cinder.Volume, error) {
	resp, err := ga.cinderClient.CreateVolume(args)
	if err != nil {
		return nil, err
	}
	return &resp.Volume, nil
}

// GetVolumesDetail is part of the OpenstackStorage interface.
func (ga *openstackStorageAdapter) GetVolumesDetail() ([]cinder.Volume, error) {
	resp, err := ga.cinderClient.GetVolumesDetail()
	if err != nil {
		return nil, err
	}
	return resp.Volumes, nil
}

// GetVolume is part of the OpenstackStorage interface.
func (ga *openstackStorageAdapter) GetVolume(volumeId string) (*cinder.Volume, error) {
	resp, err := ga.cinderClient.GetVolume(volumeId)
	if err != nil {
		if IsNotFoundError(err) {
			return nil, errors.NotFoundf("volume %q", volumeId)
		}
		return nil, err
	}
	return &resp.Volume, nil
}

// SetVolumeMetadata is part of the OpenstackStorage interface.
func (ga *openstackStorageAdapter) SetVolumeMetadata(volumeId string, metadata map[string]string) (map[string]string, error) {
	return ga.cinderClient.SetVolumeMetadata(volumeId, metadata)
}

// DeleteVolume is part of the OpenstackStorage interface.
func (ga *openstackStorageAdapter) DeleteVolume(volumeId string) error {
	if err := ga.cinderClient.DeleteVolume(volumeId); err != nil {
		if IsNotFoundError(err) {
			return errors.NotFoundf("volume %q", volumeId)
		}
		return err
	}
	return nil
}

// DetachVolume is part of the OpenstackStorage interface.
func (ga *openstackStorageAdapter) DetachVolume(serverId, attachmentId string) error {
	if err := ga.novaClient.DetachVolume(serverId, attachmentId); err != nil {
		if IsNotFoundError(err) {
			return errors.NewNotFound(nil,
				fmt.Sprintf("volume %q is not attached to server %q",
					attachmentId, serverId,
				),
			)
		}
		return err
	}
	return nil
}
