// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sutils "github.com/juju/juju/caas/kubernetes/provider/utils"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	k8swatchertest "github.com/juju/juju/caas/kubernetes/provider/watcher/test"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/assumes"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/testing"
)

type K8sSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&K8sSuite{})

func getBasicPodspec() *specs.PodSpec {
	pSpecs := &specs.PodSpec{}
	pSpecs.Containers = []specs.ContainerSpec{{
		Name:         "test",
		Ports:        []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
		ImageDetails: specs.ImageDetails{ImagePath: "juju-repo.local/juju/image", Username: "fred", Password: "secret"},
		Command:      []string{"sh", "-c"},
		Args:         []string{"doIt", "--debug"},
		WorkingDir:   "/path/to/here",
		EnvConfig: map[string]interface{}{
			"foo":        "bar",
			"restricted": "yes",
			"bar":        true,
			"switch":     true,
			"brackets":   `["hello", "world"]`,
		},
	}, {
		Name:  "test2",
		Ports: []specs.ContainerPort{{ContainerPort: 8080, Protocol: "TCP", Name: "fred"}},
		Image: "juju-repo.local/juju/image2",
	}}
	return pSpecs
}

type K8sBrokerSuite struct {
	BaseSuite
}

var _ = tc.Suite(&K8sBrokerSuite{})

func (s *K8sBrokerSuite) TestNoNamespaceBroker(c *tc.C) {
	ctrl := gomock.NewController(c)

	s.clock = testclock.NewClock(time.Time{})

	newK8sClientFunc, newK8sRestFunc := s.setupK8sRestClient(c, ctrl, "")
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}
	watcherFn := k8swatcher.NewK8sWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		return nil, errors.NewNotFound(nil, "undefined k8sWatcherFn for base test")
	})
	stringsWatcherFn := k8swatcher.NewK8sStringsWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock, e []string,
		f k8swatcher.K8sStringsWatcherFilterFunc) (k8swatcher.KubernetesStringsWatcher, error) {
		return nil, errors.NewNotFound(nil, "undefined k8sStringsWatcherFn for base test")
	})

	var err error
	s.broker, err = provider.NewK8sBroker(context.Background(), testing.ControllerTag.Id(), s.k8sRestConfig, s.cfg, "", newK8sClientFunc, newK8sRestFunc,
		watcherFn, stringsWatcherFn, randomPrefixFunc, s.clock)
	c.Assert(err, tc.ErrorIsNil)

	// Test namespace is actually empty string and a namespaced method fails.
	_, err = s.broker.GetPod(context.Background(), "test")
	c.Assert(err, tc.ErrorMatches, `bootstrap broker or no namespace not provisioned`)

	nsInput := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name: "test",
		},
	})

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), "test", v1.GetOptions{}).Times(2).
			Return(nsInput, nil),
	)

	// Check a cluster wide resource is still accessible.
	ns, err := s.broker.GetNamespace(context.Background(), "test")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ns, tc.DeepEquals, nsInput)
}

func (s *K8sBrokerSuite) TestEnsureNamespaceAnnotationForControllerUUIDMigrated(c *tc.C) {
	ctrl := gomock.NewController(c)

	newK8sClientFunc, newK8sRestFunc := s.setupK8sRestClient(c, ctrl, s.getNamespace())
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}

	newControllerUUID := names.NewControllerTag("deadbeef-1bad-500d-9000-4b1d0d06f00e").Id()
	nsBefore := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name:   "test",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "model.juju.is/name": "test"},
		},
	})
	nsAfter := *nsBefore
	nsAfter.SetAnnotations(annotations.New(nsAfter.GetAnnotations()).Add(
		k8sutils.AnnotationControllerUUIDKey(1), newControllerUUID,
	))
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), s.getNamespace(), v1.GetOptions{}).Times(2).
			Return(nsBefore, nil),
		s.mockNamespaces.EXPECT().Update(gomock.Any(), &nsAfter, v1.UpdateOptions{}).Times(1).
			Return(&nsAfter, nil),
	)
	s.setupBroker(c, ctrl, newControllerUUID, newK8sClientFunc, newK8sRestFunc, randomPrefixFunc, "").Finish()
}

