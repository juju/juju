// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"fmt"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	core "k8s.io/api/core/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsclientsetfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	k8sdynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	admissionregistrationv1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1"
	admissionregistrationv1beta1 "k8s.io/client-go/kubernetes/typed/admissionregistration/v1beta1"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	networkingv1 "k8s.io/client-go/kubernetes/typed/networking/v1"
	networkingv1beta1 "k8s.io/client-go/kubernetes/typed/networking/v1beta1"
	rbacv1 "k8s.io/client-go/kubernetes/typed/rbac/v1"
	storagev1 "k8s.io/client-go/kubernetes/typed/storage/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	k8s "github.com/juju/juju/caas/kubernetes"
	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	k8sprovider "github.com/juju/juju/internal/provider/kubernetes"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/mocks"
	"github.com/juju/juju/internal/provider/kubernetes/utils"
	k8swatcher "github.com/juju/juju/internal/provider/kubernetes/watcher"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type BaseSuite struct {
	coretesting.BaseSuite

	clock               *testclock.Clock
	broker              *k8sprovider.KubernetesClient
	cfg                 *config.Config
	k8sRestConfig       *rest.Config
	k8sWatcherFn        k8swatcher.NewK8sWatcherFunc
	k8sStringsWatcherFn k8swatcher.NewK8sStringsWatcherFunc

	namespace string

	k8sClient                  *mocks.MockInterface
	mockRestClient             *mocks.MockRestClientInterface
	mockNamespaces             *mocks.MockNamespaceInterface
	mockApps                   *mocks.MockAppsV1Interface
	mockNetworkingV1beta1      *mocks.MockNetworkingV1beta1Interface
	mockNetworkingV1           *mocks.MockNetworkingV1Interface
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
	mockIngressClasses         *mocks.MockIngressClassInterface
	mockIngressV1Beta1         *mocks.MockIngressV1Beta1Interface
	mockIngressV1              *mocks.MockIngressV1Interface
	mockNodes                  *mocks.MockNodeInterface
	mockEvents                 *mocks.MockEventInterface

	mockApiextensionsV1Beta1            *mocks.MockApiextensionsV1beta1Interface
	mockApiextensionsV1                 *mocks.MockApiextensionsV1Interface
	mockApiextensionsClient             *mocks.MockApiExtensionsClientInterface
	mockCustomResourceDefinitionV1Beta1 *mocks.MockCustomResourceDefinitionV1Beta1Interface
	mockCustomResourceDefinitionV1      *mocks.MockCustomResourceDefinitionV1Interface

	mockMutatingWebhookConfigurationV1        *mocks.MockMutatingWebhookConfigurationV1Interface
	mockValidatingWebhookConfigurationV1      *mocks.MockValidatingWebhookConfigurationV1Interface
	mockMutatingWebhookConfigurationV1Beta1   *mocks.MockMutatingWebhookConfigurationV1Beta1Interface
	mockValidatingWebhookConfigurationV1Beta1 *mocks.MockValidatingWebhookConfigurationV1Beta1Interface

	mockDynamicClient               *mocks.MockDynamicInterface
	mockResourceClient              *mocks.MockResourceInterface
	mockNamespaceableResourceClient *mocks.MockNamespaceableResourceInterface

	mockServiceAccounts     *mocks.MockServiceAccountInterface
	mockRoles               *mocks.MockRoleInterface
	mockClusterRoles        *mocks.MockClusterRoleInterface
	mockRoleBindings        *mocks.MockRoleBindingInterface
	mockClusterRoleBindings *mocks.MockClusterRoleBindingInterface

	mockDiscovery *mocks.MockDiscoveryInterface

	watchers []k8swatcher.KubernetesNotifyWatcher
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

func (s *BaseSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username":              "fred",
		"password":              "secret",
		"ClientCertificateData": "cert-data",
		"ClientKeyData":         "cert-key",
	})
	cloudSpec := environscloudspec.CloudSpec{
		Endpoint:       "some-host",
		Credential:     &cred,
		CACertificates: []string{coretesting.CACert},
	}
	var err error
	s.k8sRestConfig, err = k8s.CloudSpecToK8sRestConfig(cloudSpec)
	c.Assert(err, tc.ErrorIsNil)

	// init config for each test for easier changing config inside test.
	cfg, err := config.New(config.UseDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		config.NameKey:                  "test",
		k8sconstants.WorkloadStorageKey: "",
	}))
	c.Assert(err, tc.ErrorIsNil)
	s.cfg = cfg

	s.namespace = s.cfg.Name()
}

