// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/storage"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
	jujustorage "github.com/juju/juju/internal/storage"
)

// StorageProviderTypes is defined on the jujustorage.ProviderRegistry interface.
func (k *kubernetesClient) StorageProviderTypes() ([]jujustorage.ProviderType, error) {
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
		return nil, errors.NotFoundf("storage provider %q", t)
	}
}

func (k *kubernetesClient) deleteStorageClasses(ctx context.Context, selector k8slabels.Selector) error {
	err := k.client().StorageV1().StorageClasses().DeleteCollection(ctx, v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, v1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Annotate(err, "deleting model storage classes")
}

// ListStorageClasses returns a list of storage classes for the provided labels.
func (k *kubernetesClient) ListStorageClasses(ctx context.Context, selector k8slabels.Selector) ([]storagev1.StorageClass, error) {
	listOps := v1.ListOptions{
		LabelSelector: selector.String(),
	}
	list, err := k.client().StorageV1().StorageClasses().List(ctx, listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(list.Items) == 0 {
		return nil, errors.NotFoundf("storage classes with selector %q", selector)
	}
	return list.Items, nil
}

func (k *kubernetesClient) getPVC(ctx context.Context, name string) (*core.PersistentVolumeClaim, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	pvc, err := k.client().CoreV1().PersistentVolumeClaims(k.namespace).Get(ctx, name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("pvc %q", name)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return pvc, nil
}

// ValidateStorageClass returns an error if the storage config is not valid.
func (k *kubernetesClient) ValidateStorageClass(ctx context.Context, config map[string]interface{}) error {
	cfg, err := storage.ParseStorageConfig(config)
	if err != nil {
		return errors.Trace(err)
	}
	sc, err := k.getStorageClass(ctx, cfg.StorageClass)
	if err != nil {
		return errors.NewNotValid(err, fmt.Sprintf("storage class %q", cfg.StorageClass))
	}
	if cfg.StorageProvisioner == "" {
		return nil
	}
	if sc.Provisioner != cfg.StorageProvisioner {
		return errors.NewNotValid(
			nil,
			fmt.Sprintf("storage class %q has provisoner %q, not %q", cfg.StorageClass, sc.Provisioner, cfg.StorageProvisioner))
	}
	return nil
}

// EnsureStorageProvisioner creates a storage class with the specified config, or returns an existing one.
func (k *kubernetesClient) EnsureStorageProvisioner(ctx context.Context, cfg k8s.StorageProvisioner) (*k8s.StorageProvisioner, bool, error) {
	// First see if the named storage class exists.
	sc, err := k.getStorageClass(ctx, cfg.Name)
	if err == nil {
		return toCaaSStorageProvisioner(sc), true, nil
	}
	if !k8serrors.IsNotFound(err) {
		return nil, false, errors.Annotatef(err, "getting storage class %q", cfg.Name)
	}
	// If it's not found but there's no provisioner specified, we can't
	// create it so just return not found.
	if cfg.Provisioner == "" {
		return nil, false, errors.NewNotFound(nil,
			fmt.Sprintf("storage class %q doesn't exist, but no storage provisioner has been specified",
				cfg.Name))
	}

	// Create the storage class with the specified provisioner.
	sc = &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: constants.QualifiedStorageClassName(cfg.Namespace, cfg.Name),
		},
		Provisioner: cfg.Provisioner,
		Parameters:  cfg.Parameters,
	}
	if cfg.ReclaimPolicy != "" {
		policy := core.PersistentVolumeReclaimPolicy(cfg.ReclaimPolicy)
		sc.ReclaimPolicy = &policy
	}
	if cfg.VolumeBindingMode != "" {
		bindMode := storagev1.VolumeBindingMode(cfg.VolumeBindingMode)
		sc.VolumeBindingMode = &bindMode
	}
	if cfg.Namespace != "" {
		sc.Labels = utils.LabelsForModel(k.ModelName(), k.ModelUUID(), k.ControllerUUID(), k.LabelVersion())
	}
	_, err = k.client().StorageV1().StorageClasses().Create(ctx, sc, v1.CreateOptions{})
	if err != nil {
		return nil, false, errors.Annotatef(err, "creating storage class %q", cfg.Name)
	}
	return toCaaSStorageProvisioner(sc), false, nil
}

func (k *kubernetesClient) volumeInfoForEmptyDir(vol core.Volume, volMount core.VolumeMount, now time.Time) (*caas.FilesystemInfo, error) {
	size := uint64(0)
	if vol.EmptyDir.SizeLimit != nil {
		size = uint64(vol.EmptyDir.SizeLimit.Size())
	}
	return &caas.FilesystemInfo{
		Size:         size,
		FilesystemId: vol.Name,
		MountPoint:   volMount.MountPath,
		ReadOnly:     volMount.ReadOnly,
		Status: status.StatusInfo{
			Status: status.Attached,
			Since:  &now,
		},
		Volume: caas.VolumeInfo{
			VolumeId:   vol.Name,
			Size:       size,
			Persistent: false,
			Status: status.StatusInfo{
				Status: status.Attached,
				Since:  &now,
			},
		},
	}, nil
}

func (k *kubernetesClient) volumeInfoForPVC(ctx context.Context, vol core.Volume, volMount core.VolumeMount, claimName string, now time.Time) (*caas.FilesystemInfo, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	pvClaims := k.client().CoreV1().PersistentVolumeClaims(k.namespace)
	pvc, err := pvClaims.Get(ctx, claimName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		// Ignore claims which don't exist (yet).
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "unable to get persistent volume claim")
	}

	if pvc.Status.Phase == core.ClaimPending {
		logger.Debugf(context.TODO(), fmt.Sprintf("PersistentVolumeClaim for %v is pending", claimName))
		return nil, nil
	}

	storageName := utils.StorageNameFromLabels(pvc.Labels)
	if storageName == "" {
		if valid := constants.LegacyPVNameRegexp.MatchString(volMount.Name); valid {
			storageName = constants.LegacyPVNameRegexp.ReplaceAllString(volMount.Name, "$storageName")
		} else if valid := constants.PVNameRegexp.MatchString(volMount.Name); valid {
			storageName = constants.PVNameRegexp.ReplaceAllString(volMount.Name, "$storageName")
		}
	}

	statusMessage := ""
	since := now
	if len(pvc.Status.Conditions) > 0 {
		statusMessage = pvc.Status.Conditions[0].Message
		since = pvc.Status.Conditions[0].LastProbeTime.Time
	}
	if statusMessage == "" {
		// If there are any events for this pvc we can use the
		// most recent to set the status.
		eventList, err := k.getEvents(ctx, pvc.Name, "PersistentVolumeClaim")
		if err != nil {
			return nil, errors.Annotate(err, "unable to get events for PVC")
		}
		// Take the most recent event.
		if count := len(eventList); count > 0 {
			statusMessage = eventList[count-1].Message
		}
	}

	pVolumes := k.client().CoreV1().PersistentVolumes()
	pv, err := pVolumes.Get(ctx, pvc.Spec.VolumeName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		// Ignore volumes which don't exist (yet).
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "unable to get persistent volume")
	}

	return &caas.FilesystemInfo{
		StorageName:  storageName,
		Size:         uint64(vol.PersistentVolumeClaim.Size()),
		FilesystemId: string(pvc.UID),
		MountPoint:   volMount.MountPath,
		ReadOnly:     volMount.ReadOnly,
		Status: status.StatusInfo{
			Status:  storage.FilesystemStatus(pvc.Status.Phase),
			Message: statusMessage,
			Since:   &since,
		},
		Volume: caas.VolumeInfo{
			VolumeId:   pv.Name,
			Size:       uint64(pv.Size()),
			Persistent: true,
			Status: status.StatusInfo{
				Status:  storage.VolumeStatus(pv.Status.Phase),
				Message: pv.Status.Message,
				Since:   &since,
			},
		},
	}, nil
}
