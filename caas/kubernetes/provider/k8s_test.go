// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
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
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"

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

func (s *K8sSuite) TestPrepareWorkloadSpecNoConfigConfig(c *gc.C) {
	podSpec := specs.PodSpec{
		ServiceAccount: &specs.ServiceAccountSpec{
			ClusterRoleNames:             []string{"clusterRole1"},
			AutomountServiceAccountToken: boolPtr(true),
		},
	}

	podSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Pod: &k8sspecs.PodSpec{
				RestartPolicy:                 core.RestartPolicyOnFailure,
				ActiveDeadlineSeconds:         int64Ptr(10),
				TerminationGracePeriodSeconds: int64Ptr(20),
				SecurityContext: &core.PodSecurityContext{
					RunAsNonRoot:       boolPtr(true),
					SupplementalGroups: []int64{1, 2},
				},
				ReadinessGates: []core.PodReadinessGate{
					{ConditionType: core.PodInitialized},
				},
				DNSPolicy: core.DNSClusterFirst,
			},
		},
	}
	podSpec.Containers = []specs.ContainerSpec{
		{
			Name:            "test",
			Ports:           []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image:           "juju/image",
			ImagePullPolicy: specs.PullPolicy("Always"),
			ProviderContainer: &k8sspecs.K8sContainerSpec{
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
		ReadinessGates: []core.PodReadinessGate{
			{ConditionType: core.PodInitialized},
		},
		DNSPolicy:                    core.DNSClusterFirst,
		ServiceAccountName:           "app-name",
		AutomountServiceAccountToken: boolPtr(true),
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

func (s *K8sSuite) TestPrepareWorkloadSpecWithInitContainers(c *gc.C) {
	podSpec := specs.PodSpec{}
	podSpec.Containers = []specs.ContainerSpec{
		{
			Name:            "test",
			Ports:           []specs.ContainerPort{{ContainerPort: 80, Protocol: "TCP"}},
			Image:           "juju/image",
			ImagePullPolicy: specs.PullPolicy("Always"),
			ProviderContainer: &k8sspecs.K8sContainerSpec{
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
			Name:            "test-init",
			Init:            true,
			Ports:           []specs.ContainerPort{{ContainerPort: 90, Protocol: "TCP"}},
			Image:           "juju/image-init",
			ImagePullPolicy: specs.PullPolicy("Always"),
			WorkingDir:      "/path/to/here",
			Command:         []string{"sh", "ls"},
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
			"brackets":   `'["hello", "world"]'`,
		},
	}, {
		Name:  "test2",
		Ports: []specs.ContainerPort{{ContainerPort: 8080, Protocol: "TCP", Name: "fred"}},
		Image: "juju/image2",
	}}
	return pSpecs
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

func (s *K8sBrokerSuite) getOCIImageSecret(c *gc.C, annotations map[string]string) *core.Secret {
	secretData, err := provider.CreateDockerConfigJSON(&getBasicPodspec().Containers[0].ImageDetails)
	c.Assert(err, jc.ErrorIsNil)
	if annotations == nil {
		annotations = map[string]string{}
	}

	return &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        "app-name-test-secret",
			Namespace:   "test",
			Labels:      map[string]string{"juju-app": "app-name", "juju-model": "test"},
			Annotations: annotations,
		},
		Type: "kubernetes.io/dockerconfigjson",
		Data: map[string][]byte{".dockerconfigjson": secretData},
	}
}

func (s *K8sSuite) TestPrepareWorkloadSpecConfigPairs(c *gc.C) {
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
					{Name: "brackets", Value: `["hello", "world"]`},
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

type K8sBrokerSuite struct {
	BaseSuite
}

var _ = gc.Suite(&K8sBrokerSuite{})

func (s *K8sBrokerSuite) TestAPIVersion(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Times(1).Return(&k8sversion.Info{
			Major: "1", Minor: "16",
		}, nil),
	)

	ver, err := s.broker.APIVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ver, gc.DeepEquals, "1.16.0")
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
		ControllerConfig:         testing.FakeControllerConfig(),
		BootstrapConstraints:     constraints.MustParse("mem=3.5G"),
		SupportedBootstrapSeries: testing.FakeSupportedJujuSeries,
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
		ControllerConfig:         testing.FakeControllerConfig(),
		BootstrapConstraints:     constraints.MustParse("mem=3.5G"),
		SupportedBootstrapSeries: testing.FakeSupportedJujuSeries,
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

		// delete secrets.
		s.mockSecrets.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, nil),
			v1.ListOptions{LabelSelector: "juju-app==test,juju-model==test", IncludeUninitialized: true},
		).Times(1).Return(nil),

		// delete configmaps.
		s.mockConfigMaps.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, nil),
			v1.ListOptions{LabelSelector: "juju-app==test,juju-model==test", IncludeUninitialized: true},
		).Times(1).Return(nil),

		// delete RBAC resources.
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
		s.mockCustomResourceDefinition.EXPECT().DeleteCollection(
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
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)

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

	ociImageSecret := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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