func (s *K8sBrokerSuite) TestEnsureNamespaceAnnotationForControllerUUIDNotMigrated(c *tc.C) {
	ctrl := gomock.NewController(c)

	newK8sClientFunc, newK8sRestFunc := s.setupK8sRestClient(c, ctrl, s.getNamespace())
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}

	ns := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name:   "test",
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "model.juju.is/name": "test"},
		},
	})
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), s.getNamespace(), v1.GetOptions{}).Times(2).
			Return(ns, nil),
	)
	s.setupBroker(c, ctrl, testing.ControllerTag.Id(), newK8sClientFunc, newK8sRestFunc, randomPrefixFunc, "").Finish()
}

func (s *K8sBrokerSuite) TestEnsureNamespaceAnnotationForControllerUUIDNameSpaceNotCreatedYet(c *tc.C) {
	ctrl := gomock.NewController(c)

	newK8sClientFunc, newK8sRestFunc := s.setupK8sRestClient(c, ctrl, s.getNamespace())
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), s.getNamespace(), v1.GetOptions{}).Times(2).
			Return(nil, s.k8sNotFoundError()),
	)
	s.setupBroker(c, ctrl, testing.ControllerTag.Id(), newK8sClientFunc, newK8sRestFunc, randomPrefixFunc, "").Finish()
}

func (s *K8sBrokerSuite) TestEnsureNamespaceAnnotationForControllerUUIDNameSpaceExists(c *tc.C) {
	ctrl := gomock.NewController(c)

	newK8sClientFunc, newK8sRestFunc := s.setupK8sRestClient(c, ctrl, s.getNamespace())
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), s.getNamespace(), v1.GetOptions{}).Times(2).
			Return(&core.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name: "test",
					Labels: map[string]string{
						"model.juju.is/name": "test",
						"model.juju.is/id":   "deadbeef-1bad-500d-9000-4b1d0d06f00d",
					},
				},
			}, nil),
	)
	s.setupBroker(c, ctrl, testing.ControllerTag.Id(), newK8sClientFunc, newK8sRestFunc, randomPrefixFunc, "").Finish()
}

func (s *K8sBrokerSuite) TestAPIVersion(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "16",
		}, nil),
	)

	ver, err := s.broker.APIVersion()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ver, tc.DeepEquals, "1.16.0")
}

func (s *K8sBrokerSuite) TestConfig(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	c.Assert(s.broker.Config(), tc.DeepEquals, s.cfg)
}

func (s *K8sBrokerSuite) TestSetConfig(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.SetConfig(context.Background(), s.cfg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestBootstrapNoWorkloadStorage(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ctx := envtesting.BootstrapContext(context.Background(), c)
	bootstrapParams := environs.BootstrapParams{
		ControllerConfig:        testing.FakeControllerConfig(),
		BootstrapConstraints:    constraints.MustParse("mem=3.5G"),
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	}

	_, err := s.broker.Bootstrap(ctx, bootstrapParams)
	c.Assert(err, tc.NotNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, tc.Matches, "config without workload-storage value not valid.*")
}

func (s *K8sBrokerSuite) TestBootstrap(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// Ensure the broker is configured with workload storage.
	s.setupWorkloadStorageConfig(c)

	ctx := envtesting.BootstrapContext(context.Background(), c)
	bootstrapParams := environs.BootstrapParams{
		ControllerConfig:        testing.FakeControllerConfig(),
		BootstrapConstraints:    constraints.MustParse("mem=3.5G"),
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	}

	sc := &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "some-storage",
		},
	}
	gomock.InOrder(
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "test-some-storage", v1.GetOptions{}).
			Return(sc, nil),
	)
	result, err := s.broker.Bootstrap(ctx, bootstrapParams)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Arch, tc.Equals, "amd64")
	c.Assert(result.CaasBootstrapFinalizer, tc.NotNil)

	bootstrapParams.BootstrapBase = corebase.MustParseBaseFromString("ubuntu@22.04")
	_, err = s.broker.Bootstrap(ctx, bootstrapParams)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *K8sBrokerSuite) setupWorkloadStorageConfig(c *tc.C) {
	cfg := s.broker.Config()
	var err error
	cfg, err = cfg.Apply(map[string]interface{}{"workload-storage": "some-storage"})
	c.Assert(err, tc.ErrorIsNil)
	err = s.broker.SetConfig(context.Background(), cfg)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestPrepareForBootstrap(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// Ensure the broker is configured with workload storage.
	s.setupWorkloadStorageConfig(c)

	sc := &storagev1.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "some-storage",
		},
	}

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), "controller-ctrl-1", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockNamespaces.EXPECT().List(gomock.Any(), v1.ListOptions{}).
			Return(&core.NamespaceList{Items: []core.Namespace{}}, nil),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "controller-ctrl-1-some-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get(gomock.Any(), "some-storage", v1.GetOptions{}).
			Return(sc, nil),
	)
	ctx := envtesting.BootstrapContext(context.Background(), c)
	c.Assert(
		s.broker.PrepareForBootstrap(ctx, "ctrl-1"), tc.ErrorIsNil,
	)
	c.Assert(s.broker.Namespace(), tc.DeepEquals, "controller-ctrl-1")
}