func (s *BaseSuite) TearDownTest(c *tc.C) {
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
		return s.broker.Namespace()
	}
	return s.namespace
}

func (s *BaseSuite) setupController(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	newK8sClientFunc, newK8sRestFunc := s.setupK8sRestClient(c, ctrl, s.getNamespace())
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}

	s.mockNamespaces.EXPECT().Get(gomock.Any(), s.getNamespace(), v1.GetOptions{}).Times(2).
		Return(nil, s.k8sNotFoundError())

	return s.setupBroker(c, ctrl, coretesting.ControllerTag.Id(), newK8sClientFunc, newK8sRestFunc, randomPrefixFunc, "")
}

func (s *BaseSuite) setupBroker(
	c *tc.C, ctrl *gomock.Controller, controllerUUID string,
	newK8sClientFunc k8sprovider.NewK8sClientFunc,
	newK8sRestFunc k8sprovider.NewK8sRestClientFunc,
	randomPrefixFunc utils.RandomPrefixFunc,
	expectErr string,
) *gomock.Controller {
	s.clock = testclock.NewClock(time.Time{})

	watcherFn := k8swatcher.NewK8sWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		if s.k8sWatcherFn == nil {
			return nil, errors.NewNotFound(nil, "undefined k8sWatcherFn for base test")
		}

		w, err := s.k8sWatcherFn(i, n, c)
		if err == nil {
			s.watchers = append(s.watchers, w)
		}
		return w, err
	})

	stringsWatcherFn := k8swatcher.NewK8sStringsWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock, e []string,
		f k8swatcher.K8sStringsWatcherFilterFunc) (k8swatcher.KubernetesStringsWatcher, error) {
		if s.k8sStringsWatcherFn == nil {
			return nil, errors.NewNotFound(nil, "undefined k8sStringsWatcherFn for base test")
		}
		return s.k8sStringsWatcherFn(i, n, c, e, f)
	})

	var err error
	s.broker, err = k8sprovider.NewK8sBroker(c.Context(), controllerUUID, s.k8sRestConfig, s.cfg, s.getNamespace(), newK8sClientFunc, newK8sRestFunc,
		watcherFn, stringsWatcherFn, randomPrefixFunc, s.clock)
	if expectErr == "" {
		c.Assert(err, tc.ErrorIsNil)
	} else {
		c.Assert(err, tc.ErrorMatches, expectErr)
	}
	return ctrl
}

