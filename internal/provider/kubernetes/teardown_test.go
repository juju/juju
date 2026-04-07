// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"context"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	provider "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
	k8swatchertest "github.com/juju/juju/internal/provider/kubernetes/watcher/test"
	"github.com/juju/juju/internal/testing"
)

func (s *K8sBrokerSuite) TestDeleteClusterScopeResourcesModelTeardownSuccess(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// CRs of this Cluster scope CRD will get deleted.
	crdClusterScope := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:   "tfjobs.kubeflow.org",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.kubernetes.io/name": "test"},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "kubeflow.org",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{
					Name:    "v1alpha2",
					Served:  true,
					Storage: false,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"tfReplicaSpecs": {
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"Worker": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"PS": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type: "integer", Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"Chief": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
													Maximum: pointer.Float64Ptr(1),
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
			Scope: apiextensionsv1.ClusterScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "tfjobs",
				Kind:     "TFJob",
				Singular: "tfjob",
			},
		},
	}
	// CRs of this namespaced scope CRD will also be deleted (across all namespaces).
	crdNamespacedScope := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:   "tfjobs.kubeflow.org",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.kubernetes.io/name": "test"},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "kubeflow.org",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{
					Name:    "v1alpha2",
					Served:  true,
					Storage: false,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"tfReplicaSpecs": {
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"Worker": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"PS": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type: "integer", Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"Chief": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
													Maximum: pointer.Float64Ptr(1),
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
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "tfjobs",
				Kind:     "TFJob",
				Singular: "tfjob",
			},
		},
	}

	// timer +1.
	s.mockClusterRoleBindings.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
		Return(&rbacv1.ClusterRoleBindingList{}, nil).
		After(
			s.mockClusterRoleBindings.EXPECT().DeleteCollection(gomock.Any(),
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
			).Return(s.k8sNotFoundError()).Call,
		)

	// timer +1.
	s.mockClusterRoles.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
		Return(&rbacv1.ClusterRoleList{}, nil).
		After(
			s.mockClusterRoles.EXPECT().DeleteCollection(gomock.Any(),
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
			).Return(s.k8sNotFoundError()).Call,
		)

	crdLabelSelector := "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"

	// list cluster wide all custom resource definitions (used by removeAllCustomResourceFinalizers,
	// deleteAllCustomResourcesAllNamespaces and listAllCustomResourcesAllNamespaces).
	s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(), v1.ListOptions{
		LabelSelector: crdLabelSelector,
	}).AnyTimes().Return(
		&apiextensionsv1.CustomResourceDefinitionList{
			Items: []apiextensionsv1.CustomResourceDefinition{
				*crdClusterScope, *crdNamespacedScope,
			},
		},
		nil,
	).Times(3)

	// timer +1.
<<<<<<< HEAD
	s.mockNamespaceableResourceClient.EXPECT().List(gomock.Any(),
		// list all custom resources for crd "v1alpha2".
		v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
	).Return(&unstructured.UnstructuredList{}, nil).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1alpha2",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient).Call,
	).After(
		// list all custom resources for crd "v1".
		s.mockNamespaceableResourceClient.EXPECT().List(gomock.Any(),
			v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
		).Return(&unstructured.UnstructuredList{}, nil).Call,
	).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient).Call,
	).After(
		// list cluster wide all custom resource definitions for listing custom resources.
		s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(), v1.ListOptions{}).AnyTimes().
			Return(&apiextensionsv1.CustomResourceDefinitionList{Items: []apiextensionsv1.CustomResourceDefinition{*crdClusterScope, *crdNamespacedScope}}, nil),
	).After(
		// delete all custom resources for crd "v1alpha2".
		s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
		).Return(nil).Call,
	).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1alpha2",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient).Call,
	).After(
		// delete all custom resources for crd "v1".
		s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
		).Return(nil).Call,
	).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient).Call,
	).After(
		// list cluster wide all custom resource definitions for deleting custom resources.
		s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(), v1.ListOptions{}).AnyTimes().
			Return(&apiextensionsv1.CustomResourceDefinitionList{Items: []apiextensionsv1.CustomResourceDefinition{*crdClusterScope, *crdNamespacedScope}}, nil),
	)
