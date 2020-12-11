// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs

import (
	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/caas/ecs/constants"
	jujucontext "github.com/juju/juju/environs/context"
	jujustorage "github.com/juju/juju/storage"
)

const (

	// EBSVolumeTypeKey is the config key for volume type.
	EBSVolumeTypeKey = "volume-type"
	// EBSDriverKey is the config key for volume provision driver.
	EBSDriverKey = "driver"

	// EBSVolumeTypeValueMagnetic is the volume type of "standard" for Magnetic volumes.
	EBSVolumeTypeValueMagnetic = "magnetic"
	// EBSVolumeTypeValueSSD is the volume type of "gp2" for General Purpose (SSD) volumes.
	EBSVolumeTypeValueSSD = "gp2"

	// EBSDriverValueRexray is the Juju opinionated storage plugin driver for ECS.
	EBSDriverValueRexray = "rexray/ebs" // Fix: should we opinion on this or NOT??
)

var ebsConfigFields = schema.Fields{
	EBSVolumeTypeKey: schema.OneOf(
		schema.Const(EBSVolumeTypeValueMagnetic),
		schema.Const(EBSVolumeTypeValueSSD),
	),
	EBSDriverKey: schema.String(),
}

var ebsConfigChecker = schema.FieldMap(
	ebsConfigFields,
	schema.Defaults{
		EBSVolumeTypeKey: EBSVolumeTypeValueSSD,
		EBSDriverKey:     EBSDriverValueRexray,
	},
)

type ebsConfig struct {
	volumeType string
	driver     string
}

func newEbsConfig(attrs map[string]interface{}) (*ebsConfig, error) {
	out, err := ebsConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating EBS storage config for ecs")
	}
	coerced := out.(map[string]interface{})
	volumeType := coerced[EBSVolumeTypeKey].(string)
	driver := coerced[EBSDriverKey].(string)
	ebsConfig := &ebsConfig{
		volumeType: volumeType,
		driver:     driver,
	}
	return ebsConfig, nil
}

// StorageProvider is defined on the jujustorage.ProviderRegistry interface.
func (env *environ) StorageProvider(t jujustorage.ProviderType) (jujustorage.Provider, error) {
	if t == constants.StorageProviderType {
		return &storageProvider{env}, nil
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}

// StorageProviderTypes is defined on the jujustorage.ProviderRegistry interface.
func (*environ) StorageProviderTypes() ([]jujustorage.ProviderType, error) {
	return []jujustorage.ProviderType{constants.StorageProviderType}, nil
}

// ValidateStorageClass returns an error if the storage config is not valid.
func (*environ) ValidateStorageClass(config map[string]interface{}) error {
	// TODO(ecs): REMOVE ME!
	return nil
}

type storageProvider struct {
	client *environ
}

var _ jujustorage.Provider = (*storageProvider)(nil)

// ValidateConfig is defined on the jujustorage.Provider interface.
func (g *storageProvider) ValidateConfig(cfg *jujustorage.Config) error {
	_, err := newEbsConfig(cfg.Attrs())
	return errors.Trace(err)
}

// Supports is defined on the jujustorage.Provider interface.
func (g *storageProvider) Supports(k jujustorage.StorageKind) bool {
	return k == jujustorage.StorageKindBlock
}

// Scope is defined on the jujustorage.Provider interface.
func (g *storageProvider) Scope() jujustorage.Scope {
	return jujustorage.ScopeEnviron
}

// Dynamic is defined on the jujustorage.Provider interface.
func (g *storageProvider) Dynamic() bool {
	return true
}

// Releasable is defined on the jujustorage.Provider interface.
func (g *storageProvider) Releasable() bool {
	return true
}

// DefaultPools is defined on the jujustorage.Provider interface.
func (g *storageProvider) DefaultPools() []*jujustorage.Config {
	ssdPool, _ := jujustorage.NewConfig(
		string(constants.StorageProviderType), // name: "ecs"
		constants.StorageProviderType,
		map[string]interface{}{
			EBSVolumeTypeKey: EBSVolumeTypeValueSSD,
			EBSDriverKey:     EBSDriverValueRexray,
		},
	)
	return []*jujustorage.Config{ssdPool}
}

// VolumeSource is defined on the jujustorage.Provider interface.
func (g *storageProvider) VolumeSource(cfg *jujustorage.Config) (jujustorage.VolumeSource, error) {
	return &volumeSource{
		client: g.client,
	}, nil
}

// FilesystemSource is defined on the jujustorage.Provider interface.
func (g *storageProvider) FilesystemSource(providerConfig *jujustorage.Config) (jujustorage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

type volumeSource struct {
	client *environ
}

var _ jujustorage.VolumeSource = (*volumeSource)(nil)

// CreateVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) CreateVolumes(ctx jujucontext.ProviderCallContext, params []jujustorage.VolumeParams) (_ []jujustorage.CreateVolumesResult, err error) {
	// noop
	logger.Tracef("CreateVolumes params -> %v", params)
	return nil, nil
}

// ListVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) ListVolumes(ctx jujucontext.ProviderCallContext) ([]string, error) {
	logger.Tracef("ListVolumes called")
	return nil, nil
}

// DescribeVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) DescribeVolumes(ctx jujucontext.ProviderCallContext, volIds []string) ([]jujustorage.DescribeVolumesResult, error) {
	logger.Tracef("DescribeVolumes volIds -> %v", volIds)
	results := make([]jujustorage.DescribeVolumesResult, len(volIds))
	for i, volID := range volIds {
		results[i].VolumeInfo = &jujustorage.VolumeInfo{
			Size:       uint64(1),
			VolumeId:   volID,
			Persistent: true,
		}
	}
	return results, nil
}

// DestroyVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) DestroyVolumes(ctx jujucontext.ProviderCallContext, volIds []string) ([]error, error) {
	logger.Debugf("destroy ecs volumes: %v", volIds)
	return nil, nil
}

// ReleaseVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) ReleaseVolumes(ctx jujucontext.ProviderCallContext, volIds []string) ([]error, error) {
	// noop
	return make([]error, len(volIds)), nil
}

// ValidateVolumeParams is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) ValidateVolumeParams(params jujustorage.VolumeParams) error {
	// TODO(caas) - we need to validate params based on the underlying substrate
	return nil
}

// AttachVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) AttachVolumes(ctx jujucontext.ProviderCallContext, attachParams []jujustorage.VolumeAttachmentParams) ([]jujustorage.AttachVolumesResult, error) {
	// noop
	return nil, nil
}

// DetachVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) DetachVolumes(ctx jujucontext.ProviderCallContext, attachParams []jujustorage.VolumeAttachmentParams) ([]error, error) {
	// noop
	return make([]error, len(attachParams)), nil
}
