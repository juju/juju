// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"

	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/resources"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/storage"
	storageprovider "github.com/juju/juju/internal/storage/provider"
)

// GetMountPathForFilesystem returns mount path.
func GetMountPathForFilesystem(idx int, appName string, fs storage.KubernetesFilesystemParams) string {
	if fs.Attachment != nil {
		return fs.Attachment.Path
	}
	return fmt.Sprintf("%s/fs/%s/%s/%d", constants.StorageBaseDir, appName, fs.StorageName, idx)
}

// FilesystemStatus returns filesystem status.
func FilesystemStatus(pvcPhase corev1.PersistentVolumeClaimPhase) status.Status {
	switch pvcPhase {
	case corev1.ClaimPending:
		return status.Pending
	case corev1.ClaimBound:
		return status.Attached
	case corev1.ClaimLost:
		return status.Detached
	default:
		return status.Unknown
	}
}

// VolumeStatus returns volume status.
func VolumeStatus(pvPhase corev1.PersistentVolumePhase) status.Status {
	switch pvPhase {
	case corev1.VolumePending:
		return status.Pending
	case corev1.VolumeBound:
		return status.Attached
	case corev1.VolumeAvailable, corev1.VolumeReleased:
		return status.Detached
	case corev1.VolumeFailed:
		return status.Error
	default:
		return status.Unknown
	}
}

// VolumeSourceForFilesystem return k8s volume source.
func VolumeSourceForFilesystem(fs storage.KubernetesFilesystemParams) (*corev1.VolumeSource, error) {
	fsSize, err := resource.ParseQuantity(fmt.Sprintf("%dMi", fs.Size))
	if err != nil {
		return nil, errors.Annotatef(err, "invalid volume size %v", fs.Size)
	}
	switch fs.Provider {
	case constants.StorageProviderType:
		return nil, nil
	case storageprovider.RootfsProviderType:
		return &corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				SizeLimit: &fsSize,
			},
		}, nil
	case storageprovider.TmpfsProviderType:
		medium, ok := fs.Attributes[constants.StorageMedium]
		if !ok {
			medium = corev1.StorageMediumMemory
		}
		return &corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium:    corev1.StorageMedium(fmt.Sprintf("%v", medium)),
				SizeLimit: &fsSize,
			},
		}, nil
	default:
		return nil, errors.NotValidf("charm storage provider type %q for %v", fs.Provider, fs.StorageName)
	}
}

// StorageClassSpec converts storage provisioner config to k8s storage class.
func StorageClassSpec(cfg k8s.StorageProvisioner, labelVersion constants.LabelVersion) *storagev1.StorageClass {
	sc := storagev1.StorageClass{}
	sc.Name = constants.QualifiedStorageClassName(cfg.Namespace, cfg.Name)
	sc.Provisioner = cfg.Provisioner
	sc.Parameters = cfg.Parameters
	if cfg.ReclaimPolicy != "" {
		policy := corev1.PersistentVolumeReclaimPolicy(cfg.ReclaimPolicy)
		sc.ReclaimPolicy = &policy
	}
	if cfg.VolumeBindingMode != "" {
		bindMode := storagev1.VolumeBindingMode(cfg.VolumeBindingMode)
		sc.VolumeBindingMode = &bindMode
	}
	if cfg.ModelName != "" {
		sc.Labels = utils.LabelsForModel(cfg.ModelName, cfg.ModelUUID, cfg.ControllerUUID, labelVersion)
	}
	return &sc
}

// VolumeInfo returns volume info.
func VolumeInfo(pv *resources.PersistentVolume, now time.Time) caas.VolumeInfo {
	size := quantityAsMibiBytes(*pv.Spec.Capacity.Storage())
	return caas.VolumeInfo{
		VolumeId:   pv.Name,
		Size:       size,
		Persistent: true,
		Status: status.StatusInfo{
			Status:  VolumeStatus(pv.Status.Phase),
			Message: pv.Status.Message,
			Since:   &now,
		},
	}
}

