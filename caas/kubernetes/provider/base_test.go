// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"time"

	"github.com/golang/mock/gomock"
	jujuclock "github.com/juju/clock"
	testclock "github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	watch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/mocks"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type BaseSuite struct {
	testing.BaseSuite

	clock         *testclock.Clock
	broker        caas.Broker
	cfg           *config.Config
	k8sRestConfig *rest.Config

	namespace string

	k8sClient                  *mocks.MockInterface
	mockNamespaces             *mocks.MockNamespaceInterface
	mockApps                   *mocks.MockAppsV1Interface
	mockExtensions             *mocks.MockExtensionsV1beta1Interface
	mockSecrets                *mocks.MockSecretInterface
	mockDeployments            *mocks.MockDeploymentInterface
	mockStatefulSets           *mocks.MockStatefulSetInterface
	mockPods                   *mocks.MockPodInterface
	mockServices               *mocks.MockServiceInterface
	mockConfigMaps             *mocks.MockConfigMapInterface
	mockPersistentVolumes      *mocks.MockPersistentVolumeInterface
	mockPersistentVolumeClaims *mocks.MockPersistentVolumeClaimInterface
	mockStorage                *mocks.MockStorageV1Interface
	mockStorageClass           *mocks.MockStorageClassInterface
	mockIngressInterface       *mocks.MockIngressInterface
	mockNodes                  *mocks.MockNodeInterface

	mockApiextensionsV1          *mocks.MockApiextensionsV1beta1Interface
	mockApiextensionsClient      *mocks.MockApiExtensionsClientInterface
	mockCustomResourceDefinition *mocks.MockCustomResourceDefinitionInterface

	watcher *provider.KubernetesWatcher
}

func (s *BaseSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)

	s.namespace = "test"

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

	cfg, err := config.New(config.UseDefaults, testing.FakeConfig().Merge(testing.Attrs{
		config.NameKey: s.namespace,
	}))
	c.Assert(err, jc.ErrorIsNil)
	s.cfg = cfg
}

