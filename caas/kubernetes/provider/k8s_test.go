// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"net/url"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"
	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8sstorage "k8s.io/api/storage/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type K8sSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&K8sSuite{})

func float64Ptr(f float64) *float64 {
	return &f
}

func int32Ptr(i int32) *int32 {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func (s *K8sSuite) TestMakeSvcSpecNoConfigConfig(c *gc.C) {
	podSpec := specs.PodSpec{
		ServiceAccount: &specs.ServiceAccountSpec{
			Name:                         "serviceAccount",
			AutomountServiceAccountToken: boolPtr(true),
		},
	}

	podSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		Pod: &k8sspecs.PodSpec{
			RestartPolicy:                 core.RestartPolicyOnFailure,
			ActiveDeadlineSeconds:         int64Ptr(10),
			TerminationGracePeriodSeconds: int64Ptr(20),
			SecurityContext: &core.PodSecurityContext{
				RunAsNonRoot:       boolPtr(true),
				SupplementalGroups: []int64{1, 2},
			},
			Priority: int32Ptr(30),
			ReadinessGates: []core.PodReadinessGate{
				{ConditionType: core.PodInitialized},
			},
			DNSPolicy: core.DNSClusterFirst,
			// Hostname:                      "host",
			// Subdomain:                     "sub",
			// PriorityClassName:             "top",
			// DNSConfig: &core.PodDNSConfig{
			// 	Nameservers: []string{"ns1", "n2"},
			// },
		},
	}
	podSpec.Containers = []specs.ContainerSpec{
		{
			Name:  "test",
			Ports: []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image: "juju/image",
			ProviderContainer: &k8sspecs.K8sContainerSpec{
				ImagePullPolicy: core.PullAlways,
				ReadinessProbe: &core.Probe{
					InitialDelaySeconds: 10,
					Handler:             core.Handler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
				},
				LivenessProbe: &core.Probe{
					SuccessThreshold: 20,
					Handler:          core.Handler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
				},
				SecurityContext: &core.SecurityContext{
					RunAsNonRoot: boolPtr(true),
					Privileged:   boolPtr(true),
				},
			},
		}, {
			Name:  "test2",
			Ports: []specs.ContainerPort{{ContainerPort: 8080, Protocol: "TCP"}},
			Image: "juju/image2",
		},
	}

	spec, err := provider.PrepareWorkloadSpec("app-name", "app-name", &podSpec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.PodSpec(spec), jc.DeepEquals, core.PodSpec{
		RestartPolicy:                 core.RestartPolicyOnFailure,
		ActiveDeadlineSeconds:         int64Ptr(10),
		TerminationGracePeriodSeconds: int64Ptr(20),
		SecurityContext: &core.PodSecurityContext{
			RunAsNonRoot:       boolPtr(true),
			SupplementalGroups: []int64{1, 2},
		},
		Priority: int32Ptr(30),
		ReadinessGates: []core.PodReadinessGate{
			{ConditionType: core.PodInitialized},
		},
		DNSPolicy:                    core.DNSClusterFirst,
		ServiceAccountName:           "serviceAccount",
		AutomountServiceAccountToken: boolPtr(true),
		// Hostname:                      "host",
		// Subdomain:                     "sub",
		// PriorityClassName:             "top",
		// DNSConfig: &core.PodDNSConfig{
		// 	Nameservers: []string{"ns1", "n2"},
		// },
		Containers: []core.Container{
			{
				Name:            "test",
				Image:           "juju/image",
				Ports:           []core.ContainerPort{{ContainerPort: int32(80), Protocol: core.ProtocolTCP}},
				ImagePullPolicy: core.PullAlways,
				SecurityContext: &core.SecurityContext{
					RunAsNonRoot: boolPtr(true),
					Privileged:   boolPtr(true),
				},
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
				// Defaults since not specified.
				SecurityContext: &core.SecurityContext{
					RunAsNonRoot:             boolPtr(false),
					ReadOnlyRootFilesystem:   boolPtr(false),
					AllowPrivilegeEscalation: boolPtr(true),
				},
			},
		},
	})
}

func (s *K8sSuite) TestMakeSvcSpecWithInitContainers(c *gc.C) {
	podSpec := specs.PodSpec{}
	podSpec.Containers = []specs.ContainerSpec{
		{
			Name:  "test",
			Ports: []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image: "juju/image",
			ProviderContainer: &k8sspecs.K8sContainerSpec{
				ImagePullPolicy: core.PullAlways,
				ReadinessProbe: &core.Probe{
					InitialDelaySeconds: 10,
					Handler:             core.Handler{HTTPGet: &core.HTTPGetAction{Path: "/ready"}},
				},
				LivenessProbe: &core.Probe{
					SuccessThreshold: 20,
					Handler:          core.Handler{HTTPGet: &core.HTTPGetAction{Path: "/liveready"}},
				},
				SecurityContext: &core.SecurityContext{
					RunAsNonRoot: boolPtr(true),
					Privileged:   boolPtr(true),
				},
			},
		}, {
			Name:  "test2",
			Ports: []specs.ContainerPort{{ContainerPort: 8080, Protocol: "TCP"}},
			Image: "juju/image2",
		},
		{
			Name:       "test-init",
			Init:       true,
			Ports:      []specs.ContainerPort{{ContainerPort: 90, Protocol: "TCP"}},
			Image:      "juju/image-init",
			WorkingDir: "/path/to/here",
			Command:    []string{"sh", "ls"},
			ProviderContainer: &k8sspecs.K8sContainerSpec{
				ImagePullPolicy: core.PullAlways,
			},
		},
	}

	spec, err := provider.PrepareWorkloadSpec("app-name", "app-name", &podSpec)
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
				SecurityContext: &core.SecurityContext{
					RunAsNonRoot: boolPtr(true),
					Privileged:   boolPtr(true),
				},
			}, {
				Name:  "test2",
				Image: "juju/image2",
				Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP}},
				// Defaults since not specified.
				SecurityContext: &core.SecurityContext{
					RunAsNonRoot:             boolPtr(false),
					ReadOnlyRootFilesystem:   boolPtr(false),
					AllowPrivilegeEscalation: boolPtr(true),
				},
			},
		},
		InitContainers: []core.Container{
			{
				Name:            "test-init",
				Image:           "juju/image-init",
				Ports:           []core.ContainerPort{{ContainerPort: int32(90), Protocol: core.ProtocolTCP}},
				WorkingDir:      "/path/to/here",
				Command:         []string{"sh", "ls"},
				ImagePullPolicy: core.PullAlways,
				// Defaults since not specified.
				SecurityContext: &core.SecurityContext{
					RunAsNonRoot:             boolPtr(false),
					ReadOnlyRootFilesystem:   boolPtr(false),
					AllowPrivilegeEscalation: boolPtr(true),
				},
			},
		},
	})
}