func (s *K8sBrokerSuite) TestPrepareForBootstrapAlreadyExistNamespaceError(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "controller-ctrl-1"}}
	s.ensureJujuNamespaceAnnotations(true, ns)
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), "controller-ctrl-1", v1.GetOptions{}).
			Return(ns, nil),
	)
	ctx := envtesting.BootstrapContext(context.Background(), c)
	c.Assert(
		s.broker.PrepareForBootstrap(ctx, "ctrl-1"), tc.ErrorIs, errors.AlreadyExists,
	)
}

func (s *K8sBrokerSuite) TestPrepareForBootstrapAlreadyExistControllerAnnotations(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "controller-ctrl-1"}}
	s.ensureJujuNamespaceAnnotations(true, ns)
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), "controller-ctrl-1", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockNamespaces.EXPECT().List(gomock.Any(), v1.ListOptions{}).
			Return(&core.NamespaceList{Items: []core.Namespace{*ns}}, nil),
	)
	ctx := envtesting.BootstrapContext(context.Background(), c)
	c.Assert(
		s.broker.PrepareForBootstrap(ctx, "ctrl-1"), tc.ErrorIs, errors.AlreadyExists,
	)
}

func (s *K8sBrokerSuite) TestGetNamespace(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}}
	s.ensureJujuNamespaceAnnotations(false, ns)
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), "test", v1.GetOptions{}).
			Return(ns, nil),
	)

	out, err := s.broker.GetNamespace(context.Background(), "test")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, ns)
}

func (s *K8sBrokerSuite) TestGetNamespaceNotFound(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get(gomock.Any(), "unknown-namespace", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
	)

	out, err := s.broker.GetNamespace(context.Background(), "unknown-namespace")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(out, tc.IsNil)
}

func (s *K8sBrokerSuite) TestNamespaces(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns1 := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}})
	ns2 := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test2"}})
	gomock.InOrder(
		s.mockNamespaces.EXPECT().List(gomock.Any(), v1.ListOptions{}).
			Return(&core.NamespaceList{Items: []core.Namespace{*ns1, *ns2}}, nil),
	)

	result, err := s.broker.Namespaces(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{"test", "test2"})
}

func (s *K8sBrokerSuite) assertDestroy(c *tc.C, isController bool, destroyFunc func() error) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// CRs of this Cluster scope CRD will get deleted.
	crdClusterScope := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      "tfjobs.kubeflow.org",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
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
			Name:      "tfjobs.kubeflow.org",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
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

	ns := &core.Namespace{}
	ns.Name = "test"
	s.ensureJujuNamespaceAnnotations(isController, ns)
	namespaceWatcher, namespaceFirer := k8swatchertest.NewKubernetesTestWatcher()
	s.k8sWatcherFn = k8swatchertest.NewKubernetesTestWatcherFunc(namespaceWatcher)

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
	s.mockCustomResourceDefinitionV1.EXPECT().List(gomock.Any(), v1.ListOptions{
		LabelSelector: "juju-resource-lifecycle notin (persistent),model.juju.is/id=deadbeef-0bad-400d-8000-4b1d0d06f00d,model.juju.is/name=test",
	}).AnyTimes().
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

	errCh := make(chan error)
	go func() {
		errCh <- destroyFunc()
	}()

	err := s.clock.WaitAdvance(time.Second, testing.ShortWait, 6)
	c.Assert(err, tc.ErrorIsNil)
	err = s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case err := <-errCh:
		c.Assert(err, tc.ErrorIsNil)
		for _, watcher := range s.watchers {
			c.Assert(workertest.CheckKilled(c, watcher), tc.ErrorIsNil)
		}
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for destroyFunc return")
	}
}