=======
	s.mockDynamicClient.EXPECT().Resource(
		gomock.Any(),
	).Return(s.mockNamespaceableResourceClient).AnyTimes()
	s.mockNamespaceableResourceClient.EXPECT().List(
		gomock.Any(), gomock.Any(),
	).Return(&unstructured.UnstructuredList{}, nil).AnyTimes()
	s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		gomock.Any(),
	).Return(nil).AnyTimes()
>>>>>>> 3.6

	// timer +1.
	s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).AnyTimes().
		Return(&apiextensionsv1.CustomResourceDefinitionList{}, nil).
		After(
			s.mockCustomResourceDefinitionV1.EXPECT().DeleteCollection(gomock.Any(),
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
			).Return(s.k8sNotFoundError()).Call,
		)

	// timer +1.
	s.mockMutatingWebhookConfigurationV1.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
		Return(&admissionregistrationv1.MutatingWebhookConfigurationList{}, nil).
		After(
			s.mockMutatingWebhookConfigurationV1.EXPECT().DeleteCollection(gomock.Any(),
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
			).Return(s.k8sNotFoundError()).Call,
		)

	// timer +1.
	s.mockValidatingWebhookConfigurationV1.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
		Return(&admissionregistrationv1.ValidatingWebhookConfigurationList{}, nil).
		After(
			s.mockValidatingWebhookConfigurationV1.EXPECT().DeleteCollection(gomock.Any(),
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
			).Return(s.k8sNotFoundError()).Call,
		)

	// timer +1.
	s.mockStorageClass.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"}).
		Return(&storagev1.StorageClassList{}, nil).
		After(
			s.mockStorageClass.EXPECT().DeleteCollection(gomock.Any(),
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
			).Return(nil).Call,
		)

	var wg sync.WaitGroup
	wg.Add(1)

	errCh := make(chan error)
	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	go s.broker.DeleteClusterScopeResourcesModelTeardown(ctx, &wg, errCh)

	// 6 parallel tasks, then 1 more tick for the CR list checker.
	err := s.clock.WaitAdvance(time.Second, testing.ShortWait, 6)
	c.Assert(err, tc.ErrorIsNil)
	err = s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for DeleteClusterScopeResourcesModelTeardown return")
	}
}

func (s *K8sBrokerSuite) TestDeleteClusterScopeResourcesModelTeardownTimeout(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// CRs of this Cluster scope CRD will get deleted.
	crdClusterScope := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:   "tfjobs.kubeflow.org",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.kubernetes.io/name": "test"},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "kubeflow.org",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{
					Name: "v1alpha2", Served: true, Storage: false,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"tfReplicaSpecs": {
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"Worker": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"PS": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type: "integer", Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"Chief": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
													Maximum: pointer.Float64Ptr(1),
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
			Scope: apiextensionsv1.ClusterScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "tfjobs",
				Kind:     "TFJob",
				Singular: "tfjob",
			},
		},
	}
	// CRs of this namespaced scope CRD will also be deleted (across all namespaces).
	crdNamespacedScope := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:   "tfjobs.kubeflow.org",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.kubernetes.io/name": "test"},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "kubeflow.org",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{
					Name: "v1alpha2", Served: true, Storage: false,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"tfReplicaSpecs": {
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"Worker": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"PS": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type: "integer", Minimum: pointer.Float64Ptr(1),
												},
											},
										},
										"Chief": {
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"replicas": {
													Type:    "integer",
													Minimum: pointer.Float64Ptr(1),
													Maximum: pointer.Float64Ptr(1),
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
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "tfjobs",
				Kind:     "TFJob",
				Singular: "tfjob",
			},
		},
	}

	s.mockClusterRoleBindings.EXPECT().DeleteCollection(gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
	).Return(s.k8sNotFoundError())

	s.mockClusterRoles.EXPECT().DeleteCollection(gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
	).Return(s.k8sNotFoundError())

	crdLabelSelector := "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"

	// list cluster wide all custom resource definitions (used by removeAllCustomResourceFinalizers,
	// deleteAllCustomResourcesAllNamespaces and listAllCustomResourcesAllNamespaces).
	s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(), v1.ListOptions{
		LabelSelector: crdLabelSelector,
	}).AnyTimes().Return(
		&apiextensionsv1.CustomResourceDefinitionList{
			Items: []apiextensionsv1.CustomResourceDefinition{
				*crdClusterScope, *crdNamespacedScope,
			},
		},
		nil,
	)

	// timer +1.
	s.mockDynamicClient.EXPECT().Resource(
		gomock.Any(),
	).Return(s.mockNamespaceableResourceClient).AnyTimes()
	s.mockNamespaceableResourceClient.EXPECT().List(
		gomock.Any(), gomock.Any(),
	).Return(&unstructured.UnstructuredList{}, nil).AnyTimes()
	s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