func (s *K8sBrokerSuite) TestEnsureServiceWithConfigMapAndSecretsCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	basicPodSpec := getBasicPodspec()
	basicPodSpec.ConfigMaps = map[string]specs.ConfigMap{
		"myData": {
			"foo":   "bar",
			"hello": "world",
		},
	}
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Secrets: []k8sspecs.Secret{
				{
					Name: "build-robot-secret",
					Type: core.SecretTypeOpaque,
					StringData: map[string]string{
						"config.yaml": `
apiUrl: "https://my.api.com/api/v1"
username: fred
password: shhhh`[1:],
					},
				},
				{
					Name: "another-build-robot-secret",
					Type: core.SecretTypeOpaque,
					Data: map[string]string{
						"username": "YWRtaW4=",
						"password": "MWYyZDFlMmU2N2Rm",
					},
				},
			},
		},
	}

	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)

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

	cm := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myData",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Data: map[string]string{
			"foo":   "bar",
			"hello": "world",
		},
	}
	secrets1 := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "build-robot-secret",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Type: core.SecretTypeOpaque,
		StringData: map[string]string{
			"config.yaml": `
apiUrl: "https://my.api.com/api/v1"
username: fred
password: shhhh`[1:],
		},
	}

	secrets2Data, err := provider.ProcessSecretData(
		map[string]string{
			"username": "YWRtaW4=",
			"password": "MWYyZDFlMmU2N2Rm",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	secrets2 := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "another-build-robot-secret",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Type: core.SecretTypeOpaque,
		Data: secrets2Data,
	}

	ociImageSecret := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),

		// ensure configmaps.
		s.mockConfigMaps.EXPECT().Create(cm).Times(1).
			Return(cm, nil),

		// ensure secrets.
		s.mockSecrets.EXPECT().Create(secrets1).Times(1).
			Return(secrets1, nil),
		s.mockSecrets.EXPECT().Create(secrets2).Times(1).
			Return(secrets2, nil),

		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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

func (s *K8sBrokerSuite) TestVersion(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockDiscovery.EXPECT().ServerVersion().Times(1).Return(&k8sversion.Info{
			Major: "1", Minor: "15",
		}, nil),
	)

	ver, err := s.broker.Version()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ver, gc.DeepEquals, &version.Number{Major: 1, Minor: 15})
}