func (s *K8sBrokerSuite) TestDestroyController(c *tc.C) {
	s.assertDestroy(c, true, func() error {
		return s.broker.DestroyController(context.Background(), testing.ControllerTag.Id())
	})
}

func (s *K8sBrokerSuite) TestEnsureImageRepoSecret(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	imageRepo := docker.ImageRepoDetails{
		Repository:    "test-account",
		ServerAddress: "quay.io",
		BasicAuthConfig: docker.BasicAuthConfig{
			Auth: docker.NewToken("xxxxx=="),
		},
	}

	data, err := imageRepo.SecretData()
	c.Assert(err, tc.ErrorIsNil)

	secret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-image-pull-secret",
			Namespace: "test",
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "juju"},
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"model.juju.is/id":      testing.ModelTag.Id(),
			},
		},
		Type: core.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			core.DockerConfigJsonKey: data,
		},
	}

	gomock.InOrder(
		s.mockSecrets.EXPECT().Create(gomock.Any(), secret, v1.CreateOptions{}).
			Return(secret, nil),
	)
	err = s.broker.EnsureImageRepoSecret(context.Background(), imageRepo)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestDestroy(c *tc.C) {
	s.assertDestroy(c, false, func() error {
		return s.broker.Destroy(context.Background())
	})
}

func (s *K8sBrokerSuite) TestGetCurrentNamespace(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
	c.Assert(s.broker.Namespace(), tc.DeepEquals, s.getNamespace())
}

func (s *K8sBrokerSuite) TestCreateModelResources(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
			Name:   "test",
		},
	})
	s.mockNamespaces.EXPECT().Create(gomock.Any(), ns, v1.CreateOptions{}).
		Return(ns, nil)

	err := s.broker.CreateModelResources(
		context.Background(),
		environs.CreateParams{},
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestValidateProviderForNewModel(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.mockNamespaces.EXPECT().Get(gomock.Any(), s.getNamespace(), v1.GetOptions{}).
		Return(nil, s.k8sNotFoundError())

	err := s.broker.ValidateProviderForNewModel(
		context.Background(),
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestValidateProviderForNewModelAlreadyExists(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "model.juju.is/id": "deadbeef-0bad-400d-8000-4b1d0d06f00d", "model.juju.is/name": "test"},
			Name:   "test",
		},
	})
	s.mockNamespaces.EXPECT().Get(gomock.Any(), s.getNamespace(), v1.GetOptions{}).
		Return(ns, nil)

	err := s.broker.ValidateProviderForNewModel(
		context.Background(),
	)
	c.Assert(err, tc.ErrorIs, errors.AlreadyExists)
}

func (s *K8sBrokerSuite) TestSupportedFeatures(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Return(&k8sversion.Info{
			Major: "1", Minor: "15+",
		}, nil),
	)

	fs, err := s.broker.SupportedFeatures()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fs.AsList(), tc.DeepEquals, []assumes.Feature{
		{
			Name:        "k8s-api",
			Description: "the Kubernetes API lets charms query and manipulate the state of API objects in a Kubernetes cluster",
			Version:     &semversion.Number{Major: 1, Minor: 15},
		},
	})
}

func (s *K8sBrokerSuite) TestGetServiceSvcNotFound(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockServices.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=juju,app.kubernetes.io/name=app-name"}).
			Return(&core.ServiceList{Items: []core.Service{}}, nil),

		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDaemonSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
	)

	caasSvc, err := s.broker.GetService(context.Background(), "app-name", false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(caasSvc, tc.DeepEquals, &caas.Service{})
}

