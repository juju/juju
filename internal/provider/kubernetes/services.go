// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/juju/juju/internal/provider/kubernetes/application"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

// ensureK8sService ensures a k8s service resource.
func (k *kubernetesClient) ensureK8sService(ctx context.Context, spec *core.Service) (func(), error) {
	cleanUp := func() {}
	if k.namespace == "" {
		return cleanUp, errNoNamespace
	}

	api := k.client().CoreV1().Services(k.namespace)
	// Set any immutable fields if the service already exists.
	existing, err := api.Get(ctx, spec.Name, meta.GetOptions{})
	if err == nil {
		spec.Spec.ClusterIP = existing.Spec.ClusterIP
		spec.ObjectMeta.ResourceVersion = existing.ObjectMeta.ResourceVersion
	}
	_, err = api.Update(ctx, spec, meta.UpdateOptions{})
	if k8serrors.IsNotFound(err) {
		var svcCreated *core.Service
		svcCreated, err = api.Create(ctx, spec, meta.CreateOptions{})
		if err == nil {
			cleanUp = func() { _ = k.deleteService(ctx, svcCreated.GetName()) }
		}
	}
	return cleanUp, errors.Trace(err)
}

// deleteService deletes a service resource.
func (k *kubernetesClient) deleteService(ctx context.Context, serviceName string) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	services := k.client().CoreV1().Services(k.namespace)
	err := services.Delete(ctx, serviceName, meta.DeleteOptions{
		PropagationPolicy: constants.DefaultPropagationPolicy(),
	})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func findServiceForApplication(
	ctx context.Context,
	serviceI corev1.ServiceInterface,
	appName string,
	labelVersion constants.LabelVersion,
) (*core.Service, error) {
	labels := utils.LabelsForApp(appName, labelVersion)
	servicesList, err := serviceI.List(ctx, meta.ListOptions{
		LabelSelector: utils.LabelsToSelector(labels).String(),
	})

	if err != nil {
		return nil, errors.Annotatef(err, "finding service for application %s", appName)
	}

	if len(servicesList.Items) == 0 {
		return nil, errors.NotFoundf("finding service for application %s", appName)
	}

	services := []core.Service{}
	endpointSvcName := application.HeadlessServiceName(appName)
	// We want to filter out the endpoints services made by juju as they should
	// not be considered.
	for _, svc := range servicesList.Items {
		if svc.Name != endpointSvcName {
			services = append(services, svc)
		}
	}

	if len(services) != 1 {
		return nil, errors.NotValidf("unable to handle mutiple services %d for application %s", len(servicesList.Items), appName)
	}

	return &services[0], nil
}
