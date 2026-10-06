// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"testing"

	"github.com/juju/tc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
)

type controllerAddressServiceSuite struct{}

func TestControllerAddressServiceSuite(t *testing.T) {
	tc.Run(t, &controllerAddressServiceSuite{})
}

func (*controllerAddressServiceSuite) TestControllerServiceIdentity(c *tc.C) {
	client := fake.NewClientset()
	k := &kubernetesClient{
		namespace: "controller-test", modelName: "controller",
		clientUnlocked: client, labelVersion: constants.LastLabelVersion,
	}
	services := client.CoreV1().Services(k.namespace)
	for _, name := range []string{"controller", constants.ControllerServiceEndpointsName, constants.ControllerServiceName} {
		_, err := services.Create(c.Context(), &corev1.Service{
			Name: name, UID: types.UID(name + "-uid"),
			Labels: utils.LabelsForApp("controller", constants.LastLabelVersion),
			Spec:   corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, ClusterIP: "10.0.0.1"},
			Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{{IP: "192.0.2.1", Hostname: "api.example.com"}},
			}},
		}, metav1.CreateOptions{})
		c.Assert(err, tc.ErrorIsNil)
	}
	svc, err := k.GetService(c.Context(), "controller", true)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(svc.Id, tc.Equals, "controller-service-uid")
	c.Check(svc.Addresses, tc.DeepEquals, network.ProviderAddresses{
		network.NewMachineAddress("192.0.2.1", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		network.NewMachineAddress("api.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
	})
	// Losing the API Service must not cause either other Service to take its place.
	c.Assert(services.Delete(c.Context(), constants.ControllerServiceName, metav1.DeleteOptions{}), tc.ErrorIsNil)
	svc, err = k.GetService(c.Context(), "controller", true)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(svc.Id, tc.Equals, "")
	c.Check(svc.Addresses, tc.HasLen, 0)
}

func (s *controllerAddressServiceSuite) TestControllerNamedApplicationInWorkloadModel(c *tc.C) {
	s.assertApplicationServiceIdentity(c, "controller", "workload")
}

func (s *controllerAddressServiceSuite) TestOtherApplicationInControllerModel(c *tc.C) {
	s.assertApplicationServiceIdentity(c, "postgresql", "controller")
}

func (*controllerAddressServiceSuite) assertApplicationServiceIdentity(c *tc.C, appName, modelName string) {
	client := fake.NewClientset()
	k := &kubernetesClient{
		namespace: "test", modelName: modelName,
		clientUnlocked: client, labelVersion: constants.LastLabelVersion,
	}
	services := client.CoreV1().Services(k.namespace)
	_, err := services.Create(c.Context(), &corev1.Service{
		Name: appName, UID: "app-service-uid",
		Labels: utils.LabelsMerge(utils.LabelsForApp(appName, constants.LastLabelVersion), utils.LabelsJuju),
		Spec:   corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.2"},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)
	// A controller API Service must only supply addresses for the controller
	// application in the controller model, even when both Services exist.
	_, err = services.Create(c.Context(), &corev1.Service{
		Name: constants.ControllerServiceName, UID: "api-service-uid",
		Labels: utils.LabelsMerge(utils.LabelsForApp("controller", constants.LastLabelVersion), utils.LabelsJuju),
		Spec:   corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerIP: "192.0.2.1"},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	svc, err := k.GetService(c.Context(), appName, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(svc.Id, tc.Equals, "app-service-uid")
	c.Check(svc.Addresses, tc.DeepEquals, network.ProviderAddresses{
		network.NewMachineAddress("10.0.0.2", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
	})
}