func (s *K8sBrokerSuite) TestEnsureServiceWithConfigMapAndSecretsUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	numUnits := int32(2)
	basicPodSpec := getBasicPodspec()
	basicPodSpec.ConfigMaps = map[string]specs.ConfigMap{
		"myData": {
			"foo":   "bar",
			"hello": "world",
		},
	}
	basicPodSpec.ProviderPod = &k8sspecs.K8sPodSpec{
		KubernetesResources: &k8sspecs.KubernetesResources{
			Secrets: []k8sspecs.Secret{
				{
					Name: "build-robot-secret",
					Type: core.SecretTypeOpaque,
					StringData: map[string]string{
						"config.yaml": `
apiUrl: "https://my.api.com/api/v1"
username: fred
password: shhhh`[1:],
					},
				},
				{
					Name: "another-build-robot-secret",
					Type: core.SecretTypeOpaque,
					Data: map[string]string{
						"username": "YWRtaW4=",
						"password": "MWYyZDFlMmU2N2Rm",
					},
				},
			},
		},
	}

	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)

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

	cm := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "myData",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Data: map[string]string{
			"foo":   "bar",
			"hello": "world",
		},
	}
	secrets1 := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "build-robot-secret",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Type: core.SecretTypeOpaque,
		StringData: map[string]string{
			"config.yaml": `
apiUrl: "https://my.api.com/api/v1"
username: fred
password: shhhh`[1:],
		},
	}

	secrets2Data, err := provider.ProcessSecretData(
		map[string]string{
			"username": "YWRtaW4=",
			"password": "MWYyZDFlMmU2N2Rm",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	secrets2 := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "another-build-robot-secret",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		Type: core.SecretTypeOpaque,
		Data: secrets2Data,
	}

	ociImageSecret := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),

		// ensure configmaps.
		s.mockConfigMaps.EXPECT().Create(cm).Times(1).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockConfigMaps.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&core.ConfigMapList{Items: []core.ConfigMap{*cm}}, nil),
		s.mockConfigMaps.EXPECT().Update(cm).Times(1).
			Return(cm, nil),

		// ensure secrets.
		s.mockSecrets.EXPECT().Create(secrets1).Times(1).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockSecrets.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&core.SecretList{Items: []core.Secret{*secrets1}}, nil),
		s.mockSecrets.EXPECT().Update(secrets1).Times(1).
			Return(secrets1, nil),
		s.mockSecrets.EXPECT().Create(secrets2).Times(1).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockSecrets.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&core.SecretList{Items: []core.Secret{*secrets2}}, nil),
		s.mockSecrets.EXPECT().Update(secrets2).Times(1).
			Return(secrets2, nil),

		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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
	basicPodSpec.Service = &specs.ServiceSpec{
		ScalePolicy: "serial",
	}
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)

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
			PodManagementPolicy: apps.PodManagementPolicyType("OrderedReady"),
			ServiceName:         "app-name-endpoints",
		},
	}

	serviceArg := *basicServiceArg
	serviceArg.Spec.Type = core.ServiceTypeClusterIP
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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
			Return(statefulSetArg, nil),
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
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)

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
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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
			Return(statefulSetArg, nil),
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
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)

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

	ociImageSecret := s.getOCIImageSecret(c, nil)
	assertCalls = append(assertCalls, []*gomock.Call{
		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
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
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
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

func (s *K8sBrokerSuite) TestEnsureServiceWithServiceAccountNewRoleCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podSpec := getBasicPodspec()
	podSpec.ServiceAccount = &specs.ServiceAccountSpec{
		AutomountServiceAccountToken: boolPtr(true),
		Rules: []specs.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}

	numUnits := int32(2)
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", podSpec)
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
				Spec: provider.PodSpec(workloadSpec),
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
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		AutomountServiceAccountToken: boolPtr(true),
	}
	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
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
	rb := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "app-name",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "app-name",
				Namespace: "test",
			},
		},
	}

	secretArg := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(svcAccount).Times(1).Return(svcAccount, nil),
		s.mockRoles.EXPECT().Create(role).Times(1).Return(role, nil),
		s.mockRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&rbacv1.RoleBindingList{Items: []rbacv1.RoleBinding{}}, nil),
		s.mockRoleBindings.EXPECT().Create(rb).Times(1).Return(rb, nil),
		s.mockSecrets.EXPECT().Create(secretArg).Times(1).Return(secretArg, nil),
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

