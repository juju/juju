// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"
	"sync"

	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/storage"
	jujustorage "github.com/juju/juju/internal/storage"
)

func validateStorageAttributes(attributes map[string]interface{}) error {
	if _, err := storage.ParseStorageConfig(attributes); err != nil {
		return errors.Capture(err)
	}
	if _, err := storage.ParseStorageMode(attributes); err != nil {
		return errors.Capture(err)
	}
	return nil
}

func validateStorageMedium(attributes map[string]any) error {
	if attributes == nil {
		return nil
	}

	if mediumValue, ok := attributes[constants.StorageMedium]; ok {
		medium := core.StorageMedium(fmt.Sprintf("%v", mediumValue))
		if medium != core.StorageMediumMemory && medium != core.StorageMediumHugePages {
			return errors.Errorf(
				"storage medium %q not valid", mediumValue,
			).Add(coreerrors.NotValid)
		}
	}

	return nil
}

// noopFSSource is a [jujustorage.FilesystemSource] that performs no actions for
// the provider.
type noopFSSource struct{}

// rootfsStorageProvider is a [jujustorage.Provider] that provides rootfs
// like filesystems in a Kubernetes model.
type rootfsStorageProvider struct{}

// storageProvider is a [jujustorage.Provider] that provides storage in a
// Kubernetes model.
type storageProvider struct {
	client *kubernetesClient
}

// tmpfsStorageProvider is a [jujustorage.Provider] that provides tmpfs like
// filesystems in a Kubernetes model.
type tmpfsStorageProvider struct{}

var (
	_ jujustorage.Provider = (*rootfsStorageProvider)(nil)
	_ jujustorage.Provider = (*storageProvider)(nil)
	_ jujustorage.Provider = (*tmpfsStorageProvider)(nil)
)

// RecommendedPoolForKind returns the recommended storage pool to use for
// supplied storage kind. At the moment the only supported recommended kind is
// filesystem.
//
// This func implements the [jujustorage.PoolAdvisor] interface.
func (k *kubernetesClient) RecommendedPoolForKind(
	kind jujustorage.StorageKind,
) *jujustorage.Config {
	// NOTE (tlm): The Juju logic around if a storage provider through either
	// it's filesystem or volume source is capable of supporting a given storage
	// kind is not owned by a provider today. This determinantion is made by
	// storage provisoning where the assumption is that a volume source can be a
	// volume or filesystem source. This divorses the provider from being
	// involved in the decision making even though it provides the storage.
	//
	// For now we assume that the Kubernetes provider is always capable of
	// creating filesystems and block devices are not supported on Kubernetes.
	if kind != jujustorage.StorageKindFilesystem {
		return nil
	}

	s := storageProvider{k}
	defaultPools := s.DefaultPools()
	// We take the first default pool offered by the storage provider.
	if len(defaultPools) != 0 {
		return defaultPools[0]
	}
	return nil
}

// StorageProviderTypes is defined on the jujustorage.ProviderRegistry interface.
func (*kubernetesClient) StorageProviderTypes() ([]jujustorage.ProviderType, error) {
	return []jujustorage.ProviderType{
		constants.StorageProviderType,
		constants.StorageProviderTypeRootfs,
		constants.StorageProviderTypeTmpfs,
	}, nil
}

// StorageProvider is defined on the jujustorage.ProviderRegistry interface.
func (k *kubernetesClient) StorageProvider(t jujustorage.ProviderType) (jujustorage.Provider, error) {
	switch t {
	case constants.StorageProviderType:
		return &storageProvider{k}, nil
	case constants.StorageProviderTypeRootfs:
		return &rootfsStorageProvider{}, nil
	case constants.StorageProviderTypeTmpfs:
		return &tmpfsStorageProvider{}, nil
	default:
		return nil, errors.Errorf(
			"storage provider for type %q does not exist",
			t,
		).Add(coreerrors.NotFound)
	}
}

// AttachFilesystems is a noop operation for attaching filesystems in this
// source.
//
// Implements the [jujustorage.FilesystemSource] interface.
func (*noopFSSource) AttachFilesystems(
	_ context.Context, _ []jujustorage.FilesystemAttachmentParams,
) ([]jujustorage.AttachFilesystemsResult, error) {
	return nil, nil
}

// CreateFilesystem is a noop operation for creating filesystems in this source.
//
// Implements the [jujustorage.FilesystemSource] interface.
func (*noopFSSource) CreateFilesystems(
	_ context.Context, _ []jujustorage.FilesystemParams,
) ([]jujustorage.CreateFilesystemsResult, error) {
	return nil, nil
}

