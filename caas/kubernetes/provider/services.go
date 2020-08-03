// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8sannotations "github.com/juju/juju/core/annotations"
)

func getServiceLabels(appName string) map[string]string {
	return utils.LabelsForApp(appName)
}

func (k *kubernetesClient) ensureServicesForApp(appName string, annotations k8sannotations.Annotation, services []k8sspecs.K8sService) (cleanUps []func(), err error) {
	for _, v := range services {
		spec := &core.Service{
			ObjectMeta: v1.ObjectMeta{
				Name:        v.Name,
				Namespace:   k.namespace,
				Labels:      k8slabels.Merge(v.Labels, getServiceLabels(appName)),
				Annotations: annotations.Copy().Merge(v.Annotations),
			},
			Spec: v.Spec,
		}
		serviceCleanup, err := k.ensureK8sService(spec)
		cleanUps = append(cleanUps, serviceCleanup)
		if err != nil {
			return cleanUps, errors.Trace(err)
		}
	}
	return cleanUps, nil
}

// ensureK8sService ensures a k8s service resource.
func (k *kubernetesClient) ensureK8sService(spec *core.Service) (func(), error) {
	cleanUp := func() {}

	api := k.client().CoreV1().Services(k.namespace)
	// Set any immutable fields if the service already exists.
	existing, err := api.Get(context.TODO(), spec.Name, v1.GetOptions{})
	if err == nil {
		spec.Spec.ClusterIP = existing.Spec.ClusterIP
		spec.ObjectMeta.ResourceVersion = existing.ObjectMeta.ResourceVersion
	}
	_, err = api.Update(context.TODO(), spec, v1.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		var svcCreated *core.Service
		svcCreated, err = api.Create(context.TODO(), spec, v1.CreateOptions{})
		if err == nil {
			cleanUp = func() { _ = k.deleteService(svcCreated.GetName()) }
		}
	}
	return cleanUp, errors.Trace(err)
}

// deleteService deletes a service resource.
func (k *kubernetesClient) deleteService(serviceName string) error {
	services := k.client().CoreV1().Services(k.namespace)
	err := services.Delete(context.TODO(), serviceName, v1.DeleteOptions{
		PropagationPolicy: &constants.DefaultPropagationPolicy,
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteServices(appName string) error {
	// Service API does not have `DeleteCollection` implemented, so we have to do it like this.
	api := k.client().CoreV1().Services(k.namespace)
	services, err := api.List(context.TODO(),
		v1.ListOptions{
			LabelSelector: utils.LabelSetToSelector(getServiceLabels(appName)).String(),
		},
	)
	if err != nil {
		return errors.Trace(err)
	}
	for _, svc := range services.Items {
		if err := k.deleteService(svc.GetName()); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return errors.Trace(err)
		}
	}
	return nil
}