func getBasicPodspec() *specs.PodSpec {
	pSpecs := &specs.PodSpec{}
	pSpecs.Containers = []specs.ContainerSpec{{
		Name:         "test",
		Ports:        []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
		ImageDetails: specs.ImageDetails{ImagePath: "juju/image", Username: "fred", Password: "secret"},
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
		Ports: []specs.ContainerPort{{ContainerPort: 8080, Protocol: "TCP", Name: "fred"}},
		Image: "juju/image2",
	}}
	return pSpecs
}

var operatorPodspec = core.PodSpec{
	Containers: []core.Container{{
		Name:            "juju-operator",
		ImagePullPolicy: core.PullIfNotPresent,
		Image:           "/path/to/image",
		WorkingDir:      "/var/lib/juju",
		Command: []string{
			"/bin/sh",
		},
		Args: []string{
			"-c",
			`
export JUJU_DATA_DIR=/var/lib/juju
export JUJU_TOOLS_DIR=$JUJU_DATA_DIR/tools

mkdir -p $JUJU_TOOLS_DIR
cp /opt/jujud $JUJU_TOOLS_DIR/jujud
$JUJU_TOOLS_DIR/jujud caasoperator --application-name=test --debug
`[1:],
		},
		Env: []core.EnvVar{
			{Name: "JUJU_APPLICATION", Value: "test"},
			{
				Name: "JUJU_OPERATOR_POD_IP",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
		},
		VolumeMounts: []core.VolumeMount{{
			Name:      "test-operator-config",
			MountPath: "path/to/agent/agents/application-test/template-agent.conf",
			SubPath:   "template-agent.conf",
		}, {
			Name:      "charm",
			MountPath: "path/to/agent/agents",
		}},
	}},
	Volumes: []core.Volume{{
		Name: "test-operator-config",
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				LocalObjectReference: core.LocalObjectReference{
					Name: "test-operator-config",
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
		Name:        "app-name",
		Labels:      map[string]string{"juju-app": "app-name"},
		Annotations: map[string]string{}},
	Spec: core.ServiceSpec{
		Selector: map[string]string{"juju-app": "app-name"},
		Type:     "nodeIP",
		Ports: []core.ServicePort{
			{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
			{Port: 8080, Protocol: "TCP", Name: "fred"},
		},
		LoadBalancerIP: "10.0.0.1",
		ExternalName:   "ext-name",
	},
}

var basicHeadlessServiceArg = &core.Service{
	ObjectMeta: v1.ObjectMeta{
		Name:        "app-name-endpoints",
		Labels:      map[string]string{"juju-app": "app-name"},
		Annotations: map[string]string{"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true"},
	},
	Spec: core.ServiceSpec{
		Selector:                 map[string]string{"juju-app": "app-name"},
		Type:                     "ClusterIP",
		ClusterIP:                "None",
		PublishNotReadyAddresses: true,
	},
}

func (s *K8sBrokerSuite) secretArg(c *gc.C, annotations map[string]string) *core.Secret {
	secretData, err := provider.CreateDockerConfigJSON(&getBasicPodspec().Containers[0].ImageDetails)
	c.Assert(err, jc.ErrorIsNil)
	if annotations == nil {
		annotations = map[string]string{}
	}

	secret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        "app-name-test-secret",
			Namespace:   "test",
			Labels:      map[string]string{"juju-app": "app-name"},
			Annotations: annotations,
		},
		Type: "kubernetes.io/dockerconfigjson",
		Data: map[string][]byte{".dockerconfigjson": secretData},
	}
	return secret
}

func (s *K8sSuite) TestMakeSvcSpecConfigPairs(c *gc.C) {
	spec, err := provider.PrepareWorkloadSpec("app-name", "app-name", getBasicPodspec())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(provider.PodSpec(spec), jc.DeepEquals, core.PodSpec{
		ImagePullSecrets: []core.LocalObjectReference{{Name: "app-name-test-secret"}},
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
				// Defaults since not specified.
				SecurityContext: &core.SecurityContext{
					RunAsNonRoot:             boolPtr(false),
					ReadOnlyRootFilesystem:   boolPtr(false),
					AllowPrivilegeEscalation: boolPtr(true),
				},
			}, {
				Name:  "test2",
				Image: "juju/image2",
				Ports: []core.ContainerPort{{ContainerPort: int32(8080), Protocol: core.ProtocolTCP, Name: "fred"}},
				// Defaults since not specified.
				SecurityContext: &core.SecurityContext{
					RunAsNonRoot:             boolPtr(false),
					ReadOnlyRootFilesystem:   boolPtr(false),
					AllowPrivilegeEscalation: boolPtr(true),
				},
			},
		},
	})
}

func (s *K8sSuite) TestOperatorPodConfig(c *gc.C) {
	tags := map[string]string{
		"fred": "mary",
	}
	pod, err := provider.OperatorPod("gitlab", "gitlab", "/var/lib/juju", "jujusolutions/jujud-operator", "2.99.0", tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pod.Name, gc.Equals, "gitlab")
	c.Assert(pod.Labels, jc.DeepEquals, map[string]string{
		"juju-operator": "gitlab",
	})
	c.Assert(pod.Annotations, jc.DeepEquals, map[string]string{
		"fred":         "mary",
		"juju-version": "2.99.0",
		"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
		"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
	})
	c.Assert(pod.Spec.Containers, gc.HasLen, 1)
	c.Assert(pod.Spec.Containers[0].Image, gc.Equals, "jujusolutions/jujud-operator")
	c.Assert(pod.Spec.Containers[0].VolumeMounts, gc.HasLen, 1)
	c.Assert(pod.Spec.Containers[0].VolumeMounts[0].MountPath, gc.Equals, "/var/lib/juju/agents/application-gitlab/template-agent.conf")
}