// FilesystemInfo returns filesystem info.
func FilesystemInfo(ctx context.Context, client kubernetes.Interface,
	namespace string, volume corev1.Volume, volumeMount corev1.VolumeMount,
	now time.Time) (*caas.FilesystemInfo, error) {
	if volume.EmptyDir != nil {
		size := uint64(0)
		if volume.EmptyDir.SizeLimit != nil {
			size = quantityAsMibiBytes(*volume.EmptyDir.SizeLimit)
		}
		return &caas.FilesystemInfo{
			Size:         size,
			FilesystemId: volume.Name,
			MountPoint:   volumeMount.MountPath,
			ReadOnly:     volumeMount.ReadOnly,
			Status: status.StatusInfo{
				Status: status.Attached,
				Since:  &now,
			},
			Volume: caas.VolumeInfo{
				VolumeId:   volume.Name,
				Size:       size,
				Persistent: false,
				Status: status.StatusInfo{
					Status: status.Attached,
					Since:  &now,
				},
			},
		}, nil
	} else if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName == "" {
		// Ignore volumes which are not Juju managed filesystems.
		logger.Debugf(context.TODO(), "ignoring blank EmptyDir, PersistentVolumeClaim or ClaimName")
		return nil, errors.NotSupportedf("volume %v", volume)
	}

	// Handle PVC
	pvc := resources.NewPersistentVolumeClaim(volume.PersistentVolumeClaim.ClaimName, namespace, nil)
	err := pvc.Get(ctx, client)
	if err != nil {
		return nil, errors.Annotate(err, "unable to get persistent volume claim")
	}

	if pvc.Status.Phase == corev1.ClaimPending {
		logger.Debugf(context.TODO(), fmt.Sprintf("PersistentVolumeClaim for %v is pending", pvc.Name))
		return nil, nil
	}

	storageName := utils.StorageNameFromLabels(pvc.Labels)
	if storageName == "" {
		if valid := constants.LegacyPVNameRegexp.MatchString(volumeMount.Name); valid {
			storageName = constants.LegacyPVNameRegexp.ReplaceAllString(volumeMount.Name, "$storageName")
		} else if valid := constants.PVNameRegexp.MatchString(volumeMount.Name); valid {
			storageName = constants.PVNameRegexp.ReplaceAllString(volumeMount.Name, "$storageName")
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
		eventList, err := pvc.Events(ctx, client)
		if err != nil {
			return nil, errors.Annotate(err, "unable to get events for PVC")
		}
		// Take the most recent event.
		if count := len(eventList); count > 0 {
			statusMessage = eventList[count-1].Message
		}
	}

	pv := resources.NewPersistentVolume(pvc.Spec.VolumeName, nil)
	err = pv.Get(ctx, client)
	if errors.Is(err, errors.NotFound) {
		// Ignore volumes which don't exist (yet).
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "unable to get persistent volume")
	}

	return &caas.FilesystemInfo{
		StorageName:  storageName,
		Size:         quantityAsMibiBytes(*pvc.Spec.Resources.Requests.Storage()),
		FilesystemId: string(pvc.UID),
		MountPoint:   volumeMount.MountPath,
		ReadOnly:     volumeMount.ReadOnly,
		Status: status.StatusInfo{
			Status:  FilesystemStatus(pvc.Status.Phase),
			Message: statusMessage,
			Since:   &since,
		},
		Volume: VolumeInfo(pv, since),
	}, nil
}

// PersistentVolumeClaimSpec returns k8s PVC spec.
func PersistentVolumeClaimSpec(params VolumeParams) *corev1.PersistentVolumeClaimSpec {
	return &corev1.PersistentVolumeClaimSpec{
		StorageClassName: &params.StorageConfig.StorageClass,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: params.Size,
			},
		},
		AccessModes: []corev1.PersistentVolumeAccessMode{params.AccessMode},
	}
}

// StorageProvisioner returns storage provisioner.
func StorageProvisioner(namespace, modelName, modelUUID string, params VolumeParams) k8s.StorageProvisioner {
	return k8s.StorageProvisioner{
		Name:          params.StorageConfig.StorageClass,
		Namespace:     namespace,
		ModelName:     modelName,
		ModelUUID:     modelUUID,
		Provisioner:   params.StorageConfig.StorageProvisioner,
		Parameters:    params.StorageConfig.Parameters,
		ReclaimPolicy: string(params.StorageConfig.ReclaimPolicy),
	}
}

func quantityAsMibiBytes(q resource.Quantity) uint64 {
	return uint64(q.MilliValue()) / 1000 / 1024 / 1024
}