<<<<<<< HEAD
		v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
	).Return(nil).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1alpha2",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient).Call,
	).After(
		// delete all custom resources for crd "v1".
		s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(gomock.Any(),
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
		).Return(nil).Call,
	).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient).Call,
	).After(
		// list cluster wide all custom resource definitions for deleting custom resources.
		s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(), v1.ListOptions{}).AnyTimes().
			Return(&apiextensionsv1.CustomResourceDefinitionList{Items: []apiextensionsv1.CustomResourceDefinition{*crdClusterScope, *crdNamespacedScope}}, nil),
	)
=======
		gomock.Any(),
	).Return(nil).AnyTimes()
>>>>>>> 3.6

	s.mockCustomResourceDefinitionV1.EXPECT().DeleteCollection(gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
	).Return(s.k8sNotFoundError())

	s.mockMutatingWebhookConfigurationV1.EXPECT().DeleteCollection(gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
	).Return(s.k8sNotFoundError())

	s.mockValidatingWebhookConfigurationV1.EXPECT().DeleteCollection(gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
	).Return(s.k8sNotFoundError())

	s.mockStorageClass.EXPECT().DeleteCollection(gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"},
	).Return(nil)

	var wg sync.WaitGroup
	wg.Add(1)

	errCh := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	ctx, cancel := context.WithTimeout(c.Context(), 500*time.Millisecond)
	defer cancel()
	go s.broker.DeleteClusterScopeResourcesModelTeardown(ctx, &wg, errCh)

	err := s.clock.WaitAdvance(500*time.Millisecond, testing.ShortWait, 6)
	c.Assert(err, tc.ErrorIsNil)
	select {
	case <-done:
		c.Assert(<-errCh, tc.ErrorMatches, `context deadline exceeded`)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for DeleteClusterScopeResourcesModelTeardown return")
	}
}

