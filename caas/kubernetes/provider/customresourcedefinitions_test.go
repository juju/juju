// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/mocks"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/testing"
)

func (s *K8sBrokerSuite) assertCustomerResourceDefinitions(c *gc.C, crds []k8sspecs.K8sCustomResourceDefinitionSpec, assertCalls ...*gomock.Call) {

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
	}...)
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
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceCustomResourceDefinitionsCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crds := []k8sspecs.K8sCustomResourceDefinitionSpec{
		{
			Name: "tfjobs.kubeflow.org",
			Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
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
		},
	}

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:        "tfjobs.kubeflow.org",
			Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
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

func (s *K8sBrokerSuite) TestEnsureServiceCustomResourceDefinitionsUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crds := []k8sspecs.K8sCustomResourceDefinitionSpec{
		{
			Name: "tfjobs.kubeflow.org",
			Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
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
		},
	}

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:        "tfjobs.kubeflow.org",
			Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
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
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Return(crd, nil),
		s.mockCustomResourceDefinition.EXPECT().Update(crd).Return(crd, nil),
	)
}

func (s *K8sBrokerSuite) assertCustomerResources(c *gc.C, crs map[string][]unstructured.Unstructured, adjustClock func(), assertCalls ...*gomock.Call) {

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
	}...)
	gomock.InOrder(assertCalls...)

	errChan := make(chan error)
	go func() {
		params := &caas.ServiceParams{
			PodSpec: basicPodSpec,
			Deployment: caas.DeploymentParams{
				DeploymentType: caas.DeploymentStateful,
			},
			OperatorImagePath: "operator/image-path",
			ResourceTags:      map[string]string{"juju-controller-uuid": testing.ControllerTag.Id()},
		}
		errChan <- s.broker.EnsureService("app-name",
			func(_ string, _ status.Status, e string, _ map[string]interface{}) error {
				c.Logf("EnsureService error -> %q", e)
				return nil
			},
			params, 2, application.ConfigAttributes{
				"kubernetes-service-loadbalancer-ip": "10.0.0.1",
				"kubernetes-service-externalname":    "ext-name",
			})

	}()

	adjustClock()

	select {
	case err := <-errChan:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for EnsureService return")
	}
}

