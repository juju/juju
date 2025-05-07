// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"

	k8swatchertest "github.com/juju/juju/caas/kubernetes/provider/watcher/test"
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
	// CRs of this namespaced scope CRD will be skipped.
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

	// timer +1.
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.broker.DeleteClusterScopeResourcesModelTeardown(ctx, &wg, errCh)

	err := s.clock.WaitAdvance(time.Second, testing.ShortWait, 6)
	c.Assert(err, jc.ErrorIsNil)
	err = s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)
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
	// CRs of this namespaced scope CRD will be skipped.
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

	// delete all custom resources for crd "v1alpha2".
	s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(gomock.Any(),
		s.deleteOptions(v1.DeletePropagationForeground, ""),
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

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go s.broker.DeleteClusterScopeResourcesModelTeardown(ctx, &wg, errCh)

	err := s.clock.WaitAdvance(500*time.Millisecond, testing.ShortWait, 6)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-done:
		c.Assert(<-errCh, tc.ErrorMatches, `context deadline exceeded`)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for DeleteClusterScopeResourcesModelTeardown return")
	}
}

func (s *K8sBrokerSuite) TestDeleteNamespaceModelTeardown(c *tc.C) {
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
		Return(nil, s.k8sNotFoundError())

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
	go s.broker.DeleteNamespaceModelTeardown(ctx, &wg, errCh)

	select {
	case <-done:
		for _, watcher := range s.watchers {
			c.Assert(workertest.CheckKilled(c, watcher), jc.ErrorIsNil)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.broker.DeleteNamespaceModelTeardown(ctx, &wg, errCh)

	select {
	case <-done:
		err := <-errCh
		c.Assert(err, tc.ErrorMatches, `getting namespace "test": error bla`)
		for _, watcher := range s.watchers {
			c.Assert(workertest.CheckKilled(c, watcher), jc.ErrorIsNil)
		}
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for deleteNamespaceModelTeardown return")
	}
}
