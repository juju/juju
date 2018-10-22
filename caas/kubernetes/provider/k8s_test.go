// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
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
	spec, err := provider.MakeUnitSpec("app-name", &podSpec)
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

var basicPodspec = &caas.PodSpec{
	Containers: []caas.ContainerSpec{{
		Name:         "test",
		Ports:        []caas.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
		ImageDetails: caas.ImageDetails{ImagePath: "juju/image", Username: "fred", Password: "secret"},
		Command:      []string{"sh", "-c"},
		Args:         []string{"doIt", "--debug"},
		WorkingDir:   "/path/to/here",
		Config: map[string]interface{}{
			"foo":        "bar",
			"restricted": "'yes'",
			"bar":        true,
			"switch":     "on",
		},
	}, {
		Name:  "test2",
		Ports: []caas.ContainerPort{{ContainerPort: 8080, Protocol: "TCP", Name: "fred"}},
		Image: "juju/image2",
	}},
}

var operatorPodspec = core.PodSpec{
	Containers: []core.Container{{
		Name:            "juju-operator",
		ImagePullPolicy: core.PullIfNotPresent,
		Image:           "/path/to/image",
		Env: []core.EnvVar{
			{Name: "JUJU_APPLICATION", Value: "test"},
		},
		VolumeMounts: []core.VolumeMount{{
			Name:      "juju-operator-test-config-volume",
			MountPath: "path/to/agent/agents/application-test/template-agent.conf",
			SubPath:   "template-agent.conf",
		}, {
			Name:      "test-operator-volume",
			MountPath: "path/to/agent/agents",
		}},
	}},
	Volumes: []core.Volume{{
		Name: "juju-operator-test-config-volume",
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				LocalObjectReference: core.LocalObjectReference{
					Name: "juju-operator-test-config",
				},
				Items: []core.KeyToPath{{
					Key:  "test-agent.conf",
					Path: "template-agent.conf",
				}},
			},
		},
	}},
}

var basicServiceArg = &core.Service{
	ObjectMeta: v1.ObjectMeta{
		Name:   "juju-app-name",
		Labels: map[string]string{"juju-application": "app-name"}},
	Spec: core.ServiceSpec{
		Selector: map[string]string{"juju-application": "app-name"},
		Type:     "nodeIP",
		Ports: []core.ServicePort{
			{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
			{Port: 8080, Protocol: "TCP", Name: "fred"},
		},
		LoadBalancerIP: "10.0.0.1",
		ExternalName:   "ext-name",
	},
}

func (s *K8sBrokerSuite) secretArg(c *gc.C, labels map[string]string) *core.Secret {
	secretData, err := provider.CreateDockerConfigJSON(&basicPodspec.Containers[0].ImageDetails)
	c.Assert(err, jc.ErrorIsNil)
	secret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-app-name-test-secret",
			Namespace: "test",
			Labels:    labels,
		},
		Type: "kubernetes.io/dockerconfigjson",
		Data: map[string][]byte{".dockerconfigjson": secretData},
	}
	if secret.Labels == nil {
		secret.Labels = make(map[string]string)
	}
	secret.Labels["juju-application"] = "app-name"
	return secret
}

func (s *K8sSuite) TestMakeUnitSpecConfigPairs(c *gc.C) {
	spec, err := provider.MakeUnitSpec("app-name", basicPodspec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.PodSpec(spec), jc.DeepEquals, core.PodSpec{
		ImagePullSecrets: []core.LocalObjectReference{{Name: "juju-app-name-test-secret"}},
		Containers: []core.Container{
			{
				Name:       "test",
				Image:      "juju/image",
				Ports:      []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
				Command:    []string{"sh", "-c"},
				Args:       []string{"doIt", "--debug"},
				WorkingDir: "/path/to/here",
				Env: []core.EnvVar{
					{Name: "bar", Value: "true"},
					{Name: "foo", Value: "bar"},
					{Name: "restricted", Value: "yes"},
					{Name: "switch", Value: "true"},
				},
			}, {
				Name:  "test2",
				Image: "juju/image2",
				Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP, Name: "fred"}},
			},
		},
	})
}

