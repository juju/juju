// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	"github.com/juju/juju/caas/kubernetes/provider/storage"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	jujucontext "github.com/juju/juju/environs/context"
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

	if force {
		if err := v.forceImportVolume(ctx, vol); err != nil {
			return jujustorage.VolumeInfo{}, errors.Trace(err)
		}
	} else {
		if err := v.validateImportVolume(vol); err != nil {
			return jujustorage.VolumeInfo{}, errors.Trace(err)
		}
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

// validateImportVolume verifies whether the given PersistentVolume is eligible for import.
func (v *volumeSource) forceImportVolume(ctx jujucontext.ProviderCallContext, vol *core.PersistentVolume) error {
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	modified := false

	logger.Debugf("force importing PersistentVolume %q", vol.Name)

	// Change the ReclaimPolicy to Retain if not already set
	if vol.Spec.PersistentVolumeReclaimPolicy != core.PersistentVolumeReclaimRetain {
		vol.Spec.PersistentVolumeReclaimPolicy = core.PersistentVolumeReclaimRetain
		modified = true
	}

	// If the PV is bound to a PVC, delete the PVC and clear the claimRef.
	if vol.Spec.ClaimRef != nil {
		pvcName := vol.Spec.ClaimRef.Name
		pvcNamespace := vol.Spec.ClaimRef.Namespace
		logger.Infof("importing PersistentVolume %q is bound to PVC %s/%s, deleting PVC", vol.Name, pvcNamespace, pvcName)

		// Delete the PVC if it exists and is managed by juju.
		pvcClient := v.client.client().CoreV1().PersistentVolumeClaims(pvcNamespace)
		pvc, err := pvcClient.Get(context.TODO(), pvcName, v1.GetOptions{})
		if !k8serrors.IsNotFound(err) {
			// Make sure pvc is managed by juju.
			pvcRes := resources.PersistentVolumeClaim{PersistentVolumeClaim: *pvc}
			if err := pvcRes.Validate(nil, utils.LabelsJuju); err != nil {
				return errors.NotSupportedf(
					"importing PersistentVolume %q whose PersistentVolumeClaim is not managed by juju",
					vol.Name,
				)
			}
			err := pvcClient.Delete(ctx, pvcName, v1.DeleteOptions{})
			if err != nil {
				return errors.Annotatef(err, "failed to delete PVC %s/%s", pvcNamespace, pvcName)
			}
		}

		// Clear the claimRef to make the PV available
		vol.Spec.ClaimRef = nil
		modified = true
	}

	// Update the PV if any modifications were made.
	if modified {
		_, err := pVolumes.Update(ctx, vol, v1.UpdateOptions{})
		if err != nil {
			return errors.Annotatef(err, "failed to update PersistentVolume %s", vol.Name)
		}
		logger.Infof("successfully updated PersistentVolume %q: set reclaim policy to Retain and cleared claimRef", vol.Name)
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
