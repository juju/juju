// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8sstorage "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/pointer"

	"github.com/juju/juju/api"
	"github.com/juju/juju/controller"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/constraints"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/featureflag"
	"github.com/juju/juju/internal/provider/kubernetes"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/provider/kubernetes/mocks"
	k8swatcher "github.com/juju/juju/internal/provider/kubernetes/watcher"
	k8swatchertest "github.com/juju/juju/internal/provider/kubernetes/watcher/test"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
)

type bootstrapSuite struct {
	fakeClientSuite
	coretesting.JujuOSEnvSuite

	controllerCfg controller.Config
	pcfg          *podcfg.ControllerPodConfig

	controllerStackerGetter func() kubernetes.ControllerStackerForTest
}

func TestBootstrapSuite(t *testing.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) SetUpTest(c *tc.C) {
	s.fakeClientSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
	s.SetFeatureFlags(featureflag.DeveloperMode)
	s.broker = nil

	controllerName := "controller-1"
	s.namespace = controllerName

	cfg, err := config.New(config.UseDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		config.NameKey:                  "controller-1",
		k8sconstants.WorkloadStorageKey: "",
	}))
	c.Assert(err, tc.ErrorIsNil)
	s.cfg = cfg

	s.controllerCfg = coretesting.FakeControllerConfig()
	s.controllerCfg["juju-db-snap-channel"] = controller.DefaultJujuDBSnapChannel
	s.controllerCfg[controller.CAASImageRepo] = ""
	pcfg, err := podcfg.NewBootstrapControllerPodConfig(
		s.controllerCfg, controllerName, "ubuntu", constraints.MustParse("root-disk=10000M mem=4000M"))
	c.Assert(err, tc.ErrorIsNil)

	current := jujuversion.Current
	current.Build = 666
	pcfg.JujuVersion = current
	pcfg.APIInfo = &api.Info{
		Password: "password",
		CACert:   coretesting.CACert,
		ModelTag: coretesting.ModelTag,
	}
	pcfg.Bootstrap.ControllerModelConfig = s.cfg
	pcfg.Bootstrap.BootstrapMachineInstanceId = "instance-id"
	pcfg.Bootstrap.StateServingInfo = controller.StateServingInfo{
		Cert:         coretesting.ServerCert,
		PrivateKey:   coretesting.ServerKey,
		CAPrivateKey: coretesting.CAKey,
		StatePort:    123,
		APIPort:      456,
	}
	pcfg.Bootstrap.StateServingInfo = controller.StateServingInfo{
		Cert:         coretesting.ServerCert,
		PrivateKey:   coretesting.ServerKey,
		CAPrivateKey: coretesting.CAKey,
		StatePort:    123,
		APIPort:      456,
	}
	pcfg.Bootstrap.ControllerConfig = s.controllerCfg
	s.pcfg = pcfg
	s.controllerStackerGetter = func() kubernetes.ControllerStackerForTest {
		controllerStacker, err := kubernetes.NewcontrollerStackForTest(
			envtesting.BootstrapContext(c.Context(), c), "juju-controller-test", "some-storage", s.broker, s.pcfg,
		)
		c.Assert(err, tc.ErrorIsNil)
		return controllerStacker
	}
}

func (s *bootstrapSuite) TearDownTest(c *tc.C) {
	s.pcfg = nil
	s.controllerCfg = nil
	s.controllerStackerGetter = nil
	s.fakeClientSuite.TearDownTest(c)
	s.JujuOSEnvSuite.TearDownTest(c)
}

func (s *bootstrapSuite) TestFindControllerNamespace(c *tc.C) {
	tests := []struct {
		Namespace      core.Namespace
		ModelName      string
		ControllerUUID string
	}{
		{
			Namespace: core.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name: "controller-tlm",
					Annotations: map[string]string{
						"juju.io/controller": "abcd",
					},
					Labels: map[string]string{
						"juju-model": "controller",
					},
				},
			},
			ModelName:      "controller",
			ControllerUUID: "abcd",
		},
		{
			Namespace: core.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name: "controller-tlm",
					Annotations: map[string]string{
						"controller.juju.is/id": "abcd",
					},
					Labels: map[string]string{
						"model.juju.is/name": "controller",
					},
				},
			},
			ModelName:      "controller",
			ControllerUUID: "abcd",
		},
	}

	for _, test := range tests {
		client := fake.NewSimpleClientset()
		_, err := client.CoreV1().Namespaces().Create(
			c.Context(),
			&test.Namespace,
			v1.CreateOptions{},
		)
		c.Assert(err, tc.ErrorIsNil)
		ns, err := kubernetes.FindControllerNamespace(
			c.Context(), client, test.ControllerUUID)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(ns, tc.DeepEquals, &test.Namespace)
	}
}

type svcSpecTC struct {
	cloudType string
	spec      *kubernetes.ControllerServiceSpec
	errStr    string
	cfg       *podcfg.BootstrapConfig
}