<<<<<<< HEAD
func (s *K8sBrokerSuite) TestDeleteNamespaceModelTeardown(c *tc.C) {
=======
// TestDeleteClusterScopeAPIExtensionResourcesNamespacedCRFinalizersStripped verifies
// that when a namespaced CRD has a CR with a finalizer in a namespace other than the
// model namespace, the finalizer is patched away (against the correct namespace) before
// the DeleteCollection is issued.
func (s *K8sBrokerSuite) TestDeleteClusterScopeAPIExtensionResourcesNamespacedCRFinalizersStripped(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crdNamespaced := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{Name: "widgets.example.com"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: "widgets", Kind: "Widget", Singular: "widget",
			},
		},
	}

	crWithFinalizer := unstructured.Unstructured{}
	crWithFinalizer.SetName("my-widget")
	crWithFinalizer.SetNamespace("other-model-ns")
	crWithFinalizer.SetFinalizers([]string{"foregroundDeletion"})

	crdLabelSelector := "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"
	gvr := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}

	// CRD list is called three times: removeAllCustomResourceFinalizers,
	// deleteAllCustomResourcesAllNamespaces, listAllCustomResourcesAllNamespaces.
	s.mockCustomResourceDefinitionV1.EXPECT().List(
		gomock.Any(),
		v1.ListOptions{
			LabelSelector: crdLabelSelector,
		},
	).Return(&apiextensionsv1.CustomResourceDefinitionList{
		Items: []apiextensionsv1.CustomResourceDefinition{*crdNamespaced},
	}, nil).Times(3)

	s.mockDynamicClient.EXPECT().Resource(gvr).Return(s.mockNamespaceableResourceClient)
	s.mockNamespaceableResourceClient.EXPECT().List(
		gomock.Any(), gomock.Any(),
	).Return(&unstructured.UnstructuredList{
		Items: []unstructured.Unstructured{crWithFinalizer},
	}, nil)
	s.mockNamespaceableResourceClient.EXPECT().Namespace("other-model-ns").Return(s.mockResourceClient)
	s.mockResourceClient.EXPECT().Patch(
		gomock.Any(),
		"my-widget",
		types.MergePatchType,
		gomock.Any(),
		v1.PatchOptions{},
	).Return(&crWithFinalizer, nil)
	s.mockDynamicClient.EXPECT().Resource(gvr).Return(s.mockNamespaceableResourceClient)
	s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(
		gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		gomock.Any(),
	).Return(nil)
	s.mockDynamicClient.EXPECT().Resource(gvr).Return(s.mockNamespaceableResourceClient)
	s.mockNamespaceableResourceClient.EXPECT().List(
		gomock.Any(), gomock.Any(),
	).Return(&unstructured.UnstructuredList{}, nil)

	// CRD deletion.
	s.mockCustomResourceDefinitionV1.EXPECT().DeleteCollection(gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: crdLabelSelector},
	).Return(nil)
	// CRD checker: empty list signals deletion complete.
	s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(),
		v1.ListOptions{LabelSelector: crdLabelSelector},
	).Return(&apiextensionsv1.CustomResourceDefinitionList{}, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	errCh := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	modelSelector := k8slabels.NewSelector().Add(
		provider.LabelSetToRequirements(utils.LabelsForModel(
			s.broker.ModelName(), s.broker.ModelUUID(), s.broker.ControllerUUID(), s.broker.LabelVersion(),
		))...,
	)
	go s.broker.DeleteClusterScopeAPIExtensionResourcesModelTeardown(
		ctx, modelSelector, s.clock, &wg, errCh,
	)

	// The two sub-functions run sequentially, so we advance the clock twice:
	// once for the CR deletion checker, then once for the CRD deletion checker.
	err := s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)
	err = s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for DeleteClusterScopeAPIExtensionResourcesModelTeardown to return")
	}
	select {
	case err := <-errCh:
		c.Fatalf("unexpected error: %v", err)
	default:
	}
}

// TestDeleteClusterScopeAPIExtensionResourcesAllNamespacesDeleted verifies that CRs
// belonging to a namespaced CRD are deleted across ALL namespaces (via an unscoped
// DeleteCollection), not just the model's own namespace.
func (s *K8sBrokerSuite) TestDeleteClusterScopeAPIExtensionResourcesAllNamespacesDeleted(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crdNamespaced := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{Name: "foos.example.com"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: "foos", Kind: "Foo", Singular: "foo",
			},
		},
	}

	crdLabelSelector := "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test"
	gvr := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "foos"}

	s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(), v1.ListOptions{
		LabelSelector: crdLabelSelector,
	}).Return(
		&apiextensionsv1.CustomResourceDefinitionList{
			Items: []apiextensionsv1.CustomResourceDefinition{*crdNamespaced},
		}, nil,
	).Times(3)

	s.mockDynamicClient.EXPECT().Resource(gvr).Return(s.mockNamespaceableResourceClient)
	s.mockNamespaceableResourceClient.EXPECT().List(gomock.Any(), gomock.Any()).
		Return(&unstructured.UnstructuredList{}, nil)
	s.mockDynamicClient.EXPECT().Resource(gvr).Return(s.mockNamespaceableResourceClient)
	s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		gomock.Any(),
	).Return(nil)
	s.mockDynamicClient.EXPECT().Resource(gvr).Return(s.mockNamespaceableResourceClient)
	s.mockNamespaceableResourceClient.EXPECT().List(gomock.Any(), gomock.Any()).
		Return(&unstructured.UnstructuredList{}, nil)

	// CRD deletion.
	s.mockCustomResourceDefinitionV1.EXPECT().DeleteCollection(gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: crdLabelSelector},
	).Return(nil)
	// CRD checker: empty list signals deletion complete.
	s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(),
		v1.ListOptions{LabelSelector: crdLabelSelector},
	).Return(&apiextensionsv1.CustomResourceDefinitionList{}, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	errCh := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	modelSelector := k8slabels.NewSelector().Add(
		provider.LabelSetToRequirements(utils.LabelsForModel(
			s.broker.ModelName(), s.broker.ModelUUID(), s.broker.ControllerUUID(), s.broker.LabelVersion(),
		))...,
	)
	go s.broker.DeleteClusterScopeAPIExtensionResourcesModelTeardown(
		ctx, modelSelector, s.clock, &wg, errCh,
	)

	// The two sub-functions run sequentially, so we advance the clock twice:
	// once for the CR deletion checker, then once for the CRD deletion checker.
	err := s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)
	err = s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for DeleteClusterScopeAPIExtensionResourcesModelTeardown to return")
	}
	select {
	case err := <-errCh:
		c.Fatalf("unexpected error: %v", err)
	default:
	}
}

