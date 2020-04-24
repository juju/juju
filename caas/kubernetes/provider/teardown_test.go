// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"
	"sync"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/juju/juju/testing"
)

func (s *K8sBrokerSuite) TestDeleteClusterScopeResourcesModelTeardownSuccess(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// CRs of this Cluster scope CRD will get deleted.
	crdClusterScope := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:   "tfjobs.kubeflow.org",
			Labels: map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   "kubeflow.org",
			Version: "v1alpha2",
			Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{Name: "v1alpha2", Served: true, Storage: false},
			},
			Scope: apiextensionsv1beta1.ClusterScoped,
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
	// CRs of this namespaced scope CRD will be skipped.
	crdNamespacedScope := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:   "tfjobs.kubeflow.org",
			Labels: map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   "kubeflow.org",
			Version: "v1alpha2",
			Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{Name: "v1alpha2", Served: true, Storage: false},
			},
			Scope: apiextensionsv1beta1.NamespaceScoped,
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

	// timer +1.
	s.mockClusterRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-model=test"}).
		Return(&rbacv1.ClusterRoleBindingList{}, nil).
		After(
			s.mockClusterRoleBindings.EXPECT().DeleteCollection(
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "juju-model=test"},
			).Return(s.k8sNotFoundError()),
		)

	// timer +1.
	s.mockClusterRoles.EXPECT().List(v1.ListOptions{LabelSelector: "juju-model=test"}).
		Return(&rbacv1.ClusterRoleList{}, nil).
		After(
			s.mockClusterRoles.EXPECT().DeleteCollection(
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "juju-model=test"},
			).Return(s.k8sNotFoundError()),
		)

	// timer +1.
	s.mockNamespaceableResourceClient.EXPECT().List(
		// list all custom resources for crd "v1alpha2".
		v1.ListOptions{LabelSelector: "juju-model=test,juju-resource-lifecycle notin (persistent)"},
	).Return(&unstructured.UnstructuredList{}, nil).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1alpha2",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
	).After(
		// list all custom resources for crd "v1".
		s.mockNamespaceableResourceClient.EXPECT().List(
			v1.ListOptions{LabelSelector: "juju-model=test,juju-resource-lifecycle notin (persistent)"},
		).Return(&unstructured.UnstructuredList{}, nil),
	).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
	).After(
		// list cluster wide all custom resource definitions for listing custom resources.
		s.mockCustomResourceDefinition.EXPECT().List(v1.ListOptions{}).AnyTimes().
			Return(&apiextensionsv1beta1.CustomResourceDefinitionList{Items: []apiextensionsv1beta1.CustomResourceDefinition{*crdClusterScope, *crdNamespacedScope}}, nil),
	).After(
		// delete all custom resources for crd "v1alpha2".
		s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "juju-model=test,juju-resource-lifecycle notin (persistent)"},
		).Return(nil),
	).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1alpha2",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
	).After(
		// delete all custom resources for crd "v1".
		s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "juju-model=test,juju-resource-lifecycle notin (persistent)"},
		).Return(nil),
	).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
	).After(
		// list cluster wide all custom resource definitions for deleting custom resources.
		s.mockCustomResourceDefinition.EXPECT().List(v1.ListOptions{}).AnyTimes().
			Return(&apiextensionsv1beta1.CustomResourceDefinitionList{Items: []apiextensionsv1beta1.CustomResourceDefinition{*crdClusterScope, *crdNamespacedScope}}, nil),
	)

	// timer +1.
	s.mockCustomResourceDefinition.EXPECT().List(v1.ListOptions{LabelSelector: "juju-model=test,juju-resource-lifecycle notin (persistent)"}).AnyTimes().
		Return(&apiextensionsv1beta1.CustomResourceDefinitionList{}, nil).
		After(
			s.mockCustomResourceDefinition.EXPECT().DeleteCollection(
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "juju-model=test,juju-resource-lifecycle notin (persistent)"},
			).Return(s.k8sNotFoundError()),
		)

	// timer +1.
	s.mockMutatingWebhookConfiguration.EXPECT().List(v1.ListOptions{LabelSelector: "juju-model=test"}).
		Return(&admissionregistrationv1beta1.MutatingWebhookConfigurationList{}, nil).
		After(
			s.mockMutatingWebhookConfiguration.EXPECT().DeleteCollection(
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "juju-model=test"},
			).Return(s.k8sNotFoundError()),
		)

	// timer +1.
	s.mockValidatingWebhookConfiguration.EXPECT().List(v1.ListOptions{LabelSelector: "juju-model=test"}).
		Return(&admissionregistrationv1beta1.ValidatingWebhookConfigurationList{}, nil).
		After(
			s.mockValidatingWebhookConfiguration.EXPECT().DeleteCollection(
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "juju-model=test"},
			).Return(s.k8sNotFoundError()),
		)

	// timer +1.
	s.mockStorageClass.EXPECT().List(v1.ListOptions{LabelSelector: "juju-model=test"}).
		Return(&storagev1.StorageClassList{}, nil).
		After(
			s.mockStorageClass.EXPECT().DeleteCollection(
				s.deleteOptions(v1.DeletePropagationForeground, ""),
				v1.ListOptions{LabelSelector: "juju-model=test"},
			).Return(nil),
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

func (s *K8sBrokerSuite) TestDeleteClusterScopeResourcesModelTeardownTimeout(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// CRs of this Cluster scope CRD will get deleted.
	crdClusterScope := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:   "tfjobs.kubeflow.org",
			Labels: map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   "kubeflow.org",
			Version: "v1alpha2",
			Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{Name: "v1alpha2", Served: true, Storage: false},
			},
			Scope: apiextensionsv1beta1.ClusterScoped,
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
	// CRs of this namespaced scope CRD will be skipped.
	crdNamespacedScope := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:   "tfjobs.kubeflow.org",
			Labels: map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   "kubeflow.org",
			Version: "v1alpha2",
			Versions: []apiextensionsv1beta1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
				{Name: "v1alpha2", Served: true, Storage: false},
			},
			Scope: apiextensionsv1beta1.NamespaceScoped,
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

	s.mockClusterRoleBindings.EXPECT().DeleteCollection(
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "juju-model=test"},
	).Return(s.k8sNotFoundError())

	s.mockClusterRoles.EXPECT().DeleteCollection(
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "juju-model=test"},
	).Return(s.k8sNotFoundError())

	// delete all custom resources for crd "v1alpha2".
	s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "juju-model=test,juju-resource-lifecycle notin (persistent)"},
	).Return(nil).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1alpha2",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
	).After(
		// delete all custom resources for crd "v1".
		s.mockNamespaceableResourceClient.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "juju-model=test,juju-resource-lifecycle notin (persistent)"},
		).Return(nil),
	).After(
		s.mockDynamicClient.EXPECT().Resource(
			schema.GroupVersionResource{
				Group:    crdClusterScope.Spec.Group,
				Version:  "v1",
				Resource: crdClusterScope.Spec.Names.Plural,
			},
		).Return(s.mockNamespaceableResourceClient),
	).After(
		// list cluster wide all custom resource definitions for deleting custom resources.
		s.mockCustomResourceDefinition.EXPECT().List(v1.ListOptions{}).AnyTimes().
			Return(&apiextensionsv1beta1.CustomResourceDefinitionList{Items: []apiextensionsv1beta1.CustomResourceDefinition{*crdClusterScope, *crdNamespacedScope}}, nil),
	)

	s.mockCustomResourceDefinition.EXPECT().DeleteCollection(
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "juju-model=test,juju-resource-lifecycle notin (persistent)"},
	).Return(s.k8sNotFoundError())

	s.mockMutatingWebhookConfiguration.EXPECT().DeleteCollection(
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "juju-model=test"},
	).Return(s.k8sNotFoundError())

	s.mockValidatingWebhookConfiguration.EXPECT().DeleteCollection(
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "juju-model=test"},
	).Return(s.k8sNotFoundError())

	s.mockStorageClass.EXPECT().DeleteCollection(
		s.deleteOptions(v1.DeletePropagationForeground, ""),
		v1.ListOptions{LabelSelector: "juju-model=test"},
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
		c.Assert(<-errCh, gc.ErrorMatches, `context deadline exceeded`)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for DeleteClusterScopeResourcesModelTeardown return")
	}
}