// DefaultPools returns the default storage pools for [rootfsStorageProvider].
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) DefaultPools() []*jujustorage.Config {
	pool, _ := jujustorage.NewConfig(
		constants.StorageProviderTypeRootfs.String(),
		constants.StorageProviderTypeRootfs,
		jujustorage.Attrs{},
	)
	return []*jujustorage.Config{pool}
}

// DefaultPools returns the default storage pools for [storageProvider].
//
// Implements the [jujustorage.Provider] interface.
func (*storageProvider) DefaultPools() []*jujustorage.Config {
	pool, _ := jujustorage.NewConfig(
		constants.StorageProviderType.String(),
		constants.StorageProviderType,
		jujustorage.Attrs{},
	)
	return []*jujustorage.Config{pool}
}

// DefaultPools returns the default storage pools for [tmpfsStorageProvider].
//
// Implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) DefaultPools() []*jujustorage.Config {
	pool, _ := jujustorage.NewConfig(
		constants.StorageProviderTypeTmpfs.String(),
		constants.StorageProviderTypeTmpfs,
		jujustorage.Attrs{},
	)
	return []*jujustorage.Config{pool}
}

// DestroyFilesystems is a noop operation for destroying filesystems in this
// source.
//
// Implements the [jujustorage.FilesystemSource] interface.
func (*noopFSSource) DestroyFilesystems(
	_ context.Context, _ []string,
) ([]error, error) {
	return nil, nil
}

// DetatchFilesystems is a noop operation for detaching filesystems in this
// source.
//
// Implements the [jujustorage.FilesystemSource] interface.
func (*noopFSSource) DetachFilesystems(
	_ context.Context, _ []jujustorage.FilesystemAttachmentParams,
) ([]error, error) {
	return nil, nil
}

// Dynamic informs the caller if this provider supports creating storage after
// a machine is provisioned. This question is not applicable to Kubernetes so
// we return true.
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) Dynamic() bool {
	return true
}

// Dynamic informs the caller if this provider supports creating storage after
// a machine is provisioned. This question is not applicable to Kubernetes so
// we return true.
//
// Implements the [jujustorage.Provider] interface.
func (*storageProvider) Dynamic() bool {
	return true
}

// Dynamic informs the caller if this provider supports creating storage after
// a machine is provisioned. This question is not applicable to Kubernetes so
// we return true.
//
// Implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) Dynamic() bool {
	return true
}

// FilesystemSource returns the filesystem source for this provider.
//
// The returned filesystem source does not provision any filesystems in the
// model and results in noop operations. Actual provisioning is done by
// kubernetes as part of creating the application.
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) FilesystemSource(
	_ *jujustorage.Config,
) (jujustorage.FilesystemSource, error) {
	return &noopFSSource{}, nil
}

// FilesystemSource is defined on the jujustorage.Provider interface.
func (g *storageProvider) FilesystemSource(providerConfig *jujustorage.Config) (jujustorage.FilesystemSource, error) {
	return &filesystemSource{
		client: g.client,
	}, nil
}

// FilesystemSource returns the filesystem source for this provider.
//
// The returned filesystem source does not provision any filesystems in the
// model and results in noop operations. Actual provisioning is done by
// kubernetes as part of creating the application.
//
// Implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) FilesystemSource(
	_ *jujustorage.Config,
) (jujustorage.FilesystemSource, error) {
	return &noopFSSource{}, nil
}

// Reports if this provider is capable of releasing dynamically provisioned
// storage.
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) Releasable() bool {
	return false
}

// Reports if this provider is capable of releasing dynamically provisioned
// storage.
//
// Implements the [jujustorage.Provider] interface.
func (*storageProvider) Releasable() bool {
	return true
}

// Reports if this provider is capable of releasing dynamically provisioned
// storage.
//
// Implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) Releasable() bool {
	return false
}

// ReleaseFilesystems is a noop operation for releasing filesystems in this
// source.
//
// Implements the [jujustorage.FilesystemSource] interface.
func (*noopFSSource) ReleaseFilesystems(
	_ context.Context, _ []string,
) ([]error, error) {
	return nil, nil
}

// Scope returns the provisioning scope required for [rootfsStorageProvider].
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) Scope() jujustorage.Scope {
	return jujustorage.ScopeEnviron
}

// Scope returns the provisioning scope required for [storageProvider].
//
// Implements the [jujustorage.Provider] interface.
func (*storageProvider) Scope() jujustorage.Scope {
	return jujustorage.ScopeEnviron
}

