// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version"
	"k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
)

// AdoptResources is called when the model is moved from one
// controller to another using model migration.
func (k *kubernetesClient) AdoptResources(ctx context.ProviderCallContext, controllerUUID string, fromVersion version.Number) error {
	modelLabel := fmt.Sprintf("%v==%v", tags.JujuModel, k.modelUUID)

	pods := k.CoreV1().Pods(k.namespace)
	podsList, err := pods.List(v1.ListOptions{
		LabelSelector: modelLabel,
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, p := range podsList.Items {
		p.Labels[tags.JujuController] = controllerUUID
		if _, err := pods.Update(&p); err != nil {
			return errors.Annotatef(err, "updating labels for pod %q", p.Name)
		}
	}

	pvcs := k.CoreV1().PersistentVolumeClaims(k.namespace)
	pvcList, err := pvcs.List(v1.ListOptions{
		LabelSelector: modelLabel,
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, pvc := range pvcList.Items {
		pvc.Labels[tags.JujuController] = controllerUUID
		if _, err := pvcs.Update(&pvc); err != nil {
			return errors.Annotatef(err, "updating labels for pvc %q", pvc.Name)
		}
	}

	pvs := k.CoreV1().PersistentVolumes()
	pvList, err := pvs.List(v1.ListOptions{
		LabelSelector: modelLabel,
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, pv := range pvList.Items {
		pv.Labels[tags.JujuController] = controllerUUID
		if _, err := pvs.Update(&pv); err != nil {
			return errors.Annotatef(err, "updating labels for pvc %q", pv.Name)
		}
	}

	sSets := k.AppsV1().StatefulSets(k.namespace)
	ssList, err := sSets.List(v1.ListOptions{
		LabelSelector: modelLabel,
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, ss := range ssList.Items {
		ss.Labels[tags.JujuController] = controllerUUID
		if _, err := sSets.Update(&ss); err != nil {
			return errors.Annotatef(err, "updating labels for stateful set %q", ss.Name)
		}
	}

	deployments := k.AppsV1().Deployments(k.namespace)
	dList, err := deployments.List(v1.ListOptions{
		LabelSelector: modelLabel,
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, d := range dList.Items {
		d.Labels[tags.JujuController] = controllerUUID
		if _, err := deployments.Update(&d); err != nil {
			return errors.Annotatef(err, "updating labels for deployment %q", d.Name)
		}
	}

	return nil
}