type K8sBrokerSuite struct {
	BaseSuite
}

var _ = gc.Suite(&K8sBrokerSuite{})

func (s *K8sBrokerSuite) TestAPIVersion(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	r := rest.NewRequest(nil, "get", &url.URL{Path: "/path/"}, "", rest.ContentConfig{}, rest.Serializers{}, nil, nil, 0)
	s.mockRestClient.EXPECT().Get().Times(1).Return(r)

	// The fake request results in an error that shows the expected path was accessed.
	_, err := s.broker.APIVersion()
	c.Assert(err, gc.ErrorMatches, `get /path/version: unsupported protocol scheme ""`)
}

func (s *K8sBrokerSuite) TestConfig(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	c.Assert(s.broker.Config(), jc.DeepEquals, s.cfg)
}

func (s *K8sBrokerSuite) TestSetConfig(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	err := s.broker.SetConfig(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestBootstrapNoOperatorStorage(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ctx := envtesting.BootstrapContext(c)
	callCtx := &context.CloudCallContext{}
	bootstrapParams := environs.BootstrapParams{
		ControllerConfig:     testing.FakeControllerConfig(),
		BootstrapConstraints: constraints.MustParse("mem=3.5G"),
	}

	_, err := s.broker.Bootstrap(ctx, callCtx, bootstrapParams)
	c.Assert(err, gc.NotNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, "config without operator-storage value not valid.*")
}

func (s *K8sBrokerSuite) TestBootstrap(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// Ensure the broker is configured with operator storage.
	s.setupOperatorStorageConfig(c)

	ctx := envtesting.BootstrapContext(c)
	callCtx := &context.CloudCallContext{}
	bootstrapParams := environs.BootstrapParams{
		ControllerConfig:     testing.FakeControllerConfig(),
		BootstrapConstraints: constraints.MustParse("mem=3.5G"),
	}

	sc := &k8sstorage.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "some-storage",
		},
	}
	gomock.InOrder(
		// Check the operator storage exists.
		s.mockStorageClass.EXPECT().Get("test-some-storage", v1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get("some-storage", v1.GetOptions{}).Times(1).
			Return(sc, nil),
	)
	result, err := s.broker.Bootstrap(ctx, callCtx, bootstrapParams)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "amd64")
	c.Assert(result.CaasBootstrapFinalizer, gc.NotNil)

	bootstrapParams.BootstrapSeries = "bionic"
	result, err = s.broker.Bootstrap(ctx, callCtx, bootstrapParams)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *K8sBrokerSuite) setupOperatorStorageConfig(c *gc.C) {
	cfg := s.broker.Config()
	var err error
	cfg, err = cfg.Apply(map[string]interface{}{"operator-storage": "some-storage"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.broker.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestPrepareForBootstrap(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// Ensure the broker is configured with operator storage.
	s.setupOperatorStorageConfig(c)

	sc := &k8sstorage.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: "some-storage",
		},
	}

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get("controller-ctrl-1", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockNamespaces.EXPECT().List(v1.ListOptions{IncludeUninitialized: true}).Times(1).
			Return(&core.NamespaceList{Items: []core.Namespace{}}, nil),
		s.mockStorageClass.EXPECT().Get("controller-ctrl-1-some-storage", v1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get("some-storage", v1.GetOptions{}).Times(1).
			Return(sc, nil),
	)
	ctx := envtesting.BootstrapContext(c)
	c.Assert(
		s.broker.PrepareForBootstrap(ctx, "ctrl-1"), jc.ErrorIsNil,
	)
	c.Assert(s.broker.GetCurrentNamespace(), jc.DeepEquals, "controller-ctrl-1")
}

func (s *K8sBrokerSuite) TestPrepareForBootstrapAlreadyExistNamespaceError(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "controller-ctrl-1"}}
	s.ensureJujuNamespaceAnnotations(true, ns)
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get("controller-ctrl-1", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(ns, nil),
	)
	ctx := envtesting.BootstrapContext(c)
	c.Assert(
		s.broker.PrepareForBootstrap(ctx, "ctrl-1"), jc.Satisfies, errors.IsAlreadyExists,
	)
}

func (s *K8sBrokerSuite) TestPrepareForBootstrapAlreadyExistControllerAnnotations(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "controller-ctrl-1"}}
	s.ensureJujuNamespaceAnnotations(true, ns)
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get("controller-ctrl-1", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockNamespaces.EXPECT().List(v1.ListOptions{IncludeUninitialized: true}).Times(1).
			Return(&core.NamespaceList{Items: []core.Namespace{*ns}}, nil),
	)
	ctx := envtesting.BootstrapContext(c)
	c.Assert(
		s.broker.PrepareForBootstrap(ctx, "ctrl-1"), jc.Satisfies, errors.IsAlreadyExists,
	)
}

func (s *K8sBrokerSuite) TestGetNamespace(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}}
	s.ensureJujuNamespaceAnnotations(false, ns)
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get("test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(ns, nil),
	)

	out, err := s.broker.GetNamespace("test")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, ns)
}

func (s *K8sBrokerSuite) TestGetNamespaceNotFound(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Get("unknown-namespace", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
	)

	out, err := s.broker.GetNamespace("unknown-namespace")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(out, gc.IsNil)
}

func (s *K8sBrokerSuite) TestNamespaces(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns1 := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}})
	ns2 := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test2"}})
	gomock.InOrder(
		s.mockNamespaces.EXPECT().List(v1.ListOptions{IncludeUninitialized: true}).Times(1).
			Return(&core.NamespaceList{Items: []core.Namespace{*ns1, *ns2}}, nil),
	)

	result, err := s.broker.Namespaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.SameContents, []string{"test", "test2"})
}

