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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	"github.com/juju/juju/internal/provider/kubernetes/storage"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
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
	return nil, errors.New("filesystems are not supported").Add(
		coreerrors.NotSupported,
	)
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
	return k == jujustorage.StorageKindBlock
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
	return &volumeSource{
		client: g.client,
	}, nil
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

type volumeSource struct {
	client *kubernetesClient
}

var _ jujustorage.VolumeSource = (*volumeSource)(nil)

// CreateVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) CreateVolumes(ctx context.Context, params []jujustorage.VolumeParams) (_ []jujustorage.CreateVolumesResult, err error) {
	// noop
	return nil, nil
}

// ListVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) ListVolumes(ctx context.Context) ([]string, error) {
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	vols, err := pVolumes.List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	volumeIds := make([]string, 0, len(vols.Items))
	for _, v := range vols.Items {
		volumeIds = append(volumeIds, v.Name)
	}
	return volumeIds, nil
}

// DescribeVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) DescribeVolumes(ctx context.Context, volIds []string) ([]jujustorage.DescribeVolumesResult, error) {
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	vols, err := pVolumes.List(ctx, v1.ListOptions{
		// TODO(caas) - filter on volumes for the current model
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	byID := make(map[string]core.PersistentVolume)
	for _, vol := range vols.Items {
		byID[vol.Name] = vol
	}
	results := make([]jujustorage.DescribeVolumesResult, len(volIds))
	for i, volID := range volIds {
		vol, ok := byID[volID]
		if !ok {
			results[i].Error = errors.Errorf("volume %q not found", volID).Add(
				coreerrors.NotFound,
			)
			continue
		}
		results[i].VolumeInfo = &jujustorage.VolumeInfo{
			Size:       uint64(vol.Size()),
			VolumeId:   vol.Name,
			Persistent: true,
		}
	}
	return results, nil
}

// DestroyVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) DestroyVolumes(ctx context.Context, volIds []string) ([]error, error) {
	logger.Debugf(ctx, "destroy k8s volumes: %v", volIds)
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	return foreachVolume(volIds, func(volumeId string) error {
		vol, err := pVolumes.Get(ctx, volumeId, v1.GetOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return errors.Errorf("getting volume %v to delete: %w", volumeId, err)
		}
		if err == nil && vol.Spec.ClaimRef != nil {
			claimRef := vol.Spec.ClaimRef
			pClaims := v.client.client().CoreV1().PersistentVolumeClaims(claimRef.Namespace)
			logger.Infof(context.TODO(), "deleting PVC %s due to call to volumeSource.DestroyVolumes(%q)", claimRef.Name, volumeId)
			err := pClaims.Delete(ctx, claimRef.Name, v1.DeleteOptions{PropagationPolicy: constants.DefaultPropagationPolicy()})
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Errorf("destroying volume claim %v: %w", claimRef.Name, err)
			}
		}
		if err := pVolumes.Delete(ctx,
			volumeId,
			v1.DeleteOptions{PropagationPolicy: constants.DefaultPropagationPolicy()},
		); err != nil && !k8serrors.IsNotFound(err) {
			return errors.Errorf("destroying k8s volumes: %w", err)
		}
		return nil
	}), nil
}

// ReleaseVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) ReleaseVolumes(ctx context.Context, volIds []string) ([]error, error) {
	// noop
	return make([]error, len(volIds)), nil
}

// ValidateVolumeParams is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) ValidateVolumeParams(params jujustorage.VolumeParams) error {
	// TODO(caas) - we need to validate params based on the underlying substrate
	return nil
}

// AttachVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) AttachVolumes(ctx context.Context, attachParams []jujustorage.VolumeAttachmentParams) ([]jujustorage.AttachVolumesResult, error) {
	// noop
	return nil, nil
}

// DetachVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) DetachVolumes(ctx context.Context, attachParams []jujustorage.VolumeAttachmentParams) ([]error, error) {
	// noop
	return make([]error, len(attachParams)), nil
}