func (s *K8sBrokerSuite) assertGetService(c *tc.C, expectedSvcResult *caas.Service, assertCalls ...any) {
	selectorLabels := map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"}
	labels := k8sutils.LabelsMerge(selectorLabels, k8sutils.LabelsJuju)

	selector := k8sutils.LabelsToSelector(labels).String()
	svc := core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: labels,
			Annotations: map[string]string{
				"controller.juju.is/id": testing.ControllerTag.Id(),
				"fred":                  "mary",
				"a":                     "b",
			}},
		Spec: core.ServiceSpec{
			Selector: selectorLabels,
			Type:     core.ServiceTypeLoadBalancer,
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
		Status: core.ServiceStatus{
			LoadBalancer: core.LoadBalancerStatus{
				Ingress: []core.LoadBalancerIngress{{
					Hostname: "host.com.au",
				}},
			},
		},
	}
	svc.SetUID("uid-xxxxx")
	svcHeadless := core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name-endpoints",
			Labels: labels,
			Annotations: map[string]string{
				"juju.io/controller": testing.ControllerTag.Id(),
				"fred":               "mary",
				"a":                  "b",
			}},
		Spec: core.ServiceSpec{
			Selector: labels,
			Type:     core.ServiceTypeClusterIP,
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
			},
			ClusterIP: "192.168.1.1",
		},
	}
	gomock.InOrder(
		append([]any{
			s.mockServices.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: selector}).
				Return(&core.ServiceList{Items: []core.Service{svcHeadless, svc}}, nil),

			s.mockStatefulSets.EXPECT().Get(gomock.Any(), "juju-operator-app-name", v1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
		}, assertCalls...)...,
	)

	caasSvc, err := s.broker.GetService(context.Background(), "app-name", false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(caasSvc, tc.DeepEquals, expectedSvcResult)
}

func (s *K8sBrokerSuite) TestGetServiceSvcFoundNoWorkload(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
	s.assertGetService(c,
		&caas.Service{
			Id: "uid-xxxxx",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopePublic)).AsProviderAddress(),
				network.NewMachineAddress("host.com.au", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			},
		},
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDaemonSets.EXPECT().Get(gomock.Any(), "app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
	)
}

func (s *K8sBrokerSuite) TestGetServiceSvcFoundWithStatefulSet(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	basicPodSpec.Service = &specs.ServiceSpec{
		ScalePolicy: "serial",
	}

	appName := "app-name"
	numUnits := int32(2)
	workload := &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   appName,
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "app-name"},
			Annotations: map[string]string{
				"app.juju.is/uuid":               "appuuid",
				"controller.juju.is/id":          testing.ControllerTag.Id(),
				"charm.juju.is/modified-version": "0",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"controller.juju.is/id":                    testing.ControllerTag.Id(),
						"charm.juju.is/modified-version":           "0",
					},
				},
				Spec: core.PodSpec{},
			},
			PodManagementPolicy: appsv1.PodManagementPolicyType("OrderedReady"),
			ServiceName:         "app-name-endpoints",
		},
	}
	workload.SetGeneration(1)

	var expectedCalls []any
	expectedCalls = append(expectedCalls,
		s.mockStatefulSets.EXPECT().Get(gomock.Any(), appName, v1.GetOptions{}).
			Return(workload, nil),
		s.mockEvents.EXPECT().List(gomock.Any(),
			listOptionsFieldSelectorMatcher(fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=StatefulSet", appName)),
		).Return(&core.EventList{}, nil),
	)

	s.assertGetService(c,
		&caas.Service{
			Id: "uid-xxxxx",
			Addresses: network.ProviderAddresses{
				network.NewMachineAddress("10.0.0.1", network.WithScope(network.ScopePublic)).AsProviderAddress(),
				network.NewMachineAddress("host.com.au", network.WithScope(network.ScopePublic)).AsProviderAddress(),
			},
			Scale:      k8sutils.IntPtr(2),
			Generation: pointer.Int64Ptr(1),
			Status: status.StatusInfo{
				Status: status.Active,
			},
		},
		expectedCalls...,
	)
}