func (s *K8sBrokerSuite) assertDestroy(c *gc.C, isController bool, destroyFunc func() error) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := &core.Namespace{}
	ns.Name = "test"
	s.ensureJujuNamespaceAnnotations(isController, ns)
	namespaceWatcher := s.k8sNewFakeWatcher()

	gomock.InOrder(
		s.mockNamespaces.EXPECT().Watch(
			v1.ListOptions{
				FieldSelector:        fields.OneTermEqualSelector("metadata.name", "test").String(),
				IncludeUninitialized: true,
			},
		).
			Return(namespaceWatcher, nil),
		s.mockNamespaces.EXPECT().Get("test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(ns, nil),
		s.mockNamespaces.EXPECT().Delete("test", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(nil),
		s.mockStorageClass.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, nil),
			v1.ListOptions{LabelSelector: "juju-model==test"},
		).Times(1).
			Return(s.k8sNotFoundError()),
		// still terminating.
		s.mockNamespaces.EXPECT().Get("test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(ns, nil),
		// terminated, not found returned.
		s.mockNamespaces.EXPECT().Get("test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
	)

	go func(w *watch.RaceFreeFakeWatcher, clk *testclock.Clock) {
		for _, f := range []func(runtime.Object){w.Add, w.Modify, w.Delete} {
			if !w.IsStopped() {
				clk.WaitAdvance(time.Second, testing.ShortWait, 1)
				f(ns)
			}
		}
	}(namespaceWatcher, s.clock)

	c.Assert(destroyFunc(), jc.ErrorIsNil)
	for _, watcher := range s.watchers {
		c.Assert(workertest.CheckKilled(c, watcher), jc.ErrorIsNil)
	}
	c.Assert(namespaceWatcher.IsStopped(), jc.IsTrue)
}

func (s *K8sBrokerSuite) TestDestroyController(c *gc.C) {
	s.assertDestroy(c, true, func() error { return s.broker.DestroyController(context.NewCloudCallContext(), s.controllerUUID) })
}

func (s *K8sBrokerSuite) TestDestroy(c *gc.C) {
	s.assertDestroy(c, false, func() error { return s.broker.Destroy(context.NewCloudCallContext()) })
}

func (s *K8sBrokerSuite) TestGetCurrentNamespace(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
	c.Assert(s.broker.GetCurrentNamespace(), jc.DeepEquals, s.getNamespace())
}

func (s *K8sBrokerSuite) TestCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}})
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Create(ns).Times(1).
			Return(ns, nil),
	)

	err := s.broker.Create(
		&context.CloudCallContext{},
		environs.CreateParams{},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestCreateAlreadyExists(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ns := s.ensureJujuNamespaceAnnotations(false, &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test"}})
	gomock.InOrder(
		s.mockNamespaces.EXPECT().Create(ns).Times(1).
			Return(nil, s.k8sAlreadyExistsError()),
	)

	err := s.broker.Create(
		&context.CloudCallContext{},
		environs.CreateParams{},
	)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *K8sBrokerSuite) TestDeleteOperator(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// Delete operations below return a not found to ensure it's treated as a no-op.
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Delete("test-operator-config", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Delete("test-configurations-config", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Delete("test-operator", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
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
		s.mockSecrets.EXPECT().Delete("test-jujud-secret", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockPersistentVolumeClaims.EXPECT().Delete("test-operator-volume", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockPersistentVolumes.EXPECT().Delete("test-operator-volume", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Delete("test-operator", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(s.k8sNotFoundError()),
	)

	err := s.broker.DeleteOperator("test")
	c.Assert(err, jc.ErrorIsNil)
}

func operatorStatefulSetArg(numUnits int32, scName string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-operator",
			Labels: map[string]string{
				"juju-operator": "test",
			},
			Annotations: map[string]string{
				"juju-version": "2.99.0",
				"fred":         "mary",
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
					},
					Annotations: map[string]string{
						"fred":         "mary",
						"juju-version": "2.99.0",
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
					},
				},
				Spec: operatorPodspec,
			},
			VolumeClaimTemplates: []core.PersistentVolumeClaim{{
				ObjectMeta: v1.ObjectMeta{
					Name: "charm",
					Annotations: map[string]string{
						"foo": "bar",
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
			Name: "app-name",
			Annotations: map[string]string{
				"juju-app-uuid": "appuuid",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"juju-app": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
					},
				},
				Spec: podSpec,
			},
			VolumeClaimTemplates: []core.PersistentVolumeClaim{{
				ObjectMeta: v1.ObjectMeta{
					Name: "database-appuuid",
					Annotations: map[string]string{
						"foo":          "bar",
						"juju-storage": "database",
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
			ServiceName:         "app-name-endpoints",
		},
	}
}

func (s *K8sBrokerSuite) TestEnsureOperator(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	configMapArg := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-operator-config",
		},
		Data: map[string]string{
			"test-agent.conf": "agent-conf-data",
		},
	}
	statefulSetArg := operatorStatefulSetArg(1, "test-operator-storage")

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Update(configMapArg).Times(1),
		s.mockStorageClass.EXPECT().Get("test-operator-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "test-operator-storage"}}, nil),
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
			Attributes:   map[string]interface{}{"storage-class": "operator-storage"},
			ResourceTags: map[string]string{"foo": "bar"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureOperatorNoAgentConfig(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	statefulSetArg := operatorStatefulSetArg(1, "test-operator-storage")
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Get("test-operator-config", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-operator-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "test-operator-storage"}}, nil),
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
			Attributes:   map[string]interface{}{"storage-class": "operator-storage"},
			ResourceTags: map[string]string{"foo": "bar"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureOperatorNoAgentConfigMissingConfigMap(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Get("test-operator-config", v1.GetOptions{IncludeUninitialized: true}).Times(1).
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

func (s *K8sBrokerSuite) TestDeleteServiceForApplication(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	// Delete operations below return a not found to ensure it's treated as a no-op.
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Delete("test", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Delete("test", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockServices.EXPECT().Delete("test-endpoints", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Delete("test", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==test"}).Times(1).
			Return(&core.SecretList{Items: []core.Secret{{
				ObjectMeta: v1.ObjectMeta{Name: "secret"},
			}}}, nil),
		s.mockSecrets.EXPECT().Delete("secret", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, nil),
			v1.ListOptions{LabelSelector: "juju-app==test,juju-model==test", IncludeUninitialized: true},
		).Times(1).Return(nil),
		s.mockClusterRoleBindings.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, nil),
			v1.ListOptions{LabelSelector: "juju-app==test,juju-model==test", IncludeUninitialized: true},
		).Times(1).Return(nil),
		s.mockRoles.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, nil),
			v1.ListOptions{LabelSelector: "juju-app==test,juju-model==test", IncludeUninitialized: true},
		).Times(1).Return(nil),
		s.mockClusterRoles.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, nil),
			v1.ListOptions{LabelSelector: "juju-app==test,juju-model==test", IncludeUninitialized: true},
		).Times(1).Return(nil),
		s.mockServiceAccounts.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, nil),
			v1.ListOptions{LabelSelector: "juju-app==test,juju-model==test", IncludeUninitialized: true},
		).Times(1).Return(nil),
	)

	err := s.broker.DeleteService("test")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceNoUnits(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	two := int32(2)
	dc := &apps.Deployment{ObjectMeta: v1.ObjectMeta{Name: "juju-unit-storage"}, Spec: apps.DeploymentSpec{Replicas: &two}}
	zero := int32(0)
	emptyDc := dc
	emptyDc.Spec.Replicas = &zero
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(dc, nil),
		s.mockDeployments.EXPECT().Update(emptyDc).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{}
	err := s.broker.EnsureService("app-name", nil, params, 0, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceNoStorage(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	basicPodSpec := getBasicPodspec()
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"juju-app": "app-name"},
			Annotations: map[string]string{
				"fred": "mary",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels: map[string]string{
						"juju-app": "app-name",
					},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred": "mary",
					},
				},
				Spec: podSpec,
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"juju-app": "app-name"},
			Annotations: map[string]string{
				"fred": "mary",
				"a":    "b",
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"juju-app": "app-name"},
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
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Update(secretArg).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(serviceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(serviceArg).Times(1).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Update(deploymentArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(deploymentArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      basicPodSpec,
		ResourceTags: map[string]string{"fred": "mary"},
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-type":            "nodeIP",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceNoStorageStateful(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)

	numUnits := int32(2)
	statefulSetArg := &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: "app-name",
			Annotations: map[string]string{
				"juju-app-uuid": "appuuid",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"juju-app": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
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
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(&serviceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(&serviceArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("app-name-endpoints", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicHeadlessServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicHeadlessServiceArg).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateful,
		},
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceCustomType(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)

	numUnits := int32(2)
	statefulSetArg := &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: "app-name",
			Annotations: map[string]string{
				"juju-app-uuid": "appuuid",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"juju-app": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
					},
				},
				Spec: podSpec,
			},
			PodManagementPolicy: apps.ParallelPodManagement,
			ServiceName:         "app-name-endpoints",
		},
	}

	serviceArg := *basicServiceArg
	serviceArg.Spec.Type = core.ServiceTypeExternalName
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"juju-app-uuid": "appuuid"}}}, nil),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(&serviceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(&serviceArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("app-name-endpoints", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicHeadlessServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicHeadlessServiceArg).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			ServiceType: caas.ServiceExternal,
		},
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceServiceWithoutPortsNotValid(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	serviceArg := *basicServiceArg
	serviceArg.Spec.Type = core.ServiceTypeExternalName
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"juju-app-uuid": "appuuid"}}}, nil),
		s.mockSecrets.EXPECT().Delete("app-name-test-secret", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(nil),
	)
	caasPodSpec := getBasicPodspec()
	for k, v := range caasPodSpec.Containers {
		v.Ports = []specs.ContainerPort{}
		caasPodSpec.Containers[k] = v
	}
	c.Assert(caasPodSpec.OmitServiceFrontend, jc.IsFalse)
	for _, v := range caasPodSpec.Containers {
		c.Check(len(v.Ports), jc.DeepEquals, 0)
	}
	params := &caas.ServiceParams{
		PodSpec: caasPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateful,
			ServiceType:    caas.ServiceExternal,
		},
	}
	err := s.broker.EnsureService(
		"app-name",
		func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil },
		params, 2,
		application.ConfigAttributes{
			"kubernetes-service-loadbalancer-ip": "10.0.0.1",
			"kubernetes-service-externalname":    "ext-name",
		},
	)
	c.Assert(err, gc.ErrorMatches, `ports are required for kubernetes service`)
}

