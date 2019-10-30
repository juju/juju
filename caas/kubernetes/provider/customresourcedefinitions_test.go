// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	// "strings"
	// "time"

	"github.com/golang/mock/gomock"
	// "github.com/juju/clock/testclock"
	// "github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/fields"
	// "k8s.io/apimachinery/pkg/runtime"
	// "k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	// k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	// "k8s.io/apimachinery/pkg/watch"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	// "github.com/juju/juju/caas/specs"
	"github.com/juju/juju/core/application"
	// "github.com/juju/juju/core/constraints"
	// "github.com/juju/juju/core/devices"
	// "github.com/juju/juju/core/status"
	// "github.com/juju/juju/environs"
	// "github.com/juju/juju/environs/context"
	// envtesting "github.com/juju/juju/environs/testing"
	// "github.com/juju/juju/storage"
	// "github.com/juju/juju/testing"
)

func (s *K8sBrokerSuite) assertCustomerResourceDefinitions(c *gc.C, crds map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec, assertCalls ...*gomock.Call) {

	basicPodSpec := getBasicPodspec()
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			CustomResourceDefinitions: crds,
		},
	}
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec, "operator/image-path")
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)

	numUnits := int32(2)
	statefulSetArg := &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: "app-name",
			Annotations: map[string]string{
				"juju-app-uuid": "appuuid",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"juju-app": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
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
			s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).
				Return(nil, s.k8sNotFoundError()),
		},
		assertCalls...,
	)

	ociImageSecret := s.getOCIImageSecret(c, nil)
	assertCalls = append(assertCalls, []*gomock.Call{
		s.mockSecrets.EXPECT().Create(ociImageSecret).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(&serviceArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(&serviceArg).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("app-name-endpoints", v1.GetOptions{IncludeUninitialized: true}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicHeadlessServiceArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicHeadlessServiceArg).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).
			Return(nil, nil),
	}...)
	gomock.InOrder(assertCalls...)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateful,
		},
		OperatorImagePath: "operator/image-path",
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureCustomResourceDefinitionsCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crds := map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec{
		"tfjobs.kubeflow.org": {
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Kind:     "TFJob",
				Singular: "tfjob",
				Plural:   "tfjobs",
			},
			Version: "v1alpha2",
			Group:   "kubeflow.org",
			Scope:   "Namespaced",
			Validation: &apiextensionsv1beta1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextensionsv1beta1.JSONSchemaProps{
					Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
						"tfReplicaSpecs": {
							Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
								"Worker": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type:    "integer",
											Minimum: float64Ptr(1),
										},
									},
								},
								"PS": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type: "integer", Minimum: float64Ptr(1),
										},
									},
								},
								"Chief": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type:    "integer",
											Minimum: float64Ptr(1),
											Maximum: float64Ptr(1),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      "tfjobs.kubeflow.org",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   "kubeflow.org",
			Version: "v1alpha2",
			Scope:   "Namespaced",
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural:   "tfjobs",
				Kind:     "TFJob",
				Singular: "tfjob",
			},
			Validation: &apiextensionsv1beta1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextensionsv1beta1.JSONSchemaProps{
					Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
						"tfReplicaSpecs": {
							Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
								"Worker": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type:    "integer",
											Minimum: float64Ptr(1),
										},
									},
								},
								"PS": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type: "integer", Minimum: float64Ptr(1),
										},
									},
								},
								"Chief": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type:    "integer",
											Minimum: float64Ptr(1),
											Maximum: float64Ptr(1),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	s.assertCustomerResourceDefinitions(
		c, crds,
		s.mockCustomResourceDefinition.EXPECT().Create(crd).Return(crd, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureCustomResourceDefinitionsUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crds := map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec{
		"tfjobs.kubeflow.org": {
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Kind:     "TFJob",
				Singular: "tfjob",
				Plural:   "tfjobs",
			},
			Version: "v1alpha2",
			Group:   "kubeflow.org",
			Scope:   "Namespaced",
			Validation: &apiextensionsv1beta1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextensionsv1beta1.JSONSchemaProps{
					Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
						"tfReplicaSpecs": {
							Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
								"Worker": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type:    "integer",
											Minimum: float64Ptr(1),
										},
									},
								},
								"PS": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type: "integer", Minimum: float64Ptr(1),
										},
									},
								},
								"Chief": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type:    "integer",
											Minimum: float64Ptr(1),
											Maximum: float64Ptr(1),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      "tfjobs.kubeflow.org",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   "kubeflow.org",
			Version: "v1alpha2",
			Scope:   "Namespaced",
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural:   "tfjobs",
				Kind:     "TFJob",
				Singular: "tfjob",
			},
			Validation: &apiextensionsv1beta1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextensionsv1beta1.JSONSchemaProps{
					Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
						"tfReplicaSpecs": {
							Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
								"Worker": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type:    "integer",
											Minimum: float64Ptr(1),
										},
									},
								},
								"PS": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type: "integer", Minimum: float64Ptr(1),
										},
									},
								},
								"Chief": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"replicas": {
											Type:    "integer",
											Minimum: float64Ptr(1),
											Maximum: float64Ptr(1),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	s.assertCustomerResourceDefinitions(
		c, crds,
		s.mockCustomResourceDefinition.EXPECT().Create(crd).Return(crd, s.k8sAlreadyExistsError()),
		s.mockCustomResourceDefinition.EXPECT().Update(crd).Return(crd, nil),
	)
}