func (s *K8sBrokerSuite) TestDeleteNamespaceModelTeardown(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{}
	ns.Name = "test"
	s.ensureJujuNamespaceAnnotations(false, ns)
	namespaceWatcher, namespaceFirer := newKubernetesTestWatcher()
	s.k8sWatcherFn = newK8sWatcherFunc(namespaceWatcher)

	s.mockNamespaces.EXPECT().Get("test", v1.GetOptions{}).
		Return(ns, nil)
	s.mockNamespaces.EXPECT().Delete("test", s.deleteOptions(v1.DeletePropagationForeground, "")).
		Return(nil)
	// still terminating.
	s.mockNamespaces.EXPECT().Get("test", v1.GetOptions{}).
		DoAndReturn(func(_, _ interface{}) (*core.Namespace, error) {
			namespaceFirer()
			return ns, nil
		})
	// terminated, not found returned.
	s.mockNamespaces.EXPECT().Get("test", v1.GetOptions{}).
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

func (s *K8sBrokerSuite) TestDeleteNamespaceModelTeardownFailed(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{}
	ns.Name = "test"
	s.ensureJujuNamespaceAnnotations(false, ns)
	namespaceWatcher, namespaceFirer := newKubernetesTestWatcher()
	s.k8sWatcherFn = newK8sWatcherFunc(namespaceWatcher)

	s.mockNamespaces.EXPECT().Get("test", v1.GetOptions{}).
		Return(ns, nil)
	s.mockNamespaces.EXPECT().Delete("test", s.deleteOptions(v1.DeletePropagationForeground, "")).
		Return(nil)
	// still terminating.
	s.mockNamespaces.EXPECT().Get("test", v1.GetOptions{}).
		DoAndReturn(func(_, _ interface{}) (*core.Namespace, error) {
			namespaceFirer()
			return ns, nil
		})
	// terminated, not found returned.
	s.mockNamespaces.EXPECT().Get("test", v1.GetOptions{}).
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
		c.Assert(err, gc.ErrorMatches, `getting namespace "test": error bla`)
		for _, watcher := range s.watchers {
			c.Assert(workertest.CheckKilled(c, watcher), jc.ErrorIsNil)
		}
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for deleteNamespaceModelTeardown return")
	}
}