func (s *K8sBrokerSuite) assertCustomerResourceDefinitions(c *gc.C, crds map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec, assertCalls ...*gomock.Call) {

	basicPodSpec := getBasicPodspec()
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			CustomResourceDefinitions: crds,
		},
	}
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)

	numUnits := int32(2)
	statefulSetArg := &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: "app-name",
			Annotations: map[string]string{
				"juju-app-uuid": "appuuid",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{"juju-app": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
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
			s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
				Return(nil, s.k8sNotFoundError()),
		},
		assertCalls...,
	)

	assertCalls = append(assertCalls, []*gomock.Call{
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(&serviceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(&serviceArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("app-name-endpoints", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicHeadlessServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicHeadlessServiceArg).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
	}...)
	gomock.InOrder(assertCalls...)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Deployment: caas.DeploymentParams{
			DeploymentType: caas.DeploymentStateful,
		},
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureCustomResourceDefinitionCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crds := map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec{
		"tfjobs.kubeflow.org": {
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

	s.assertCustomerResourceDefinitions(
		c, crds,
		s.mockCustomResourceDefinition.EXPECT().Create(crd).Times(1).Return(crd, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureCustomResourceDefinitionUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	crds := map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec{
		"tfjobs.kubeflow.org": {
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

	s.assertCustomerResourceDefinitions(
		c, crds,
		s.mockCustomResourceDefinition.EXPECT().Create(crd).Times(1).Return(crd, s.k8sAlreadyExistsError()),
		s.mockCustomResourceDefinition.EXPECT().Get("tfjobs.kubeflow.org", v1.GetOptions{}).Times(1).Return(crd, nil),
		s.mockCustomResourceDefinition.EXPECT().Update(crd).Times(1).Return(crd, nil),
	)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithServiceAccountNewClusterRoleCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podSpec := getBasicPodspec()
	podSpec.ServiceAccount = &specs.ServiceAccountSpec{
		Name:                         "build-robot",
		AutomountServiceAccountToken: boolPtr(true),
		Capabilities: &specs.Capabilities{
			RoleBinding: &specs.RoleBindingSpec{
				Name: "read-pods",
				Type: specs.ClusterRoleBinding,
			},
			Role: &specs.RoleSpec{
				Name: "pod-reader",
				Type: specs.ClusterRole,
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"pods"},
						Verbs:     []string{"get", "watch", "list"},
					},
				},
			},
		},
	}

	numUnits := int32(2)
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", podSpec)
	c.Assert(err, jc.ErrorIsNil)

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"juju-app": "app-name"},
			Annotations: map[string]string{
				"fred": "mary",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels: map[string]string{
						"juju-app": "app-name",
					},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred": "mary",
					},
				},
				Spec: provider.PodSpec(unitSpec),
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"juju-app": "app-name"},
			Annotations: map[string]string{
				"fred": "mary",
				"a":    "b",
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"juju-app": "app-name"},
			Type:     "nodeIP",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "build-robot",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		AutomountServiceAccountToken: boolPtr(true),
	}
	cr := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod-reader",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "read-pods",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "pod-reader",
			Kind: "ClusterRole",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "build-robot",
				Namespace: "test",
			},
		},
	}

	secretArg := s.secretArg(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(svcAccount).Times(1).Return(svcAccount, nil),
		s.mockClusterRoles.EXPECT().Create(cr).Times(1).Return(cr, nil),
		s.mockClusterRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&rbacv1.ClusterRoleBindingList{Items: []rbacv1.ClusterRoleBinding{}}, nil),
		s.mockClusterRoleBindings.EXPECT().Create(crb).Times(1).Return(crb, nil),
		s.mockSecrets.EXPECT().Update(secretArg).Times(1).Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(secretArg).Times(1).Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(serviceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(serviceArg).Times(1).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Update(deploymentArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(deploymentArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      podSpec,
		ResourceTags: map[string]string{"fred": "mary"},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, application.ConfigAttributes{
		"kubernetes-service-type":            "nodeIP",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithServiceAccountNewClusterRoleUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podSpec := getBasicPodspec()
	podSpec.ServiceAccount = &specs.ServiceAccountSpec{
		Name:                         "build-robot",
		AutomountServiceAccountToken: boolPtr(true),
		Capabilities: &specs.Capabilities{
			RoleBinding: &specs.RoleBindingSpec{
				Name: "read-pods",
				Type: specs.ClusterRoleBinding,
			},
			Role: &specs.RoleSpec{
				Name: "pod-reader",
				Type: specs.ClusterRole,
				Rules: []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"pods"},
						Verbs:     []string{"get", "watch", "list"},
					},
				},
			},
		},
	}

	numUnits := int32(2)
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", podSpec)
	c.Assert(err, jc.ErrorIsNil)

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"juju-app": "app-name"},
			Annotations: map[string]string{
				"fred": "mary",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels: map[string]string{
						"juju-app": "app-name",
					},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred": "mary",
					},
				},
				Spec: provider.PodSpec(unitSpec),
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"juju-app": "app-name"},
			Annotations: map[string]string{
				"fred": "mary",
				"a":    "b",
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"juju-app": "app-name"},
			Type:     "nodeIP",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "build-robot",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		AutomountServiceAccountToken: boolPtr(true),
	}
	cr := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod-reader",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "read-pods",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "pod-reader",
			Kind: "ClusterRole",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "build-robot",
				Namespace: "test",
			},
		},
	}
	crbUID := crb.GetUID()

	secretArg := s.secretArg(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(svcAccount).Times(1).Return(nil, s.k8sAlreadyExistsError()),
		s.mockServiceAccounts.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&core.ServiceAccountList{Items: []core.ServiceAccount{*svcAccount}}, nil),
		s.mockServiceAccounts.EXPECT().Update(svcAccount).Times(1).Return(svcAccount, nil),
		s.mockClusterRoles.EXPECT().Create(cr).Times(1).Return(nil, s.k8sAlreadyExistsError()),
		s.mockClusterRoles.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&rbacv1.ClusterRoleList{Items: []rbacv1.ClusterRole{*cr}}, nil),
		s.mockClusterRoles.EXPECT().Update(cr).Times(1).Return(cr, nil),
		s.mockClusterRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&rbacv1.ClusterRoleBindingList{Items: []rbacv1.ClusterRoleBinding{*crb}}, nil),
		s.mockClusterRoleBindings.EXPECT().Delete("read-pods", s.deleteOptions(v1.DeletePropagationForeground, &crbUID)).Times(1).Return(nil),
		s.mockClusterRoleBindings.EXPECT().Create(crb).Times(1).Return(crb, nil),
		s.mockSecrets.EXPECT().Update(secretArg).Times(1).Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(secretArg).Times(1).Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(serviceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(serviceArg).Times(1).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Update(deploymentArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(deploymentArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      podSpec,
		ResourceTags: map[string]string{"fred": "mary"},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, application.ConfigAttributes{
		"kubernetes-service-type":            "nodeIP",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithServiceAccountReferenceExistingClusterRole(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podSpec := getBasicPodspec()
	podSpec.ServiceAccount = &specs.ServiceAccountSpec{
		Name:                         "build-robot",
		AutomountServiceAccountToken: boolPtr(true),
		Capabilities: &specs.Capabilities{
			RoleBinding: &specs.RoleBindingSpec{
				Name: "read-pods",
				Type: specs.ClusterRoleBinding,
			},
			Role: &specs.RoleSpec{
				Name: "pod-reader",
				Type: specs.ClusterRole,
				// No Rules specified, Get an existing Role to use.
			},
		},
	}

	numUnits := int32(2)
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", podSpec)
	c.Assert(err, jc.ErrorIsNil)

	deploymentArg := &appsv1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"juju-app": "app-name"},
			Annotations: map[string]string{
				"fred": "mary",
			}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels: map[string]string{
						"juju-app": "app-name",
					},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred": "mary",
					},
				},
				Spec: provider.PodSpec(unitSpec),
			},
		},
	}
	serviceArg := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"juju-app": "app-name"},
			Annotations: map[string]string{
				"fred": "mary",
				"a":    "b",
			}},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"juju-app": "app-name"},
			Type:     "nodeIP",
			Ports: []core.ServicePort{
				{Port: 80, TargetPort: intstr.FromInt(80), Protocol: "TCP"},
				{Port: 8080, Protocol: "TCP", Name: "fred"},
			},
			LoadBalancerIP: "10.0.0.1",
			ExternalName:   "ext-name",
		},
	}

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "build-robot",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		AutomountServiceAccountToken: boolPtr(true),
	}
	cr := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod-reader",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "read-pods",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "pod-reader",
			Kind: "ClusterRole",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "build-robot",
				Namespace: "test",
			},
		},
	}
	crbUID := crb.GetUID()

	secretArg := s.secretArg(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(svcAccount).Times(1).Return(svcAccount, nil),
		s.mockClusterRoles.EXPECT().Get("pod-reader", v1.GetOptions{IncludeUninitialized: true}).Times(1).Return(cr, nil),
		s.mockClusterRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&rbacv1.ClusterRoleBindingList{Items: []rbacv1.ClusterRoleBinding{*crb}}, nil),
		s.mockClusterRoleBindings.EXPECT().Delete("read-pods", s.deleteOptions(v1.DeletePropagationForeground, &crbUID)).Times(1).Return(nil),
		s.mockClusterRoleBindings.EXPECT().Create(crb).Times(1).Return(crb, nil),
		s.mockSecrets.EXPECT().Update(secretArg).Times(1).Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(secretArg).Times(1).Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(serviceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(serviceArg).Times(1).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Update(deploymentArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(deploymentArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec:      podSpec,
		ResourceTags: map[string]string{"fred": "mary"},
	}
	err = s.broker.EnsureService("app-name", func(_ string, _ status.Status, _ string, _ map[string]interface{}) error { return nil }, params, 2, application.ConfigAttributes{
		"kubernetes-service-type":            "nodeIP",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
		"kubernetes-service-annotations":     map[string]interface{}{"a": "b"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithStorage(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)
	podSpec.Containers[0].VolumeMounts = []core.VolumeMount{{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	}, {
		Name:      "logs-1",
		MountPath: "path/to/there",
	}}
	size, err := resource.ParseQuantity("200Mi")
	c.Assert(err, jc.ErrorIsNil)
	podSpec.Volumes = []core.Volume{{
		Name: "logs-1",
		VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{
			SizeLimit: &size,
			Medium:    "Memory",
		}},
	}}
	statefulSetArg := unitStatefulSetArg(2, "workload-storage", podSpec)

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"juju-app-uuid": "appuuid"}}}, nil),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicServiceArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("app-name-endpoints", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicHeadlessServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicHeadlessServiceArg).Times(1).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-workload-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get("workload-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attributes:  map[string]interface{}{"storage-class": "workload-storage"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			ResourceTags: map[string]string{"foo": "bar"},
		}, {
			StorageName: "logs",
			Size:        200,
			Provider:    "tmpfs",
			Attributes:  map[string]interface{}{"storage-medium": "Memory"},
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/there",
			},
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
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	basicPodSpec := getBasicPodspec()
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
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
			Name:        "app-name",
			Labels:      map[string]string{"juju-app": "app-name"},
			Annotations: map[string]string{}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numUnits,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"juju-app": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
					},
				},
				Spec: podSpec,
			},
		},
	}

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicServiceArg).Times(1).
			Return(nil, nil),
		s.mockDeployments.EXPECT().Update(deploymentArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Create(deploymentArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
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
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)
	podSpec.Containers[0].VolumeMounts = []core.VolumeMount{{
		Name:      "database-appuuid",
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
	statefulSetArg := unitStatefulSetArg(2, "workload-storage", podSpec)

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"juju-app-uuid": "appuuid"}}}, nil),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicServiceArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("app-name-endpoints", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicHeadlessServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicHeadlessServiceArg).Times(1).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-workload-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get("workload-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			Attributes:   map[string]interface{}{"storage-class": "workload-storage"},
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
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)
	podSpec.Containers[0].VolumeMounts = []core.VolumeMount{{
		Name:      "database-appuuid",
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
	statefulSetArg := unitStatefulSetArg(2, "workload-storage", podSpec)

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"juju-app-uuid": "appuuid"}}}, nil),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicServiceArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("app-name-endpoints", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicHeadlessServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicHeadlessServiceArg).Times(1).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-workload-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get("workload-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			Attributes:   map[string]interface{}{"storage-class": "workload-storage"},
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

func (s *K8sBrokerSuite) TestEnsureServiceWithNodeAffinity(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)
	podSpec.Containers[0].VolumeMounts = []core.VolumeMount{{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	}}
	podSpec.Affinity = &core.Affinity{
		NodeAffinity: &core.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
				NodeSelectorTerms: []core.NodeSelectorTerm{{
					MatchExpressions: []core.NodeSelectorRequirement{{
						Key:      "foo",
						Operator: core.NodeSelectorOpIn,
						Values:   []string{"a", "b", "c"},
					}, {
						Key:      "bar",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"d", "e", "f"},
					}, {
						Key:      "foo",
						Operator: core.NodeSelectorOpNotIn,
						Values:   []string{"g", "h"},
					}},
				}},
			},
		},
	}
	statefulSetArg := unitStatefulSetArg(2, "workload-storage", podSpec)

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"juju-app-uuid": "appuuid"}}}, nil),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicServiceArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("app-name-endpoints", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicHeadlessServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicHeadlessServiceArg).Times(1).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-workload-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get("workload-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			Attributes:   map[string]interface{}{"storage-class": "workload-storage"},
			ResourceTags: map[string]string{"foo": "bar"},
		}},
		Constraints: constraints.MustParse(`tags=foo=a|b|c,^bar=d|e|f,^foo=g|h`),
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-type":            "nodeIP",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureServiceWithZones(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	unitSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(unitSpec)
	podSpec.Containers[0].VolumeMounts = []core.VolumeMount{{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	}}
	podSpec.Affinity = &core.Affinity{
		NodeAffinity: &core.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &core.NodeSelector{
				NodeSelectorTerms: []core.NodeSelectorTerm{{
					MatchExpressions: []core.NodeSelectorRequirement{{
						Key:      "failure-domain.beta.kubernetes.io/zone",
						Operator: core.NodeSelectorOpIn,
						Values:   []string{"a", "b", "c"},
					}},
				}},
			},
		},
	}
	statefulSetArg := unitStatefulSetArg(2, "workload-storage", podSpec)

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Update(s.secretArg(c, nil)).Times(1).
			Return(nil, nil),
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&appsv1.StatefulSet{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{"juju-app-uuid": "appuuid"}}}, nil),
		s.mockServices.EXPECT().Get("app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicServiceArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("app-name-endpoints", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(basicHeadlessServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(basicHeadlessServiceArg).Times(1).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-workload-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStorageClass.EXPECT().Get("workload-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "workload-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(nil, nil),
	)

	params := &caas.ServiceParams{
		PodSpec: basicPodSpec,
		Filesystems: []storage.KubernetesFilesystemParams{{
			StorageName: "database",
			Size:        100,
			Provider:    "kubernetes",
			Attachment: &storage.KubernetesFilesystemAttachmentParams{
				Path: "path/to/here",
			},
			Attributes:   map[string]interface{}{"storage-class": "workload-storage"},
			ResourceTags: map[string]string{"foo": "bar"},
		}},
		Constraints: constraints.MustParse(`zones=a,b,c`),
	}
	err = s.broker.EnsureService("app-name", nil, params, 2, application.ConfigAttributes{
		"kubernetes-service-type":            "nodeIP",
		"kubernetes-service-loadbalancer-ip": "10.0.0.1",
		"kubernetes-service-externalname":    "ext-name",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestOperator(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	opPod := core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-operator",
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
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator==test"}).Times(1).
			Return(&core.PodList{Items: []core.Pod{}}, nil),
	)

	_, err := s.broker.Operator("test")
	c.Assert(err, gc.ErrorMatches, "operator pod for application \"test\" not found")
}

func (s *K8sBrokerSuite) TestWatchService(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ssWatcher := watch.NewRaceFreeFake()
	deployWatcher := watch.NewRaceFreeFake()

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Watch(v1.ListOptions{
			LabelSelector: "juju-app==test",
			Watch:         true,
		}).Return(ssWatcher, nil),
		s.mockDeployments.EXPECT().Watch(v1.ListOptions{
			LabelSelector: "juju-app==test",
			Watch:         true,
		}).Return(deployWatcher, nil),
	)

	w, err := s.broker.WatchService("test")
	c.Assert(err, jc.ErrorIsNil)

	// Send an event to one of the watchers; multi-watcher should fire.
	ss := &apps.StatefulSet{ObjectMeta: v1.ObjectMeta{Name: "test"}}
	go func(w *watch.RaceFreeFakeWatcher, clk *testclock.Clock) {
		if !w.IsStopped() {
			clk.WaitAdvance(time.Second, testing.ShortWait, 1)
			w.Modify(ss)
		}
	}(ssWatcher, s.clock)

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatal("timed out waiting for event")
	}
}