func (s *K8sBrokerSuite) assertCustomerResources(c *gc.C, crs map[string][]unstructured.Unstructured, assertCalls ...*gomock.Call) {

	basicPodSpec := getBasicPodspec()
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			CustomResources: crs,
		},
	}
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec, "operator/image-path")
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)

	numUnits := int32(2)
	statefulSetArg := &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: "app-name",
			Annotations: map[string]string{
				"juju-app-uuid": "appuuid",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"juju-app": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
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
			s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).
				Return(nil, s.k8sNotFoundError()),
		},
		assertCalls...,
	)

	ociImageSecret := s.getOCIImageSecret(c, nil)
	assertCalls = append(assertCalls, []*gomock.Call{
		s.mockSecrets.EXPECT().Create(ociImageSecret).
			Return(ociImageSecret, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(&serviceArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(&serviceArg).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("app-name-endpoints", v1.GetOptions{IncludeUninitialized: true}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicHeadlessServiceArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicHeadlessServiceArg).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).
			Return(nil, nil),
	}...)
	gomock.InOrder(assertCalls...)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateful,
		},
		OperatorImagePath: "operator/image-path",
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureCustomResourcesCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crRaw1 := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubeflow.org/v1",
			"metadata": map[string]interface{}{
				"name": "dist-mnist-for-e2e-test-1",
			},
			"kind": "TFJob",
			"spec": map[string]interface{}{
				"tfReplicaSpecs": map[string]interface{}{
					"PS": map[string]interface{}{
						"replicas":      int64(1),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
					"Worker": map[string]interface{}{
						"replicas":      int64(1),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	cr1 := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubeflow.org/v1",
			"metadata": map[string]interface{}{
				"name":   "dist-mnist-for-e2e-test-1",
				"labels": map[string]interface{}{"juju-app": "app-name"},
			},
			"kind": "TFJob",
			"spec": map[string]interface{}{
				"tfReplicaSpecs": map[string]interface{}{
					"PS": map[string]interface{}{
						"replicas":      int64(1),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
					"Worker": map[string]interface{}{
						"replicas":      int64(1),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	crRaw2 := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubeflow.org/v1beta2",
			"metadata": map[string]interface{}{
				"name": "dist-mnist-for-e2e-test-2",
			},
			"kind": "TFJob",
			"spec": map[string]interface{}{
				"tfReplicaSpecs": map[string]interface{}{
					"PS": map[string]interface{}{
						"replicas":      int64(2),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
					"Worker": map[string]interface{}{
						"replicas":      int64(2),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	cr2 := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubeflow.org/v1beta2",
			"metadata": map[string]interface{}{
				"name":   "dist-mnist-for-e2e-test-2",
				"labels": map[string]interface{}{"juju-app": "app-name"},
			},
			"kind": "TFJob",
			"spec": map[string]interface{}{
				"tfReplicaSpecs": map[string]interface{}{
					"PS": map[string]interface{}{
						"replicas":      int64(2),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
					"Worker": map[string]interface{}{
						"replicas":      int64(2),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	crs := map[string][]unstructured.Unstructured{
		"tfjobs.kubeflow.org": {
			crRaw1, crRaw2,
			// cr1, cr2,
		},
	}

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      "tfjobs.kubeflow.org",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   "kubeflow.org",
			Version: "v1",
			Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{Name: "v1beta2", Served: true, Storage: false},
			},
			Scope: "Namespaced",
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Kind:     "TFJob",
				Plural:   "tfjobs",
				Singular: "tfjob",
			},
			Validation: &apiextensionsv1beta1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextensionsv1beta1.JSONSchemaProps{
					Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
						"spec": {
							Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
								"tfReplicaSpecs": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"PS": {
											Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
												"replicas": {
													Type: "integer", Minimum: float64Ptr(1),
												},
											},
										},
										"Chief": {
											Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: float64Ptr(1),
													Maximum: float64Ptr(1),
												},
											},
										},
										"Worker": {
											Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: float64Ptr(1),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	s.assertCustomerResources(
		c, crs,
		// ensuring cr1.
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Return(crd, nil),
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1",
				Resource: crd.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().Create(&cr1).Return(&cr1, nil),

		// ensuring cr2.
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Return(crd, nil),
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1beta2",
				Resource: crd.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().Create(&cr2).Return(&cr2, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureCustomResourcesUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crRaw1 := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubeflow.org/v1",
			"metadata": map[string]interface{}{
				"name": "dist-mnist-for-e2e-test-1",
			},
			"kind": "TFJob",
			"spec": map[string]interface{}{
				"tfReplicaSpecs": map[string]interface{}{
					"PS": map[string]interface{}{
						"replicas":      int64(1),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
					"Worker": map[string]interface{}{
						"replicas":      int64(1),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	cr1 := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubeflow.org/v1",
			"metadata": map[string]interface{}{
				"name":   "dist-mnist-for-e2e-test-1",
				"labels": map[string]interface{}{"juju-app": "app-name"},
			},
			"kind": "TFJob",
			"spec": map[string]interface{}{
				"tfReplicaSpecs": map[string]interface{}{
					"PS": map[string]interface{}{
						"replicas":      int64(1),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
					"Worker": map[string]interface{}{
						"replicas":      int64(1),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	crRaw2 := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubeflow.org/v1beta2",
			"metadata": map[string]interface{}{
				"name": "dist-mnist-for-e2e-test-2",
			},
			"kind": "TFJob",
			"spec": map[string]interface{}{
				"tfReplicaSpecs": map[string]interface{}{
					"PS": map[string]interface{}{
						"replicas":      int64(2),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
					"Worker": map[string]interface{}{
						"replicas":      int64(2),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	cr2 := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubeflow.org/v1beta2",
			"metadata": map[string]interface{}{
				"name":   "dist-mnist-for-e2e-test-2",
				"labels": map[string]interface{}{"juju-app": "app-name"},
			},
			"kind": "TFJob",
			"spec": map[string]interface{}{
				"tfReplicaSpecs": map[string]interface{}{
					"PS": map[string]interface{}{
						"replicas":      int64(2),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
					"Worker": map[string]interface{}{
						"replicas":      int64(2),
						"restartPolicy": "Never",
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "tensorflow",
										"image": "kubeflow/tf-dist-mnist-test:1.0",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	crs := map[string][]unstructured.Unstructured{
		"tfjobs.kubeflow.org": {
			crRaw1, crRaw2,
			// cr1, cr2,
		},
	}

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      "tfjobs.kubeflow.org",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   "kubeflow.org",
			Version: "v1",
			Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{Name: "v1beta2", Served: true, Storage: false},
			},
			Scope: "Namespaced",
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Kind:     "TFJob",
				Plural:   "tfjobs",
				Singular: "tfjob",
			},
			Validation: &apiextensionsv1beta1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextensionsv1beta1.JSONSchemaProps{
					Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
						"spec": {
							Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
								"tfReplicaSpecs": {
									Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
										"PS": {
											Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
												"replicas": {
													Type: "integer", Minimum: float64Ptr(1),
												},
											},
										},
										"Chief": {
											Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: float64Ptr(1),
													Maximum: float64Ptr(1),
												},
											},
										},
										"Worker": {
											Properties: map[string]apiextensionsv1beta1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: float64Ptr(1),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	s.assertCustomerResources(
		c, crs,
		// ensuring cr1.
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Return(crd, nil),
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1",
				Resource: crd.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().Create(&cr1).Return(nil, s.k8sAlreadyExistsError()),
		s.mockResourceClient.EXPECT().Update(&cr1).Return(&cr1, nil),

		// ensuring cr2.
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Return(crd, nil),
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1beta2",
				Resource: crd.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().Create(&cr2).Return(nil, s.k8sAlreadyExistsError()),
		s.mockResourceClient.EXPECT().Update(&cr2).Return(&cr2, nil),
	)
}