func (s *bootstrapSuite) TestGetControllerSvcSpec(c *tc.C) {
	s.namespace = "controller-1"

	getCfg := func(externalName, controllerServiceType string, controllerExternalIPs []string) *podcfg.BootstrapConfig {
		o := new(podcfg.BootstrapConfig)
		*o = *s.pcfg.Bootstrap
		if len(externalName) > 0 {
			o.ControllerExternalName = externalName
		}
		if len(controllerServiceType) > 0 {
			o.ControllerServiceType = controllerServiceType
		}
		if len(controllerExternalIPs) > 0 {
			o.ControllerExternalIPs = controllerExternalIPs
		}
		return o
	}

	for i, t := range []svcSpecTC{
		{
			cloudType: "azure",
			spec: &kubernetes.ControllerServiceSpec{
				ServiceType: core.ServiceTypeLoadBalancer,
			},
		},
		{
			cloudType: "ec2",
			spec: &kubernetes.ControllerServiceSpec{
				ServiceType: core.ServiceTypeLoadBalancer,
				Annotations: k8sannotations.New(nil).
					Add("service.beta.kubernetes.io/aws-load-balancer-backend-protocol", "tcp"),
			},
		},
		{
			cloudType: "gce",
			spec: &kubernetes.ControllerServiceSpec{
				ServiceType: core.ServiceTypeLoadBalancer,
			},
		},
		{
			cloudType: "microk8s",
			spec: &kubernetes.ControllerServiceSpec{
				ServiceType: core.ServiceTypeClusterIP,
			},
		},
		{
			cloudType: "openstack",
			spec: &kubernetes.ControllerServiceSpec{
				ServiceType: core.ServiceTypeLoadBalancer,
			},
		},
		{
			cloudType: "maas",
			spec: &kubernetes.ControllerServiceSpec{
				ServiceType: core.ServiceTypeLoadBalancer,
			},
		},
		{
			cloudType: "lxd",
			spec: &kubernetes.ControllerServiceSpec{
				ServiceType: core.ServiceTypeClusterIP,
			},
		},
		{
			cloudType: "unknown-cloud",
			spec: &kubernetes.ControllerServiceSpec{
				ServiceType: core.ServiceTypeClusterIP,
			},
		},
		{
			cloudType: "microk8s",
			spec: &kubernetes.ControllerServiceSpec{
				ServiceType: core.ServiceTypeLoadBalancer,
				ExternalIP:  "1.1.1.1",
				ExternalIPs: []string{"1.1.1.1"},
			},
			cfg: getCfg("", "loadbalancer", []string{"1.1.1.1"}),
		},
		{
			cloudType: "microk8s",
			errStr:    `external name "example.com" provided but service type was set to "LoadBalancer"`,
			cfg:       getCfg("example.com", "loadbalancer", []string{"1.1.1.1"}),
		},
		{
			cloudType: "microk8s",
			spec: &kubernetes.ControllerServiceSpec{
				ServiceType:  core.ServiceTypeExternalName,
				ExternalName: "example.com",
				ExternalIPs:  []string{"1.1.1.1"},
			},
			cfg: getCfg("example.com", "external", []string{"1.1.1.1"}),
		},
		{
			cloudType: "microk8s",
			spec: &kubernetes.ControllerServiceSpec{
				ServiceType:  core.ServiceTypeExternalName,
				ExternalName: "example.com",
			},
			cfg: getCfg("example.com", "external", nil),
		},
	} {
		c.Logf("testing %d %q", i, t.cloudType)
		spec, err := s.controllerStackerGetter().GetControllerSvcSpec(t.cloudType, t.cfg)
		if len(t.errStr) == 0 {
			c.Check(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.ErrorMatches, t.errStr)
		}
		c.Check(spec, tc.DeepEquals, t.spec)
	}
}

func int64Ptr(a int64) *int64 {
	return &a
}

