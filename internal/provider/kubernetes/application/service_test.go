// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/provider/kubernetes/application"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	k8swatcher "github.com/juju/juju/internal/provider/kubernetes/watcher"
)

type controllerServiceSuite struct{}

func TestControllerServiceSuite(t *testing.T) {
	tc.Run(t, &controllerServiceSuite{})
}

func (*controllerServiceSuite) TestServiceAddresses(c *tc.C) {
	client := fake.NewClientset(&appsv1.StatefulSet{
		Name: "controller", Namespace: "controller-test",
	})
	app := newControllerServiceApplication(client)
	services := client.CoreV1().Services("controller-test")
	// Neither the charm's default Service nor headless pod DNS is a source
	// of controller API Service addresses.
	for _, name := range []string{"controller", constants.ControllerServiceEndpointsName} {
		_, err := services.Create(c.Context(), &corev1.Service{
			Name: name,
			UID:  types.UID(name),
			Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.99"},
		}, metav1.CreateOptions{})
		c.Assert(err, tc.ErrorIsNil)
	}
	_, err := app.Service()
	c.Check(err, tc.ErrorIs, caas.ServiceNotFound)
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	_, err = services.Create(c.Context(), &corev1.Service{
		Name: constants.ControllerServiceName, UID: "api-service-uid",
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, ClusterIP: "10.0.0.1"},
		Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{{IP: "192.0.2.1", Hostname: "api.example.com"}},
		}},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)
	svc, err := app.Service()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(svc.Id, tc.Equals, "api-service-uid")
	c.Check(svc.Addresses, tc.DeepEquals, network.ProviderAddresses{
		network.NewMachineAddress("192.0.2.1", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		network.NewMachineAddress("api.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress(),
		network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
	})
}

func (*controllerServiceSuite) TestMissingWorkloadIsNotMissingService(c *tc.C) {
	client := fake.NewClientset(&corev1.Service{
		Name: constants.ControllerServiceName, Namespace: "controller-test", UID: "api-service-uid",
	})
	_, err := newControllerServiceApplication(client).Service()
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Check(errors.Is(err, caas.ServiceNotFound), tc.IsFalse)
}

func (*controllerServiceSuite) TestWatchServiceLifecycle(c *tc.C) {
	client := fake.NewClientset()
	app := newControllerServiceApplication(client)
	w, err := app.Watch(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
	_, ok := <-w.Changes()
	c.Assert(ok, tc.IsTrue)
	// All informers have subscribed before Watch returns. Check that the
	// workload and Service use their own identities, including initial lists.
	for _, action := range client.Actions() {
		var selector string
		switch action := action.(type) {
		case k8stesting.ListAction:
			selector = action.GetListRestrictions().Fields.String()
		case k8stesting.WatchAction:
			selector = action.GetWatchRestrictions().Fields.String()
		default:
			continue
		}
		switch action.GetResource().Resource {
		case "services":
			c.Check(selector, tc.Equals, "metadata.name=controller-service")
		case "statefulsets":
			c.Check(selector, tc.Equals, "metadata.name=controller,metadata.namespace=controller-test")
		}
	}
	services := client.CoreV1().Services("controller-test")
	svc, err := services.Create(c.Context(), &corev1.Service{
		Name: constants.ControllerServiceName, UID: "first",
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
	}, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)
	_, ok = <-w.Changes()
	c.Assert(ok, tc.IsTrue)
	svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{Hostname: "api.example.com"}}
	_, err = services.UpdateStatus(c.Context(), svc, metav1.UpdateOptions{})
	c.Assert(err, tc.ErrorIsNil)
	_, ok = <-w.Changes()
	c.Assert(ok, tc.IsTrue)
	c.Assert(services.Delete(c.Context(), svc.Name, metav1.DeleteOptions{}), tc.ErrorIsNil)
	_, ok = <-w.Changes()
	c.Assert(ok, tc.IsTrue)
	svc.UID = "replacement"
	_, err = services.Create(c.Context(), svc, metav1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)
	_, ok = <-w.Changes()
	c.Assert(ok, tc.IsTrue)
}

func (*controllerServiceSuite) TestWatchInitialServiceChange(c *tc.C) {
	client := fake.NewClientset()
	// Change the Service during the initial list, before the watch starts.
	// The initial notification must make this source visible to the reader.
	client.PrependReactor("list", "services", func(action k8stesting.Action) (bool, runtime.Object, error) {
		err := client.Tracker().Create(corev1.SchemeGroupVersion.WithResource("services"), &corev1.Service{
			Name: constants.ControllerServiceName, Namespace: "controller-test", UID: "initial",
		}, "controller-test")
		return false, nil, err
	})
	w, err := newControllerServiceApplication(client).Watch(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
	_, ok := <-w.Changes()
	c.Assert(ok, tc.IsTrue)
	svc, err := client.CoreV1().Services("controller-test").Get(c.Context(), constants.ControllerServiceName, metav1.GetOptions{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(svc.UID), tc.Equals, "initial")
}

func newControllerServiceApplication(client *fake.Clientset) caas.Application {
	return application.NewApplication("controller", "controller-test", "model-uuid", "controller",
		constants.LastLabelVersion, caas.DeploymentStateful, client, nil, nil,
		k8swatcher.NewKubernetesNotifyWatcher, clock.WallClock, "controller-uuid", func() bool { return true })
}