func (s *BaseSuite) setupK8sRestClient(
	c *tc.C, ctrl *gomock.Controller, namespace string,
) (k8sprovider.NewK8sClientFunc, k8sprovider.NewK8sRestClientFunc) {
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

	s.mockStatefulSets = mocks.NewMockStatefulSetInterface(ctrl)
	s.mockDeployments = mocks.NewMockDeploymentInterface(ctrl)
	s.mockDaemonSets = mocks.NewMockDaemonSetInterface(ctrl)
	s.mockApps.EXPECT().StatefulSets(namespace).AnyTimes().Return(s.mockStatefulSets)
	s.mockApps.EXPECT().Deployments(namespace).AnyTimes().Return(s.mockDeployments)
	s.mockApps.EXPECT().DaemonSets(namespace).AnyTimes().Return(s.mockDaemonSets)
	s.k8sClient.EXPECT().AppsV1().AnyTimes().Return(s.mockApps)

	s.mockIngressV1Beta1 = mocks.NewMockIngressV1Beta1Interface(ctrl)
	s.mockNetworkingV1beta1 = mocks.NewMockNetworkingV1beta1Interface(ctrl)
	s.mockNetworkingV1beta1.EXPECT().Ingresses(namespace).AnyTimes().Return(s.mockIngressV1Beta1)
	s.k8sClient.EXPECT().NetworkingV1beta1().AnyTimes().Return(s.mockNetworkingV1beta1)
	s.mockIngressClasses = mocks.NewMockIngressClassInterface(ctrl)

	s.mockIngressV1 = mocks.NewMockIngressV1Interface(ctrl)
	s.mockNetworkingV1 = mocks.NewMockNetworkingV1Interface(ctrl)
	s.mockNetworkingV1.EXPECT().Ingresses(namespace).AnyTimes().Return(s.mockIngressV1)
	s.mockNetworkingV1.EXPECT().IngressClasses().AnyTimes().Return(s.mockIngressClasses)
	s.k8sClient.EXPECT().NetworkingV1().AnyTimes().Return(s.mockNetworkingV1)

	s.mockStorage = mocks.NewMockStorageV1Interface(ctrl)
	s.mockStorageClass = mocks.NewMockStorageClassInterface(ctrl)
	s.k8sClient.EXPECT().StorageV1().AnyTimes().Return(s.mockStorage)
	s.mockStorage.EXPECT().StorageClasses().AnyTimes().Return(s.mockStorageClass)

	s.mockApiextensionsClient = mocks.NewMockApiExtensionsClientInterface(ctrl)
	s.mockApiextensionsV1Beta1 = mocks.NewMockApiextensionsV1beta1Interface(ctrl)
	s.mockApiextensionsV1 = mocks.NewMockApiextensionsV1Interface(ctrl)
	s.mockCustomResourceDefinitionV1Beta1 = mocks.NewMockCustomResourceDefinitionV1Beta1Interface(ctrl)
	s.mockCustomResourceDefinitionV1 = mocks.NewMockCustomResourceDefinitionV1Interface(ctrl)
	s.mockApiextensionsClient.EXPECT().ApiextensionsV1beta1().AnyTimes().Return(s.mockApiextensionsV1Beta1)
	s.mockApiextensionsClient.EXPECT().ApiextensionsV1().AnyTimes().Return(s.mockApiextensionsV1)
	s.mockApiextensionsV1Beta1.EXPECT().CustomResourceDefinitions().AnyTimes().Return(s.mockCustomResourceDefinitionV1Beta1)
	s.mockApiextensionsV1.EXPECT().CustomResourceDefinitions().AnyTimes().Return(s.mockCustomResourceDefinitionV1)

	s.mockDynamicClient = mocks.NewMockDynamicInterface(ctrl)
	s.mockResourceClient = mocks.NewMockResourceInterface(ctrl)
	s.mockNamespaceableResourceClient = mocks.NewMockNamespaceableResourceInterface(ctrl)
	s.mockNamespaceableResourceClient.EXPECT().Namespace(namespace).AnyTimes().Return(s.mockResourceClient)

	s.mockServiceAccounts = mocks.NewMockServiceAccountInterface(ctrl)
	mockCoreV1.EXPECT().ServiceAccounts(namespace).AnyTimes().Return(s.mockServiceAccounts)

	s.mockMutatingWebhookConfigurationV1Beta1 = mocks.NewMockMutatingWebhookConfigurationV1Beta1Interface(ctrl)
	s.mockMutatingWebhookConfigurationV1 = mocks.NewMockMutatingWebhookConfigurationV1Interface(ctrl)
	s.mockValidatingWebhookConfigurationV1Beta1 = mocks.NewMockValidatingWebhookConfigurationV1Beta1Interface(ctrl)
	s.mockValidatingWebhookConfigurationV1 = mocks.NewMockValidatingWebhookConfigurationV1Interface(ctrl)

	mockAdmissionregistrationV1Beta1 := mocks.NewMockAdmissionregistrationV1beta1Interface(ctrl)
	mockAdmissionregistrationV1Beta1.EXPECT().MutatingWebhookConfigurations().AnyTimes().Return(s.mockMutatingWebhookConfigurationV1Beta1)
	mockAdmissionregistrationV1Beta1.EXPECT().ValidatingWebhookConfigurations().AnyTimes().Return(s.mockValidatingWebhookConfigurationV1Beta1)
	mockAdmissionregistrationV1 := mocks.NewMockAdmissionregistrationV1Interface(ctrl)
	mockAdmissionregistrationV1.EXPECT().MutatingWebhookConfigurations().AnyTimes().Return(s.mockMutatingWebhookConfigurationV1)
	mockAdmissionregistrationV1.EXPECT().ValidatingWebhookConfigurations().AnyTimes().Return(s.mockValidatingWebhookConfigurationV1)
	s.k8sClient.EXPECT().AdmissionregistrationV1beta1().AnyTimes().Return(mockAdmissionregistrationV1Beta1)
	s.k8sClient.EXPECT().AdmissionregistrationV1().AnyTimes().Return(mockAdmissionregistrationV1)

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
			c.Assert(cfg.Username, tc.Equals, "fred")
			c.Assert(cfg.Password, tc.Equals, "secret")
			c.Assert(cfg.Host, tc.Equals, "some-host")
			c.Assert(cfg.TLSClientConfig, tc.DeepEquals, rest.TLSClientConfig{
				CertData: []byte("cert-data"),
				KeyData:  []byte("cert-key"),
				CAData:   []byte(coretesting.CACert),
			})
			return s.k8sClient, s.mockApiextensionsClient, s.mockDynamicClient, nil
		}, func(cfg *rest.Config) (rest.Interface, error) {
			return s.mockRestClient, nil
		}
}

