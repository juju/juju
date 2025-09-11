// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"
	"sync"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	jujucontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/resources"
	"github.com/juju/juju/internal/provider/kubernetes/storage"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
	jujustorage "github.com/juju/juju/storage"
)

func validateStorageAttributes(attributes map[string]interface{}) error {
	if _, err := storage.ParseStorageConfig(attributes); err != nil {
		return errors.Trace(err)
	}
	if _, err := storage.ParseStorageMode(attributes); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type storageProvider struct {
	client *kubernetesClient
}

var _ jujustorage.Provider = (*storageProvider)(nil)

// ValidateStorageProvider returns an error if the storage type and config is not valid
// for a Kubernetes deployment.
func (g *storageProvider) ValidateForK8s(attributes map[string]any) error {

	if attributes == nil {
		return nil
	}

	if mediumValue, ok := attributes[constants.StorageMedium]; ok {
		medium := core.StorageMedium(fmt.Sprintf("%v", mediumValue))
		if medium != core.StorageMediumMemory && medium != core.StorageMediumHugePages {
			return errors.NotValidf("storage medium %q", mediumValue)
		}
	}

	if err := validateStorageAttributes(attributes); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// ValidateConfig is defined on the jujustorage.Provider interface.
func (g *storageProvider) ValidateConfig(cfg *jujustorage.Config) error {
	return errors.Trace(validateStorageAttributes(cfg.Attrs()))
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
	return nil
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
	client *kubernetesClient
}

var _ jujustorage.VolumeSource = (*volumeSource)(nil)

// CreateVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) CreateVolumes(ctx jujucontext.ProviderCallContext, params []jujustorage.VolumeParams) (_ []jujustorage.CreateVolumesResult, err error) {
	// noop
	return nil, nil
}

// ListVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) ListVolumes(ctx jujucontext.ProviderCallContext) ([]string, error) {
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	vols, err := pVolumes.List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	volumeIds := make([]string, 0, len(vols.Items))
	for _, v := range vols.Items {
		volumeIds = append(volumeIds, v.Name)
	}
	return volumeIds, nil
}

// DescribeVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) DescribeVolumes(ctx jujucontext.ProviderCallContext, volIds []string) ([]jujustorage.DescribeVolumesResult, error) {
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	vols, err := pVolumes.List(context.TODO(), v1.ListOptions{
		// TODO(caas) - filter on volumes for the current model
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	byID := make(map[string]core.PersistentVolume)
	for _, vol := range vols.Items {
		byID[vol.Name] = vol
	}
	results := make([]jujustorage.DescribeVolumesResult, len(volIds))
	for i, volID := range volIds {
		vol, ok := byID[volID]
		if !ok {
			results[i].Error = errors.NotFoundf("%s", volID)
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
func (v *volumeSource) DestroyVolumes(ctx jujucontext.ProviderCallContext, volIds []string) ([]error, error) {
	logger.Debugf("destroy k8s volumes: %v", volIds)
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	return foreachVolume(volIds, func(volumeId string) error {
		vol, err := pVolumes.Get(context.TODO(), volumeId, v1.GetOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return errors.Annotatef(err, "getting volume %v to delete", volumeId)
		}
		if err == nil && vol.Spec.ClaimRef != nil {
			claimRef := vol.Spec.ClaimRef
			pClaims := v.client.client().CoreV1().PersistentVolumeClaims(claimRef.Namespace)
			logger.Infof("deleting PVC %s due to call to volumeSource.DestroyVolumes(%q)", claimRef.Name, volumeId)
			err := pClaims.Delete(context.TODO(), claimRef.Name, v1.DeleteOptions{PropagationPolicy: constants.DefaultPropagationPolicy()})
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Annotatef(err, "destroying volume claim %v", claimRef.Name)
			}
		}
		if err := pVolumes.Delete(context.TODO(),
			volumeId,
			v1.DeleteOptions{PropagationPolicy: constants.DefaultPropagationPolicy()},
		); !k8serrors.IsNotFound(err) {
			return errors.Annotate(err, "destroying k8s volumes")
		}
		return nil
	}), nil
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

// ImportVolue is specified on the jujustorage.VolumeImporter interface.
func (v *volumeSource) ImportVolume(
	ctx jujucontext.ProviderCallContext,
	volumeId string,
	storageName string,
	resourceTags map[string]string,
	force bool,
) (jujustorage.VolumeInfo, error) {
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	vol, err := pVolumes.Get(ctx, volumeId, v1.GetOptions{})

	if k8serrors.IsNotFound(err) {
		return jujustorage.VolumeInfo{}, errors.NotFoundf("persistent volume %q", volumeId)
	} else if err != nil {
		return jujustorage.VolumeInfo{}, err
	}

	var importErr error
	if force {
		importErr = v.prepareVolumeForImport(ctx, vol, storageName)
	} else {
		importErr = v.validateImportVolume(vol)
	}
	if importErr != nil {
		return jujustorage.VolumeInfo{}, errors.Trace(importErr)
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
		return errors.NewNotSupported(
			nil,
			fmt.Sprintf(
				"importing volume %q with reclaim policy %q not supported (must be %q)",
				vol.Name, vol.Spec.PersistentVolumeReclaimPolicy, core.PersistentVolumeReclaimRetain,
			),
		)
	}
	// The PersistentVolume must not be bound to any PersistentVolumeClaim.
	if vol.Spec.ClaimRef != nil {
		return errors.NotSupportedf("importing volume %q already bound to a claim", vol.Name)
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
func (v *volumeSource) prepareVolumeForImport(ctx jujucontext.ProviderCallContext, vol *core.PersistentVolume, storageName string) error {
	logger.Debugf("force importing PersistentVolume %q", vol.Name)

	// Ensures the PV's reclaim policy is Retain before deleting the PVC.
	if err := v.patchPersistentVolumeReclaimToRetain(ctx, vol); err != nil {
		return errors.Trace(err)
	}

	if err := v.makePersistentVolumeAvailable(ctx, vol, storageName); err != nil {
		return errors.Trace(err)
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
func (v *volumeSource) patchPersistentVolumeReclaimToRetain(ctx jujucontext.ProviderCallContext, vol *core.PersistentVolume) error {
	if vol.Spec.PersistentVolumeReclaimPolicy == core.PersistentVolumeReclaimRetain {
		return nil
	}

	vol.Spec.PersistentVolumeReclaimPolicy = core.PersistentVolumeReclaimRetain
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, vol)
	if err != nil {
		return errors.Annotatef(err, "failed to encode PersistentVolume %s", vol.Name)
	}

	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	_, err = pVolumes.Patch(ctx, vol.Name, types.StrategicMergePatchType, data, v1.PatchOptions{FieldManager: resources.JujuFieldManager})
	if err != nil {
		return errors.Annotatef(err, "failed to patch PersistentVolume %s", vol.Name)
	}

	// Make sure the change is applyed.
	pv, err := pVolumes.Get(ctx, vol.Name, v1.GetOptions{})
	if err != nil {
		return errors.Annotatef(err, "failed to get PersistentVolume %s", vol.Name)
	}

	if pv.Spec.PersistentVolumeReclaimPolicy != core.PersistentVolumeReclaimRetain {
		return errors.Errorf("persistent volume %s reclaim policy is not Retain", vol.Name)
	}

	logger.Infof("successfully patched PersistentVolume %q: set reclaim policy to Retain", vol.Name)
	return nil
}

// makePersistentVolumeAvailable deletes the PVC and clears the claimRef if the PV is bound to a PVC.
// This transitions the PersistentVolume from bound/released state to available state for import.
func (v *volumeSource) makePersistentVolumeAvailable(ctx jujucontext.ProviderCallContext, vol *core.PersistentVolume, storageName string) error {
	if vol.Spec.ClaimRef == nil {
		return nil
	}

	pvcName := vol.Spec.ClaimRef.Name
	pvcNamespace := vol.Spec.ClaimRef.Namespace
	logger.Infof("importing PersistentVolume %q is bound to PVC %s/%s, deleting PVC", vol.Name, pvcNamespace, pvcName)

	// Delete the PVC if it exists and is managed by juju.
	pvcClient := v.client.client().CoreV1().PersistentVolumeClaims(pvcNamespace)
	pvc, err := pvcClient.Get(context.TODO(), pvcName, v1.GetOptions{})
	if err == nil {
		if _, err := utils.MatchStorageMetaLabelVersion(pvc.ObjectMeta, storageName); err != nil {
			return errors.NewNotSupported(
				err,
				fmt.Sprintf(
					"importing PersistentVolume %q whose PersistentVolumeClaim is not managed by juju",
					vol.Name,
				),
			)
		}

		err := pvcClient.Delete(ctx, pvcName, v1.DeleteOptions{})
		if err != nil {
			return errors.Annotatef(err, "failed to delete PVC %s/%s", pvcNamespace, pvcName)
		}
		logger.Infof("successfully deleted PVC %s/%s", pvcNamespace, pvcName)
	} else if !k8serrors.IsNotFound(err) {
		return errors.Trace(err)
	}

	// Clear the claimRef to make the PV available
	vol.Spec.ClaimRef = nil

	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, vol)
	if err != nil {
		return errors.Annotatef(err, "failed to encode PersistentVolume %s", vol.Name)
	}

	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	_, err = pVolumes.Patch(ctx, vol.Name, types.StrategicMergePatchType, data, v1.PatchOptions{FieldManager: resources.JujuFieldManager})
	if err != nil {
		return errors.Annotatef(err, "failed to patch PersistentVolume %s", vol.Name)
	}
	logger.Infof("successfully patched PersistentVolume %q: set claimRef to nil", vol.Name)
	return nil
}