func (s *K8sBrokerSuite) TestEnsureServiceWithServiceAccountNewRoleUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	podSpec := getBasicPodspec()
	podSpec.ServiceAccount = &specs.ServiceAccountSpec{
		AutomountServiceAccountToken: boolPtr(true),
		Rules: []specs.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}

	numUnits := int32(2)
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", podSpec)
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
				Spec: provider.PodSpec(workloadSpec),
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
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		AutomountServiceAccountToken: boolPtr(true),
	}
	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
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
	rb := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "app-name",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "app-name",
				Namespace: "test",
			},
		},
	}
	rbUID := rb.GetUID()

	secretArg := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(svcAccount).Times(1).Return(nil, s.k8sAlreadyExistsError()),
		s.mockServiceAccounts.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&core.ServiceAccountList{Items: []core.ServiceAccount{*svcAccount}}, nil),
		s.mockServiceAccounts.EXPECT().Update(svcAccount).Times(1).Return(svcAccount, nil),
		s.mockRoles.EXPECT().Create(role).Times(1).Return(nil, s.k8sAlreadyExistsError()),
		s.mockRoles.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&rbacv1.RoleList{Items: []rbacv1.Role{*role}}, nil),
		s.mockRoles.EXPECT().Update(role).Times(1).Return(role, nil),
		s.mockRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&rbacv1.RoleBindingList{Items: []rbacv1.RoleBinding{*rb}}, nil),
		s.mockRoleBindings.EXPECT().Delete("app-name", s.deleteOptions(v1.DeletePropagationForeground, &rbUID)).Times(1).Return(nil),
		s.mockRoleBindings.EXPECT().Create(rb).Times(1).Return(rb, nil),
		s.mockSecrets.EXPECT().Create(secretArg).Times(1).Return(secretArg, nil),
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
		AutomountServiceAccountToken: boolPtr(true),
		ClusterRoleNames: []string{
			"existingClusterRole1",
			"existingClusterRole2",
		},
	}

	numUnits := int32(2)
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", podSpec)
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
				Spec: provider.PodSpec(workloadSpec),
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
			Name:      "app-name",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		AutomountServiceAccountToken: boolPtr(true),
	}
	existingClusterRole1 := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name: "existingClusterRole1",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	existingClusterRole2 := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name: "existingClusterRole2",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "watch", "list"},
			},
		},
	}
	rb1 := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name-existingClusterRole1",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "existingClusterRole1",
			Kind: "ClusterRole",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "app-name",
				Namespace: "test",
			},
		},
	}
	rbUID1 := rb1.GetUID()
	rb2 := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:      "app-name-existingClusterRole2",
			Namespace: "test",
			Labels:    map[string]string{"juju-app": "app-name", "juju-model": "test"},
		},
		RoleRef: rbacv1.RoleRef{
			Name: "existingClusterRole2",
			Kind: "ClusterRole",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "app-name",
				Namespace: "test",
			},
		},
	}
	rbUID2 := rb2.GetUID()

	secretArg := s.getOCIImageSecret(c, map[string]string{"fred": "mary"})
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Create(svcAccount).Times(1).Return(svcAccount, nil),
		s.mockClusterRoles.EXPECT().Get("existingClusterRole1", v1.GetOptions{IncludeUninitialized: true}).Times(1).Return(existingClusterRole1, nil),
		s.mockRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&rbacv1.RoleBindingList{Items: []rbacv1.RoleBinding{*rb1}}, nil),
		s.mockRoleBindings.EXPECT().Delete("app-name-existingClusterRole1", s.deleteOptions(v1.DeletePropagationForeground, &rbUID1)).Times(1).Return(nil),
		s.mockRoleBindings.EXPECT().Create(rb1).Times(1).Return(rb1, nil),
		s.mockClusterRoles.EXPECT().Get("existingClusterRole2", v1.GetOptions{IncludeUninitialized: true}).Times(1).Return(existingClusterRole2, nil),
		s.mockRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app==app-name,juju-model==test", IncludeUninitialized: true}).Times(1).
			Return(&rbacv1.RoleBindingList{Items: []rbacv1.RoleBinding{*rb2}}, nil),
		s.mockRoleBindings.EXPECT().Delete("app-name-existingClusterRole2", s.deleteOptions(v1.DeletePropagationForeground, &rbUID2)).Times(1).Return(nil),
		s.mockRoleBindings.EXPECT().Create(rb2).Times(1).Return(rb1, nil),
		s.mockSecrets.EXPECT().Create(secretArg).Times(1).Return(secretArg, nil),
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
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)
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
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)
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
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)
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
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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
			Return(statefulSetArg, nil),
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
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)
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
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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
			Return(statefulSetArg, nil),
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
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)
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
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec)
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)
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
	ociImageSecret := s.getOCIImageSecret(c, nil)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(ociImageSecret).Times(1).
			Return(ociImageSecret, nil),
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