func (s *bootstrapSuite) TestBootstrap(c *tc.C) {
	podWatcher, podFirer := k8swatchertest.NewKubernetesTestWatcher()
	eventWatcher, _ := k8swatchertest.NewKubernetesTestWatcher()
	<-podWatcher.Changes()
	<-eventWatcher.Changes()
	watchers := []k8swatcher.KubernetesNotifyWatcher{podWatcher, eventWatcher}
	watchCallCount := 0

	s.k8sWatcherFn = func(_ cache.SharedIndexInformer, n string, _ jujuclock.Clock) (k8swatcher.KubernetesNotifyWatcher, error) {
		if watchCallCount >= len(watchers) {
			return nil, errors.NotFoundf("no watcher available for index %d", watchCallCount)
		}
		w := watchers[watchCallCount]
		watchCallCount++
		return w, nil
	}

	// Eventually the namespace wil be set to controllerName.
	// So we have to specify the final namespace(controllerName) for later use.
	newK8sClientFunc, newK8sRestClientFunc := s.setupK8sRestClient(c, s.pcfg.ControllerName)
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}
	_, err := s.mockNamespaces.Get(c.Context(), s.namespace, v1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)

	var bootstrapWatchers []k8swatcher.KubernetesNotifyWatcher
	s.setupBroker(c, newK8sClientFunc, newK8sRestClientFunc, randomPrefixFunc, &bootstrapWatchers)

	// Broker's namespace is "controller" now - controllerModelConfig.Name()
	c.Assert(s.broker.Namespace(), tc.DeepEquals, s.namespace)
	c.Assert(
		s.broker.GetAnnotations().ToMap(), tc.DeepEquals,
		map[string]string{
			"model.juju.is/id":      s.cfg.UUID(),
			"controller.juju.is/id": coretesting.ControllerTag.Id(),
		},
	)

	// Done in broker.Bootstrap method actually.
	s.broker.GetAnnotations().Add("controller.juju.is/is-controller", "true")

	s.pcfg.Bootstrap.Timeout = 10 * time.Minute
	s.pcfg.Bootstrap.ControllerExternalIPs = []string{"10.0.0.1"}
	s.pcfg.Bootstrap.IgnoreProxy = true

	controllerStacker := s.controllerStackerGetter()

	sharedSecret, sslKey := controllerStacker.GetSharedSecretAndSSLKey(c)

	scName := "some-storage"
	sc := k8sstorage.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: scName,
		},
	}

	apiPort := s.controllerCfg.APIPort()
	sshServerPort := s.controllerCfg.SSHServerPort()
	ns := &core.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name:   s.namespace,
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "model.juju.is/name": "controller-1"},
		},
	}
	ns.Name = s.namespace
	s.ensureJujuNamespaceAnnotations(true, ns)
	svcNotFullyProvisioned := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-controller-test-service",
			Namespace:   s.namespace,
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "juju-controller-test"},
			Annotations: map[string]string{"controller.juju.is/id": coretesting.ControllerTag.Id()},
		},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "juju-controller-test"},
			Type:     core.ServiceType("ClusterIP"),
			Ports: []core.ServicePort{
				{
					Name:       "api-server",
					TargetPort: intstr.FromInt(apiPort),
					Port:       int32(apiPort),
				},
				{
					Name:       "ssh-server",
					TargetPort: intstr.FromInt(sshServerPort),
					Port:       int32(sshServerPort),
				},
			},
			ExternalIPs: []string{"10.0.0.1"},
		},
	}

	svcPublicIP := "1.1.1.1"
	svcProvisioned := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-controller-test-service",
			Namespace:   s.namespace,
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "juju-controller-test"},
			Annotations: map[string]string{"controller.juju.is/id": coretesting.ControllerTag.Id()},
		},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "juju-controller-test"},
			Type:     core.ServiceType("ClusterIP"),
			Ports: []core.ServicePort{
				{
					Name:       "api-server",
					TargetPort: intstr.FromInt(apiPort),
					Port:       int32(apiPort),
				},
				{
					Name:       "ssh-server",
					TargetPort: intstr.FromInt(sshServerPort),
					Port:       int32(sshServerPort),
				},
			},
			ClusterIP:   svcPublicIP,
			ExternalIPs: []string{"10.0.0.1"},
		},
	}

	secretWithServerPEMAdded := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-controller-test-secret",
			Namespace:   s.namespace,
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "juju-controller-test"},
			Annotations: map[string]string{"controller.juju.is/id": coretesting.ControllerTag.Id()},
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"shared-secret": []byte(sharedSecret),
			"server.pem":    []byte(sslKey),
		},
	}

	secretControllerAppConfig := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-controller-test-application-config",
			Namespace:   s.namespace,
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "juju-controller-test"},
			Annotations: map[string]string{"controller.juju.is/id": coretesting.ControllerTag.Id()},
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"JUJU_K8S_UNIT_PASSWORD": []byte(controllerStacker.GetControllerUnitAgentPassword()),
		},
	}

	repoDetails, err := docker.NewImageRepoDetails(s.controllerCfg.CAASImageRepo())
	c.Assert(err, tc.ErrorIsNil)
	secretCAASImageRepoData, err := repoDetails.SecretData()
	c.Assert(err, tc.ErrorIsNil)

	secretCAASImageRepo := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-image-pull-secret",
			Namespace:   s.namespace,
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "juju-controller-test"},
			Annotations: map[string]string{"controller.juju.is/id": coretesting.ControllerTag.Id()},
		},
		Type: core.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			core.DockerConfigJsonKey: secretCAASImageRepoData,
		},
	}

	bootstrapParamsContent, err := s.pcfg.Bootstrap.StateInitializationParams.Marshal()
	c.Assert(err, tc.ErrorIsNil)

	configMapWithAgentConfAdded := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-controller-test-configmap",
			Namespace:   s.namespace,
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "juju-controller-test"},
			Annotations: map[string]string{"controller.juju.is/id": coretesting.ControllerTag.Id()},
		},
		Data: map[string]string{
			"bootstrap-params":           string(bootstrapParamsContent),
			"controller-agent.conf":      controllerStacker.GetControllerAgentConfigContent(c),
			"controller-unit-agent.conf": controllerStacker.GetControllerUnitAgentConfigContent(c),
		},
	}

	numberOfPods := int32(1)
	statefulSetSpec := &apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test",
			Namespace: s.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":  "juju",
				"app.kubernetes.io/name":        "juju-controller-test",
				"model.juju.is/disable-webhook": "true",
			},
			Annotations: map[string]string{"controller.juju.is/id": coretesting.ControllerTag.Id()},
		},
		Spec: apps.StatefulSetSpec{
			ServiceName: "juju-controller-test-service",
			Replicas:    &numberOfPods,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "juju-controller-test"},
			},
			VolumeClaimTemplates: []core.PersistentVolumeClaim{
				{
					ObjectMeta: v1.ObjectMeta{
						Name:        "storage",
						Labels:      map[string]string{"app.kubernetes.io/managed-by": "juju", "app.kubernetes.io/name": "juju-controller-test"},
						Annotations: map[string]string{"controller.juju.is/id": coretesting.ControllerTag.Id()},
					},
					Spec: core.PersistentVolumeClaimSpec{
						StorageClassName: &scName,
						AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
						Resources: core.VolumeResourceRequirements{
							Requests: core.ResourceList{
								core.ResourceStorage: resource.MustParse("10000Mi"),
							},
						},
					},
				},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Name:      "controller-0",
					Namespace: s.namespace,
					Labels: map[string]string{
						"app.kubernetes.io/name":        "juju-controller-test",
						"model.juju.is/disable-webhook": "true",
					},
					Annotations: map[string]string{"controller.juju.is/id": coretesting.ControllerTag.Id()},
				},
				Spec: core.PodSpec{
					ServiceAccountName:            "controller",
					AutomountServiceAccountToken:  pointer.Bool(true),
					TerminationGracePeriodSeconds: int64Ptr(30),
					SecurityContext: &core.PodSecurityContext{
						SupplementalGroups: []int64{170},
						FSGroup:            pointer.Int64(170),
					},
					Volumes: []core.Volume{
						{
							Name: "charm-data",
							VolumeSource: core.VolumeSource{
								EmptyDir: &core.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "mongo-scratch",
							VolumeSource: core.VolumeSource{
								EmptyDir: &core.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "apiserver-scratch",
							VolumeSource: core.VolumeSource{
								EmptyDir: &core.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "juju-controller-test-server-pem",
							VolumeSource: core.VolumeSource{
								Secret: &core.SecretVolumeSource{
									SecretName:  "juju-controller-test-secret",
									DefaultMode: pointer.Int32(0400),
									Items: []core.KeyToPath{
										{
											Key:  "server.pem",
											Path: "template-server.pem",
										},
									},
								},
							},
						},
						{
							Name: "juju-controller-test-shared-secret",
							VolumeSource: core.VolumeSource{
								Secret: &core.SecretVolumeSource{
									SecretName:  "juju-controller-test-secret",
									DefaultMode: pointer.Int32(0660),
									Items: []core.KeyToPath{
										{
											Key:  "shared-secret",
											Path: "shared-secret",
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
	volAgentConf := core.Volume{
		Name: "juju-controller-test-agent-conf",
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				Items: []core.KeyToPath{
					{
						Key:  "controller-agent.conf",
						Path: "controller-agent.conf",
					}, {
						Key:  "controller-unit-agent.conf",
						Path: "controller-unit-agent.conf",
					},
				},
			},
		},
	}
	volAgentConf.VolumeSource.ConfigMap.Name = "juju-controller-test-configmap"
	volBootstrapParams := core.Volume{
		Name: "juju-controller-test-bootstrap-params",
		VolumeSource: core.VolumeSource{
			ConfigMap: &core.ConfigMapVolumeSource{
				Items: []core.KeyToPath{
					{
						Key:  "bootstrap-params",
						Path: "bootstrap-params",
					},
				},
			},
		},
	}
	volBootstrapParams.VolumeSource.ConfigMap.Name = "juju-controller-test-configmap"
	statefulSetSpec.Spec.Template.Spec.Volumes = append(statefulSetSpec.Spec.Template.Spec.Volumes,
		[]core.Volume{
			volAgentConf, volBootstrapParams,
		}...,
	)

	expectedVersion := jujuversion.Current
	expectedVersion.Build = 666
	probCmds := &core.ExecAction{
		Command: []string{
			"mongo",
			fmt.Sprintf("--port=%d", s.controllerCfg.StatePort()),
			"--eval",
			"db.adminCommand('ping')",
		},
	}
	statefulSetSpec.Spec.Template.Spec.Containers = []core.Container{
		{
			Name:            "charm",
			ImagePullPolicy: core.PullIfNotPresent,
			Image:           "docker.io/jujusolutions/charm-base:ubuntu-24.04",
			WorkingDir:      "/var/lib/juju",
			Command:         []string{"/charm/bin/pebble"},
			Args:            []string{"run", "--http", ":38812", "--verbose"},
			Resources: core.ResourceRequirements{
				Requests: core.ResourceList{
					core.ResourceMemory: resource.MustParse("4000Mi"),
				},
				Limits: core.ResourceList{
					core.ResourceMemory: resource.MustParse("4000Mi"),
				},
			},
			Env: []core.EnvVar{
				{
					Name:  "JUJU_CONTAINER_NAMES",
					Value: "api-server",
				},
				{
					Name:  osenv.JujuFeatureFlagEnvKey,
					Value: "developer-mode",
				},
			},
			SecurityContext: &core.SecurityContext{
				RunAsUser:  int64Ptr(170),
				RunAsGroup: int64Ptr(170),
			},
			VolumeMounts: []core.VolumeMount{
				{
					Name:      "charm-data",
					ReadOnly:  true,
					MountPath: "/charm/bin",
					SubPath:   "charm/bin",
				},
				{
					Name:      "charm-data",
					MountPath: "/charm/containers",
					SubPath:   "charm/containers",
				},
				{
					Name:      "charm-data",
					MountPath: "/var/lib/pebble/default",
					SubPath:   "containeragent/pebble",
				},
				{
					Name:      "charm-data",
					MountPath: "/var/log/juju",
					SubPath:   "containeragent/var/log/juju",
				},
				{
					Name:      "charm-data",
					ReadOnly:  true,
					MountPath: "/usr/bin/juju-introspect",
					SubPath:   "charm/bin/containeragent",
				},
				{
					Name:      "charm-data",
					ReadOnly:  true,
					MountPath: "/etc/profile.d/juju-introspection.sh",
					SubPath:   "containeragent/etc/profile.d/juju-introspection.sh",
				},
				{
					Name:      "charm-data",
					ReadOnly:  true,
					MountPath: "/usr/bin/juju-exec",
					SubPath:   "charm/bin/containeragent",
				},
				{
					Name:      "juju-controller-test-agent-conf",
					MountPath: "/var/lib/juju/template-agent.conf",
					SubPath:   "controller-unit-agent.conf",
				},
				{
					Name:      "storage",
					MountPath: "/var/lib/juju",
				},
			},
		},
		{
			Name:            "mongodb",
			ImagePullPolicy: core.PullIfNotPresent,
			Image:           "docker.io/jujusolutions/juju-db:4.4",
			Command: []string{
				"/bin/sh",
			},
			Args: []string{
				"-c",
				`printf 'args="--dbpath=/var/lib/juju/db --port=1234 --journal --replSet=juju --quiet --oplogSize=1024 --noauth --storageEngine=wiredTiger --bind_ip_all"\nipv6Disabled=$(sysctl net.ipv6.conf.all.disable_ipv6 -n)\nif [ $ipv6Disabled -eq 0 ]; then\n  args="${args} --ipv6"\nfi\nSHARED_SECRET_SRC="/var/lib/juju/shared-secret.temp"\nSHARED_SECRET_DST="/var/lib/juju/shared-secret"\nrm "${SHARED_SECRET_DST}" || true\ncp "${SHARED_SECRET_SRC}" "${SHARED_SECRET_DST}"\nchown 170:170 "${SHARED_SECRET_DST}"\nchmod 600 "${SHARED_SECRET_DST}"\nls -lah "${SHARED_SECRET_DST}"\nwhile [ ! -f "/var/lib/juju/server.pem" ]; do\n  echo "Waiting for /var/lib/juju/server.pem to be created..."\n  sleep 1\ndone\nexec mongod ${args}\n'>/tmp/mongo.sh && chmod a+x /tmp/mongo.sh && exec /tmp/mongo.sh`,
			},
			Ports: []core.ContainerPort{
				{
					Name:          "mongodb",
					ContainerPort: int32(s.controllerCfg.StatePort()),
					Protocol:      "TCP",
				},
			},
			StartupProbe: &core.Probe{
				ProbeHandler: core.ProbeHandler{
					Exec: probCmds,
				},
				FailureThreshold:    120,
				InitialDelaySeconds: 1,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				TimeoutSeconds:      1,
			},
			ReadinessProbe: &core.Probe{
				ProbeHandler: core.ProbeHandler{
					Exec: probCmds,
				},
				FailureThreshold:    3,
				InitialDelaySeconds: 5,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				TimeoutSeconds:      1,
			},
			LivenessProbe: &core.Probe{
				ProbeHandler: core.ProbeHandler{
					Exec: probCmds,
				},
				FailureThreshold:    3,
				InitialDelaySeconds: 30,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				TimeoutSeconds:      5,
			},
			VolumeMounts: []core.VolumeMount{
				{
					Name:      "mongo-scratch",
					ReadOnly:  false,
					MountPath: "/var/log",
					SubPath:   "var/log",
				},
				{
					Name:      "mongo-scratch",
					ReadOnly:  false,
					MountPath: "/tmp",
					SubPath:   "tmp",
				},
				{
					Name:      "storage",
					ReadOnly:  false,
					MountPath: "/var/lib/juju",
					SubPath:   "",
				},
				{
					Name:      "storage",
					ReadOnly:  false,
					MountPath: "/var/lib/juju/db",
					SubPath:   "db",
				},
				{
					Name:      "juju-controller-test-server-pem",
					ReadOnly:  true,
					MountPath: "/var/lib/juju/template-server.pem",
					SubPath:   "template-server.pem",
				},
				{
					Name:      "juju-controller-test-shared-secret",
					ReadOnly:  true,
					MountPath: "/var/lib/juju/shared-secret.temp",
					SubPath:   "shared-secret",
				},
			},
			SecurityContext: &core.SecurityContext{
				RunAsUser:              int64Ptr(170),
				RunAsGroup:             int64Ptr(170),
				ReadOnlyRootFilesystem: pointer.Bool(true),
			},
		},
		{
			Name:            "api-server",
			ImagePullPolicy: core.PullIfNotPresent,
			Image:           "docker.io/jujusolutions/jujud-operator:" + expectedVersion.String(),
			Env: []core.EnvVar{{
				Name:  osenv.JujuFeatureFlagEnvKey,
				Value: "developer-mode",
			}},
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

test -e $JUJU_DATA_DIR/agents/controller-0/agent.conf || JUJU_DEV_FEATURE_FLAGS=developer-mode $JUJU_TOOLS_DIR/jujud bootstrap-state --data-dir $JUJU_DATA_DIR --debug --timeout 10m0s

mkdir -p /var/lib/pebble/default/layers
cat > /var/lib/pebble/default/layers/001-jujud.yaml <<EOF
summary: jujud service
services:
    jujud:
        summary: Juju controller agent
        startup: enabled
        override: replace
        command: $JUJU_TOOLS_DIR/jujud machine --data-dir $JUJU_DATA_DIR --controller-id 0 --log-to-stderr --debug
        environment:
            JUJU_DEV_FEATURE_FLAGS: developer-mode

EOF

exec /opt/pebble run --http :38811 --verbose
`[1:],
			},
			WorkingDir: "/var/lib/juju",
			EnvFrom: []core.EnvFromSource{{
				SecretRef: &core.SecretEnvSource{
					LocalObjectReference: core.LocalObjectReference{
						Name: "juju-controller-test-application-config",
					},
				},
			}},
			VolumeMounts: []core.VolumeMount{
				{
					Name:      "apiserver-scratch",
					MountPath: "/tmp",
					SubPath:   "tmp",
				},
				{
					Name:      "apiserver-scratch",
					MountPath: "/var/lib/pebble",
					SubPath:   "var/lib/pebble",
				},
				{
					Name:      "apiserver-scratch",
					MountPath: "/var/log/juju",
					SubPath:   "var/log/juju",
				},
				{
					Name:      "storage",
					MountPath: "/var/lib/juju",
				},
				{
					Name:      "storage",
					MountPath: "/var/lib/juju/agents/controller-0",
					SubPath:   "agents/controller-0",
				},
				{
					Name:      "juju-controller-test-agent-conf",
					ReadOnly:  true,
					MountPath: "/var/lib/juju/agents/controller-0/template-agent.conf",
					SubPath:   "controller-agent.conf",
				},
				{
					Name:      "juju-controller-test-server-pem",
					ReadOnly:  true,
					MountPath: "/var/lib/juju/template-server.pem",
					SubPath:   "template-server.pem",
				},
				{
					Name:      "juju-controller-test-shared-secret",
					ReadOnly:  true,
					MountPath: "/var/lib/juju/shared-secret",
					SubPath:   "shared-secret",
				},
				{
					Name:      "juju-controller-test-bootstrap-params",
					ReadOnly:  true,
					MountPath: "/var/lib/juju/bootstrap-params",
					SubPath:   "bootstrap-params",
				},
				{
					Name:      "charm-data",
					MountPath: "/charm/container",
					SubPath:   "charm/containers/api-server",
				},
				{
					Name:      "charm-data",
					ReadOnly:  true,
					MountPath: "/etc/profile.d/juju-introspection.sh",
					SubPath:   "containeragent/etc/profile.d/juju-introspection.sh",
				},
				{
					Name:      "charm-data",
					ReadOnly:  true,
					MountPath: "/usr/bin/juju-introspect",
					SubPath:   "charm/bin/containeragent",
				},
				{
					Name:      "charm-data",
					ReadOnly:  true,
					MountPath: "/usr/bin/juju-exec",
					SubPath:   "charm/bin/containeragent",
				},
				{
					Name:      "charm-data",
					ReadOnly:  true,
					MountPath: "/usr/bin/juju-dumplogs",
					SubPath:   "charm/bin/containeragent",
				},
			},
			StartupProbe: &core.Probe{
				ProbeHandler: core.ProbeHandler{
					HTTPGet: &core.HTTPGetAction{
						Path: "/v1/health?level=alive",
						Port: intstr.Parse("38811"),
					},
				},
				InitialDelaySeconds: 3,
				TimeoutSeconds:      3,
				PeriodSeconds:       3,
				SuccessThreshold:    1,
				FailureThreshold:    200,
			},
			LivenessProbe: &core.Probe{
				ProbeHandler: core.ProbeHandler{
					HTTPGet: &core.HTTPGetAction{
						Path: "/v1/health?level=alive",
						Port: intstr.Parse("38811"),
					},
				},
				InitialDelaySeconds: 1,
				TimeoutSeconds:      3,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				FailureThreshold:    2,
			},
			ReadinessProbe: &core.Probe{
				ProbeHandler: core.ProbeHandler{
					HTTPGet: &core.HTTPGetAction{
						Path: "/v1/health?level=ready",
						Port: intstr.Parse("38811"),
					},
				},
				InitialDelaySeconds: 1,
				TimeoutSeconds:      3,
				PeriodSeconds:       5,
				SuccessThreshold:    1,
				FailureThreshold:    2,
			},
			SecurityContext: &core.SecurityContext{
				RunAsUser:              pointer.Int64(170),
				RunAsGroup:             pointer.Int64(170),
				ReadOnlyRootFilesystem: pointer.Bool(true),
			},
		},
	}
	statefulSetSpec.Spec.Template.Spec.InitContainers = []core.Container{{
		Name:            "charm-init",
		ImagePullPolicy: core.PullIfNotPresent,
		Image:           "docker.io/jujusolutions/jujud-operator:" + expectedVersion.String(),
		WorkingDir:      "/var/lib/juju",
		Command:         []string{"/opt/containeragent"},
		Args: []string{
			"init",
			"--containeragent-pebble-dir", "/containeragent/pebble",
			"--charm-modified-version", "0",
			"--data-dir", "/var/lib/juju",
			"--bin-dir", "/charm/bin",
			"--profile-dir", "/containeragent/etc/profile.d",
			"--pebble-identities-file", "/charm/etc/pebble/identities.yaml",
			"--pebble-charm-identity", "170",
			"--controller",
		},
		Env: []core.EnvVar{
			{
				Name:  "JUJU_CONTAINER_NAMES",
				Value: "api-server",
			},
			{
				Name: "JUJU_K8S_POD_NAME",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath: "metadata.name",
					},
				},
			},
			{
				Name: "JUJU_K8S_POD_UUID",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath: "metadata.uid",
					},
				},
			},
		},
		EnvFrom: []core.EnvFromSource{
			{
				SecretRef: &core.SecretEnvSource{
					LocalObjectReference: core.LocalObjectReference{
						Name: "controller-application-config",
					},
				},
			},
		},
		VolumeMounts: []core.VolumeMount{
			{
				Name:      "charm-data",
				MountPath: "/var/lib/juju",
				SubPath:   "var/lib/juju",
			}, {
				Name:      "charm-data",
				MountPath: "/charm/bin",
				SubPath:   "charm/bin",
			}, {
				Name:      "charm-data",
				MountPath: "/charm/containers",
				SubPath:   "charm/containers",
			}, {
				Name:      "charm-data",
				MountPath: "/containeragent/pebble",
				SubPath:   "containeragent/pebble",
			}, {
				Name:      "charm-data",
				MountPath: "/containeragent/etc/profile.d",
				SubPath:   "containeragent/etc/profile.d",
			}, {
				Name:      "charm-data",
				MountPath: "/charm/etc/pebble/",
				SubPath:   "charm/etc/pebble/",
			}, {
				Name:      "juju-controller-test-agent-conf",
				MountPath: "/var/lib/juju/template-agent.conf",
				SubPath:   "controller-unit-agent.conf",
			},
		},
		SecurityContext: &core.SecurityContext{
			RunAsUser:  int64Ptr(170),
			RunAsGroup: int64Ptr(170),
			//TODO: this should be set
			//ReadOnlyRootFilesystem: pointer.Bool(true),
		},
	}}

	controllerServiceAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:      "controller",
			Namespace: "controller-1",
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":  "juju",
				"app.kubernetes.io/name":        "juju-controller-test",
				"model.juju.is/disable-webhook": "true",
			},
			Annotations: map[string]string{"controller.juju.is/id": coretesting.ControllerTag.Id()},
		},
		AutomountServiceAccountToken: pointer.BoolPtr(true),
	}

	controllerServiceAccountPatchedWithSecretCAASImageRepo := &core.ServiceAccount{}
	*controllerServiceAccountPatchedWithSecretCAASImageRepo = *controllerServiceAccount
	controllerServiceAccountPatchedWithSecretCAASImageRepo.ImagePullSecrets = append(
		controllerServiceAccountPatchedWithSecretCAASImageRepo.ImagePullSecrets,
		core.LocalObjectReference{Name: secretCAASImageRepo.Name},
	)

	controllerServiceCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name: "controller-1",
			Labels: map[string]string{
				"model.juju.is/name":    "controller",
				"controller.juju.is/id": "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			},
			Annotations: map[string]string{"controller.juju.is/id": coretesting.ControllerTag.Id()},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "controller",
				Namespace: "controller-1",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
	}

	errChan := make(chan error)
	done := make(chan struct{})
	s.AddCleanup(func(c *tc.C) {
		close(done)
	})

	// Ensure storage class is inplace.
	_, err = s.mockStorageClass.Create(c.Context(), &sc, v1.CreateOptions{})
	c.Assert(err, tc.ErrorIsNil)

	serviceWatcher, err := s.mockServices.Watch(c.Context(), v1.ListOptions{LabelSelector: "app.kubernetes.io/name=juju-controller-test"})
	c.Assert(err, tc.ErrorIsNil)
	defer serviceWatcher.Stop()
	serviceChanges := serviceWatcher.ResultChan()

	statefulsetWatcher, err := s.mockStatefulSets.Watch(c.Context(), v1.ListOptions{LabelSelector: "app.kubernetes.io/name=juju-controller-test"})
	c.Assert(err, tc.ErrorIsNil)
	defer statefulsetWatcher.Stop()
	statefulsetChanges := statefulsetWatcher.ResultChan()

	go func() {
		errChan <- controllerStacker.Deploy(c.Context())
	}()
	go func(clk *testclock.Clock) {
		for {
			select {
			case <-done:
				return
			case <-serviceChanges:
				// Ensure service address is available.
				svc, err := s.mockServices.Get(c.Context(), "juju-controller-test-service", v1.GetOptions{})
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(svc, tc.DeepEquals, svcNotFullyProvisioned)

				svc.Spec.ClusterIP = svcPublicIP
				svc, err = s.mockServices.Update(c.Context(), svc, v1.UpdateOptions{})
				c.Assert(err, tc.ErrorIsNil)
				c.Assert(svc, tc.DeepEquals, svcProvisioned)
				err = clk.WaitAdvance(3*time.Second, coretesting.ShortWait, 1)
				c.Assert(err, tc.ErrorIsNil)
				serviceChanges = nil
			case <-statefulsetChanges:
				// Ensure pod created - the fake client does not automatically create pods for the statefulset.
				podName := s.pcfg.GetPodName()
				ss, err := s.mockStatefulSets.Get(c.Context(), `juju-controller-test`, v1.GetOptions{})
				c.Assert(err, tc.ErrorIsNil)

				mc := tc.NewMultiChecker()
				mc.AddExpr(`_.Spec.Template.Spec.Containers[_].VolumeMounts`,
					tc.UnorderedMatch[[]core.VolumeMount](tc.DeepEquals), tc.ExpectedValue)
				mc.AddExpr(`_.Spec.Template.Spec.InitContainers[_].VolumeMounts`,
					tc.UnorderedMatch[[]core.VolumeMount](tc.DeepEquals), tc.ExpectedValue)
				c.Assert(ss, mc, statefulSetSpec)
				p := &core.Pod{
					ObjectMeta: v1.ObjectMeta{
						Name: podName,
						Labels: map[string]string{
							"app.kubernetes.io/name":        "juju-controller-test",
							"model.juju.is/disable-webhook": "true",
						},
						Annotations: map[string]string{"controller.juju.is/id": coretesting.ControllerTag.Id()},
					},
				}
				p.Spec = ss.Spec.Template.Spec

				pp, err := s.mockPods.Create(c.Context(), p, v1.CreateOptions{})
				c.Assert(err, tc.ErrorIsNil)

				_, err = s.broker.GetPod(c.Context(), podName)
				c.Assert(err, tc.ErrorIsNil)
				podFirer()
				pp.Status.Phase = core.PodRunning
				_, err = s.mockPods.Update(c.Context(), pp, v1.UpdateOptions{})
				c.Assert(err, tc.ErrorIsNil)
				podFirer()
				statefulsetChanges = nil
			}
		}
	}(s.clock)

	select {
	case err := <-errChan:
		c.Assert(err, tc.ErrorIsNil)

		ss, err := s.mockStatefulSets.Get(c.Context(), `juju-controller-test`, v1.GetOptions{})
		c.Assert(err, tc.ErrorIsNil)
		mc := tc.NewMultiChecker()
		mc.AddExpr(`_.Spec.Template.Spec.Containers[_].VolumeMounts`,
			tc.UnorderedMatch[[]core.VolumeMount](tc.DeepEquals), tc.ExpectedValue)
		mc.AddExpr(`_.Spec.Template.Spec.InitContainers[_].VolumeMounts`,
			tc.UnorderedMatch[[]core.VolumeMount](tc.DeepEquals), tc.ExpectedValue)
		c.Assert(ss, mc, statefulSetSpec)

		svc, err := s.mockServices.Get(c.Context(), `juju-controller-test-service`, v1.GetOptions{})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(svc, tc.DeepEquals, svcProvisioned)

		secret, err := s.mockSecrets.Get(c.Context(), "juju-controller-test-secret", v1.GetOptions{})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(secret, tc.DeepEquals, secretWithServerPEMAdded)

		secret, err = s.mockSecrets.Get(c.Context(), "juju-controller-test-application-config", v1.GetOptions{})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(secret, tc.DeepEquals, secretControllerAppConfig)

		configmap, err := s.mockConfigMaps.Get(c.Context(), "juju-controller-test-configmap", v1.GetOptions{})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(configmap, tc.DeepEquals, configMapWithAgentConfAdded)

		crb, err := s.mockClusterRoleBindings.Get(c.Context(), `controller-1`, v1.GetOptions{})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(crb, tc.DeepEquals, controllerServiceCRB)

		c.Assert(bootstrapWatchers, tc.HasLen, 2)
		c.Assert(workertest.CheckKilled(c, bootstrapWatchers[0]), tc.ErrorIsNil)
		c.Assert(workertest.CheckKilled(c, bootstrapWatchers[1]), tc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for deploy return")
	}
}

func (s *bootstrapSuite) TestBootstrapFailedTimeout(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Eventually the namespace wil be set to controllerName.
	// So we have to specify the final namespace(controllerName) for later use.
	newK8sClientFunc, newK8sRestClientFunc := s.setupK8sRestClient(c, s.pcfg.ControllerName)
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}
	_, err := s.mockNamespaces.Get(c.Context(), s.namespace, v1.GetOptions{})
	c.Assert(err, tc.Satisfies, k8serrors.IsNotFound)

	var watchers []k8swatcher.KubernetesNotifyWatcher
	s.setupBroker(c, newK8sClientFunc, newK8sRestClientFunc, randomPrefixFunc, &watchers)

	// Broker's namespace is "controller" now - controllerModelConfig.Name()
	c.Assert(s.broker.Namespace(), tc.DeepEquals, s.namespace)
	c.Assert(
		s.broker.GetAnnotations().ToMap(), tc.DeepEquals,
		map[string]string{
			"model.juju.is/id":      s.cfg.UUID(),
			"controller.juju.is/id": coretesting.ControllerTag.Id(),
		},
	)

	// Done in broker.Bootstrap method actually.
	s.broker.GetAnnotations().Add("controller.juju.is/is-controller", "true")

	s.pcfg.Bootstrap.Timeout = 10 * time.Minute
	s.pcfg.Bootstrap.ControllerExternalIPs = []string{"10.0.0.1"}
	s.pcfg.Bootstrap.IgnoreProxy = true

	controllerStacker := s.controllerStackerGetter()
	mockStdCtx := mocks.NewMockContext(ctrl)

	ns := &core.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name:   s.namespace,
			Labels: map[string]string{"app.kubernetes.io/managed-by": "juju", "model.juju.is/name": "controller-1"},
		},
	}
	ns.Name = s.namespace
	s.ensureJujuNamespaceAnnotations(true, ns)

	ctxDoneChan := make(chan struct{}, 1)

	gomock.InOrder(
		mockStdCtx.EXPECT().Err().Return(nil),
		mockStdCtx.EXPECT().Done().DoAndReturn(func() <-chan struct{} {
			ctxDoneChan <- struct{}{}
			return ctxDoneChan
		}),
		mockStdCtx.EXPECT().Err().Return(context.DeadlineExceeded).MinTimes(1),
	)

	errChan := make(chan error)
	go func() {
		errChan <- controllerStacker.Deploy(mockStdCtx)
	}()

	select {
	case err := <-errChan:
		c.Assert(err, tc.ErrorMatches, `creating service for controller: waiting for controller service address fully provisioned timeout`)
		c.Assert(watchers, tc.HasLen, 0)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for deploy return")
	}
}
