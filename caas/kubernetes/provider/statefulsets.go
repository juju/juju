// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/errors"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/caas/specs"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/storage"
)

func (k *kubernetesClient) getStatefulSetLabels(appName string) map[string]string {
	return utils.LabelsForApp(appName, k.IsLegacyLabels())
}

func updateStrategyForStatefulSet(strategy specs.UpdateStrategy) (o apps.StatefulSetUpdateStrategy, err error) {
	switch strategyType := apps.StatefulSetUpdateStrategyType(strategy.Type); strategyType {
	case apps.RollingUpdateStatefulSetStrategyType, apps.OnDeleteStatefulSetStrategyType:
		if strategy.RollingUpdate == nil {
			return o, errors.New("rolling update spec is required")
		}
		if strategy.RollingUpdate.MaxSurge != nil || strategy.RollingUpdate.MaxUnavailable != nil {
			return o, errors.NotValidf("rolling update spec for statefulset")
		}
		if strategy.RollingUpdate.Partition == nil {
			return o, errors.New("rolling update spec partition is missing")
		}
		return apps.StatefulSetUpdateStrategy{
			Type: strategyType,
			RollingUpdate: &apps.RollingUpdateStatefulSetStrategy{
				Partition: strategy.RollingUpdate.Partition,
			},
		}, nil
	default:
		return o, errors.NotValidf("strategy type %q for statefulset", strategyType)
	}
}

func (k *kubernetesClient) configureStatefulSet(
	appName, deploymentName string, annotations k8sannotations.Annotation, workloadSpec *workloadSpec,
	containers []specs.ContainerSpec, replicas *int32, filesystems []storage.KubernetesFilesystemParams,
) error {
	logger.Debugf("creating/updating stateful set for %s", appName)

	// Add the specified file to the pod spec.
	cfgName := func(fileSetName string) string {
		return applicationConfigMapName(deploymentName, fileSetName)
	}

	storageUniqueID, err := k.getStorageUniqPrefix(func() (annotationGetter, error) {
		return k.getStatefulSet(deploymentName)
	})
	if err != nil {
		return errors.Trace(err)
	}

	selectorLabels := k.getStatefulSetLabels(appName)
	labels := selectorLabels
	if !k.IsLegacyLabels() {
		labels = utils.LabelsMerge(labels, utils.LabelsJuju)
	}

	statefulSet := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName,
			Labels: labels,
			Annotations: k8sannotations.New(nil).
				Merge(annotations).
				Add(annotationKeyApplicationUUID, storageUniqueID).ToMap(),
		},
		Spec: apps.StatefulSetSpec{
			Replicas: replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			RevisionHistoryLimit: int32Ptr(statefulSetRevisionHistoryLimit),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:      selectorLabels,
					Annotations: podAnnotations(k8sannotations.New(workloadSpec.Pod.Annotations).Merge(annotations).Copy()).ToMap(),
				},
			},
			PodManagementPolicy: getPodManagementPolicy(workloadSpec.Service),
			ServiceName:         headlessServiceName(deploymentName),
		},
	}
	if workloadSpec.Service != nil && workloadSpec.Service.UpdateStrategy != nil {
		if statefulSet.Spec.UpdateStrategy, err = updateStrategyForStatefulSet(*workloadSpec.Service.UpdateStrategy); err != nil {
			return errors.Trace(err)
		}
	}

	if err := k.configurePodFiles(appName, annotations, workloadSpec, containers, cfgName); err != nil {
		return errors.Trace(err)
	}
	podSpec := workloadSpec.Pod.PodSpec
	existingPodSpec := podSpec

	handlePVC := func(pvc core.PersistentVolumeClaim, mountPath string, readOnly bool) error {
		if readOnly {
			logger.Warningf("set storage mode to ReadOnlyMany if read only storage is needed")
		}
		if err := pushUniqueVolumeClaimTemplate(&statefulSet.Spec, pvc); err != nil {
			return errors.Trace(err)
		}
		podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, core.VolumeMount{
			Name:      pvc.Name,
			MountPath: mountPath,
		})
		return nil
	}
	if err = k.configureStorage(appName, isLegacyName(deploymentName), storageUniqueID, filesystems, &podSpec, handlePVC); err != nil {
		return errors.Trace(err)
	}
	statefulSet.Spec.Template.Spec = podSpec
	return k.ensureStatefulSet(statefulSet, existingPodSpec)
}

