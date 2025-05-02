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
	"github.com/juju/juju/caas/kubernetes/provider/storage"
	jujustorage "github.com/juju/juju/internal/storage"
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
func (v *volumeSource) CreateVolumes(ctx context.Context, params []jujustorage.VolumeParams) (_ []jujustorage.CreateVolumesResult, err error) {
	// noop
	return nil, nil
}

// ListVolumes is specified on the jujustorage.VolumeSource interface.
func (v *volumeSource) ListVolumes(ctx context.Context) ([]string, error) {
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	vols, err := pVolumes.List(ctx, v1.ListOptions{})
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
func (v *volumeSource) DescribeVolumes(ctx context.Context, volIds []string) ([]jujustorage.DescribeVolumesResult, error) {
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	vols, err := pVolumes.List(ctx, v1.ListOptions{
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
func (v *volumeSource) DestroyVolumes(ctx context.Context, volIds []string) ([]error, error) {
	logger.Debugf(context.TODO(), "destroy k8s volumes: %v", volIds)
	pVolumes := v.client.client().CoreV1().PersistentVolumes()
	return foreachVolume(volIds, func(volumeId string) error {
		vol, err := pVolumes.Get(ctx, volumeId, v1.GetOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return errors.Annotatef(err, "getting volume %v to delete", volumeId)
		}
		if err == nil && vol.Spec.ClaimRef != nil {
			claimRef := vol.Spec.ClaimRef
			pClaims := v.client.client().CoreV1().PersistentVolumeClaims(claimRef.Namespace)
			logger.Infof(context.TODO(), "deleting PVC %s due to call to volumeSource.DestroyVolumes(%q)", claimRef.Name, volumeId)
			err := pClaims.Delete(ctx, claimRef.Name, v1.DeleteOptions{PropagationPolicy: constants.DefaultPropagationPolicy()})
			if err != nil && !k8serrors.IsNotFound(err) {
				return errors.Annotatef(err, "destroying volume claim %v", claimRef.Name)
			}
		}
		if err := pVolumes.Delete(ctx,
			volumeId,
			v1.DeleteOptions{PropagationPolicy: constants.DefaultPropagationPolicy()},
		); !k8serrors.IsNotFound(err) {
			return errors.Annotate(err, "destroying k8s volumes")
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
