// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/mocks"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.BaseSuite

	clock               *testclock.Clock
	broker              *provider.KubernetesClient
	cfg                 *config.Config
	k8sRestConfig       *rest.Config
	k8sWatcherFn        provider.NewK8sWatcherFunc
	k8sStringsWatcherFn provider.NewK8sStringsWatcherFunc

	namespace string

	k8sClient                  *mocks.MockInterface
	mockRestClient             *mocks.MockRestClientInterface
	mockNamespaces             *mocks.MockNamespaceInterface
	mockApps                   *mocks.MockAppsV1Interface
	mockExtensions             *mocks.MockExtensionsV1beta1Interface
	mockSecrets                *mocks.MockSecretInterface
	mockDeployments            *mocks.MockDeploymentInterface
	mockStatefulSets           *mocks.MockStatefulSetInterface
	mockDaemonSets             *mocks.MockDaemonSetInterface
	mockPods                   *mocks.MockPodInterface
	mockServices               *mocks.MockServiceInterface
	mockConfigMaps             *mocks.MockConfigMapInterface
	mockPersistentVolumes      *mocks.MockPersistentVolumeInterface
	mockPersistentVolumeClaims *mocks.MockPersistentVolumeClaimInterface
	mockStorage                *mocks.MockStorageV1Interface
	mockStorageClass           *mocks.MockStorageClassInterface
	mockIngressInterface       *mocks.MockIngressInterface
	mockNodes                  *mocks.MockNodeInterface
	mockEvents                 *mocks.MockEventInterface

	mockApiextensionsV1          *mocks.MockApiextensionsV1beta1Interface
	mockApiextensionsClient      *mocks.MockApiExtensionsClientInterface
	mockCustomResourceDefinition *mocks.MockCustomResourceDefinitionInterface

	mockMutatingWebhookConfiguration   *mocks.MockMutatingWebhookConfigurationInterface
	mockValidatingWebhookConfiguration *mocks.MockValidatingWebhookConfigurationInterface

	mockDynamicClient               *mocks.MockDynamicInterface
	mockResourceClient              *mocks.MockResourceInterface
	mockNamespaceableResourceClient *mocks.MockNamespaceableResourceInterface

	mockServiceAccounts     *mocks.MockServiceAccountInterface
	mockRoles               *mocks.MockRoleInterface
	mockClusterRoles        *mocks.MockClusterRoleInterface
	mockRoleBindings        *mocks.MockRoleBindingInterface
	mockClusterRoleBindings *mocks.MockClusterRoleBindingInterface

	mockDiscovery *mocks.MockDiscoveryInterface

	watchers []provider.KubernetesNotifyWatcher
}

type genericMatcher struct {
	description string
	matcher     func(interface{}) (bool, string)
}

func genericMatcherFn(matcher func(interface{}) (bool, string)) *genericMatcher {
	return &genericMatcher{
		matcher: matcher,
	}
}

func (g *genericMatcher) Matches(i interface{}) bool {
	if g.matcher == nil {
		return false
	}
	rval, des := g.matcher(i)
	g.description = des
	return rval
}

func (g *genericMatcher) String() string {
	return g.description
}

func listOptionsFieldSelectorMatcher(fieldSelector string) gomock.Matcher {
	return genericMatcherFn(
		func(i interface{}) (bool, string) {
			lo, ok := i.(v1.ListOptions)
			if !ok {
				return false, "is list options, not a valid corev1.ListOptions"
			}
			return lo.FieldSelector == fieldSelector,
				fmt.Sprintf("is list options field %v == %v", lo.FieldSelector, fieldSelector)
		})
}