func getCR1() unstructured.Unstructured {
	return unstructured.Unstructured{
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
}

func getCR2() unstructured.Unstructured {
	return unstructured.Unstructured{
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
}

func (s *K8sBrokerSuite) TestEnsureServiceCustomResourcesCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crRaw1 := getCR1()
	crRaw2 := getCR2()

	cr1 := getCR1()
	cr1.SetLabels(map[string]string{"juju-app": "app-name"})
	cr1.SetAnnotations(map[string]string{"juju.io/controller": testing.ControllerTag.Id()})
	cr2 := getCR2()
	cr2.SetLabels(map[string]string{"juju-app": "app-name"})
	cr2.SetAnnotations(map[string]string{"juju.io/controller": testing.ControllerTag.Id()})

	crs := map[string][]unstructured.Unstructured{
		"tfjobs.kubeflow.org": {
			crRaw1, crRaw2,
		},
	}

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:        "tfjobs.kubeflow.org",
			Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
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
		func() {
			// CRD is ready in 1st time checking.
		},
		// waits CRD stablised.
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Return(crd, nil),
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1",
				Resource: crd.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().List(v1.ListOptions{}).Return(&unstructured.UnstructuredList{}, nil),

		// ensuring cr1.
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1",
				Resource: crd.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().Create(&cr1, v1.CreateOptions{}).Return(&cr1, nil),

		// ensuring cr2.
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1beta2",
				Resource: crd.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().Create(&cr2, v1.CreateOptions{}).Return(&cr2, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceCustomResourcesUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crRaw1 := getCR1()
	crRaw2 := getCR2()

	cr1 := getCR1()
	cr1.SetLabels(map[string]string{"juju-app": "app-name"})
	cr1.SetAnnotations(map[string]string{"juju.io/controller": testing.ControllerTag.Id()})
	cr2 := getCR2()
	cr2.SetLabels(map[string]string{"juju-app": "app-name"})
	cr2.SetAnnotations(map[string]string{"juju.io/controller": testing.ControllerTag.Id()})

	crUpdatedResourceVersion1 := getCR1()
	crUpdatedResourceVersion1.SetLabels(map[string]string{"juju-app": "app-name"})
	crUpdatedResourceVersion1.SetAnnotations(map[string]string{"juju.io/controller": testing.ControllerTag.Id()})
	crUpdatedResourceVersion1.SetResourceVersion("11111")

	crUpdatedResourceVersion2 := getCR2()
	crUpdatedResourceVersion2.SetLabels(map[string]string{"juju-app": "app-name"})
	crUpdatedResourceVersion2.SetAnnotations(map[string]string{"juju.io/controller": testing.ControllerTag.Id()})
	crUpdatedResourceVersion2.SetResourceVersion("11111")

	crs := map[string][]unstructured.Unstructured{
		"tfjobs.kubeflow.org": {
			crRaw1, crRaw2,
		},
	}

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:        "tfjobs.kubeflow.org",
			Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
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
		func() {
			err := s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
			c.Assert(err, jc.ErrorIsNil)

			err = s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
			c.Assert(err, jc.ErrorIsNil)
		},
		// waits CRD stablised.
		// 1. CRD not found.
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Times(1).Return(nil, s.k8sNotFoundError()),
		// 2. CRD resource type not ready yet.
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Times(1).Return(crd, nil),
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1",
				Resource: crd.Spec.Names.Plural,
			},
		).Times(1).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().List(v1.ListOptions{}).Times(1).Return(nil, s.k8sNotFoundError()),
		// 3. CRD is ready.
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Times(1).Return(crd, nil),
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1",
				Resource: crd.Spec.Names.Plural,
			},
		).Times(1).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().List(v1.ListOptions{}).Times(1).Return(&unstructured.UnstructuredList{}, nil),

		// ensuring cr1.
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1",
				Resource: crd.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().Create(&cr1, v1.CreateOptions{}).Return(nil, s.k8sAlreadyExistsError()),
		s.mockResourceClient.EXPECT().Get("dist-mnist-for-e2e-test-1", v1.GetOptions{}).Return(&crUpdatedResourceVersion1, nil),
		s.mockResourceClient.EXPECT().Update(&crUpdatedResourceVersion1, v1.UpdateOptions{}).Return(&crUpdatedResourceVersion1, nil),

		// ensuring cr2.
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1beta2",
				Resource: crd.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().Create(&cr2, v1.CreateOptions{}).Return(nil, s.k8sAlreadyExistsError()),
		s.mockResourceClient.EXPECT().Get("dist-mnist-for-e2e-test-2", v1.GetOptions{}).Return(&crUpdatedResourceVersion2, nil),
		s.mockResourceClient.EXPECT().Update(&crUpdatedResourceVersion2, v1.UpdateOptions{}).Return(&crUpdatedResourceVersion2, nil),
	)
}

func (s *K8sBrokerSuite) TestCRDGetter(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crdGetter := provider.CRDGetter{s.broker}

	badCRDNoVersion := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:        "tfjobs.kubeflow.org",
			Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group: "kubeflow.org",
			Scope: "Namespaced",
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

	// Test 1: Invalid CRD found - no version.
	gomock.InOrder(
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Times(1).Return(badCRDNoVersion, nil),
	)
	result, err := crdGetter.Get("tfjobs.kubeflow.org")
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(result, gc.IsNil)

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:        "tfjobs.kubeflow.org",
			Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   "kubeflow.org",
			Version: "v1",
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

	// Test 2: not found CRD.
	gomock.InOrder(
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Times(1).Return(nil, s.k8sNotFoundError()),
	)
	result, err = crdGetter.Get("tfjobs.kubeflow.org")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(result, gc.IsNil)

	// Test 3: found CRD but CRD is not stablised yet.
	gomock.InOrder(
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Times(1).Return(crd, nil),
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1",
				Resource: crd.Spec.Names.Plural,
			},
		).Times(1).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().List(v1.ListOptions{}).Times(1).Return(nil, s.k8sNotFoundError()),
	)
	result, err = crdGetter.Get("tfjobs.kubeflow.org")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(result, gc.IsNil)

	// Test 4: all good.
	gomock.InOrder(
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Times(1).Return(crd, nil),
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  "v1",
				Resource: crd.Spec.Names.Plural,
			},
		).Times(1).Return(s.mockNamespaceableResourceClient),
		s.mockResourceClient.EXPECT().List(v1.ListOptions{}).Times(1).Return(&unstructured.UnstructuredList{}, nil),
	)
	result, err = crdGetter.Get("tfjobs.kubeflow.org")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, crd)
}

