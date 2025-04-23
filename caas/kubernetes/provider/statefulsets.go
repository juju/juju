// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	k8sstorage "github.com/juju/juju/caas/kubernetes/provider/storage"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	"github.com/juju/juju/caas/specs"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/storage"
)

// https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#update-strategies
func updateStrategyForStatefulSet(strategy specs.UpdateStrategy) (o apps.StatefulSetUpdateStrategy, err error) {
	strategyType := apps.StatefulSetUpdateStrategyType(strategy.Type)

	o = apps.StatefulSetUpdateStrategy{Type: strategyType}
	switch strategyType {
	case apps.OnDeleteStatefulSetStrategyType:
		if strategy.RollingUpdate != nil {
			return o, errors.NewNotValid(nil, fmt.Sprintf("rolling update spec is not supported for %q", strategyType))
		}
	case apps.RollingUpdateStatefulSetStrategyType:
		if strategy.RollingUpdate != nil {
			if strategy.RollingUpdate.MaxSurge != nil || strategy.RollingUpdate.MaxUnavailable != nil {
				return o, errors.NotValidf("rolling update spec for statefulset")
			}
			if strategy.RollingUpdate.Partition == nil {
				return o, errors.New("rolling update spec partition is missing")
			}
			o.RollingUpdate = &apps.RollingUpdateStatefulSetStrategy{
				Partition: strategy.RollingUpdate.Partition,
			}
		}
	default:
		return o, errors.NotValidf("strategy type %q for statefulset", strategyType)
	}
	return o, nil
}

func (k *kubernetesClient) configureStatefulSet(
	appName, deploymentName string, annotations k8sannotations.Annotation, workloadSpec *workloadSpec,
	containers []specs.ContainerSpec, replicas *int32, filesystems []storage.KubernetesFilesystemParams,
	storageID string,
) error {
	logger.Debugf("creating/updating stateful set for %s", appName)

	// Add the specified file to the pod spec.
	cfgName := func(fileSetName string) string {
		return applicationConfigMapName(deploymentName, fileSetName)
	}

	storageUniqueID, err := k.getStorageUniqPrefix(storageID, func() (annotationGetter, error) {
		return k.getStatefulSet(deploymentName)
	})
	if err != nil {
		return errors.Trace(err)
	}

	selectorLabels := utils.SelectorLabelsForApp(appName, k.LabelVersion())
	statefulSet := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   deploymentName,
			Labels: utils.LabelsForApp(appName, k.LabelVersion()),
			Annotations: k8sannotations.New(nil).
				Merge(annotations).
				Add(utils.AnnotationKeyApplicationUUID(k.LabelVersion()), storageUniqueID).ToMap(),
		},
		Spec: apps.StatefulSetSpec{
			Replicas: replicas,
			Selector: &v1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			RevisionHistoryLimit: pointer.Int32Ptr(statefulSetRevisionHistoryLimit),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:      utils.LabelsMerge(workloadSpec.Pod.Labels, selectorLabels),
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
		if err := k8sstorage.PushUniqueVolumeClaimTemplate(&statefulSet.Spec, pvc); err != nil {
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
	existing.Spec.Template.Spec.Volumes = existingPodSpec.Volumes
	existing.Spec.Template.SetAnnotations(spec.Spec.Template.GetAnnotations())
	// TODO(caas) - allow storage `request` configurable - currently we only allow `limit`.
	existing.Spec.Template.Spec.InitContainers = existingPodSpec.InitContainers
	existing.Spec.Template.Spec.Containers = existingPodSpec.Containers
	existing.Spec.Template.Spec.ServiceAccountName = existingPodSpec.ServiceAccountName
	existing.Spec.Template.Spec.AutomountServiceAccountToken = existingPodSpec.AutomountServiceAccountToken
	// NB: we can't update the Spec.ServiceName as it is immutable.
	_, err = k.updateStatefulSet(existing)
	return errors.Trace(err)
}

func (k *kubernetesClient) createStatefulSet(spec *apps.StatefulSet) (*apps.StatefulSet, error) {
	if k.namespace == "" {
		return nil, errNoNamespace
	}
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
	if k.namespace == "" {
		return nil, errNoNamespace
	}
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
	if k.namespace == "" {
		return nil, errNoNamespace
	}
	out, err := k.client().AppsV1().StatefulSets(k.namespace).Get(context.TODO(), name, v1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NotFoundf("stateful set %q", name)
	}
	return out, errors.Trace(err)
}

// deleteStatefulSet deletes a statefulset resource.
func (k *kubernetesClient) deleteStatefulSet(name string) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	err := k.client().AppsV1().StatefulSets(k.namespace).Delete(context.TODO(), name, v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

// deleteStatefulSet deletes all statefulset resources for an application.
func (k *kubernetesClient) deleteStatefulSets(appName string) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	labels := utils.LabelsForApp(appName, k.LabelVersion())
	err := k.client().AppsV1().StatefulSets(k.namespace).DeleteCollection(context.TODO(), v1.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	}, v1.ListOptions{
		LabelSelector: utils.LabelsToSelector(labels).String(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}