// Scope returns the provisioning scope required for [tmpfsStorageProvider].
//
// Implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) Scope() jujustorage.Scope {
	return jujustorage.ScopeEnviron
}

// Supports tells the caller if this provider supports the given storage kind.
// Currently only filesystems are supported.
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) Supports(k jujustorage.StorageKind) bool {
	return k == jujustorage.StorageKindFilesystem
}

// Support tells the caller if this provider supports the given storage kind.
// Currently only block storage is supported.
//
// Implements the [jujustorage.Provider] interface.
func (g *storageProvider) Supports(k jujustorage.StorageKind) bool {
	return k == jujustorage.StorageKindFilesystem
}

// Support tells the caller if this provider supports the given storage kind.
// Currently only filesystems are supported.
//
// Implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) Supports(k jujustorage.StorageKind) bool {
	return k == jujustorage.StorageKindFilesystem
}

// ValidateConfig validates the supplied configuration for the rootfs storage
// provider. This func implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) ValidateConfig(cfg *jujustorage.Config) error {
	return errors.Capture(validateStorageMedium(cfg.Attrs()))
}

// ValidateConfig is defined on the jujustorage.Provider interface.
func (g *storageProvider) ValidateConfig(cfg *jujustorage.Config) error {
	return errors.Capture(validateStorageAttributes(cfg.Attrs()))
}

// ValidateConfig validates the supplied configuration for the tmpfs storage
// provider. This func implements the [jujustorage.Provider] interface.
func (*tmpfsStorageProvider) ValidateConfig(cfg *jujustorage.Config) error {
	return errors.Capture(validateStorageMedium(cfg.Attrs()))
}

func (*rootfsStorageProvider) ValidateForK8s(attributes map[string]any) error {
	return errors.Capture(validateStorageMedium(attributes))
}

// ValidateStorageProvider returns an error if the storage type and config is not valid
// for a Kubernetes deployment.
func (g *storageProvider) ValidateForK8s(attributes map[string]any) error {
	if attributes == nil {
		return nil
	}

	if err := validateStorageMedium(attributes); err != nil {
		return errors.Capture(err)
	}

	if err := validateStorageAttributes(attributes); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (*tmpfsStorageProvider) ValidateForK8s(attributes map[string]any) error {
	return errors.Capture(validateStorageMedium(attributes))
}

// ValidateFilesystemParams validates the parameters for a fielsystem. No
// validation is performed by this source and results in a noop operation.
func (*noopFSSource) ValidateFilesystemParams(_ jujustorage.FilesystemParams) error {
	return nil
}

// VolumeSource provides the [jujustorage.VolumeSource] implementation for tmpfs
// storage.
//
// This always results in an error as volumes are not support for tmpfs storage.
//
// Implements the [jujustorage.Provider] interface.
func (*rootfsStorageProvider) VolumeSource(*jujustorage.Config) (
	jujustorage.VolumeSource, error,
) {
	return nil, errors.New("volumes are not supported by rootfs storage").Add(
		coreerrors.NotSupported,
	)
}

// VolumeSource is defined on the jujustorage.Provider interface.
func (g *storageProvider) VolumeSource(cfg *jujustorage.Config) (jujustorage.VolumeSource, error) {
	return nil, errors.New("volumes are not supported").Add(
		coreerrors.NotSupported,
	)
}

// VolumeSource provides the [jujustorage.VolumeSource] implementation for tmpfs
// storage.
//
// This always results in an error as volumes are not support for tmpfs storage.
//
// Implements the [jujustorage.Provider] interface.
func (p *tmpfsStorageProvider) VolumeSource(*jujustorage.Config) (
	jujustorage.VolumeSource, error,
) {
	return nil, errors.New("volumes are not supported by tmpfs").Add(
		coreerrors.NotSupported,
	)
}

type filesystemSource struct {
	client *kubernetesClient
}

var _ jujustorage.FilesystemSource = (*filesystemSource)(nil)

// ValidateFilesystemParams is specified on the jujustorage.FilesystemSource interface.
func (v *filesystemSource) ValidateFilesystemParams(params jujustorage.FilesystemParams) error {
	return nil
}

// CreateFilesystems is specified on the jujustorage.FilesystemSource interface.
func (v *filesystemSource) CreateFilesystems(
	ctx context.Context,
	params []jujustorage.FilesystemParams,
) (_ []jujustorage.CreateFilesystemsResult, err error) {
	// noop
	return nil, nil
}

// DestroyFilesystems is specified on the jujustorage.FilesystemSource interface.
func (v *filesystemSource) DestroyFilesystems(ctx context.Context, filesystemIds []string) ([]error, error) {
	logger.Debugf(ctx, "destroy k8s filesystems: %v", filesystemIds)
	pvAPI := v.client.client().CoreV1().PersistentVolumes()
	return foreachFilesystem(filesystemIds, func(filesystemId string) error {
		vol, err := pvAPI.Get(ctx, filesystemId, v1.GetOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return errors.Errorf("getting filesystem %v to delete: %w", filesystemId, err)
		}
		if err == nil && vol.Spec.ClaimRef != nil {
			claimRef := vol.Spec.ClaimRef
			pvcAPI := v.client.client().CoreV1().PersistentVolumeClaims(claimRef.Namespace)
			logger.Infof(
				context.TODO(), "deleting PVC %s due to call to filesystemSource.DestroyVolumes(%q)",
				claimRef.Name, filesystemId,
			)
			err := pvcAPI.Delete(
				ctx, claimRef.Name,
				v1.DeleteOptions{PropagationPolicy: constants.DefaultPropagationPolicy()},
			)
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Errorf("destroying volume claim %v: %w", claimRef.Name, err)
			}
		}
		if err := pvAPI.Delete(ctx,
			filesystemId,
			v1.DeleteOptions{PropagationPolicy: constants.DefaultPropagationPolicy()},
		); err != nil && !k8serrors.IsNotFound(err) {
			return errors.Errorf("destroying k8s filesystem %q: %w", filesystemId, err)
		}
		return nil
	}), nil
}

