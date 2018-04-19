// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"fmt"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/caas/kubernetes/provider/mocks"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type K8sSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&K8sSuite{})

func (s *K8sSuite) TestMakeUnitSpecNoConfigConfig(c *gc.C) {
	podSpec := caas.PodSpec{
		Containers: []caas.ContainerSpec{{
			Name:  "test",
			Ports: []caas.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image: "juju/image",
			ProviderContainer: &provider.K8sContainerSpec{
				ImagePullPolicy: core.PullAlways,
				ReadinessProbe: &core.Probe{
					InitialDelaySeconds: 10,
					Handler:             core.Handler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
				},
				LivenessProbe: &core.Probe{
					SuccessThreshold: 20,
					Handler:          core.Handler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
				},
			},
		}, {
			Name:  "test2",
			Ports: []caas.ContainerPort{{ContainerPort: 8080, Protocol: "TCP"}},
			Image: "juju/image2",
		}},
	}
	spec, err := provider.MakeUnitSpec(&podSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.PodSpec(spec), jc.DeepEquals, core.PodSpec{
		Containers: []core.Container{
			{
				Name:            "test",
				Image:           "juju/image",
				Ports:           []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
				ImagePullPolicy: core.PullAlways,
				ReadinessProbe: &core.Probe{
					InitialDelaySeconds: 10,
					Handler:             core.Handler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
				},
				LivenessProbe: &core.Probe{
					SuccessThreshold: 20,
					Handler:          core.Handler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
				},
			}, {
				Name:  "test2",
				Image: "juju/image2",
				Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP}},
			},
		},
	})
}

func (s *K8sSuite) TestMakeUnitSpecConfigPairs(c *gc.C) {
	podSpec := caas.PodSpec{
		Containers: []caas.ContainerSpec{{
			Name:  "test",
			Ports: []caas.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image: "juju/image",
			Config: map[string]string{
				"foo": "bar",
			},
		}, {
			Name:  "test2",
			Ports: []caas.ContainerPort{{ContainerPort: 8080, Protocol: "TCP"}},
			Image: "juju/image2",
		}}}
	spec, err := provider.MakeUnitSpec(&podSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.PodSpec(spec), jc.DeepEquals, core.PodSpec{
		Containers: []core.Container{
			{
				Name:  "test",
				Image: "juju/image",
				Ports: []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
				Env: []core.EnvVar{
					{Name: "foo", Value: "bar"},
				},
			}, {
				Name:  "test2",
				Image: "juju/image2",
				Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP}},
			},
		},
	})
}

func (s *K8sSuite) TestOperatorPodConfig(c *gc.C) {
	pod := provider.OperatorPod("gitlab", "/var/lib/juju")
	vers := version.Current
	vers.Build = 0
	c.Assert(pod.Name, gc.Equals, "juju-operator-gitlab")
	c.Assert(pod.Spec.Containers, gc.HasLen, 1)
	c.Assert(pod.Spec.Containers[0].Image, gc.Equals, fmt.Sprintf("jujusolutions/caas-jujud-operator:%s", vers.String()))
	c.Assert(pod.Spec.Containers[0].VolumeMounts, gc.HasLen, 1)
	c.Assert(pod.Spec.Containers[0].VolumeMounts[0].MountPath, gc.Equals, "/var/lib/juju/agents/application-gitlab/agent.conf")
}

type K8sBrokerSuite struct {
	testing.BaseSuite

	broker caas.Broker

	k8sClient                  *mocks.MockInterface
	mockNamespaces             *mocks.MockNamespaceInterface
	mockExtensions             *mocks.MockExtensionsV1beta1Interface
	mockDeploymentInterface    *mocks.MockDeploymentInterface
	mockPods                   *mocks.MockPodInterface
	mockServices               *mocks.MockServiceInterface
	mockConfigMaps             *mocks.MockConfigMapInterface
	mockPersistentVolumes      *mocks.MockPersistentVolumeInterface
	mockPersistentVolumeClaims *mocks.MockPersistentVolumeClaimInterface
	mockStorage                *mocks.MockStorageV1Interface
	mockStorageClass           *mocks.MockStorageClassInterface
	mockIngressInterface       *mocks.MockIngressInterface
}

var _ = gc.Suite(&K8sBrokerSuite{})

const testNamespace = "test"