func listOptionsLabelSelectorMatcher(labelSelector string) gomock.Matcher {
	return genericMatcherFn(
		func(i interface{}) (bool, string) {
			lo, ok := i.(v1.ListOptions)
			if !ok {
				return false, "is list options, not a valid corev1.ListOptions"
			}
			return lo.LabelSelector == labelSelector,
				fmt.Sprintf("is list options label %v == %v", lo.LabelSelector, labelSelector)
		})
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username":              "fred",
		"password":              "secret",
		"ClientCertificateData": "cert-data",
		"ClientKeyData":         "cert-key",
	})
	cloudSpec := environs.CloudSpec{
		Endpoint:       "some-host",
		Credential:     &cred,
		CACertificates: []string{testing.CACert},
	}
	var err error
	s.k8sRestConfig, err = provider.CloudSpecToK8sRestConfig(cloudSpec)
	c.Assert(err, jc.ErrorIsNil)

	// init config for each test for easier changing config inside test.
	cfg, err := config.New(config.UseDefaults, testing.FakeConfig().Merge(testing.Attrs{
		config.NameKey:              "test",
		provider.OperatorStorageKey: "",
		provider.WorkloadStorageKey: "",
	}))
	c.Assert(err, jc.ErrorIsNil)
	s.cfg = cfg

	s.namespace = s.cfg.Name()
}

func (s *BaseSuite) TearDownTest(c *gc.C) {
	// ensure previous broker setup all are all cleaned up because it should be re-initialized in setupController or errors.
	s.broker = nil
	s.clock = nil
	s.k8sClient = nil
	s.mockApiextensionsClient = nil
	s.watchers = nil

	s.BaseSuite.TearDownTest(c)
}

func (s *BaseSuite) getNamespace() string {
	if s.broker != nil {
		return s.broker.GetCurrentNamespace()
	}
	return s.namespace
}

func (s *BaseSuite) setupController(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	newK8sRestClientFunc := s.setupK8sRestClient(c, ctrl, s.getNamespace())
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}
	return s.setupBroker(c, ctrl, newK8sRestClientFunc, randomPrefixFunc)
}

func (s *BaseSuite) setupBroker(c *gc.C, ctrl *gomock.Controller,
	newK8sRestClientFunc provider.NewK8sClientFunc,
	randomPrefixFunc provider.RandomPrefixFunc) *gomock.Controller {
	s.clock = testclock.NewClock(time.Time{})

	watcherFn := provider.NewK8sWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock) (provider.KubernetesNotifyWatcher, error) {
		if s.k8sWatcherFn == nil {
			return nil, errors.NewNotFound(nil, "undefined k8sWatcherFn for base test")
		}

		w, err := s.k8sWatcherFn(i, n, c)
		if err == nil {
			s.watchers = append(s.watchers, w)
		}
		return w, err
	})

	stringsWatcherFn := provider.NewK8sStringsWatcherFunc(
		func(i cache.SharedIndexInformer, n string, c jujuclock.Clock, e []string,
			f provider.K8sStringsWatcherFilterFunc) (provider.KubernetesStringsWatcher, error) {
			if s.k8sStringsWatcherFn == nil {
				return nil, errors.NewNotFound(nil, "undefined k8sStringsWatcherFn for base test")
			}
			return s.k8sStringsWatcherFn(i, n, c, e, f)
		})

	var err error
	s.broker, err = provider.NewK8sBroker(testing.ControllerTag.Id(), s.k8sRestConfig, s.cfg, newK8sRestClientFunc,
		watcherFn, stringsWatcherFn, randomPrefixFunc, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl
}

