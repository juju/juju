// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/storage"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/core/status"
	jujustorage "github.com/juju/juju/storage"
)

// StorageProviderTypes is defined on the jujustorage.ProviderRegistry interface.
func (k *kubernetesClient) StorageProviderTypes() ([]jujustorage.ProviderType, error) {
	return []jujustorage.ProviderType{constants.StorageProviderType}, nil
}

// StorageProvider is defined on the jujustorage.ProviderRegistry interface.
func (k *kubernetesClient) StorageProvider(t jujustorage.ProviderType) (jujustorage.Provider, error) {
	if t == constants.StorageProviderType {
		return &storageProvider{k}, nil
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}

func (k *kubernetesClient) deleteStorageClasses(selector k8slabels.Selector) error {
	err := k.client().StorageV1().StorageClasses().DeleteCollection(context.TODO(), v1.DeleteOptions{
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
func (k *kubernetesClient) ListStorageClasses(selector k8slabels.Selector) ([]storagev1.StorageClass, error) {
	listOps := v1.ListOptions{
		LabelSelector: selector.String(),
	}
	list, err := k.client().StorageV1().StorageClasses().List(context.TODO(), listOps)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(list.Items) == 0 {
		return nil, errors.NotFoundf("storage classes with selector %q", selector)
	}
	return list.Items, nil
}

func (k *kubernetesClient) ensurePVC(pvc *core.PersistentVolumeClaim) (*core.PersistentVolumeClaim, func(), error) {
	cleanUp := func() {}
	out, err := k.createPVC(pvc)
	if err == nil {
		// Only do cleanup for the first time!
		cleanUp = func() { _ = k.deletePVC(out.GetName(), out.GetUID()) }
		return out, cleanUp, nil
	}
	if !errors.IsAlreadyExists(err) {
		return nil, cleanUp, errors.Trace(err)
	}
	existing, err := k.getPVC(pvc.GetName())
	if err != nil {
		return nil, cleanUp, errors.Trace(err)
	}
	// PVC is immutable after creation except resources.requests for bound claims.
	// TODO(caas): support requests - currently we only support limits which means updating here is a no ops for now.
	existing.Spec.Resources.Requests = pvc.Spec.Resources.Requests
	out, err = k.updatePVC(existing)
	return out, cleanUp, errors.Trace(err)
}

func (k *kubernetesClient) createPVC(pvc *core.PersistentVolumeClaim) (*core.PersistentVolumeClaim, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().CoreV1().PersistentVolumeClaims(k.namespace).Create(context.TODO(), pvc, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("PVC %q", pvc.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updatePVC(pvc *core.PersistentVolumeClaim) (*core.PersistentVolumeClaim, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().CoreV1().PersistentVolumeClaims(k.namespace).Update(context.TODO(), pvc, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("PVC %q", pvc.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deletePVC(name string, uid types.UID) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	logger.Infof("deleting PVC %s due to call to kubernetesClient.deletePVC", name)
	err := k.client().CoreV1().PersistentVolumeClaims(k.namespace).Delete(context.TODO(), name, utils.NewPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) getPVC(name string) (*core.PersistentVolumeClaim, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	pvc, err := k.client().CoreV1().PersistentVolumeClaims(k.namespace).Get(context.TODO(), name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("pvc %q", name)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return pvc, nil
}

// ValidateStorageClass returns an error if the storage config is not valid.
func (k *kubernetesClient) ValidateStorageClass(config map[string]interface{}) error {
	cfg, err := storage.ParseStorageConfig(config)
	if err != nil {
		return errors.Trace(err)
	}
	sc, err := k.getStorageClass(cfg.StorageClass)
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
func (k *kubernetesClient) EnsureStorageProvisioner(cfg k8s.StorageProvisioner) (*k8s.StorageProvisioner, bool, error) {
	// First see if the named storage class exists.
	sc, err := k.getStorageClass(cfg.Name)
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
	_, err = k.client().StorageV1().StorageClasses().Create(context.TODO(), sc, v1.CreateOptions{})
	if err != nil {
		return nil, false, errors.Annotatef(err, "creating storage class %q", cfg.Name)
	}
	return toCaaSStorageProvisioner(sc), false, nil
}

// maybeGetVolumeClaimSpec returns a persistent volume claim spec for the given
// parameters. If no suitable storage class is available, return a NotFound error.
func (k *kubernetesClient) maybeGetVolumeClaimSpec(params storage.VolumeParams) (*core.PersistentVolumeClaimSpec, error) {
	storageClassName := params.StorageConfig.StorageClass
	haveStorageClass := false
	if storageClassName == "" {
		return nil, errors.New("cannot create a volume claim spec without a storage class")
	}
	// See if the requested storage class exists already.
	sc, err := k.getStorageClass(storageClassName)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, errors.Annotatef(err, "looking for storage class %q", storageClassName)
	}
	if err == nil {
		haveStorageClass = true
		storageClassName = sc.Name
	}
	if !haveStorageClass {
		params.StorageConfig.StorageClass = storageClassName
		sc, _, err := k.EnsureStorageProvisioner(k8s.StorageProvisioner{
			Name:          params.StorageConfig.StorageClass,
			Namespace:     k.namespace,
			Provisioner:   params.StorageConfig.StorageProvisioner,
			Parameters:    params.StorageConfig.Parameters,
			ReclaimPolicy: string(params.StorageConfig.ReclaimPolicy),
		})
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if err == nil {
			haveStorageClass = true
			storageClassName = sc.Name
		}
	}
	if !haveStorageClass {
		return nil, errors.NewNotFound(nil, fmt.Sprintf(
			"cannot create persistent volume as storage class %q cannot be found", storageClassName))
	}
	return &core.PersistentVolumeClaimSpec{
		StorageClassName: &storageClassName,
		Resources: core.VolumeResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceStorage: params.Size,
			},
		},
		AccessModes: []core.PersistentVolumeAccessMode{params.AccessMode},
	}, nil
}

func (k *kubernetesClient) filesystemToVolumeInfo(
	i int, fs jujustorage.KubernetesFilesystemParams,
	pvcNameGetter func(int, string) string,
) (vol *core.Volume, pvc *core.PersistentVolumeClaim, err error) {
	fsSize, err := resource.ParseQuantity(fmt.Sprintf("%dMi", fs.Size))
	if err != nil {
		return nil, nil, errors.Annotatef(err, "invalid volume size %v", fs.Size)
	}

	volumeSource, err := storage.VolumeSourceForFilesystem(fs)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if volumeSource != nil {
		volName := fmt.Sprintf("%s-%d", fs.StorageName, i)
		vol = &core.Volume{
			Name:         volName,
			VolumeSource: *volumeSource,
		}
		return vol, pvc, nil
	}
	params, err := storage.ParseVolumeParams(pvcNameGetter(i, fs.StorageName), fsSize, fs.Attributes)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "getting volume params for %s", fs.StorageName)
	}
	pvcSpec, err := k.maybeGetVolumeClaimSpec(*params)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "finding volume for %s", fs.StorageName)
	}

	labels := utils.LabelsMerge(
		utils.LabelsForStorage(fs.StorageName, k.LabelVersion()),
		utils.LabelsJuju)

	pvc = &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name: params.Name,
			Annotations: utils.ResourceTagsToAnnotations(fs.ResourceTags, k.LabelVersion()).
				Merge(utils.AnnotationsForStorage(fs.StorageName, k.LabelVersion())).
				ToMap(),
			Labels: labels,
		},
		Spec: *pvcSpec,
	}
	return vol, pvc, nil
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

func (k *kubernetesClient) volumeInfoForPVC(vol core.Volume, volMount core.VolumeMount, claimName string, now time.Time) (*caas.FilesystemInfo, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	pvClaims := k.client().CoreV1().PersistentVolumeClaims(k.namespace)
	pvc, err := pvClaims.Get(context.TODO(), claimName, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		// Ignore claims which don't exist (yet).
		return nil, nil
	}
	if err != nil {
		return nil, errors.Annotate(err, "unable to get persistent volume claim")
	}

	if pvc.Status.Phase == core.ClaimPending {
		logger.Debugf(fmt.Sprintf("PersistentVolumeClaim for %v is pending", claimName))
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
		eventList, err := k.getEvents(pvc.Name, "PersistentVolumeClaim")
		if err != nil {
			return nil, errors.Annotate(err, "unable to get events for PVC")
		}
		// Take the most recent event.
		if count := len(eventList); count > 0 {
			statusMessage = eventList[count-1].Message
		}
	}

	pVolumes := k.client().CoreV1().PersistentVolumes()
	pv, err := pVolumes.Get(context.TODO(), pvc.Spec.VolumeName, v1.GetOptions{})
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

func (k *kubernetesClient) fileSetToVolume(
	appName string,
	annotations map[string]string,
	workloadSpec *workloadSpec,
	fileSet specs.FileSet,
	cfgMapName configMapNameFunc,
) (core.Volume, error) {
	fileRefsToVolItems := func(fs []specs.FileRef) (out []core.KeyToPath) {
		for _, f := range fs {
			out = append(out, core.KeyToPath{
				Key:  f.Key,
				Path: f.Path,
				Mode: f.Mode,
			})
		}
		return out
	}

	vol := core.Volume{Name: fileSet.Name}
	if len(fileSet.Files) > 0 {
		vol.Name = cfgMapName(fileSet.Name)
		if _, err := k.ensureConfigMapLegacy(filesetConfigMap(vol.Name, k.getConfigMapLabels(appName), annotations, &fileSet)); err != nil {
			return vol, errors.Annotatef(err, "creating or updating ConfigMap for file set %v", vol.Name)
		}
		vol.ConfigMap = &core.ConfigMapVolumeSource{
			LocalObjectReference: core.LocalObjectReference{
				Name: vol.Name,
			},
		}
		for _, f := range fileSet.Files {
			vol.ConfigMap.Items = append(vol.ConfigMap.Items, core.KeyToPath{
				Key:  f.Path,
				Path: f.Path,
				Mode: f.Mode,
			})
		}
	} else if fileSet.HostPath != nil {
		t := core.HostPathType(fileSet.HostPath.Type)
		vol.HostPath = &core.HostPathVolumeSource{
			Path: fileSet.HostPath.Path,
			Type: &t,
		}
	} else if fileSet.EmptyDir != nil {
		vol.EmptyDir = &core.EmptyDirVolumeSource{
			Medium:    core.StorageMedium(fileSet.EmptyDir.Medium),
			SizeLimit: fileSet.EmptyDir.SizeLimit,
		}
	} else if fileSet.ConfigMap != nil {
		found := false
		refName := fileSet.ConfigMap.Name
		for cfgN := range workloadSpec.ConfigMaps {
			if cfgN == refName {
				found = true
				break
			}
		}
		if !found {
			return vol, errors.NewNotValid(nil, fmt.Sprintf(
				"cannot mount a volume using a config map if the config map %q is not specified in the pod spec YAML", refName,
			))
		}

		vol.ConfigMap = &core.ConfigMapVolumeSource{
			LocalObjectReference: core.LocalObjectReference{
				Name: refName,
			},
			DefaultMode: fileSet.ConfigMap.DefaultMode,
			Items:       fileRefsToVolItems(fileSet.ConfigMap.Files),
		}
	} else if fileSet.Secret != nil {
		found := false
		refName := fileSet.Secret.Name
		for _, secret := range workloadSpec.Secrets {
			if secret.Name == refName {
				found = true
				break
			}
		}
		if !found {
			return vol, errors.NewNotValid(nil, fmt.Sprintf(
				"cannot mount a volume using a secret if the secret %q is not specified in the pod spec YAML", refName,
			))
		}

		vol.Secret = &core.SecretVolumeSource{
			SecretName:  refName,
			DefaultMode: fileSet.Secret.DefaultMode,
			Items:       fileRefsToVolItems(fileSet.Secret.Files),
		}
	} else {
		// This should never happen because FileSet validation has been in k8s spec level.
		return vol, errors.NotValidf("fileset %q is empty", fileSet.Name)
	}
	return vol, nil
}
