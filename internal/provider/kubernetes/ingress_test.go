// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/testing"
)

func (s *K8sBrokerSuite) assertIngressResources(c *gc.C, ingressResources []k8sspecs.K8sIngress, expectedErrString string, assertCalls ...any) {
	basicPodSpec := getBasicPodspec()
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			IngressResources: ingressResources,
		},
	}
	workloadSpec, err := provider.PrepareWorkloadSpec(
		"app-name", "app-name", basicPodSpec, resources.DockerImageDetails{RegistryPath: "operator/image-path"},
	)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.Pod(workloadSpec).PodSpec

	numUnits := int32(2)
	statefulSetArg := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"app.juju.is/uuid":               "appuuid",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"charm.juju.is/modified-version": "0",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			RevisionHistoryLimit: pointer.Int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: podSpec,
			},
			PodManagementPolicy: apps.ParallelPodManagement,
			ServiceName:         "app-name-endpoints",
		},
	}

	serviceArg := *basicServiceArg
	serviceArg.Spec.Type = core.ServiceTypeClusterIP

	assertCalls = append(
		[]any{
			s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", metav1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
		},
		assertCalls...,
	)

	ociImageSecret := s.getOCIImageSecret(c, nil)
	if expectedErrString == "" {
		// no error expected, so continue to check following assertions.
		assertCalls = append(assertCalls, []any{
			s.mockSecrets.EXPECT().Create(gomock.Any(), ociImageSecret, metav1.CreateOptions{}).
				Return(ociImageSecret, nil),
			s.mockServices.EXPECT().Get(gomock.Any(), "app-name", metav1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
			s.mockServices.EXPECT().Update(gomock.Any(), &serviceArg, metav1.UpdateOptions{}).
				Return(nil, s.k8sNotFoundError()),
			s.mockServices.EXPECT().Create(gomock.Any(), &serviceArg, metav1.CreateOptions{}).
				Return(nil, nil),
			s.mockServices.EXPECT().Get(gomock.Any(), "app-name-endpoints", metav1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
			s.mockServices.EXPECT().Update(gomock.Any(), basicHeadlessServiceArg, metav1.UpdateOptions{}).
				Return(nil, s.k8sNotFoundError()),
			s.mockServices.EXPECT().Create(gomock.Any(), basicHeadlessServiceArg, metav1.CreateOptions{}).
				Return(nil, nil),
			s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", metav1.GetOptions{}).
				Return(statefulSetArg, nil),
			s.mockStatefulSets.EXPECT().Create(gomock.Any(), statefulSetArg, metav1.CreateOptions{}).
				Return(nil, nil),
		}...)
	}
	gomock.InOrder(assertCalls...)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateful,
		},
		ImageDetails: resources.DockerImageDetails{RegistryPath: "operator/image-path"},
		ResourceTags: map[string]string{"juju-controller-uuid": testing.ControllerTag.Id()},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, e string, _ map[string]interface{}) error {
		c.Logf("EnsureService error -> %q", e)
		return nil
	}, params, 2, config.ConfigAttributes{
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	if expectedErrString != "" {
		c.Assert(err, gc.ErrorMatches, expectedErrString)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *K8sBrokerSuite) TestEnsureServiceIngressResourcesCreateV1Beta1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "18",
		}, nil),
	)

	ingress1Rule1 := networkingv1beta1.IngressRule{
		IngressRuleValue: networkingv1beta1.IngressRuleValue{
			HTTP: &networkingv1beta1.HTTPIngressRuleValue{
				Paths: []networkingv1beta1.HTTPIngressPath{
					{
						Path: "/testpath",
						Backend: networkingv1beta1.IngressBackend{
							ServiceName: "test",
							ServicePort: intstr.IntOrString{IntVal: 80},
						},
					},
				},
			},
		},
	}
	ingress1 := k8sspecs.K8sIngress{
		Meta: k8sspecs.Meta{
			Name: "test-ingress",
			Labels: map[string]string{
				"foo": "bar",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
		},
		Spec: k8sspecs.K8sIngressSpec{
			Version: k8sspecs.K8sIngressV1Beta1,
			SpecV1Beta1: networkingv1beta1.IngressSpec{
				Rules: []networkingv1beta1.IngressRule{ingress1Rule1},
			},
		},
	}

	ingressResources := []k8sspecs.K8sIngress{ingress1}
	ingress := &networkingv1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-ingress",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "foo": "bar"},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
				"controller.juju.is/id":                      "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			},
		},
		Spec: networkingv1beta1.IngressSpec{
			Rules: []networkingv1beta1.IngressRule{ingress1Rule1},
		},
	}
	s.assertIngressResources(
		c, ingressResources, "",
		s.mockIngressV1Beta1.EXPECT().Create(gomock.Any(), ingress, metav1.CreateOptions{}).Return(ingress, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceIngressResourcesUpdateV1Beta1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "18",
		}, nil),
	)

	ingress1Rule1 := networkingv1beta1.IngressRule{
		IngressRuleValue: networkingv1beta1.IngressRuleValue{
			HTTP: &networkingv1beta1.HTTPIngressRuleValue{
				Paths: []networkingv1beta1.HTTPIngressPath{
					{
						Path: "/testpath",
						Backend: networkingv1beta1.IngressBackend{
							ServiceName: "test",
							ServicePort: intstr.IntOrString{IntVal: 80},
						},
					},
				},
			},
		},
	}
	ingress1 := k8sspecs.K8sIngress{
		Meta: k8sspecs.Meta{
			Name: "test-ingress",
			Labels: map[string]string{
				"foo": "bar",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
		},
		Spec: k8sspecs.K8sIngressSpec{
			Version: k8sspecs.K8sIngressV1Beta1,
			SpecV1Beta1: networkingv1beta1.IngressSpec{
				Rules: []networkingv1beta1.IngressRule{ingress1Rule1},
			},
		},
	}

	ingressResources := []k8sspecs.K8sIngress{ingress1}
	ingress := &networkingv1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-ingress",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "foo": "bar"},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
				"controller.juju.is/id":                      "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			},
		},
		Spec: networkingv1beta1.IngressSpec{
			Rules: []networkingv1beta1.IngressRule{ingress1Rule1},
		},
	}
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, ingress)
	c.Assert(err, jc.ErrorIsNil)
	s.assertIngressResources(
		c, ingressResources, "",
		s.mockIngressV1Beta1.EXPECT().Create(gomock.Any(), ingress, metav1.CreateOptions{}).Return(nil, s.k8sAlreadyExistsError()),
		s.mockIngressV1Beta1.EXPECT().Get(gomock.Any(), "test-ingress", metav1.GetOptions{}).Return(ingress, nil),
		s.mockIngressV1Beta1.EXPECT().
			Patch(gomock.Any(), ingress.GetName(), k8stypes.StrategicMergePatchType, data, metav1.PatchOptions{FieldManager: "juju"}).
			Return(ingress, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceIngressResourcesUpdateConflictWithExistingNonJujuManagedIngressV1Beta1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "18",
		}, nil),
	)

	ingress1Rule1 := networkingv1beta1.IngressRule{
		IngressRuleValue: networkingv1beta1.IngressRuleValue{
			HTTP: &networkingv1beta1.HTTPIngressRuleValue{
				Paths: []networkingv1beta1.HTTPIngressPath{
					{
						Path: "/testpath",
						Backend: networkingv1beta1.IngressBackend{
							ServiceName: "test",
							ServicePort: intstr.IntOrString{IntVal: 80},
						},
					},
				},
			},
		},
	}
	ingress1 := k8sspecs.K8sIngress{
		Meta: k8sspecs.Meta{
			Name: "test-ingress",
			Labels: map[string]string{
				"foo": "bar",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
		},

		Spec: k8sspecs.K8sIngressSpec{
			Version: k8sspecs.K8sIngressV1Beta1,
			SpecV1Beta1: networkingv1beta1.IngressSpec{
				Rules: []networkingv1beta1.IngressRule{ingress1Rule1},
			},
		},
	}

	ingressResources := []k8sspecs.K8sIngress{ingress1}

	getIngress := func() *networkingv1beta1.Ingress {
		return &networkingv1beta1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-ingress",
				Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "foo": "bar"},
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/rewrite-target": "/",
					"controller.juju.is/id":                      "deadbeef-1bad-500d-9000-4b1d0d06f00d",
				},
			},
			Spec: networkingv1beta1.IngressSpec{
				Rules: []networkingv1beta1.IngressRule{ingress1Rule1},
			},
		}
	}
	ingress := getIngress()
	existingNonJujuManagedIngress := getIngress()
	existingNonJujuManagedIngress.SetLabels(map[string]string{})
	s.assertIngressResources(
		c, ingressResources, `creating or updating ingress resources: ensuring ingress "test-ingress" with version "v1beta1": existing ingress "test-ingress" found which does not belong to "app-name"`,
		s.mockIngressV1Beta1.EXPECT().Create(gomock.Any(), ingress, gomock.Any()).Return(nil, s.k8sAlreadyExistsError()),
		s.mockIngressV1Beta1.EXPECT().Get(gomock.Any(), "test-ingress", metav1.GetOptions{}).Return(existingNonJujuManagedIngress, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceIngressResourcesCreateV1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "22",
		}, nil),
	)

	ingress1Rule1 := networkingv1.IngressRule{
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{
					{
						Path: "/testpath",
						Backend: networkingv1.IngressBackend{
							Resource: &core.TypedLocalObjectReference{
								APIGroup: pointer.StringPtr("k8s.example.com"),
								Kind:     "StorageBucket",
								Name:     "icon-assets",
							},
						},
					},
				},
			},
		},
	}
	ingress1 := k8sspecs.K8sIngress{
		Meta: k8sspecs.Meta{
			Name: "test-ingress",
			Labels: map[string]string{
				"foo": "bar",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
		},
		Spec: k8sspecs.K8sIngressSpec{
			Version: k8sspecs.K8sIngressV1,
			SpecV1: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{ingress1Rule1},
			},
		},
	}

	ingressResources := []k8sspecs.K8sIngress{ingress1}
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ingress",
			Labels: map[string]string{
				"foo":                          "bar",
				"app.kubernetes.io/name":       "app-name",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
				"controller.juju.is/id":                      "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{ingress1Rule1},
		},
	}
	s.assertIngressResources(
		c, ingressResources, "",
		s.mockIngressV1.EXPECT().Create(gomock.Any(), ingress, gomock.Any()).Return(ingress, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceIngressResourcesUpdateV1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "22",
		}, nil),
	)

	ingress1Rule1 := networkingv1.IngressRule{
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{
					{
						Path: "/testpath",
						Backend: networkingv1.IngressBackend{
							Resource: &core.TypedLocalObjectReference{
								APIGroup: pointer.StringPtr("k8s.example.com"),
								Kind:     "StorageBucket",
								Name:     "icon-assets",
							},
						},
					},
				},
			},
		},
	}
	ingress1 := k8sspecs.K8sIngress{
		Meta: k8sspecs.Meta{
			Name: "test-ingress",
			Labels: map[string]string{
				"foo": "bar",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
		},
		Spec: k8sspecs.K8sIngressSpec{
			Version: k8sspecs.K8sIngressV1,
			SpecV1: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{ingress1Rule1},
			},
		},
	}

	ingressResources := []k8sspecs.K8sIngress{ingress1}
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ingress",
			Labels: map[string]string{
				"foo":                          "bar",
				"app.kubernetes.io/name":       "app-name",
				"app.kubernetes.io/managed-by": "juju",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
				"controller.juju.is/id":                      "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{ingress1Rule1},
		},
	}
	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, ingress)
	c.Assert(err, jc.ErrorIsNil)
	s.assertIngressResources(
		c, ingressResources, "",
		s.mockIngressV1.EXPECT().Create(gomock.Any(), ingress, gomock.Any()).Return(nil, s.k8sAlreadyExistsError()),
		s.mockIngressV1.EXPECT().Get(gomock.Any(), "test-ingress", metav1.GetOptions{}).Return(ingress, nil),
		s.mockIngressV1.EXPECT().
			Patch(gomock.Any(), ingress.GetName(), k8stypes.StrategicMergePatchType, data, metav1.PatchOptions{FieldManager: "juju"}).
			Return(ingress, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceIngressResourcesUpdateConflictWithExistingNonJujuManagedIngressV1(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "22",
		}, nil),
	)

	ingress1Rule1 := networkingv1.IngressRule{
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{
					{
						Path: "/testpath",
						Backend: networkingv1.IngressBackend{
							Resource: &core.TypedLocalObjectReference{
								APIGroup: pointer.StringPtr("k8s.example.com"),
								Kind:     "StorageBucket",
								Name:     "icon-assets",
							},
						},
					},
				},
			},
		},
	}
	ingress1 := k8sspecs.K8sIngress{
		Meta: k8sspecs.Meta{
			Name: "test-ingress",
			Labels: map[string]string{
				"foo": "bar",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
		},

		Spec: k8sspecs.K8sIngressSpec{
			Version: k8sspecs.K8sIngressV1,
			SpecV1: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{ingress1Rule1},
			},
		},
	}

	ingressResources := []k8sspecs.K8sIngress{ingress1}

	getIngress := func() *networkingv1.Ingress {
		return &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-ingress",
				Labels: map[string]string{
					"foo":                          "bar",
					"app.kubernetes.io/name":       "app-name",
					"app.kubernetes.io/managed-by": "juju",
				},
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/rewrite-target": "/",
					"controller.juju.is/id":                      "deadbeef-1bad-500d-9000-4b1d0d06f00d",
				},
			},
			Spec: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{ingress1Rule1},
			},
		}
	}
	ingress := getIngress()
	existingNonJujuManagedIngress := getIngress()
	existingNonJujuManagedIngress.SetLabels(map[string]string{})
	s.assertIngressResources(
		c, ingressResources, `creating or updating ingress resources: ensuring ingress "test-ingress" with version "v1": existing ingress "test-ingress" found which does not belong to "app-name"`,
		s.mockIngressV1.EXPECT().Create(gomock.Any(), ingress, gomock.Any()).Return(nil, s.k8sAlreadyExistsError()),
		s.mockIngressV1.EXPECT().Get(gomock.Any(), "test-ingress", metav1.GetOptions{}).Return(existingNonJujuManagedIngress, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceIngressResourcesUpdateConflictWithIngressCreatedByJujuExpose(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "22",
		}, nil),
	)

	ingress1Rule1 := networkingv1beta1.IngressRule{
		IngressRuleValue: networkingv1beta1.IngressRuleValue{
			HTTP: &networkingv1beta1.HTTPIngressRuleValue{
				Paths: []networkingv1beta1.HTTPIngressPath{
					{
						Path: "/testpath",
						Backend: networkingv1beta1.IngressBackend{
							ServiceName: "test",
							ServicePort: intstr.IntOrString{IntVal: 80},
						},
					},
				},
			},
		},
	}
	ingress1 := k8sspecs.K8sIngress{
		Meta: k8sspecs.Meta{
			Name: "app-name",
			Labels: map[string]string{
				"foo": "bar",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
		},
		Spec: k8sspecs.K8sIngressSpec{
			Version: k8sspecs.K8sIngressV1Beta1,
			SpecV1Beta1: networkingv1beta1.IngressSpec{
				Rules: []networkingv1beta1.IngressRule{ingress1Rule1},
			},
		},
	}

	ingressResources := []k8sspecs.K8sIngress{ingress1}
	s.assertIngressResources(
		c, ingressResources, `creating or updating ingress resources: ingress name "app-name" is reserved for juju expose not valid`,
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceIngressResourcesV1Beta1OnV1Cluster(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "19",
		}, nil),
	)

	ingress1Rule1 := networkingv1beta1.IngressRule{
		IngressRuleValue: networkingv1beta1.IngressRuleValue{
			HTTP: &networkingv1beta1.HTTPIngressRuleValue{
				Paths: []networkingv1beta1.HTTPIngressPath{
					{
						Path: "/testpath",
						Backend: networkingv1beta1.IngressBackend{
							ServiceName: "test",
							ServicePort: intstr.IntOrString{IntVal: 80},
						},
					},
				},
			},
		},
	}
	ingress1 := k8sspecs.K8sIngress{
		Meta: k8sspecs.Meta{
			Name: "test-ingress",
		},
		Spec: k8sspecs.K8sIngressSpec{
			Version: k8sspecs.K8sIngressV1Beta1,
			SpecV1Beta1: networkingv1beta1.IngressSpec{
				Rules: []networkingv1beta1.IngressRule{ingress1Rule1},
			},
		},
	}

	pathType := networkingv1.PathTypeImplementationSpecific
	ingressResources := []k8sspecs.K8sIngress{ingress1}
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-ingress",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id": "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     "/testpath",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "test",
										Port: networkingv1.ServiceBackendPort{Number: 80},
									},
								},
							},
						},
					},
				},
			}},
		},
	}
	s.assertIngressResources(
		c, ingressResources, "",
		s.mockIngressV1.EXPECT().Create(gomock.Any(), ingress, metav1.CreateOptions{}).Return(ingress, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceIngressResourcesV1OnV1BetaCluster(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "18",
		}, nil),
	)

	ingress1Rule1 := networkingv1.IngressRule{
		IngressRuleValue: networkingv1.IngressRuleValue{
			HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{
					{
						Path: "/testpath",
						Backend: networkingv1.IngressBackend{
							Resource: &core.TypedLocalObjectReference{
								APIGroup: pointer.StringPtr("k8s.example.com"),
								Kind:     "StorageBucket",
								Name:     "icon-assets",
							},
						},
					},
				},
			},
		},
	}
	ingress1 := k8sspecs.K8sIngress{
		Meta: k8sspecs.Meta{
			Name: "test-ingress",
		},
		Spec: k8sspecs.K8sIngressSpec{
			Version: k8sspecs.K8sIngressV1,
			SpecV1: networkingv1.IngressSpec{
				Rules: []networkingv1.IngressRule{ingress1Rule1},
			},
		},
	}

	ingressResources := []k8sspecs.K8sIngress{ingress1}
	ingress := &networkingv1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-ingress",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"controller.juju.is/id": "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			},
		},
		Spec: networkingv1beta1.IngressSpec{
			Rules: []networkingv1beta1.IngressRule{{
				IngressRuleValue: networkingv1beta1.IngressRuleValue{
					HTTP: &networkingv1beta1.HTTPIngressRuleValue{
						Paths: []networkingv1beta1.HTTPIngressPath{
							{
								Path: "/testpath",
								Backend: networkingv1beta1.IngressBackend{
									Resource: &core.TypedLocalObjectReference{
										APIGroup: pointer.StringPtr("k8s.example.com"),
										Kind:     "StorageBucket",
										Name:     "icon-assets",
									},
								},
							},
						},
					},
				},
			}},
		},
	}
	s.assertIngressResources(
		c, ingressResources, "",
		s.mockIngressV1Beta1.EXPECT().Create(gomock.Any(), ingress, metav1.CreateOptions{}).Return(ingress, nil),
	)
}