func (s *K8sBrokerSuite) newBroker(c *gc.C) *gomock.Controller {
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

	ctrl := gomock.NewController(c)

	// Set up the mock k8sclient we pass to our broker under test.
	s.k8sClient = mocks.NewMockInterface(ctrl)
	newClient := func(cfg *rest.Config) (kubernetes.Interface, error) {
		c.Assert(cfg.Username, gc.Equals, "fred")
		c.Assert(cfg.Password, gc.Equals, "secret")
		c.Assert(cfg.Host, gc.Equals, "some-host")
		c.Assert(cfg.TLSClientConfig, jc.DeepEquals, rest.TLSClientConfig{
			CertData: []byte("cert-data"),
			KeyData:  []byte("cert-key"),
			CAData:   []byte(testing.CACert),
		})
		return s.k8sClient, nil
	}

	// Plug in the various k8s client modules we need.
	// eg namespaces, pods, services, ingress resources, volumes etc.
	// We make the mock and assign it to the corresponding core getter function.
	mockCoreV1 := mocks.NewMockCoreV1Interface(ctrl)
	s.k8sClient.EXPECT().CoreV1().AnyTimes().Return(mockCoreV1)

	s.mockNamespaces = mocks.NewMockNamespaceInterface(ctrl)
	mockCoreV1.EXPECT().Namespaces().AnyTimes().Return(s.mockNamespaces)

	s.mockPods = mocks.NewMockPodInterface(ctrl)
	mockCoreV1.EXPECT().Pods(testNamespace).AnyTimes().Return(s.mockPods)

	s.mockServices = mocks.NewMockServiceInterface(ctrl)
	mockCoreV1.EXPECT().Services(testNamespace).AnyTimes().Return(s.mockServices)

	s.mockConfigMaps = mocks.NewMockConfigMapInterface(ctrl)
	mockCoreV1.EXPECT().ConfigMaps(testNamespace).AnyTimes().Return(s.mockConfigMaps)

	s.mockPersistentVolumes = mocks.NewMockPersistentVolumeInterface(ctrl)
	mockCoreV1.EXPECT().PersistentVolumes().AnyTimes().Return(s.mockPersistentVolumes)

	s.mockPersistentVolumeClaims = mocks.NewMockPersistentVolumeClaimInterface(ctrl)
	mockCoreV1.EXPECT().PersistentVolumeClaims(testNamespace).AnyTimes().Return(s.mockPersistentVolumeClaims)

	s.mockExtensions = mocks.NewMockExtensionsV1beta1Interface(ctrl)
	s.mockDeploymentInterface = mocks.NewMockDeploymentInterface(ctrl)
	s.mockIngressInterface = mocks.NewMockIngressInterface(ctrl)
	s.k8sClient.EXPECT().ExtensionsV1beta1().AnyTimes().Return(s.mockExtensions)
	s.mockExtensions.EXPECT().Deployments(testNamespace).AnyTimes().Return(s.mockDeploymentInterface)
	s.mockExtensions.EXPECT().Ingresses(testNamespace).AnyTimes().Return(s.mockIngressInterface)

	s.mockStorage = mocks.NewMockStorageV1Interface(ctrl)
	s.mockStorageClass = mocks.NewMockStorageClassInterface(ctrl)
	s.k8sClient.EXPECT().StorageV1().AnyTimes().Return(s.mockStorage)
	s.mockStorage.EXPECT().StorageClasses().AnyTimes().Return(s.mockStorageClass)

	var err error
	s.broker, err = provider.NewK8sBroker(cloudSpec, testNamespace, newClient)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl
}

func (s *K8sBrokerSuite) k8sNotFoundError() *k8serrors.StatusError {
	return k8serrors.NewNotFound(schema.GroupResource{}, "test")
}

func (s *K8sBrokerSuite) deleteOptions(orphanDependents bool) *v1.DeleteOptions {
	return &v1.DeleteOptions{OrphanDependents: &orphanDependents}
}

func (s *K8sBrokerSuite) TestEnsureNamespace(c *gc.C) {
	ctrl := s.newBroker(c)
	defer ctrl.Finish()

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}}
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Update(ns).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockNamespaces.EXPECT().Create(ns).Times(1),
		// Idempotent check.
		s.mockNamespaces.EXPECT().Update(ns).Times(1),
	)

	err := s.broker.EnsureNamespace()
	c.Assert(err, jc.ErrorIsNil)

	// Check idempotent.
	err = s.broker.EnsureNamespace()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestDeleteService(c *gc.C) {
	ctrl := s.newBroker(c)
	defer ctrl.Finish()

	// Delete operations below return a not found to ensure it's treated as a no-op.
	gomock.InOrder(
		s.mockServices.EXPECT().Delete("juju-test", s.deleteOptions(false)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockDeploymentInterface.EXPECT().Delete("juju-test", s.deleteOptions(false)).Times(1).
			Return(s.k8sNotFoundError()),
	)

	err := s.broker.DeleteService("test")
	c.Assert(err, jc.ErrorIsNil)
}