func (s *BaseSuite) setupK8sRestClient(c *gc.C, ctrl *gomock.Controller, namespace string) provider.NewK8sClientFunc {
	s.k8sClient = mocks.NewMockInterface(ctrl)

	// Plug in the various k8s client modules we need.
	// eg namespaces, pods, services, ingress resources, volumes etc.
	// We make the mock and assign it to the corresponding core getter function.
	mockCoreV1 := mocks.NewMockCoreV1Interface(ctrl)
	s.k8sClient.EXPECT().CoreV1().AnyTimes().Return(mockCoreV1)

	s.mockRestClient = mocks.NewMockRestClientInterface(ctrl)
	mockCoreV1.EXPECT().RESTClient().AnyTimes().Return(s.mockRestClient)

	s.mockNamespaces = mocks.NewMockNamespaceInterface(ctrl)
	mockCoreV1.EXPECT().Namespaces().AnyTimes().Return(s.mockNamespaces)

	s.mockPods = mocks.NewMockPodInterface(ctrl)
	mockCoreV1.EXPECT().Pods(namespace).AnyTimes().Return(s.mockPods)

	s.mockServices = mocks.NewMockServiceInterface(ctrl)
	mockCoreV1.EXPECT().Services(namespace).AnyTimes().Return(s.mockServices)

	s.mockConfigMaps = mocks.NewMockConfigMapInterface(ctrl)
	mockCoreV1.EXPECT().ConfigMaps(namespace).AnyTimes().Return(s.mockConfigMaps)

	s.mockPersistentVolumes = mocks.NewMockPersistentVolumeInterface(ctrl)
	mockCoreV1.EXPECT().PersistentVolumes().AnyTimes().Return(s.mockPersistentVolumes)

	s.mockPersistentVolumeClaims = mocks.NewMockPersistentVolumeClaimInterface(ctrl)
	mockCoreV1.EXPECT().PersistentVolumeClaims(namespace).AnyTimes().Return(s.mockPersistentVolumeClaims)

	s.mockSecrets = mocks.NewMockSecretInterface(ctrl)
	mockCoreV1.EXPECT().Secrets(namespace).AnyTimes().Return(s.mockSecrets)

	s.mockNodes = mocks.NewMockNodeInterface(ctrl)
	mockCoreV1.EXPECT().Nodes().AnyTimes().Return(s.mockNodes)

	s.mockEvents = mocks.NewMockEventInterface(ctrl)
	mockCoreV1.EXPECT().Events(namespace).AnyTimes().Return(s.mockEvents)

	s.mockApps = mocks.NewMockAppsV1Interface(ctrl)
	s.mockExtensions = mocks.NewMockExtensionsV1beta1Interface(ctrl)
	s.mockStatefulSets = mocks.NewMockStatefulSetInterface(ctrl)
	s.mockDeployments = mocks.NewMockDeploymentInterface(ctrl)
	s.mockDaemonSets = mocks.NewMockDaemonSetInterface(ctrl)
	s.mockIngressInterface = mocks.NewMockIngressInterface(ctrl)
	s.k8sClient.EXPECT().ExtensionsV1beta1().AnyTimes().Return(s.mockExtensions)
	s.k8sClient.EXPECT().AppsV1().AnyTimes().Return(s.mockApps)
	s.mockApps.EXPECT().StatefulSets(namespace).AnyTimes().Return(s.mockStatefulSets)
	s.mockApps.EXPECT().Deployments(namespace).AnyTimes().Return(s.mockDeployments)
	s.mockApps.EXPECT().DaemonSets(namespace).AnyTimes().Return(s.mockDaemonSets)
	s.mockExtensions.EXPECT().Ingresses(namespace).AnyTimes().Return(s.mockIngressInterface)

	s.mockStorage = mocks.NewMockStorageV1Interface(ctrl)
	s.mockStorageClass = mocks.NewMockStorageClassInterface(ctrl)
	s.k8sClient.EXPECT().StorageV1().AnyTimes().Return(s.mockStorage)
	s.mockStorage.EXPECT().StorageClasses().AnyTimes().Return(s.mockStorageClass)

	s.mockApiextensionsClient = mocks.NewMockApiExtensionsClientInterface(ctrl)
	s.mockApiextensionsV1 = mocks.NewMockApiextensionsV1beta1Interface(ctrl)
	s.mockCustomResourceDefinition = mocks.NewMockCustomResourceDefinitionInterface(ctrl)
	s.mockApiextensionsClient.EXPECT().ApiextensionsV1beta1().AnyTimes().Return(s.mockApiextensionsV1)
	s.mockApiextensionsV1.EXPECT().CustomResourceDefinitions().AnyTimes().Return(s.mockCustomResourceDefinition)

	s.mockDynamicClient = mocks.NewMockDynamicInterface(ctrl)
	s.mockResourceClient = mocks.NewMockResourceInterface(ctrl)
	s.mockNamespaceableResourceClient = mocks.NewMockNamespaceableResourceInterface(ctrl)
	s.mockNamespaceableResourceClient.EXPECT().Namespace(namespace).AnyTimes().Return(s.mockResourceClient)

	s.mockServiceAccounts = mocks.NewMockServiceAccountInterface(ctrl)
	mockCoreV1.EXPECT().ServiceAccounts(namespace).AnyTimes().Return(s.mockServiceAccounts)

	mockAdmissionregistration := mocks.NewMockAdmissionregistrationV1beta1Interface(ctrl)
	s.mockMutatingWebhookConfiguration = mocks.NewMockMutatingWebhookConfigurationInterface(ctrl)
	mockAdmissionregistration.EXPECT().MutatingWebhookConfigurations().AnyTimes().Return(s.mockMutatingWebhookConfiguration)
	s.mockValidatingWebhookConfiguration = mocks.NewMockValidatingWebhookConfigurationInterface(ctrl)
	mockAdmissionregistration.EXPECT().ValidatingWebhookConfigurations().AnyTimes().Return(s.mockValidatingWebhookConfiguration)
	s.k8sClient.EXPECT().AdmissionregistrationV1beta1().AnyTimes().Return(mockAdmissionregistration)

	mockRbacV1 := mocks.NewMockRbacV1Interface(ctrl)
	s.k8sClient.EXPECT().RbacV1().AnyTimes().Return(mockRbacV1)

	s.mockRoles = mocks.NewMockRoleInterface(ctrl)
	mockRbacV1.EXPECT().Roles(namespace).AnyTimes().Return(s.mockRoles)
	s.mockClusterRoles = mocks.NewMockClusterRoleInterface(ctrl)
	mockRbacV1.EXPECT().ClusterRoles().AnyTimes().Return(s.mockClusterRoles)
	s.mockRoleBindings = mocks.NewMockRoleBindingInterface(ctrl)
	mockRbacV1.EXPECT().RoleBindings(namespace).AnyTimes().Return(s.mockRoleBindings)
	s.mockClusterRoleBindings = mocks.NewMockClusterRoleBindingInterface(ctrl)
	mockRbacV1.EXPECT().ClusterRoleBindings().AnyTimes().Return(s.mockClusterRoleBindings)

	s.mockDiscovery = mocks.NewMockDiscoveryInterface(ctrl)
	s.k8sClient.EXPECT().Discovery().AnyTimes().Return(s.mockDiscovery)

	return func(cfg *rest.Config) (kubernetes.Interface, apiextensionsclientset.Interface, dynamic.Interface, error) {
		c.Assert(cfg.Username, gc.Equals, "fred")
		c.Assert(cfg.Password, gc.Equals, "secret")
		c.Assert(cfg.Host, gc.Equals, "some-host")
		c.Assert(cfg.TLSClientConfig, jc.DeepEquals, rest.TLSClientConfig{
			CertData: []byte("cert-data"),
			KeyData:  []byte("cert-key"),
			CAData:   []byte(testing.CACert),
		})
		return s.k8sClient, s.mockApiextensionsClient, s.mockDynamicClient, nil
	}
}

func (s *BaseSuite) k8sNotFoundError() *k8serrors.StatusError {
	return k8serrors.NewNotFound(schema.GroupResource{}, "test")
}

func (s *BaseSuite) k8sAlreadyExistsError() *k8serrors.StatusError {
	return k8serrors.NewAlreadyExists(schema.GroupResource{}, "test")
}

func (s *BaseSuite) deleteOptions(policy v1.DeletionPropagation, uid types.UID) *v1.DeleteOptions {
	ops := &v1.DeleteOptions{
		PropagationPolicy: &policy,
	}
	if uid != "" {
		ops.Preconditions = &v1.Preconditions{UID: &uid}
	}
	return ops
}

func (s *BaseSuite) k8sNewFakeWatcher() *watch.RaceFreeFakeWatcher {
	return watch.NewRaceFreeFake()
}

func (s *BaseSuite) ensureJujuNamespaceAnnotations(isController bool, ns *core.Namespace) *core.Namespace {
	annotations := map[string]string{
		"juju.io/controller": testing.ControllerTag.Id(),
		"juju.io/model":      s.cfg.UUID(),
	}
	if isController {
		annotations["juju.io/is-controller"] = "true"
	}
	ns.SetAnnotations(annotations)
	return ns
}