func (s *BaseSuite) k8sNotFoundError() *k8serrors.StatusError {
	return k8serrors.NewNotFound(schema.GroupResource{}, "test")
}

func (s *BaseSuite) deleteOptions(policy v1.DeletionPropagation, uid types.UID) v1.DeleteOptions {
	ops := v1.DeleteOptions{
		PropagationPolicy: &policy,
	}
	if uid != "" {
		ops.Preconditions = &v1.Preconditions{UID: &uid}
	}
	return ops
}

func (s *BaseSuite) ensureJujuNamespaceAnnotations(isController bool, ns *core.Namespace) *core.Namespace {
	return ensureJujuNamespaceAnnotations(s.cfg.UUID(), isController, ns)
}

func ensureJujuNamespaceAnnotations(modelUUID string, isController bool, ns *core.Namespace) *core.Namespace {
	annotations := map[string]string{
		"controller.juju.is/id": coretesting.ControllerTag.Id(),
		"model.juju.is/id":      modelUUID,
	}
	if isController {
		annotations["controller.juju.is/is-controller"] = "true"
	}
	ns.SetAnnotations(annotations)
	return ns
}

type fakeClientSuite struct {
	testhelpers.IsolationSuite

	clock               *testclock.Clock
	broker              *k8sprovider.KubernetesClient
	cfg                 *config.Config
	k8sRestConfig       *rest.Config
	k8sWatcherFn        k8swatcher.NewK8sWatcherFunc
	k8sStringsWatcherFn k8swatcher.NewK8sStringsWatcherFunc

	namespace string

	clientset *fake.Clientset

	k8sClient                  kubernetes.Interface
	mockCoreV1                 corev1.CoreV1Interface
	mockRestClient             rest.Interface
	mockNamespaces             corev1.NamespaceInterface
	mockApps                   appsv1.AppsV1Interface
	mockNetworkingV1beta1      networkingv1beta1.NetworkingV1beta1Interface
	mockNetworkingV1           networkingv1.NetworkingV1Interface
	mockSecrets                corev1.SecretInterface
	mockDeployments            appsv1.DeploymentInterface
	mockStatefulSets           appsv1.StatefulSetInterface
	mockDaemonSets             appsv1.DaemonSetInterface
	mockPods                   corev1.PodInterface
	mockServices               corev1.ServiceInterface
	mockConfigMaps             corev1.ConfigMapInterface
	mockPersistentVolumes      corev1.PersistentVolumeInterface
	mockPersistentVolumeClaims corev1.PersistentVolumeClaimInterface
	mockStorage                storagev1.StorageV1Interface
	mockStorageClass           storagev1.StorageClassInterface
	mockIngressClasses         networkingv1.IngressClassInterface
	mockIngressV1Beta1         networkingv1beta1.IngressInterface
	mockIngressV1              networkingv1.IngressInterface
	mockNodes                  corev1.NodeInterface
	mockEvents                 corev1.EventInterface

	mockApiextensionsClient             apiextensionsclientset.Interface
	mockApiextensionsV1Beta1            apiextensionsv1beta1.ApiextensionsV1beta1Interface
	mockApiextensionsV1                 apiextensionsv1.ApiextensionsV1Interface
	mockCustomResourceDefinitionV1Beta1 apiextensionsv1beta1.CustomResourceDefinitionInterface
	mockCustomResourceDefinitionV1      apiextensionsv1.CustomResourceDefinitionInterface

	mockMutatingWebhookConfigurationV1        admissionregistrationv1.MutatingWebhookConfigurationInterface
	mockValidatingWebhookConfigurationV1      admissionregistrationv1.ValidatingWebhookConfigurationInterface
	mockMutatingWebhookConfigurationV1Beta1   admissionregistrationv1beta1.MutatingWebhookConfigurationInterface
	mockValidatingWebhookConfigurationV1Beta1 admissionregistrationv1beta1.ValidatingWebhookConfigurationInterface

	mockDynamicClient dynamic.Interface

	mockServiceAccounts     corev1.ServiceAccountInterface
	mockRoles               rbacv1.RoleInterface
	mockClusterRoles        rbacv1.ClusterRoleInterface
	mockRoleBindings        rbacv1.RoleBindingInterface
	mockClusterRoleBindings rbacv1.ClusterRoleBindingInterface

	mockDiscovery discovery.DiscoveryInterface
}

