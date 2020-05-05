// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"
	"reflect"
	"time"

	"github.com/juju/errors"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

// StorageProviderTypes is defined on the storage.ProviderRegistry interface.
func (k *kubernetesClient) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{K8s_ProviderType}, nil
}

// StorageProvider is defined on the storage.ProviderRegistry interface.
func (k *kubernetesClient) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	if t == K8s_ProviderType {
		return &storageProvider{k}, nil
	}
	return nil, errors.NotFoundf("storage provider %q", t)
}

func (k *kubernetesClient) deleteStorageClasses(selector k8slabels.Selector) error {
	err := k.client().StorageV1().StorageClasses().DeleteCollection(&v1.DeleteOptions{
		PropagationPolicy: &defaultPropagationPolicy,
	}, v1.ListOptions{
		LabelSelector: selector.String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Annotate(err, "deleting model storage classes")
}

func (k *kubernetesClient) listStorageClasses(selector k8slabels.Selector) ([]storagev1.StorageClass, error) {
	listOps := v1.ListOptions{
		LabelSelector: selector.String(),
	}
	list, err := k.client().StorageV1().StorageClasses().List(listOps)
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
	out, err := k.client().CoreV1().PersistentVolumeClaims(k.namespace).Create(pvc)
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("PVC %q", pvc.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updatePVC(pvc *core.PersistentVolumeClaim) (*core.PersistentVolumeClaim, error) {
	out, err := k.client().CoreV1().PersistentVolumeClaims(k.namespace).Update(pvc)
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("PVC %q", pvc.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) deletePVC(name string, uid types.UID) error {
	err := k.client().CoreV1().PersistentVolumeClaims(k.namespace).Delete(name, newPreconditionDeleteOptions(uid))
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) getPVC(name string) (*core.PersistentVolumeClaim, error) {
	pvc, err := k.client().CoreV1().PersistentVolumeClaims(k.namespace).Get(name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("pvc %q", name)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return pvc, nil
}

// ValidateStorageClass returns an error if the storage config is not valid.
func (k *kubernetesClient) ValidateStorageClass(config map[string]interface{}) error {
	cfg, err := newStorageConfig(config)
	if err != nil {
		return errors.Trace(err)
	}
	sc, err := k.getStorageClass(cfg.storageClass)
	if err != nil {
		return errors.NewNotValid(err, fmt.Sprintf("storage class %q", cfg.storageClass))
	}
	if cfg.storageProvisioner == "" {
		return nil
	}
	if sc.Provisioner != cfg.storageProvisioner {
		return errors.NewNotValid(
			nil,
			fmt.Sprintf("storage class %q has provisoner %q, not %q", cfg.storageClass, sc.Provisioner, cfg.storageProvisioner))
	}
	return nil
}

// EnsureStorageProvisioner creates a storage class with the specified config, or returns an existing one.
func (k *kubernetesClient) EnsureStorageProvisioner(cfg caas.StorageProvisioner) (*caas.StorageProvisioner, bool, error) {
	// First see if the named storage class exists.
	sc, err := k.getStorageClass(cfg.Name)
	if err == nil {
		return toCaaSStorageProvisioner(*sc), true, nil
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
			Name: qualifiedStorageClassName(cfg.Namespace, cfg.Name),
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
		sc.Labels = map[string]string{labelModel: k.namespace}
	}
	_, err = k.client().StorageV1().StorageClasses().Create(sc)
	if err != nil {
		return nil, false, errors.Annotatef(err, "creating storage class %q", cfg.Name)
	}
	return toCaaSStorageProvisioner(*sc), false, nil
}

type volumeParams struct {
	storageConfig       *storageConfig
	pvcName             string
	requestedVolumeSize resource.Quantity
	accessMode          core.PersistentVolumeAccessMode
}

func newVolumeParams(pvcName string, size resource.Quantity, storageAttr map[string]interface{}) (params volumeParams, err error) {
	storageConfig, err := newStorageConfig(storageAttr)
	if err != nil {
		return params, errors.Annotatef(err, "invalid storage configuration for %v", pvcName)
	}
	accessMode, err := getStorageMode(storageAttr)
	if err != nil {
		return params, errors.Annotatef(err, "invalid storage mode for %v", pvcName)
	}
	return volumeParams{
		pvcName:             pvcName,
		requestedVolumeSize: size,
		storageConfig:       storageConfig,
		accessMode:          *accessMode,
	}, nil
}

// maybeGetVolumeClaimSpec returns a persistent volume claim spec for the given
// parameters. If no suitable storage class is available, return a NotFound error.
func (k *kubernetesClient) maybeGetVolumeClaimSpec(params volumeParams) (*core.PersistentVolumeClaimSpec, error) {
	storageClassName := params.storageConfig.storageClass
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
		params.storageConfig.storageClass = storageClassName
		sc, _, err := k.EnsureStorageProvisioner(caas.StorageProvisioner{
			Name:          params.storageConfig.storageClass,
			Namespace:     k.namespace,
			Provisioner:   params.storageConfig.storageProvisioner,
			Parameters:    params.storageConfig.parameters,
			ReclaimPolicy: string(params.storageConfig.reclaimPolicy),
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
		Resources: core.ResourceRequirements{
			Requests: core.ResourceList{
				core.ResourceStorage: params.requestedVolumeSize,
			},
		},
		AccessModes: []core.PersistentVolumeAccessMode{params.accessMode},
	}, nil
}

func (k *kubernetesClient) filesystemToVolumeInfo(
	i int, fs storage.KubernetesFilesystemParams,
	pvcNameGetter func(int, string) string,
) (vol *core.Volume, pvc *core.PersistentVolumeClaim, err error) {
	fsSize, err := resource.ParseQuantity(fmt.Sprintf("%dMi", fs.Size))
	if err != nil {
		return nil, nil, errors.Annotatef(err, "invalid volume size %v", fs.Size)
	}

	var volumeSource *core.VolumeSource
	switch fs.Provider {
	case K8s_ProviderType:
	case provider.RootfsProviderType:
		volumeSource = &core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{
				SizeLimit: &fsSize,
			},
		}
	case provider.TmpfsProviderType:
		medium, ok := fs.Attributes[storageMedium]
		if !ok {
			medium = core.StorageMediumMemory
		}
		volumeSource = &core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{
				Medium:    core.StorageMedium(fmt.Sprintf("%v", medium)),
				SizeLimit: &fsSize,
			},
		}
	default:
		return nil, nil, errors.NotValidf("charm storage provider type %q for %v", fs.Provider, fs.StorageName)
	}
	if volumeSource != nil {
		volName := fmt.Sprintf("%s-%d", fs.StorageName, i)
		vol = &core.Volume{
			Name:         volName,
			VolumeSource: *volumeSource,
		}
		return vol, pvc, nil
	}
	params, err := newVolumeParams(pvcNameGetter(i, fs.StorageName), fsSize, fs.Attributes)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "getting volume params for %s", fs.StorageName)
	}
	pvcSpec, err := k.maybeGetVolumeClaimSpec(params)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "finding volume for %s", fs.StorageName)
	}

	pvc = &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name: params.pvcName,
			Annotations: resourceTagsToAnnotations(fs.ResourceTags).
				Add(labelStorage, fs.StorageName).ToMap(),
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
	pvClaims := k.client().CoreV1().PersistentVolumeClaims(k.namespace)
	pvc, err := pvClaims.Get(claimName, v1.GetOptions{})
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

	storageName := pvc.Labels[labelStorage]
	if storageName == "" {
		if valid := legacyJujuPVNameRegexp.MatchString(volMount.Name); valid {
			storageName = legacyJujuPVNameRegexp.ReplaceAllString(volMount.Name, "$storageName")
		} else if valid := jujuPVNameRegexp.MatchString(volMount.Name); valid {
			storageName = jujuPVNameRegexp.ReplaceAllString(volMount.Name, "$storageName")
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
	pv, err := pVolumes.Get(pvc.Spec.VolumeName, v1.GetOptions{})
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
			Status:  k.jujuFilesystemStatus(pvc.Status.Phase),
			Message: statusMessage,
			Since:   &since,
		},
		Volume: caas.VolumeInfo{
			VolumeId:   pv.Name,
			Size:       uint64(pv.Size()),
			Persistent: pv.Spec.PersistentVolumeReclaimPolicy == core.PersistentVolumeReclaimRetain,
			Status: status.StatusInfo{
				Status:  k.jujuVolumeStatus(pv.Status.Phase),
				Message: pv.Status.Message,
				Since:   &since,
			},
		},
	}, nil
}

func getMountPathForFilesystem(i int, appName string, fs storage.KubernetesFilesystemParams) string {
	if fs.Attachment != nil {
		return fs.Attachment.Path
	}
	return fmt.Sprintf("%s/fs/%s/%s/%d", k8sStorageBaseDir, appName, fs.StorageName, i)
}

// pushUniqueVolume ensures to only add unique volumes because k8s will not schedule pods if it has duplicated volumes.
// The existing volume will be replaced if force sets to true.
func pushUniqueVolume(podSpec *core.PodSpec, vol core.Volume, force bool) error {
	for i, v := range podSpec.Volumes {
		if v.Name != vol.Name {
			continue
		}
		if reflect.DeepEqual(v, vol) {
			return nil
		}
		if force {
			podSpec.Volumes[i] = vol
			return nil
		}
		return errors.NotValidf("duplicated volume %q", vol.Name)
	}
	podSpec.Volumes = append(podSpec.Volumes, vol)
	return nil
}

// pushUniqueVolumeMount ensures to only add unique volume mount to a container.
func pushUniqueVolumeMount(container *core.Container, volMount core.VolumeMount) {
	for _, v := range container.VolumeMounts {
		if reflect.DeepEqual(v, volMount) {
			return
		}
	}
	container.VolumeMounts = append(container.VolumeMounts, volMount)
}

func pushUniqueVolumeClaimTemplate(spec *apps.StatefulSetSpec, pvc core.PersistentVolumeClaim) error {
	for _, v := range spec.VolumeClaimTemplates {
		if v.Name == pvc.Name {
			// PVC name has to be unique.
			return errors.NotValidf("duplicated PVC %q", pvc.Name)
		}
	}
	spec.VolumeClaimTemplates = append(spec.VolumeClaimTemplates, pvc)
	return nil
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
