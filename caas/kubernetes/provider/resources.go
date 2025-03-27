// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/core/semversion"
	jujucontext "github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/tags"
)

// AdoptResources is called when the model is moved from one
// controller to another using model migration.
func (k *kubernetesClient) AdoptResources(ctx jujucontext.ProviderCallContext, controllerUUID string, fromVersion semversion.Number) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	modelLabel := fmt.Sprintf("%v==%v", tags.JujuModel, k.modelUUID)

	pods := k.client().CoreV1().Pods(k.namespace)
	podsList, err := pods.List(ctx, v1.ListOptions{
		LabelSelector: modelLabel,
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, p := range podsList.Items {
		p.Labels[tags.JujuController] = controllerUUID
		if _, err := pods.Update(ctx, &p, v1.UpdateOptions{}); err != nil {
			return errors.Annotatef(err, "updating labels for pod %q", p.Name)
		}
	}

	pvcs := k.client().CoreV1().PersistentVolumeClaims(k.namespace)
	pvcList, err := pvcs.List(ctx, v1.ListOptions{
		LabelSelector: modelLabel,
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, pvc := range pvcList.Items {
		pvc.Labels[tags.JujuController] = controllerUUID
		if _, err := pvcs.Update(ctx, &pvc, v1.UpdateOptions{}); err != nil {
			return errors.Annotatef(err, "updating labels for pvc %q", pvc.Name)
		}
	}

	pvs := k.client().CoreV1().PersistentVolumes()
	pvList, err := pvs.List(ctx, v1.ListOptions{
		LabelSelector: modelLabel,
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, pv := range pvList.Items {
		pv.Labels[tags.JujuController] = controllerUUID
		if _, err := pvs.Update(ctx, &pv, v1.UpdateOptions{}); err != nil {
			return errors.Annotatef(err, "updating labels for pvc %q", pv.Name)
		}
	}

	sSets := k.client().AppsV1().StatefulSets(k.namespace)
	ssList, err := sSets.List(ctx, v1.ListOptions{
		LabelSelector: modelLabel,
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, ss := range ssList.Items {
		ss.Labels[tags.JujuController] = controllerUUID
		if _, err := sSets.Update(ctx, &ss, v1.UpdateOptions{}); err != nil {
			return errors.Annotatef(err, "updating labels for stateful set %q", ss.Name)
		}
	}

	deployments := k.client().AppsV1().Deployments(k.namespace)
	dList, err := deployments.List(ctx, v1.ListOptions{
		LabelSelector: modelLabel,
	})
	if err != nil {
		return errors.Trace(err)
	}
	for _, d := range dList.Items {
		d.Labels[tags.JujuController] = controllerUUID
		if _, err := deployments.Update(ctx, &d, v1.UpdateOptions{}); err != nil {
			return errors.Annotatef(err, "updating labels for deployment %q", d.Name)
		}
	}

	return nil
}