func (s *K8sBrokerSuite) TestUnits(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podWithStorage := core.Pod{
		TypeMeta: v1.TypeMeta{},
		ObjectMeta: v1.ObjectMeta{
			Name:              "pod-name",
			UID:               types.UID("uuid"),
			DeletionTimestamp: &v1.Time{},
			OwnerReferences:   []v1.OwnerReference{{Kind: "StatefulSet"}},
		},
		Status: core.PodStatus{
			Message: "running",
			PodIP:   "10.0.0.1",
		},
		Spec: core.PodSpec{
			Containers: []core.Container{{
				Ports: []core.ContainerPort{{
					ContainerPort: 666,
					Protocol:      "TCP",
				}},
				VolumeMounts: []core.VolumeMount{{
					Name:      "v1",
					MountPath: "/path/to/here",
					ReadOnly:  true,
				}},
			}},
			Volumes: []core.Volume{{
				Name: "v1",
				VolumeSource: core.VolumeSource{
					PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
						ClaimName: "v1-claim",
					},
				},
			}},
		},
	}
	podList := &core.PodList{
		Items: []core.Pod{{
			TypeMeta: v1.TypeMeta{},
			ObjectMeta: v1.ObjectMeta{
				Name: "pod-name",
				UID:  types.UID("uuid"),
			},
			Status: core.PodStatus{
				Message: "running",
			},
			Spec: core.PodSpec{
				Containers: []core.Container{{}},
			},
		}, podWithStorage},
	}

	pvc := &core.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			UID:    "pvc-uuid",
			Labels: map[string]string{"juju-storage": "database"},
		},
		Spec: core.PersistentVolumeClaimSpec{VolumeName: "v1"},
		Status: core.PersistentVolumeClaimStatus{
			Conditions: []core.PersistentVolumeClaimCondition{{Message: "mounted"}},
			Phase:      core.ClaimBound,
		},
	}
	pv := &core.PersistentVolume{
		Spec: core.PersistentVolumeSpec{
			Capacity: core.ResourceList{
				"size": resource.MustParse("10Mi"),
			},
		},
		Status: core.PersistentVolumeStatus{
			Message: "vol-mounted",
			Phase:   core.VolumeBound,
		},
	}
	gomock.InOrder(
		s.mockPods.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: "app.kubernetes.io/name=app-name"}).Return(podList, nil),
		s.mockPersistentVolumeClaims.EXPECT().Get(gomock.Any(), "v1-claim", v1.GetOptions{}).
			Return(pvc, nil),
		s.mockPersistentVolumes.EXPECT().Get(gomock.Any(), "v1", v1.GetOptions{}).
			Return(pv, nil),
	)

	units, err := s.broker.Units(context.Background(), "app-name")
	c.Assert(err, tc.ErrorIsNil)
	now := s.clock.Now()
	c.Assert(units, tc.DeepEquals, []caas.Unit{{
		Id:       "uuid",
		Address:  "",
		Ports:    nil,
		Dying:    false,
		Stateful: false,
		Status: status.StatusInfo{
			Status:  "unknown",
			Message: "running",
			Since:   &now,
		},
		FilesystemInfo: nil,
	}, {
		Id:       "pod-name",
		Address:  "10.0.0.1",
		Ports:    []string{"666/TCP"},
		Dying:    true,
		Stateful: true,
		Status: status.StatusInfo{
			Status:  "terminated",
			Message: "running",
			Since:   &now,
		},
		FilesystemInfo: []caas.FilesystemInfo{{
			StorageName:  "database",
			FilesystemId: "pvc-uuid",
			Size:         uint64(podWithStorage.Spec.Volumes[0].PersistentVolumeClaim.Size()),
			MountPoint:   "/path/to/here",
			ReadOnly:     true,
			Status: status.StatusInfo{
				Status:  "attached",
				Message: "mounted",
				Since:   &now,
			},
			Volume: caas.VolumeInfo{
				Size: uint64(pv.Size()),
				Status: status.StatusInfo{
					Status:  "attached",
					Message: "vol-mounted",
					Since:   &now,
				},
				Persistent: true,
			},
		}},
	}})
}

