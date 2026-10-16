// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/internal/provider/kubernetes/application"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

// controllerServiceSSHPortName is the name of the SSH server port on the
// controller Service.
const controllerServiceSSHPortName = "ssh-server"

var _ caas.ControllerServiceUpdater = (*kubernetesClient)(nil)

// UpdateControllerServiceSSHPort sets the SSH server port on the controller
// Service. The port is set at bootstrap, but the controller charm can change
// it after, so the Service has to follow it.
func (k *kubernetesClient) UpdateControllerServiceSSHPort(ctx context.Context, port int) error {
	if k.namespace == "" {
		return errNoNamespace
	}
	return updateControllerServiceSSHPort(ctx, k.client().CoreV1().Services(k.namespace), port)
}

func updateControllerServiceSSHPort(ctx context.Context, api corev1.ServiceInterface, port int) error {
	svc, err := api.Get(ctx, constants.ControllerServiceName, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return errors.NotFoundf("controller service %q", constants.ControllerServiceName)
	} else if err != nil {
		return errors.Trace(err)
	}

	want := core.ServicePort{
		Name:       controllerServiceSSHPortName,
		Port:       int32(port),
		TargetPort: intstr.FromInt(port),
	}
	found := false
	for i, p := range svc.Spec.Ports {
		if p.Name != controllerServiceSSHPortName {
			continue
		}
		found = true
		if p.Port == want.Port && p.TargetPort == want.TargetPort {
			return nil
		}
		svc.Spec.Ports[i].Port = want.Port
		svc.Spec.Ports[i].TargetPort = want.TargetPort
	}
	if !found {
		svc.Spec.Ports = append(svc.Spec.Ports, want)
	}

	_, err = api.Update(ctx, svc, meta.UpdateOptions{})
	return errors.Trace(err)
}

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
			cleanUp = func() {
				if err := k.deleteService(ctx, svcCreated.GetName()); err != nil {
					logger.Warningf(ctx,
						"could not clean up service %q, it may be left dangling: %v",
						svcCreated.GetName(), err)
				}
			}
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
	// We want to filter out the headless endpoints services made by juju as
	// they carry no routable address and should not be considered.
	for _, svc := range servicesList.Items {
		if !application.IsManagedHeadlessService(svc) {
			services = append(services, svc)
		}
	}

	if len(services) != 1 {
		return nil, errors.NotValidf("unable to handle multiple services %d for application %s", len(services), appName)
	}

	return &services[0], nil
}
