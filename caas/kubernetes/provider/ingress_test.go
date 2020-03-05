// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/testing"
)

func (s *K8sBrokerSuite) assertIngressResources(c *gc.C, IngressResources []k8sspecs.K8sIngressSpec, expectedErrString string, assertCalls ...*gomock.Call) {
	basicPodSpec := getBasicPodspec()
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			IngressResources: IngressResources,
		},
	}
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec, "operator/image-path")
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)

	numUnits := int32(2)
	statefulSetArg := &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"juju-app": "app-name"},
			Annotations: map[string]string{
				"juju-app-uuid":      "appuuid",
				"juju.io/controller": testing.ControllerTag.Id(),
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			RevisionHistoryLimit: int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"juju-app": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"juju.io/controller":                       testing.ControllerTag.Id(),
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
		[]*gomock.Call{
			s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
		},
		assertCalls...,
	)

	ociImageSecret := s.getOCIImageSecret(c, nil)
	if expectedErrString == "" {
		// no error expected, so continue to check following assertions.
		assertCalls = append(assertCalls, []*gomock.Call{
			s.mockSecrets.EXPECT().Create(ociImageSecret).
				Return(ociImageSecret, nil),
			s.mockServices.EXPECT().Get("app-name", v1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
			s.mockServices.EXPECT().Update(&serviceArg).
				Return(nil, s.k8sNotFoundError()),
			s.mockServices.EXPECT().Create(&serviceArg).
				Return(nil, nil),
			s.mockServices.EXPECT().Get("app-name-endpoints", v1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
			s.mockServices.EXPECT().Update(basicHeadlessServiceArg).
				Return(nil, s.k8sNotFoundError()),
			s.mockServices.EXPECT().Create(basicHeadlessServiceArg).
				Return(nil, nil),
			s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{}).
				Return(statefulSetArg, nil),
			s.mockStatefulSets.EXPECT().Create(statefulSetArg).
				Return(nil, nil),
			s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{}).
				Return(statefulSetArg, nil),
			s.mockStatefulSets.EXPECT().Update(statefulSetArg).
				Return(nil, nil),
		}...)
	}
	gomock.InOrder(assertCalls...)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateful,
		},
		OperatorImagePath: "operator/image-path",
		ResourceTags:      map[string]string{"juju-controller-uuid": testing.ControllerTag.Id()},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, e string, _ map[string]interface{}) error {
		c.Logf("EnsureService error -> %q", e)
		return nil
	}, params, 2, application.ConfigAttributes{
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	if expectedErrString != "" {
		c.Assert(err, gc.ErrorMatches, expectedErrString)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *K8sBrokerSuite) TestEnsureServiceIngressResourcesCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ingress1Rule1 := extensionsv1beta1.IngressRule{
		IngressRuleValue: extensionsv1beta1.IngressRuleValue{
			HTTP: &extensionsv1beta1.HTTPIngressRuleValue{
				Paths: []extensionsv1beta1.HTTPIngressPath{
					{
						Path: "/testpath",
						Backend: extensionsv1beta1.IngressBackend{
							ServiceName: "test",
							ServicePort: intstr.IntOrString{IntVal: 80},
						},
					},
				},
			},
		},
	}
	ingress1 := k8sspecs.K8sIngressSpec{
		Name: "test-ingress",
		Labels: map[string]string{
			"foo": "bar",
		},
		Annotations: map[string]string{
			"nginx.ingress.kubernetes.io/rewrite-target": "/",
		},
		Spec: extensionsv1beta1.IngressSpec{
			Rules: []extensionsv1beta1.IngressRule{ingress1Rule1},
		},
	}

	IngressResources := []k8sspecs.K8sIngressSpec{ingress1}
	ingress := &extensionsv1beta1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-ingress",
			Labels: map[string]string{
				"foo":      "bar",
				"juju-app": "app-name",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
				"juju.io/controller":                         "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			},
		},
		Spec: extensionsv1beta1.IngressSpec{
			Rules: []extensionsv1beta1.IngressRule{ingress1Rule1},
		},
	}
	s.assertIngressResources(
		c, IngressResources, "",
		s.mockIngressInterface.EXPECT().Create(ingress).Return(ingress, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceIngressResourcesUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ingress1Rule1 := extensionsv1beta1.IngressRule{
		IngressRuleValue: extensionsv1beta1.IngressRuleValue{
			HTTP: &extensionsv1beta1.HTTPIngressRuleValue{
				Paths: []extensionsv1beta1.HTTPIngressPath{
					{
						Path: "/testpath",
						Backend: extensionsv1beta1.IngressBackend{
							ServiceName: "test",
							ServicePort: intstr.IntOrString{IntVal: 80},
						},
					},
				},
			},
		},
	}
	ingress1 := k8sspecs.K8sIngressSpec{
		Name: "test-ingress",
		Labels: map[string]string{
			"foo": "bar",
		},
		Annotations: map[string]string{
			"nginx.ingress.kubernetes.io/rewrite-target": "/",
		},
		Spec: extensionsv1beta1.IngressSpec{
			Rules: []extensionsv1beta1.IngressRule{ingress1Rule1},
		},
	}

	IngressResources := []k8sspecs.K8sIngressSpec{ingress1}
	ingress := &extensionsv1beta1.Ingress{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-ingress",
			Labels: map[string]string{
				"foo":      "bar",
				"juju-app": "app-name",
			},
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
				"juju.io/controller":                         "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			},
		},
		Spec: extensionsv1beta1.IngressSpec{
			Rules: []extensionsv1beta1.IngressRule{ingress1Rule1},
		},
	}
	s.assertIngressResources(
		c, IngressResources, "",
		s.mockIngressInterface.EXPECT().Create(ingress).Return(nil, s.k8sAlreadyExistsError()),
		s.mockIngressInterface.EXPECT().Get("test-ingress", v1.GetOptions{}).Return(ingress, nil),
		s.mockIngressInterface.EXPECT().Update(ingress).Return(ingress, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceIngressResourcesUpdateConflictWithExistingNonJujuManagedIngress(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ingress1Rule1 := extensionsv1beta1.IngressRule{
		IngressRuleValue: extensionsv1beta1.IngressRuleValue{
			HTTP: &extensionsv1beta1.HTTPIngressRuleValue{
				Paths: []extensionsv1beta1.HTTPIngressPath{
					{
						Path: "/testpath",
						Backend: extensionsv1beta1.IngressBackend{
							ServiceName: "test",
							ServicePort: intstr.IntOrString{IntVal: 80},
						},
					},
				},
			},
		},
	}
	ingress1 := k8sspecs.K8sIngressSpec{
		Name: "test-ingress",
		Labels: map[string]string{
			"foo": "bar",
		},
		Annotations: map[string]string{
			"nginx.ingress.kubernetes.io/rewrite-target": "/",
		},
		Spec: extensionsv1beta1.IngressSpec{
			Rules: []extensionsv1beta1.IngressRule{ingress1Rule1},
		},
	}

	IngressResources := []k8sspecs.K8sIngressSpec{ingress1}

	getIngress := func() *extensionsv1beta1.Ingress {
		return &extensionsv1beta1.Ingress{
			ObjectMeta: v1.ObjectMeta{
				Name: "test-ingress",
				Labels: map[string]string{
					"foo":      "bar",
					"juju-app": "app-name",
				},
				Annotations: map[string]string{
					"nginx.ingress.kubernetes.io/rewrite-target": "/",
					"juju.io/controller":                         "deadbeef-1bad-500d-9000-4b1d0d06f00d",
				},
			},
			Spec: extensionsv1beta1.IngressSpec{
				Rules: []extensionsv1beta1.IngressRule{ingress1Rule1},
			},
		}
	}
	ingress := getIngress()
	existingNonJujuManagedIngress := getIngress()
	existingNonJujuManagedIngress.SetLabels(map[string]string{})
	s.assertIngressResources(
		c, IngressResources, `creating or updating ingress resources: existing ingress "test-ingress" found which does not belong to "app-name"`,
		s.mockIngressInterface.EXPECT().Create(ingress).Return(nil, s.k8sAlreadyExistsError()),
		s.mockIngressInterface.EXPECT().Get("test-ingress", v1.GetOptions{}).Return(existingNonJujuManagedIngress, nil),
	)
}