func (s *K8sBrokerSuite) TestDeleteNamespaceModelTeardown(c *gc.C) {
>>>>>>> 3.6
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{}
	ns.Name = "test"
	s.ensureJujuNamespaceAnnotations(false, ns)
	namespaceWatcher, namespaceFirer := k8swatchertest.NewKubernetesTestWatcher()
	s.k8sWatcherFn = k8swatchertest.NewKubernetesTestWatcherFunc(namespaceWatcher)

	s.mockNamespaces.EXPECT().Delete(gomock.Any(), "test", s.deleteOptions(v1.DeletePropagationForeground, "")).
		DoAndReturn(func(_ context.Context, _ string, _ v1.DeleteOptions) error {
			namespaceFirer()
			return nil
		})
	// still terminating.
	s.mockNamespaces.EXPECT().Get(gomock.Any(), "test", v1.GetOptions{}).
		DoAndReturn(func(_ context.Context, _ string, _ v1.GetOptions) (*core.Namespace, error) {
			namespaceFirer()
			return ns, nil
		})
	// terminated, not found returned.
	s.mockNamespaces.EXPECT().Get(gomock.Any(), "test", v1.GetOptions{}).
		Return(nil, s.k8sNotFoundError())

	var wg sync.WaitGroup
	wg.Add(1)

	errCh := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	go s.broker.DeleteNamespaceModelTeardown(ctx, &wg, errCh)

	select {
	case <-done:
		var err error
		select {
		case err = <-errCh:
		default:
		}
		c.Assert(err, tc.ErrorIsNil)
		for _, watcher := range s.watchers {
			c.Assert(workertest.CheckKilled(c, watcher), tc.ErrorIsNil)
		}
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for deleteNamespaceModelTeardown return")
	}
}

func (s *K8sBrokerSuite) TestDeleteNamespaceModelTeardownFailed(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{}
	ns.Name = "test"
	s.ensureJujuNamespaceAnnotations(false, ns)
	namespaceWatcher, namespaceFirer := k8swatchertest.NewKubernetesTestWatcher()
	s.k8sWatcherFn = k8swatchertest.NewKubernetesTestWatcherFunc(namespaceWatcher)

	s.mockNamespaces.EXPECT().Get(gomock.Any(), "test", v1.GetOptions{}).
		Return(ns, nil)
	s.mockNamespaces.EXPECT().Delete(gomock.Any(), "test", s.deleteOptions(v1.DeletePropagationForeground, "")).
		Return(nil)
	// still terminating.
	s.mockNamespaces.EXPECT().Get(gomock.Any(), "test", v1.GetOptions{}).
		DoAndReturn(func(_ context.Context, _ string, _ v1.GetOptions) (*core.Namespace, error) {
			namespaceFirer()
			return ns, nil
		})
	// terminated, not found returned.
	s.mockNamespaces.EXPECT().Get(gomock.Any(), "test", v1.GetOptions{}).
		Return(nil, errors.New("error bla"))

	var wg sync.WaitGroup
	wg.Add(1)

	errCh := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	go s.broker.DeleteNamespaceModelTeardown(ctx, &wg, errCh)

	select {
	case <-done:
		var err error
		select {
		case err = <-errCh:
		default:
		}
		c.Assert(err, tc.ErrorMatches, `getting namespace "test": error bla`)
		for _, watcher := range s.watchers {
			c.Assert(workertest.CheckKilled(c, watcher), tc.ErrorIsNil)
		}
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for deleteNamespaceModelTeardown return")
	}
}