func (s *fakeClientSuite) ensureJujuNamespaceAnnotations(isController bool, ns *core.Namespace) *core.Namespace {
	return ensureJujuNamespaceAnnotations(s.cfg.UUID(), isController, ns)
}

func (s *fakeClientSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username":              "fred",
		"password":              "secret",
		"ClientCertificateData": "cert-data",
		"ClientKeyData":         "cert-key",
	})
	cloudSpec := environscloudspec.CloudSpec{
		Endpoint:       "some-host",
		Credential:     &cred,
		CACertificates: []string{coretesting.CACert},
	}
	var err error
	s.k8sRestConfig, err = k8s.CloudSpecToK8sRestConfig(cloudSpec)
	c.Assert(err, tc.ErrorIsNil)

	s.cfg, err = config.New(config.UseDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		config.NameKey:                  "test",
		k8sconstants.WorkloadStorageKey: "",
	}))
	c.Assert(err, tc.ErrorIsNil)

	s.namespace = s.cfg.Name()
	s.clock = testclock.NewClock(time.Time{})

	newK8sClientFunc, newK8sRestFunc := s.setupK8sRestClient(c, s.getNamespace())
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}
	s.setupBroker(c, newK8sClientFunc, newK8sRestFunc, randomPrefixFunc, nil)
}

func (s *fakeClientSuite) getNamespace() string {
	if s.broker != nil {
		return s.broker.Namespace()
	}
	return s.namespace
}

