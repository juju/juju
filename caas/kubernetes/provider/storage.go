// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/schema"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

const (
	// K8s_ProviderType defines the Juju storage type which can be used
	// to provision storage on k8s models.
	K8s_ProviderType = storage.ProviderType("kubernetes")

	// K8s storage pool attributes.

	// StorageClass is the name of a storage class resource.
	StorageClass       = "storage-class"
	storageProvisioner = "storage-provisioner"
	storageMedium      = "storage-medium"
	storageMode        = "storage-mode"
)

//ValidateStorageProvider returns an error if the storage type and config is not valid
// for a Kubernetes deployment.
func ValidateStorageProvider(providerType storage.ProviderType, attributes map[string]interface{}) error {
	switch providerType {
	case K8s_ProviderType:
	case provider.RootfsProviderType:
	case provider.TmpfsProviderType:
	default:
		return errors.NotValidf("storage provider type %q", providerType)
	}
	if attributes == nil {
		return nil
	}
	if mediumValue, ok := attributes[storageMedium]; ok {
		medium := core.StorageMedium(fmt.Sprintf("%v", mediumValue))
		if medium != core.StorageMediumMemory && medium != core.StorageMediumHugePages {
			return errors.NotValidf("storage medium %q", mediumValue)
		}
	}
	if providerType == K8s_ProviderType {
		if err := validateStorageAttributes(attributes); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func validateStorageAttributes(attributes map[string]interface{}) error {
	if _, err := newStorageConfig(attributes); err != nil {
		return errors.Trace(err)
	}
	if _, err := getStorageMode(attributes); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type storageProvider struct {
	client *kubernetesClient
}

var _ storage.Provider = (*storageProvider)(nil)

var storageConfigFields = schema.Fields{
	StorageClass:       schema.String(),
	storageProvisioner: schema.String(),
}

var storageConfigChecker = schema.FieldMap(
	storageConfigFields,
	schema.Defaults{
		StorageClass:       schema.Omit,
		storageProvisioner: schema.Omit,
	},
)

type storageConfig struct {
	// storageClass defines a storage class
	// which will be created with the specified
	// provisioner and parameters if it doesn't
	// exist.
	storageClass string

	// storageProvisioner is the provisioner class to use.
	storageProvisioner string

	// parameters define attributes of the storage class.
	parameters map[string]string

	// reclaimPolicy defines the volume reclaim policy.
	reclaimPolicy core.PersistentVolumeReclaimPolicy
}

const (
	storageConfigParameterPrefix = "parameters."
)

func newStorageConfig(attrs map[string]interface{}) (*storageConfig, error) {
	out, err := storageConfigChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating storage config")
	}
	coerced := out.(map[string]interface{})
	storageConfig := &storageConfig{}
	if storageClassName, ok := coerced[StorageClass].(string); ok {
		storageConfig.storageClass = storageClassName
	}
	if storageProvisioner, ok := coerced[storageProvisioner].(string); ok {
		storageConfig.storageProvisioner = storageProvisioner
	}
	if storageConfig.storageProvisioner != "" && storageConfig.storageClass == "" {
		return nil, errors.New("storage-class must be specified if storage-provisioner is specified")
	}
	// By default, we'll retain volumes used for charm storage.
	storageConfig.reclaimPolicy = core.PersistentVolumeReclaimRetain
	storageConfig.parameters = make(map[string]string)
	for k, v := range attrs {
		if !strings.HasPrefix(k, storageConfigParameterPrefix) {
			continue
		}
		k = strings.TrimPrefix(k, storageConfigParameterPrefix)
		storageConfig.parameters[k] = fmt.Sprintf("%v", v)
	}
	return storageConfig, nil
}

var storageModeFields = schema.Fields{
	storageMode: schema.String(),
}

var storageModeChecker = schema.FieldMap(
	storageModeFields,
	schema.Defaults{
		storageMode: "ReadWriteOnce",
	},
)

func getStorageMode(attrs map[string]interface{}) (*core.PersistentVolumeAccessMode, error) {
	parseMode := func(m string) (*core.PersistentVolumeAccessMode, error) {
		var out core.PersistentVolumeAccessMode
		switch m {
		case "ReadOnlyMany", "ROX":
			out = core.ReadOnlyMany
		case "ReadWriteMany", "RWX":
			out = core.ReadWriteMany
		case "ReadWriteOnce", "RWO":
			out = core.ReadWriteOnce
		default:
			return nil, errors.NotSupportedf("storage mode %q", m)
		}
		return &out, nil
	}

	out, err := storageModeChecker.Coerce(attrs, nil)
	if err != nil {
		return nil, errors.Annotate(err, "validating storage mode")
	}
	coerced := out.(map[string]interface{})
	return parseMode(coerced[storageMode].(string))
}

// ValidateConfig is defined on the storage.Provider interface.
func (g *storageProvider) ValidateConfig(cfg *storage.Config) error {
	return errors.Trace(validateStorageAttributes(cfg.Attrs()))
}

// Supports is defined on the storage.Provider interface.
func (g *storageProvider) Supports(k storage.StorageKind) bool {
	return k == storage.StorageKindBlock
}

// Scope is defined on the storage.Provider interface.
func (g *storageProvider) Scope() storage.Scope {
	return storage.ScopeEnviron
}

// Dynamic is defined on the storage.Provider interface.
func (g *storageProvider) Dynamic() bool {
	return true
}

// Releasable is defined on the storage.Provider interface.
func (g *storageProvider) Releasable() bool {
	return true
}

// DefaultPools is defined on the storage.Provider interface.
func (g *storageProvider) DefaultPools() []*storage.Config {
	return nil
}

// VolumeSource is defined on the storage.Provider interface.
func (g *storageProvider) VolumeSource(cfg *storage.Config) (storage.VolumeSource, error) {
	return &volumeSource{
		client: g.client,
	}, nil
}

// FilesystemSource is defined on the storage.Provider interface.
func (g *storageProvider) FilesystemSource(providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return nil, errors.NotSupportedf("filesystems")
}

type volumeSource struct {
	client *kubernetesClient
}

var _ storage.VolumeSource = (*volumeSource)(nil)

// CreateVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) CreateVolumes(ctx context.ProviderCallContext, params []storage.VolumeParams) (_ []storage.CreateVolumesResult, err error) {
	// noop
	return nil, nil
}

// ListVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) ListVolumes(ctx context.ProviderCallContext) ([]string, error) {
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	vols, err := pVolumes.List(v1.ListOptions{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	volumeIds := make([]string, 0, len(vols.Items))
	for _, v := range vols.Items {
		volumeIds = append(volumeIds, v.Name)
	}
	return volumeIds, nil
}

// DescribeVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) DescribeVolumes(ctx context.ProviderCallContext, volIds []string) ([]storage.DescribeVolumesResult, error) {
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	vols, err := pVolumes.List(v1.ListOptions{
		// TODO(caas) - filter on volumes for the current model
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	byId := make(map[string]core.PersistentVolume)
	for _, vol := range vols.Items {
		byId[vol.Name] = vol
	}
	results := make([]storage.DescribeVolumesResult, len(volIds))
	for i, volId := range volIds {
		vol, ok := byId[volId]
		if !ok {
			results[i].Error = errors.NotFoundf("%s", volId)
			continue
		}
		results[i].VolumeInfo = &storage.VolumeInfo{
			Size:       uint64(vol.Size()),
			VolumeId:   vol.Name,
			Persistent: vol.Spec.PersistentVolumeReclaimPolicy == core.PersistentVolumeReclaimRetain,
		}
	}
	return results, nil
}

// DestroyVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) DestroyVolumes(ctx context.ProviderCallContext, volIds []string) ([]error, error) {
	logger.Debugf("destroy k8s volumes: %v", volIds)
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	return foreachVolume(volIds, func(volumeId string) error {
		vol, err := pVolumes.Get(volumeId, v1.GetOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return errors.Annotatef(err, "getting volume %v to delete", volumeId)
		}
		if err == nil && vol.Spec.ClaimRef != nil {
			claimRef := vol.Spec.ClaimRef
			pClaims := v.client.client().CoreV1().PersistentVolumeClaims(claimRef.Namespace)
			err := pClaims.Delete(claimRef.Name, &v1.DeleteOptions{PropagationPolicy: &defaultPropagationPolicy})
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Annotatef(err, "destroying volume claim %v", claimRef.Name)
			}
		}
		if err := pVolumes.Delete(
			volumeId,
			&v1.DeleteOptions{PropagationPolicy: &defaultPropagationPolicy},
		); !k8serrors.IsNotFound(err) {
			return errors.Annotate(err, "destroying k8s volumes")
		}
		return nil
	}), nil
}

// ReleaseVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) ReleaseVolumes(ctx context.ProviderCallContext, volIds []string) ([]error, error) {
	// noop
	return make([]error, len(volIds)), nil
}

// ValidateVolumeParams is specified on the storage.VolumeSource interface.
func (v *volumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	// TODO(caas) - we need to validate params based on the underlying substrate
	return nil
}

// AttachVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) AttachVolumes(ctx context.ProviderCallContext, attachParams []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	// noop
	return nil, nil
}

// DetachVolumes is specified on the storage.VolumeSource interface.
func (v *volumeSource) DetachVolumes(ctx context.ProviderCallContext, attachParams []storage.VolumeAttachmentParams) ([]error, error) {
	// noop
	return make([]error, len(attachParams)), nil
}

func foreachVolume(volumeIds []string, f func(string) error) []error {
	results := make([]error, len(volumeIds))
	var wg sync.WaitGroup
	for i, volumeId := range volumeIds {
		wg.Add(1)
		go func(i int, volumeId string) {
			defer wg.Done()
			results[i] = f(volumeId)
		}(i, volumeId)
	}
	wg.Wait()
	return results
}