func (s *BaseSuite) setupBroker(c *gc.C) *gomock.Controller {

	ctrl := gomock.NewController(c)
	s.k8sClient = mocks.NewMockInterface(ctrl)

	// Plug in the various k8s client modules we need.
	// eg namespaces, pods, services, ingress resources, volumes etc.
	// We make the mock and assign it to the corresponding core getter function.
	mockCoreV1 := mocks.NewMockCoreV1Interface(ctrl)
	s.k8sClient.EXPECT().CoreV1().AnyTimes().Return(mockCoreV1)

	s.mockNamespaces = mocks.NewMockNamespaceInterface(ctrl)
	mockCoreV1.EXPECT().Namespaces().AnyTimes().Return(s.mockNamespaces)

	s.mockPods = mocks.NewMockPodInterface(ctrl)
	mockCoreV1.EXPECT().Pods(s.namespace).AnyTimes().Return(s.mockPods)

	s.mockServices = mocks.NewMockServiceInterface(ctrl)
	mockCoreV1.EXPECT().Services(s.namespace).AnyTimes().Return(s.mockServices)

	s.mockConfigMaps = mocks.NewMockConfigMapInterface(ctrl)
	mockCoreV1.EXPECT().ConfigMaps(s.namespace).AnyTimes().Return(s.mockConfigMaps)

	s.mockPersistentVolumes = mocks.NewMockPersistentVolumeInterface(ctrl)
	mockCoreV1.EXPECT().PersistentVolumes().AnyTimes().Return(s.mockPersistentVolumes)

	s.mockPersistentVolumeClaims = mocks.NewMockPersistentVolumeClaimInterface(ctrl)
	mockCoreV1.EXPECT().PersistentVolumeClaims(s.namespace).AnyTimes().Return(s.mockPersistentVolumeClaims)

	s.mockSecrets = mocks.NewMockSecretInterface(ctrl)
	mockCoreV1.EXPECT().Secrets(s.namespace).AnyTimes().Return(s.mockSecrets)

	s.mockNodes = mocks.NewMockNodeInterface(ctrl)
	mockCoreV1.EXPECT().Nodes().AnyTimes().Return(s.mockNodes)

	s.mockApps = mocks.NewMockAppsV1Interface(ctrl)
	s.mockExtensions = mocks.NewMockExtensionsV1beta1Interface(ctrl)
	s.mockStatefulSets = mocks.NewMockStatefulSetInterface(ctrl)
	s.mockDeployments = mocks.NewMockDeploymentInterface(ctrl)
	s.mockIngressInterface = mocks.NewMockIngressInterface(ctrl)
	s.k8sClient.EXPECT().ExtensionsV1beta1().AnyTimes().Return(s.mockExtensions)
	s.k8sClient.EXPECT().AppsV1().AnyTimes().Return(s.mockApps)
	s.mockApps.EXPECT().StatefulSets(s.namespace).AnyTimes().Return(s.mockStatefulSets)
	s.mockApps.EXPECT().Deployments(s.namespace).AnyTimes().Return(s.mockDeployments)
	s.mockExtensions.EXPECT().Ingresses(s.namespace).AnyTimes().Return(s.mockIngressInterface)

	s.mockStorage = mocks.NewMockStorageV1Interface(ctrl)
	s.mockStorageClass = mocks.NewMockStorageClassInterface(ctrl)
	s.k8sClient.EXPECT().StorageV1().AnyTimes().Return(s.mockStorage)
	s.mockStorage.EXPECT().StorageClasses().AnyTimes().Return(s.mockStorageClass)

	s.mockApiextensionsClient = mocks.NewMockApiExtensionsClientInterface(ctrl)
	s.mockApiextensionsV1 = mocks.NewMockApiextensionsV1beta1Interface(ctrl)
	s.mockCustomResourceDefinition = mocks.NewMockCustomResourceDefinitionInterface(ctrl)
	s.mockApiextensionsClient.EXPECT().ApiextensionsV1beta1().AnyTimes().Return(s.mockApiextensionsV1)
	s.mockApiextensionsV1.EXPECT().CustomResourceDefinitions().AnyTimes().Return(s.mockCustomResourceDefinition)

	// Set up the mock k8sClient we pass to our broker under test.
	newClient := func(cfg *rest.Config) (kubernetes.Interface, apiextensionsclientset.Interface, error) {
		c.Assert(cfg.Username, gc.Equals, "fred")
		c.Assert(cfg.Password, gc.Equals, "secret")
		c.Assert(cfg.Host, gc.Equals, "some-host")
		c.Assert(cfg.TLSClientConfig, jc.DeepEquals, rest.TLSClientConfig{
			CertData: []byte("cert-data"),
			KeyData:  []byte("cert-key"),
			CAData:   []byte(testing.CACert),
		})
		return s.k8sClient, s.mockApiextensionsClient, nil
	}

	s.clock = testclock.NewClock(time.Time{})
	newK8sWatcherForTest := func(wi watch.Interface, name string, clock jujuclock.Clock) (*provider.KubernetesWatcher, error) {
		w, err := provider.NewKubernetesWatcher(wi, name, clock)
		c.Assert(err, jc.ErrorIsNil)
		s.watcher = w
		return s.watcher, err
	}
	var err error
	s.broker, err = provider.NewK8sBroker(s.k8sRestConfig, s.cfg, newClient, newK8sWatcherForTest, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl
}

func (s *BaseSuite) k8sNotFoundError() *k8serrors.StatusError {
	return k8serrors.NewNotFound(schema.GroupResource{}, "test")
}

func (s *BaseSuite) k8sAlreadyExistsError() *k8serrors.StatusError {
	return k8serrors.NewAlreadyExists(schema.GroupResource{}, "test")
}

func (s *BaseSuite) deleteOptions(policy v1.DeletionPropagation) *v1.DeleteOptions {
	return &v1.DeleteOptions{PropagationPolicy: &policy}
}

func (s *BaseSuite) k8sNewFakeWatcher() *watch.RaceFreeFakeWatcher {
	return watch.NewRaceFreeFake()
}