func (s *fakeClientSuite) setupBroker(
	c *tc.C,
	newK8sClientFunc k8sprovider.NewK8sClientFunc,
	newK8sRestFunc k8sprovider.NewK8sRestClientFunc,
	randomPrefixFunc utils.RandomPrefixFunc,
	watchers *[]k8swatcher.KubernetesNotifyWatcher,
) {
	watcherFn := k8swatcher.NewK8sWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		if s.k8sWatcherFn == nil {
			return nil, errors.NewNotFound(nil, "undefined k8sWatcherFn for base test")
		}

		w, err := s.k8sWatcherFn(i, n, c)
		if err == nil && watchers != nil {
			*watchers = append(*watchers, w)
		}
		return w, err
	})

	stringsWatcherFn := k8swatcher.NewK8sStringsWatcherFunc(func(i cache.SharedIndexInformer, n string, c jujuclock.Clock, e []string,
		f k8swatcher.K8sStringsWatcherFilterFunc) (k8swatcher.KubernetesStringsWatcher, error) {
		if s.k8sStringsWatcherFn == nil {
			return nil, errors.NewNotFound(nil, "undefined k8sStringsWatcherFn for base test")
		}
		return s.k8sStringsWatcherFn(i, n, c, e, f)
	})

	var err error
	s.broker, err = k8sprovider.NewK8sBroker(c.Context(), coretesting.ControllerTag.Id(), s.k8sRestConfig, s.cfg, s.getNamespace(), newK8sClientFunc, newK8sRestFunc,
		watcherFn, stringsWatcherFn, randomPrefixFunc, s.clock)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *fakeClientSuite) setupK8sRestClient(c *tc.C, namespace string) (k8sprovider.NewK8sClientFunc, k8sprovider.NewK8sRestClientFunc) {
	s.clientset = fake.NewSimpleClientset()
	s.k8sClient = s.clientset
	s.mockCoreV1 = s.k8sClient.CoreV1()
	s.mockRestClient = s.mockCoreV1.RESTClient()
	s.mockNamespaces = s.mockCoreV1.Namespaces()
	s.mockApps = s.clientset.AppsV1()
	s.mockNetworkingV1beta1 = s.k8sClient.NetworkingV1beta1()
	s.mockNetworkingV1 = s.k8sClient.NetworkingV1()
	s.mockSecrets = s.mockCoreV1.Secrets(namespace)
	s.mockDeployments = s.mockApps.Deployments(namespace)
	s.mockStatefulSets = s.mockApps.StatefulSets(namespace)
	s.mockDaemonSets = s.mockApps.DaemonSets(namespace)
	s.mockPods = s.mockCoreV1.Pods(namespace)
	s.mockServices = s.mockCoreV1.Services(namespace)
	s.mockConfigMaps = s.mockCoreV1.ConfigMaps(namespace)
	s.mockPersistentVolumes = s.mockCoreV1.PersistentVolumes()
	s.mockPersistentVolumeClaims = s.mockCoreV1.PersistentVolumeClaims(namespace)
	s.mockStorage = s.k8sClient.StorageV1()
	s.mockStorageClass = s.mockStorage.StorageClasses()
	s.mockIngressClasses = s.mockNetworkingV1.IngressClasses()
	s.mockIngressV1Beta1 = s.mockNetworkingV1beta1.Ingresses(namespace)
	s.mockIngressV1 = s.mockNetworkingV1.Ingresses(namespace)
	s.mockNodes = s.mockCoreV1.Nodes()
	s.mockEvents = s.mockCoreV1.Events(namespace)

	s.mockApiextensionsClient = apiextensionsclientsetfake.NewSimpleClientset()
	s.mockApiextensionsV1Beta1 = s.mockApiextensionsClient.ApiextensionsV1beta1()
	s.mockApiextensionsV1 = s.mockApiextensionsClient.ApiextensionsV1()
	s.mockCustomResourceDefinitionV1Beta1 = s.mockApiextensionsV1Beta1.CustomResourceDefinitions()
	s.mockCustomResourceDefinitionV1 = s.mockApiextensionsV1.CustomResourceDefinitions()

	s.mockMutatingWebhookConfigurationV1 = s.k8sClient.AdmissionregistrationV1().MutatingWebhookConfigurations()
	s.mockValidatingWebhookConfigurationV1 = s.k8sClient.AdmissionregistrationV1().ValidatingWebhookConfigurations()
	s.mockMutatingWebhookConfigurationV1Beta1 = s.k8sClient.AdmissionregistrationV1beta1().MutatingWebhookConfigurations()
	s.mockValidatingWebhookConfigurationV1Beta1 = s.k8sClient.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations()

	s.mockDynamicClient = k8sdynamicfake.NewSimpleDynamicClient(k8sruntime.NewScheme())

	s.mockServiceAccounts = s.mockCoreV1.ServiceAccounts(namespace)
	s.mockRoles = s.k8sClient.RbacV1().Roles(namespace)
	s.mockClusterRoles = s.k8sClient.RbacV1().ClusterRoles()
	s.mockRoleBindings = s.k8sClient.RbacV1().RoleBindings(namespace)
	s.mockClusterRoleBindings = s.k8sClient.RbacV1().ClusterRoleBindings()

	s.mockDiscovery = s.clientset.Discovery()

	c.Cleanup(func() {
		s.clientset = nil
		s.k8sClient = nil
		s.mockCoreV1 = nil
		s.mockRestClient = nil
		s.mockNamespaces = nil
		s.mockApps = nil
		s.mockNetworkingV1beta1 = nil
		s.mockNetworkingV1 = nil
		s.mockSecrets = nil
		s.mockDeployments = nil
		s.mockStatefulSets = nil
		s.mockDaemonSets = nil
		s.mockPods = nil
		s.mockServices = nil
		s.mockConfigMaps = nil
		s.mockPersistentVolumes = nil
		s.mockPersistentVolumeClaims = nil
		s.mockStorage = nil
		s.mockStorageClass = nil
		s.mockIngressClasses = nil
		s.mockIngressV1Beta1 = nil
		s.mockIngressV1 = nil
		s.mockNodes = nil
		s.mockEvents = nil

		s.mockApiextensionsClient = nil
		s.mockApiextensionsV1Beta1 = nil
		s.mockApiextensionsV1 = nil
		s.mockCustomResourceDefinitionV1Beta1 = nil
		s.mockCustomResourceDefinitionV1 = nil

		s.mockMutatingWebhookConfigurationV1 = nil
		s.mockValidatingWebhookConfigurationV1 = nil
		s.mockMutatingWebhookConfigurationV1Beta1 = nil
		s.mockValidatingWebhookConfigurationV1Beta1 = nil

		s.mockDynamicClient = nil

		s.mockServiceAccounts = nil
		s.mockRoles = nil
		s.mockClusterRoles = nil
		s.mockRoleBindings = nil
		s.mockClusterRoleBindings = nil

		s.mockDiscovery = nil
	})

	return func(cfg *rest.Config) (kubernetes.Interface, apiextensionsclientset.Interface, dynamic.Interface, error) {
			c.Assert(cfg.Username, tc.Equals, "fred")
			c.Assert(cfg.Password, tc.Equals, "secret")
			c.Assert(cfg.Host, tc.Equals, "some-host")
			c.Assert(cfg.TLSClientConfig, tc.DeepEquals, rest.TLSClientConfig{
				CertData: []byte("cert-data"),
				KeyData:  []byte("cert-key"),
				CAData:   []byte(coretesting.CACert),
			})
			return s.k8sClient, s.mockApiextensionsClient, s.mockDynamicClient, nil
		}, func(cfg *rest.Config) (rest.Interface, error) {
			return s.mockRestClient, nil
		}
}