func (k *kubernetesClient) ensureStatefulSet(spec *apps.StatefulSet, existingPodSpec core.PodSpec) error {
	_, err := k.createStatefulSet(spec)
	if errors.IsNotValid(err) {
		return errors.Annotatef(err, "ensuring stateful set %q", spec.GetName())
	} else if errors.IsAlreadyExists(err) {
		// continue
	} else if err != nil {
		return errors.Trace(err)
	} else {
		return nil
	}
	// The statefulset already exists so all we are allowed to update is replicas,
	// template, update strategy. Juju may hand out info with a slightly different
	// requested volume size due to trying to adapt the unit model to the k8s world.
	existing, err := k.getStatefulSet(spec.GetName())
	if err != nil {
		return errors.Trace(err)
	}
	existing.SetAnnotations(spec.GetAnnotations())
	existing.Spec.Replicas = spec.Spec.Replicas
	existing.Spec.UpdateStrategy = spec.Spec.UpdateStrategy
	existing.Spec.Template.SetAnnotations(spec.Spec.Template.GetAnnotations())
	// TODO(caas) - allow storage `request` configurable - currently we only allow `limit`.
	existing.Spec.Template.Spec.Containers = existingPodSpec.Containers
	existing.Spec.Template.Spec.ServiceAccountName = existingPodSpec.ServiceAccountName
	existing.Spec.Template.Spec.AutomountServiceAccountToken = existingPodSpec.AutomountServiceAccountToken
	// NB: we can't update the Spec.ServiceName as it is immutable.
	_, err = k.updateStatefulSet(existing)
	return errors.Trace(err)
}

func (k *kubernetesClient) createStatefulSet(spec *apps.StatefulSet) (*apps.StatefulSet, error) {
	out, err := k.client().AppsV1().StatefulSets(k.namespace).Create(context.TODO(), spec, v1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil, errors.AlreadyExistsf("stateful set %q", spec.GetName())
	}
	if k8serrors.IsInvalid(err) {
		return nil, errors.NotValidf("stateful set %q", spec.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) updateStatefulSet(spec *apps.StatefulSet) (*apps.StatefulSet, error) {
	out, err := k.client().AppsV1().StatefulSets(k.namespace).Update(context.TODO(), spec, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("stateful set %q", spec.GetName())
	}
	if k8serrors.IsInvalid(err) {
		return nil, errors.NotValidf("stateful set %q", spec.GetName())
	}
	return out, errors.Trace(err)
}

func (k *kubernetesClient) getStatefulSet(name string) (*apps.StatefulSet, error) {
	out, err := k.client().AppsV1().StatefulSets(k.namespace).Get(context.TODO(), name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("stateful set %q", name)
	}
	return out, errors.Trace(err)
}

// deleteStatefulSet deletes a statefulset resource.
func (k *kubernetesClient) deleteStatefulSet(name string) error {
	err := k.client().AppsV1().StatefulSets(k.namespace).Delete(context.TODO(), name, v1.DeleteOptions{
		PropagationPolicy: &constants.DefaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

// deleteStatefulSet deletes all statefulset resources for an application.
func (k *kubernetesClient) deleteStatefulSets(appName string) error {
	labels := k.getStatefulSetLabels(appName)
	if !k.IsLegacyLabels() {
		labels = utils.LabelsMerge(labels, utils.LabelsJuju)
	}
	err := k.client().AppsV1().StatefulSets(k.namespace).DeleteCollection(context.TODO(), v1.DeleteOptions{
		PropagationPolicy: &constants.DefaultPropagationPolicy,
	}, v1.ListOptions{
		LabelSelector: utils.LabelSetToSelector(labels).String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}