func (s *K8sSuite) TestOperatorPodConfig(c *gc.C) {
	tags := map[string]string{
		"juju-operator": "gitlab",
	}
	pod := provider.OperatorPod("gitlab", "/var/lib/juju", "jujusolutions/caas-jujud-operator", "2.99.0", tags)
	c.Assert(pod.Name, gc.Equals, "juju-operator-gitlab")
	c.Assert(pod.Labels, jc.DeepEquals, map[string]string{
		"juju-operator": "gitlab",
		"juju-version":  "2.99.0",
	})
	c.Assert(pod.Spec.Containers, gc.HasLen, 1)
	c.Assert(pod.Spec.Containers[0].Image, gc.Equals, "jujusolutions/caas-jujud-operator")
	c.Assert(pod.Spec.Containers[0].VolumeMounts, gc.HasLen, 1)
	c.Assert(pod.Spec.Containers[0].VolumeMounts[0].MountPath, gc.Equals, "/var/lib/juju/agents/application-gitlab/template-agent.conf")
}

type K8sBrokerSuite struct {
	BaseSuite
}

var _ = gc.Suite(&K8sBrokerSuite{})

func (s *K8sBrokerSuite) TestConfig(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()
	c.Assert(s.broker.Config(), jc.DeepEquals, s.cfg)
}

func (s *K8sBrokerSuite) TestSetConfig(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()
	err := s.broker.SetConfig(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureNamespace(c *gc.C) {
	ctrl := s.setupBroker(c)
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

func (s *K8sBrokerSuite) TestNamespaces(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	ns1 := core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}}
	ns2 := core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test2"}}
	gomock.InOrder(
		s.mockNamespaces.EXPECT().List(v1.ListOptions{IncludeUninitialized: true}).Times(1).
			Return(&core.NamespaceList{Items: []core.Namespace{ns1, ns2}}, nil),
	)

	result, err := s.broker.Namespaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []string{"test", "test2"})
}

func (s *K8sBrokerSuite) TestDestroy(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	// Delete operations below return a not found to ensure it's treated as a no-op.
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Delete("test", s.deleteOptions(v1.DeletePropagationForeground)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground),
			v1.ListOptions{LabelSelector: "juju-model==test"},
		).Times(1).
			Return(s.k8sNotFoundError()),
	)

	err := s.broker.Destroy(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestDeleteOperator(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	// Delete operations below return a not found to ensure it's treated as a no-op.
	gomock.InOrder(
		s.mockConfigMaps.EXPECT().Delete("juju-operator-test-config", s.deleteOptions(v1.DeletePropagationForeground)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Delete("juju-test-configurations-config", s.deleteOptions(v1.DeletePropagationForeground)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Delete("juju-operator-test", s.deleteOptions(v1.DeletePropagationForeground)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator==test"}).
			Return(&core.PodList{Items: []core.Pod{{
				Spec: core.PodSpec{
					Containers: []core.Container{{
						Name:         "jujud",
						VolumeMounts: []core.VolumeMount{{Name: "test-operator-volume"}},
					}},
					Volumes: []core.Volume{{
						Name: "test-operator-volume", VolumeSource: core.VolumeSource{
							PersistentVolumeClaim: &core.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-operator-volume"}},
					}},
				},
			}}}, nil),
		s.mockSecrets.EXPECT().Delete("juju-test-jujud-secret", s.deleteOptions(v1.DeletePropagationForeground)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockPersistentVolumeClaims.EXPECT().Delete("test-operator-volume", s.deleteOptions(v1.DeletePropagationForeground)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockPersistentVolumes.EXPECT().Delete("test-operator-volume", s.deleteOptions(v1.DeletePropagationForeground)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Delete("juju-operator-test", s.deleteOptions(v1.DeletePropagationForeground)).Times(1).
			Return(s.k8sNotFoundError()),
	)

	err := s.broker.DeleteOperator("test")
	c.Assert(err, jc.ErrorIsNil)
}

func operatorStatefulSetArg(numUnits int32, scName string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: "juju-operator-test",
			Labels: map[string]string{
				"juju-operator": "test",
				"juju-version":  "2.99.0",
				"fred":          "mary",
			}},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-operator": "test"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"juju-operator": "test",
						"fred":          "mary",
						"juju-version":  "2.99.0",
					},
				},
				Spec: operatorPodspec,
			},
			VolumeClaimTemplates: []core.PersistentVolumeClaim{{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-operator-volume",
					Labels: map[string]string{
						"juju-operator": "test",
						"foo":           "bar",
					}},
				Spec: core.PersistentVolumeClaimSpec{
					StorageClassName: &scName,
					AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
					Resources: core.ResourceRequirements{
						Requests: core.ResourceList{
							core.ResourceStorage: resource.MustParse("10Mi"),
						},
					},
				},
			}},
			PodManagementPolicy: apps.ParallelPodManagement,
		},
	}
}

func unitStatefulSetArg(numUnits int32, scName string, podSpec core.PodSpec) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   "juju-app-name",
			Labels: map[string]string{"juju-application": "app-name"}},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-application": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"juju-application": "app-name"},
				},
				Spec: podSpec,
			},
			VolumeClaimTemplates: []core.PersistentVolumeClaim{{
				ObjectMeta: v1.ObjectMeta{
					Name: "juju-database-0",
					Labels: map[string]string{
						"juju-application": "app-name",
						"foo":              "bar",
					}},
				Spec: core.PersistentVolumeClaimSpec{
					StorageClassName: &scName,
					AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
					Resources: core.ResourceRequirements{
						Requests: core.ResourceList{
							core.ResourceStorage: resource.MustParse("100Mi"),
						},
					},
				},
			}},
			PodManagementPolicy: apps.ParallelPodManagement,
		},
	}
}