// ReleaseFilesystems is specified on the jujustorage.FilesystemSource interface.
func (v *filesystemSource) ReleaseFilesystems(ctx context.Context, ids []string) ([]error, error) {
	// noop
	return make([]error, len(ids)), nil
}

// AttachFilesystems is specified on the jujustorage.FilesystemSource interface.
func (v *filesystemSource) AttachFilesystems(
	ctx context.Context,
	params []jujustorage.FilesystemAttachmentParams,
) ([]jujustorage.AttachFilesystemsResult, error) {
	// noop
	return nil, nil
}

// DetachFilesystems is specified on the jujustorage.FilesystemSource interface.
func (v *filesystemSource) DetachFilesystems(
	ctx context.Context,
	params []jujustorage.FilesystemAttachmentParams,
) ([]error, error) {
	// noop
	return make([]error, len(params)), nil
}

// ImportFilesystem is specified on the jujustorage.FilesystemImporter interface.
func (v *filesystemSource) ImportFilesystem(
	ctx context.Context,
	filesystemId string,
	resourceTags map[string]string,
) (jujustorage.FilesystemInfo, error) {
	pv, err := v.client.client().CoreV1().PersistentVolumes().Get(ctx, filesystemId, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return jujustorage.FilesystemInfo{}, errors.Errorf(
			"persistent volume %q not found", filesystemId,
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return jujustorage.FilesystemInfo{}, errors.Capture(err)
	}

	if err := v.validateImportPV(pv); err != nil {
		return jujustorage.FilesystemInfo{}, errors.Capture(err)
	}
	return jujustorage.FilesystemInfo{
		Size:       uint64(pv.Size()),
		ProviderId: pv.Name,
	}, nil
}

// validateImportPV verifies whether the given PersistentVolume is eligible for import.
func (v *filesystemSource) validateImportPV(vol *core.PersistentVolume) error {
	// The PersistentVolume's reclaim policy must be set to Retain.
	if vol.Spec.PersistentVolumeReclaimPolicy != core.PersistentVolumeReclaimRetain {
		return errors.Errorf(
			"importing kubernetes persistent volume %q with reclaim policy %q is not supported (must be %q)",
			vol.Name,
			vol.Spec.PersistentVolumeReclaimPolicy,
			core.PersistentVolumeReclaimRetain,
		).Add(coreerrors.NotSupported)
	}
	// The PersistentVolume must not be bound to any PersistentVolumeClaim.
	if vol.Spec.ClaimRef != nil {
		return errors.Errorf(
			"importing kubernetes persistent volume %q already bound to a claim is not supported",
			vol.Name,
		).Add(coreerrors.NotSupported)
	}
	return nil
}

func foreachFilesystem(ids []string, f func(string) error) []error {
	results := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			results[i] = f(id)
		}(i, id)
	}
	wg.Wait()
	return results
}
