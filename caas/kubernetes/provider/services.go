// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/caas/kubernetes/provider/constants"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/kubernetes/provider/utils"
	k8sannotations "github.com/juju/juju/core/annotations"
)

func getServiceLabels(appName string, legacy bool) map[string]string {
	return utils.LabelsForApp(appName, legacy)
}

func (k *kubernetesClient) ensureServicesForApp(appName, deploymentName string, annotations k8sannotations.Annotation, services []k8sspecs.K8sService) (cleanUps []func(), err error) {
	for _, v := range services {
		if v.Name == deploymentName {
			return cleanUps, errors.NewNotValid(nil, fmt.Sprintf("%q is a reserved service name", deploymentName))
		}
		spec := &core.Service{
			ObjectMeta: meta.ObjectMeta{
				Name:        v.Name,
				Namespace:   k.namespace,
				Labels:      utils.LabelsMerge(v.Labels, getServiceLabels(appName, k.IsLegacyLabels())),
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
	if k.namespace == "" {
		return cleanUp, errNoNamespace
	}

	api := k.client().CoreV1().Services(k.namespace)
	// Set any immutable fields if the service already exists.
	existing, err := api.Get(context.TODO(), spec.Name, meta.GetOptions{})
	if err == nil {
		spec.Spec.ClusterIP = existing.Spec.ClusterIP
		spec.ObjectMeta.ResourceVersion = existing.ObjectMeta.ResourceVersion
	}
	_, err = api.Update(context.TODO(), spec, meta.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		var svcCreated *core.Service
		svcCreated, err = api.Create(context.TODO(), spec, meta.CreateOptions{})
		if err == nil {
			cleanUp = func() { _ = k.deleteService(svcCreated.GetName()) }
		}
	}
	return cleanUp, errors.Trace(err)
}

// deleteService deletes a service resource.
func (k *kubernetesClient) deleteService(serviceName string) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	services := k.client().CoreV1().Services(k.namespace)
	err := services.Delete(context.TODO(), serviceName, meta.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (k *kubernetesClient) deleteServices(appName string) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	// Service API does not have `DeleteCollection` implemented, so we have to do it like this.
	api := k.client().CoreV1().Services(k.namespace)
	services, err := api.List(context.TODO(),
		meta.ListOptions{
			LabelSelector: utils.LabelsToSelector(
				getServiceLabels(appName, k.IsLegacyLabels())).String(),
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

func (k *kubernetesClient) findServiceForApplication(appName string) (*core.Service, error) {
	labels := utils.LabelsForApp(appName, k.IsLegacyLabels())
	servicesList, err := k.client().CoreV1().Services(k.namespace).List(context.TODO(), meta.ListOptions{
		LabelSelector: utils.LabelsToSelector(labels).String(),
	})

	if err != nil {
		return nil, errors.Annotatef(err, "finding service for application %s", appName)
	}

	if len(servicesList.Items) == 0 {
		return nil, errors.NotFoundf("finding service for application %s", appName)
	} else if len(servicesList.Items) != 1 {
		return nil, errors.NotValidf("unable to handle mutiple services %d for application %s", len(servicesList.Items), appName)
	}

	return &servicesList.Items[0], nil
}