func (s *K8sBrokerSuite) TestEnsureOperator(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	configMapArg := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: "juju-operator-test-config",
		},
		Data: map[string]string{
			"test-agent.conf": "agent-conf-data",
		},
	}
	statefulSetArg := operatorStatefulSetArg(1, "test-juju-operator-storage")

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Update(&core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}}).Times(1),
		s.mockConfigMaps.EXPECT().Update(configMapArg).Times(1),
		s.mockStorageClass.EXPECT().Get("test-juju-operator-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "test-juju-operator-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
	)

	err := s.broker.EnsureOperator("test", "path/to/agent", &caas.OperatorConfig{
		OperatorImagePath: "/path/to/image",
		Version:           version.MustParse("2.99.0"),
		AgentConf:         []byte("agent-conf-data"),
		ResourceTags:      map[string]string{"fred": "mary"},
		CharmStorage: caas.CharmStorageParams{
			Size:         uint64(10),
			Provider:     "kubernetes",
			ResourceTags: map[string]string{"foo": "bar"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureOperatorNoAgentConfig(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	statefulSetArg := operatorStatefulSetArg(1, "test-juju-operator-storage")

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Update(&core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}}).Times(1),
		s.mockConfigMaps.EXPECT().Get("juju-operator-test-config", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-juju-operator-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "test-juju-operator-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
	)

	err := s.broker.EnsureOperator("test", "path/to/agent", &caas.OperatorConfig{
		OperatorImagePath: "/path/to/image",
		Version:           version.MustParse("2.99.0"),
		ResourceTags:      map[string]string{"fred": "mary"},
		CharmStorage: caas.CharmStorageParams{
			Size:         uint64(10),
			Provider:     "kubernetes",
			ResourceTags: map[string]string{"foo": "bar"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureOperatorNoAgentConfigMissingConfigMap(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Update(&core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}}).Times(1),
		s.mockConfigMaps.EXPECT().Get("juju-operator-test-config", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
	)

	err := s.broker.EnsureOperator("test", "path/to/agent", &caas.OperatorConfig{
		OperatorImagePath: "/path/to/image",
		Version:           version.MustParse("2.99.0"),
		CharmStorage: caas.CharmStorageParams{
			Size:     uint64(10),
			Provider: "kubernetes",
		},
	})
	c.Assert(err, gc.ErrorMatches, `config map for "test" should already exist:  "test" not found`)
}

func (s *K8sBrokerSuite) TestDeleteService(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	// Delete operations below return a not found to ensure it's treated as a no-op.
	gomock.InOrder(
		s.mockServices.EXPECT().Delete("juju-test", s.deleteOptions(v1.DeletePropagationForeground)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Delete("juju-test", s.deleteOptions(v1.DeletePropagationForeground)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Delete("juju-test", s.deleteOptions(v1.DeletePropagationForeground)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: "juju-application==test"}).
			Return(&core.PodList{Items: []core.Pod{}}, nil),
		s.mockSecrets.EXPECT().List(v1.ListOptions{LabelSelector: "juju-application==test"}).Times(1).
			Return(&core.SecretList{Items: []core.Secret{{
				ObjectMeta: v1.ObjectMeta{Name: "secret"},
			}}}, nil),
		s.mockSecrets.EXPECT().Delete("secret", s.deleteOptions(v1.DeletePropagationForeground)).Times(1).
			Return(s.k8sNotFoundError()),
	)

	err := s.broker.DeleteService("test")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceNoUnits(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	two := int32(2)
	dc := &apps.Deployment{ObjectMeta: v1.ObjectMeta{Name: "juju-unit-storage"}, Spec: apps.DeploymentSpec{Replicas: &two}}
	zero := int32(0)
	emptyDc := dc
	emptyDc.Spec.Replicas = &zero
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get("juju-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(dc, nil),
		s.mockDeployments.EXPECT().Update(emptyDc).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{}
	err := s.broker.EnsureService("app-name", nil, params, 0, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceNoStorage(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	unitSpec, err := provider.MakeUnitSpec("app-name", basicPodspec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name: "juju-app-name",
			Labels: map[string]string{
				"juju-application": "app-name",
				"fred":             "mary",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-application": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "juju-app-name-",
					Labels: map[string]string{
						"juju-application": "app-name",
						"fred":             "mary",
					},
				},
				Spec: podSpec,
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name: "juju-app-name",
			Labels: map[string]string{
				"juju-application": "app-name",
				"fred":             "mary",
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"juju-application": "app-name"},
			Type:     "nodeIP",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	secretArg := s.secretArg(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockSecrets.EXPECT().Update(secretArg).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("juju-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Update(deploymentArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(deploymentArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("juju-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(serviceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(serviceArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodspec,
		ResourceTags: map[string]string{"fred": "mary"},
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-type":            "nodeIP",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureCustomResourceDefinitionCreate(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	podSpec := basicPodspec
	podSpec.CustomResourceDefinitions = []caas.CustomResourceDefinition{
		{
			Kind:    "TFJob",
			Group:   "kubeflow.org",
			Version: "v1alpha2",
			Scope:   "Namespaced",
			Validation: caas.CustomResourceDefinitionValidation{
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
	}

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      "tfjobs.kubeflow.org",
			Namespace: "test",
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

	gomock.InOrder(
		s.mockCustomResourceDefinition.EXPECT().Create(crd).Times(1).Return(crd, nil),
	)
	err := s.broker.EnsureCustomResourceDefinition("test", podSpec)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureCustomResourceDefinitionUpdate(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	podSpec := basicPodspec
	podSpec.CustomResourceDefinitions = []caas.CustomResourceDefinition{
		{
			Kind:    "TFJob",
			Group:   "kubeflow.org",
			Version: "v1alpha2",
			Scope:   "Namespaced",
			Validation: caas.CustomResourceDefinitionValidation{
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
	}

	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      "tfjobs.kubeflow.org",
			Namespace: "test",
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
	gomock.InOrder(
		s.mockCustomResourceDefinition.EXPECT().Create(crd).Times(1).Return(crd, s.k8sAlreadyExists()),
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Times(1).Return(crd, nil),
		s.mockCustomResourceDefinition.EXPECT().Update(crd).Times(1).Return(crd, nil),
	)
	err := s.broker.EnsureCustomResourceDefinition("test", podSpec)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithStorage(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	unitSpec, err := provider.MakeUnitSpec("app-name", basicPodspec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)
	podSpec.Containers[0].VolumeMounts = []core.VolumeMount{{
		Name:      "juju-database-0",
		MountPath: "path/to/here",
	}}
	statefulSetArg := unitStatefulSetArg(2, "juju-unit-storage", podSpec)

	gomock.InOrder(
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-juju-unit-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get("juju-unit-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "juju-unit-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("juju-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicServiceArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodspec,
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		}},
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-type":            "nodeIP",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForDeploymentWithDevices(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	unitSpec, err := provider.MakeUnitSpec("app-name", basicPodspec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)
	podSpec.NodeSelector = map[string]string{"accelerator": "nvidia-tesla-p100"}
	for i := range podSpec.Containers {
		podSpec.Containers[i].Resources = core.ResourceRequirements{
			Limits: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
			Requests: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
		}
	}

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "juju-app-name",
			Labels: map[string]string{"juju-application": "app-name"}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-application": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "juju-app-name-",
					Labels:       map[string]string{"juju-application": "app-name"},
				},
				Spec: podSpec,
			},
		},
	}

	gomock.InOrder(
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("juju-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Update(deploymentArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(deploymentArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("juju-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicServiceArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodspec,
		Devices: []devices.KubernetesDeviceParams{
			{
				Type:       "nvidia.com/gpu",
				Count:      3,
				Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
			},
		},
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-type":            "nodeIP",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceForStatefulSetWithDevices(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	unitSpec, err := provider.MakeUnitSpec("app-name", basicPodspec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)
	podSpec.Containers[0].VolumeMounts = []core.VolumeMount{{
		Name:      "juju-database-0",
		MountPath: "path/to/here",
	}}
	podSpec.NodeSelector = map[string]string{"accelerator": "nvidia-tesla-p100"}
	for i := range podSpec.Containers {
		podSpec.Containers[i].Resources = core.ResourceRequirements{
			Limits: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
			Requests: core.ResourceList{
				"nvidia.com/gpu": *resource.NewQuantity(3, resource.DecimalSI),
			},
		}
	}
	statefulSetArg := unitStatefulSetArg(2, "juju-unit-storage", podSpec)

	gomock.InOrder(
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-juju-unit-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get("juju-unit-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "juju-unit-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("juju-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicServiceArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodspec,
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		}},
		Devices: []devices.KubernetesDeviceParams{
			{
				Type:       "nvidia.com/gpu",
				Count:      3,
				Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
			},
		},
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-type":            "nodeIP",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithConstraints(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	unitSpec, err := provider.MakeUnitSpec("app-name", basicPodspec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)
	podSpec.Containers[0].VolumeMounts = []core.VolumeMount{{
		Name:      "juju-database-0",
		MountPath: "path/to/here",
	}}
	for i := range podSpec.Containers {
		podSpec.Containers[i].Resources = core.ResourceRequirements{
			Limits: core.ResourceList{
				"memory": resource.MustParse("64Mi"),
				"cpu":    resource.MustParse("500m"),
			},
		}
	}
	statefulSetArg := unitStatefulSetArg(2, "juju-unit-storage", podSpec)

	gomock.InOrder(
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-juju-unit-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get("juju-unit-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "juju-unit-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("juju-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicServiceArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodspec,
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		}},
		Constraints: constraints.MustParse("mem=64 cpu-power=500"),
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-type":            "nodeIP",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithPlacement(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	unitSpec, err := provider.MakeUnitSpec("app-name", basicPodspec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)
	podSpec.Containers[0].VolumeMounts = []core.VolumeMount{{
		Name:      "juju-database-0",
		MountPath: "path/to/here",
	}}
	podSpec.NodeSelector = map[string]string{"a": "b"}
	statefulSetArg := unitStatefulSetArg(2, "juju-unit-storage", podSpec)

	gomock.InOrder(
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-juju-unit-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get("juju-unit-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "juju-unit-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("juju-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicServiceArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodspec,
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		}},
		Placement: "a=b",
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-type":            "nodeIP",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestOperator(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	opPod := core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "juju-operator-test",
		},
		Status: core.PodStatus{
			Phase:   core.PodPending,
			Message: "test message.",
		},
	}
	gomock.InOrder(
		s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator==test"}).Times(1).
			Return(&core.PodList{Items: []core.Pod{opPod}}, nil),
	)

	operator, err := s.broker.Operator("test")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(operator.Status.Status, gc.Equals, status.Allocating)
	c.Assert(operator.Status.Message, gc.Equals, "test message.")
}

func (s *K8sBrokerSuite) TestOperatorNoPodFound(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator==test"}).Times(1).
			Return(&core.PodList{Items: []core.Pod{}}, nil),
	)

	_, err := s.broker.Operator("test")
	c.Assert(err, gc.ErrorMatches, "operator pod for application \"test\" not found")
}