// ImportVolue is specified on the jujustorage.VolumeImporter interface.
func (v *volumeSource) ImportVolume(
	ctx context.Context,
	volumeId string,
	storageName string,
	resourceTags map[string]string,
	force bool,
) (jujustorage.VolumeInfo, error) {
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	vol, err := pVolumes.Get(ctx, volumeId, v1.GetOptions{})

	if k8serrors.IsNotFound(err) {
		return jujustorage.VolumeInfo{}, errors.Errorf(
			"persistent volume %q not found", volumeId,
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return jujustorage.VolumeInfo{}, errors.Capture(err)
	}

	var importErr error
	if force {
		importErr = v.prepareVolumeForImport(ctx, vol, storageName)
	} else {
		importErr = v.validateImportVolume(vol)
	}
	if importErr != nil {
		return jujustorage.VolumeInfo{}, errors.Capture(importErr)
	}
	return jujustorage.VolumeInfo{
		Size:       uint64(vol.Size()),
		VolumeId:   vol.Name,
		Persistent: true,
	}, nil
}

// validateImportVolume verifies whether the given PersistentVolume is eligible for import.
func (v *volumeSource) validateImportVolume(vol *core.PersistentVolume) error {
	// The PersistentVolume's reclaim policy must be set to Retain.
	if vol.Spec.PersistentVolumeReclaimPolicy != core.PersistentVolumeReclaimRetain {
		return errors.Errorf(
			"importing volume %q with reclaim policy %q not supported (must be %q)",
			vol.Name,
			vol.Spec.PersistentVolumeReclaimPolicy,
			core.PersistentVolumeReclaimRetain,
		).Add(coreerrors.NotSupported)
	}
	// The PersistentVolume must not be bound to any PersistentVolumeClaim.
	if vol.Spec.ClaimRef != nil {
		return errors.Errorf(
			"importing volume %q already bound to a claim not supported",
			vol.Name,
		).Add(coreerrors.NotSupported)
	}
	return nil
}

// prepareVolumeForImport prepares a PersistentVolume for forced import by Juju.
// This function modifies the PersistentVolume to ensure it can be imported by:
//   - Setting the reclaim policy to Retain to prevent deletion
//   - Deleting any bound PersistentVolumeClaim if it's managed by Juju
//   - Clearing the claimRef to make the volume available for new claims
//
// Returns an error if the PVC is not managed by Juju or if any Kubernetes operations fail.
func (v *volumeSource) prepareVolumeForImport(ctx context.Context, vol *core.PersistentVolume, storageName string) error {
	logger.Debugf(ctx, "force importing PersistentVolume %q", vol.Name)

	// Ensures the PV's reclaim policy is Retain before deleting the PVC.
	if err := v.patchPersistentVolumeReclaimToRetain(ctx, vol); err != nil {
		return errors.Capture(err)
	}

	if err := v.makePersistentVolumeAvailable(ctx, vol, storageName); err != nil {
		return errors.Capture(err)
	}
	return nil
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

// patchPersistentVolumeReclaimToRetain patches the persistent volume's reclaim policy to Retain.
// This prevents the volume from being deleted when its claim is removed.
func (v *volumeSource) patchPersistentVolumeReclaimToRetain(ctx context.Context, vol *core.PersistentVolume) error {
	if vol.Spec.PersistentVolumeReclaimPolicy == core.PersistentVolumeReclaimRetain {
		return nil
	}

	vol.Spec.PersistentVolumeReclaimPolicy = core.PersistentVolumeReclaimRetain
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, vol)
	if err != nil {
		return errors.Errorf("failed to encode PersistentVolume %s: %w", vol.Name, err)
	}

	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	_, err = pVolumes.Patch(ctx, vol.Name, types.StrategicMergePatchType, data, v1.PatchOptions{FieldManager: resources.JujuFieldManager})
	if err != nil {
		return errors.Errorf("failed to patch PersistentVolume %s: %w", vol.Name, err)
	}

	// Make sure the change is applyed.
	pv, err := pVolumes.Get(ctx, vol.Name, v1.GetOptions{})
	if err != nil {
		return errors.Errorf("failed to get PersistentVolume %s: %w", vol.Name, err)
	}

	if pv.Spec.PersistentVolumeReclaimPolicy != core.PersistentVolumeReclaimRetain {
		return errors.Errorf("persistent volume %s reclaim policy is not Retain", vol.Name)
	}

	logger.Infof(ctx, "successfully patched PersistentVolume %q: set reclaim policy to Retain", vol.Name)
	return nil
}

// makePersistentVolumeAvailable deletes the PVC and clears the claimRef if the PV is bound to a PVC.
// This transitions the PersistentVolume from bound/released state to available state for import.
func (v *volumeSource) makePersistentVolumeAvailable(ctx context.Context, vol *core.PersistentVolume, storageName string) error {
	if vol.Spec.ClaimRef == nil {
		return nil
	}

	pvcName := vol.Spec.ClaimRef.Name
	pvcNamespace := vol.Spec.ClaimRef.Namespace
	logger.Infof(ctx, "importing PersistentVolume %q is bound to PVC %s/%s, deleting PVC", vol.Name, pvcNamespace, pvcName)

	// Delete the PVC if it exists and is managed by juju.
	pvcClient := v.client.client().CoreV1().PersistentVolumeClaims(pvcNamespace)
	pvc, err := pvcClient.Get(context.TODO(), pvcName, v1.GetOptions{})
	if err == nil {
		if _, err := utils.MatchStorageMetaLabelVersion(pvc.ObjectMeta, storageName); err != nil {
			return errors.Errorf(
				"%w: importing PersistentVolume %q whose PersistentVolumeClaim is not managed by juju",
				err, vol.Name,
			).Add(coreerrors.NotSupported)
		}

		err := pvcClient.Delete(ctx, pvcName, v1.DeleteOptions{})
		if err != nil {
			return errors.Errorf("failed to delete PVC %s/%s: %w", pvcNamespace, pvcName, err)
		}
		logger.Infof(ctx, "successfully deleted PVC %s/%s", pvcNamespace, pvcName)
	} else if !k8serrors.IsNotFound(err) {
		return errors.Capture(err)
	}

	// Clear the claimRef to make the PV available
	vol.Spec.ClaimRef = nil

	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, vol)
	if err != nil {
		return errors.Errorf("failed to encode PersistentVolume %s: %w", vol.Name, err)
	}

	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	_, err = pVolumes.Patch(ctx, vol.Name, types.StrategicMergePatchType, data, v1.PatchOptions{FieldManager: resources.JujuFieldManager})
	if err != nil {
		return errors.Errorf("failed to patch PersistentVolume %s: %w", vol.Name, err)
	}
	logger.Infof(ctx, "successfully patched PersistentVolume %q: set claimRef to nil", vol.Name)
	return nil
}