func (s *K8sBrokerSuite) TestGetCRDsForCRsAllGood(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crd1 := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:        "tfjobs.kubeflow.org",
			Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   "kubeflow.org",
			Version: "v1",
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
	crd2 := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:        "scheduledworkflows.kubeflow.org",
			Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   "kubeflow.org",
			Version: "v1beta1",
			Scope:   "Namespaced",
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural:   "scheduledworkflows",
				Kind:     "ScheduledWorkflow",
				Singular: "scheduledworkflow",
				ListKind: "ScheduledWorkflowList",
				ShortNames: []string{
					"swf",
				},
			},
		},
	}

	expectedResult := map[string]*apiextensionsv1beta1.CustomResourceDefinition{
		crd1.GetName(): crd1,
		crd2.GetName(): crd2,
	}

	mockCRDGetter := mocks.NewMockCRDGetterInterface(ctrl)

	// round 1. crd1 not found.
	mockCRDGetter.EXPECT().Get("tfjobs.kubeflow.org").Times(1).Return(nil, errors.NotFoundf(""))
	// round 1. crd2 not found.
	mockCRDGetter.EXPECT().Get("scheduledworkflows.kubeflow.org").Times(1).Return(nil, errors.NotFoundf(""))

	// round 2. crd1 not found.
	mockCRDGetter.EXPECT().Get("tfjobs.kubeflow.org").Times(1).Return(nil, errors.NotFoundf(""))
	// round 2. crd2 found.
	mockCRDGetter.EXPECT().Get("scheduledworkflows.kubeflow.org").Times(1).Return(crd2, nil)

	// round 3. crd1 found.
	mockCRDGetter.EXPECT().Get("tfjobs.kubeflow.org").Times(1).Return(crd1, nil)

	resultChan := make(chan map[string]*apiextensionsv1beta1.CustomResourceDefinition)
	errChan := make(chan error)

	go func(broker *provider.KubernetesClient) {
		crs := map[string][]unstructured.Unstructured{
			"tfjobs.kubeflow.org":             {},
			"scheduledworkflows.kubeflow.org": {},
		}
		result, err := broker.GetCRDsForCRs(crs, mockCRDGetter)
		errChan <- err
		resultChan <- result
	}(s.broker)

	err := s.clock.WaitAdvance(time.Second, testing.ShortWait, 2)
	c.Assert(err, jc.ErrorIsNil)

	err = s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case err := <-errChan:
		c.Assert(err, jc.ErrorIsNil)
		result := <-resultChan
		c.Assert(result, gc.DeepEquals, expectedResult)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for GetCRDsForCRs return")
	}
}

func (s *K8sBrokerSuite) TestGetCRDsForCRsFailEarly(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	mockCRDGetter := mocks.NewMockCRDGetterInterface(ctrl)
	unExpectedErr := errors.New("a non not found error")

	// round 1. crd1 not found.
	mockCRDGetter.EXPECT().Get("tfjobs.kubeflow.org").AnyTimes().Return(nil, errors.NotFoundf(""))
	// round 1. crd2 un expected error - will not retry but abort the whole wg.
	mockCRDGetter.EXPECT().Get("scheduledworkflows.kubeflow.org").Times(1).Return(nil, unExpectedErr)

	resultChan := make(chan map[string]*apiextensionsv1beta1.CustomResourceDefinition)
	errChan := make(chan error)

	go func(broker *provider.KubernetesClient) {
		crs := map[string][]unstructured.Unstructured{
			"tfjobs.kubeflow.org":             {},
			"scheduledworkflows.kubeflow.org": {},
		}
		result, err := broker.GetCRDsForCRs(crs, mockCRDGetter)
		errChan <- err
		resultChan <- result
	}(s.broker)

	err := s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case err := <-errChan:
		c.Assert(err, gc.ErrorMatches, `getting custom resources: a non not found error`)
		result := <-resultChan
		c.Assert(result, gc.IsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for GetCRDsForCRs return")
	}
}