func (s *K8sBrokerSuite) TestUpgradeController(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ss := apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: "controller",
			Annotations: map[string]string{
				"juju-version": "1.1.1",
			},
			Labels: map[string]string{"juju-operator": "controller"},
		},
		Spec: apps.StatefulSetSpec{
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"juju-version": "1.1.1",
					},
				},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{Image: "foo"},
						{Image: "jujud-operator:1.1.1"},
					},
				},
			},
		},
	}
	updated := ss
	updated.Annotations["juju-version"] = "6.6.6"
	updated.Spec.Template.Annotations["juju-version"] = "6.6.6"
	updated.Spec.Template.Spec.Containers[1].Image = "juju-operator:6.6.6"
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("controller", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&ss, nil),
		s.mockStatefulSets.EXPECT().Update(&updated).Times(1).
			Return(nil, nil),
	)

	err := s.broker.Upgrade("controller", version.MustParse("6.6.6"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestUpgradeOperator(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ss := apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-app-operator",
			Annotations: map[string]string{
				"juju-version": "1.1.1",
			},
			Labels: map[string]string{"juju-app": "test-app"},
		},
		Spec: apps.StatefulSetSpec{
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Annotations: map[string]string{
						"juju-version": "1.1.1",
					},
				},
				Spec: core.PodSpec{
					Containers: []core.Container{
						{Image: "foo"},
						{Image: "jujud-operator:1.1.1"},
					},
				},
			},
		},
	}
	updated := ss
	updated.Annotations["juju-version"] = "6.6.6"
	updated.Spec.Template.Annotations["juju-version"] = "6.6.6"
	updated.Spec.Template.Spec.Containers[1].Image = "juju-operator:6.6.6"
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test-app", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("test-app-operator", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&ss, nil),
		s.mockStatefulSets.EXPECT().Update(&updated).Times(1).
			Return(nil, nil),
	)

	err := s.broker.Upgrade("test-app", version.MustParse("6.6.6"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestUpgradeNotSupported(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test-app", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("test-app-operator", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
	)

	err := s.broker.Upgrade("test-app", version.MustParse("6.6.6"))
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}