func (s *K8sBrokerSuite) TestAnnotateUnit(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	pod := &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "pod-name",
		},
	}

	updatePod := &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        "pod-name",
			Annotations: map[string]string{"unit.juju.is/id": "appname/0"},
		},
	}

	patch := []byte(`{"metadata":{"annotations":{"unit.juju.is/id":"appname/0"}}}`)

	gomock.InOrder(
		s.mockPods.EXPECT().Get(gomock.Any(), "pod-name", v1.GetOptions{}).Return(pod, nil),
		s.mockPods.EXPECT().Patch(gomock.Any(), "pod-name", types.MergePatchType, patch, v1.PatchOptions{}).Return(updatePod, nil),
	)

	err := s.broker.AnnotateUnit(context.Background(), "appname", "pod-name", names.NewUnitTag("appname/0"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestAnnotateUnitByUID(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podList := &core.PodList{
		Items: []core.Pod{{ObjectMeta: v1.ObjectMeta{
			Name: "pod-name",
			UID:  types.UID("uuid"),
		}}},
	}

	updatePod := &core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:        "pod-name",
			UID:         types.UID("uuid"),
			Annotations: map[string]string{"unit.juju.is/id": "appname/0"},
		},
	}

	patch := []byte(`{"metadata":{"annotations":{"unit.juju.is/id":"appname/0"}}}`)

	labelSelector := "app.kubernetes.io/name=appname"
	gomock.InOrder(
		s.mockPods.EXPECT().Get(gomock.Any(), "uuid", v1.GetOptions{}).Return(nil, s.k8sNotFoundError()),
		s.mockPods.EXPECT().List(gomock.Any(), v1.ListOptions{LabelSelector: labelSelector}).Return(podList, nil),
		s.mockPods.EXPECT().Patch(gomock.Any(), "pod-name", types.MergePatchType, patch, v1.PatchOptions{}).Return(updatePod, nil),
	)

	err := s.broker.AnnotateUnit(context.Background(), "appname", "uuid", names.NewUnitTag("appname/0"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestWatchUnits(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podWatcher, podFirer := k8swatchertest.NewKubernetesTestWatcher()
	s.k8sWatcherFn = func(si cache.SharedIndexInformer, n string, _ jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		c.Assert(n, tc.Equals, "test")
		return podWatcher, nil
	}

	w, err := s.broker.WatchUnits("test")
	c.Assert(err, tc.ErrorIsNil)

	podFirer()

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, tc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}
}

func (s *K8sBrokerSuite) TestUpdateStrategyForStatefulSet(c *tc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	_, err := provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{})
	c.Assert(err, tc.ErrorMatches, `strategy type "" for statefulset not valid`)

	o, err := provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type: "RollingUpdate",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(o, tc.DeepEquals, appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
	})

	_, err = provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type:          "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{},
	})
	c.Assert(err, tc.ErrorMatches, `rolling update spec partition is missing`)

	_, err = provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type: "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{
			Partition: pointer.Int32Ptr(10),
			MaxSurge:  &specs.IntOrString{IntVal: 10},
		},
	})
	c.Assert(err, tc.ErrorMatches, `rolling update spec for statefulset not valid`)

	_, err = provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type: "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{
			Partition:      pointer.Int32Ptr(10),
			MaxUnavailable: &specs.IntOrString{IntVal: 10},
		},
	})
	c.Assert(err, tc.ErrorMatches, `rolling update spec for statefulset not valid`)

	o, err = provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type: "OnDelete",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(o, tc.DeepEquals, appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.OnDeleteStatefulSetStrategyType,
	})

	_, err = provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type: "OnDelete",
		RollingUpdate: &specs.RollingUpdateSpec{
			Partition: pointer.Int32Ptr(10),
		},
	})
	c.Assert(err, tc.ErrorMatches, `rolling update spec is not supported for "OnDelete"`)

	o, err = provider.UpdateStrategyForStatefulSet(specs.UpdateStrategy{
		Type: "RollingUpdate",
		RollingUpdate: &specs.RollingUpdateSpec{
			Partition: pointer.Int32Ptr(10),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(o, tc.DeepEquals, appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
		RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
			Partition: pointer.Int32Ptr(10),
		},
	})
}
