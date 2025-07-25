// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"
	"fmt"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
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
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/caas/kubernetes/provider/mocks"
	k8swatcher "github.com/juju/juju/caas/kubernetes/provider/watcher"
	k8swatchertest "github.com/juju/juju/caas/kubernetes/provider/watcher/test"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/controller"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type bootstrapSuite struct {
	fakeClientSuite
	coretesting.JujuOSEnvSuite

	controllerCfg controller.Config
	pcfg          *podcfg.ControllerPodConfig

	controllerStackerGetter func() provider.ControllerStackerForTest
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.fakeClientSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
	s.SetFeatureFlags(feature.DeveloperMode)
	s.broker = nil

	controllerName := "controller-1"
	s.namespace = controllerName

	cfg, err := config.New(config.UseDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		config.NameKey:                  "controller-1",
		k8sconstants.OperatorStorageKey: "",
		k8sconstants.WorkloadStorageKey: "",
	}))
	c.Assert(err, jc.ErrorIsNil)
	s.cfg = cfg

	s.controllerCfg = coretesting.FakeControllerConfig()
	s.controllerCfg["juju-db-snap-channel"] = controller.DefaultJujuDBSnapChannel
	s.controllerCfg[controller.CAASImageRepo] = `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}`[1:]
	pcfg, err := podcfg.NewBootstrapControllerPodConfig(
		s.controllerCfg, controllerName, "ubuntu", constraints.MustParse("root-disk=10000M mem=4000M"))
	c.Assert(err, jc.ErrorIsNil)

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
	pcfg.Bootstrap.InitialModelConfig = map[string]interface{}{
		"name": "my-model",
	}
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
	s.controllerStackerGetter = func() provider.ControllerStackerForTest {
		controllerStacker, err := provider.NewcontrollerStackForTest(
			envtesting.BootstrapContext(context.TODO(), c), "juju-controller-test", "some-storage", s.broker, s.pcfg,
		)
		c.Assert(err, jc.ErrorIsNil)
		return controllerStacker
	}
}

func (s *bootstrapSuite) TearDownTest(c *gc.C) {
	s.pcfg = nil
	s.controllerCfg = nil
	s.controllerStackerGetter = nil
	s.fakeClientSuite.TearDownTest(c)
	s.JujuOSEnvSuite.TearDownTest(c)
}

func (s *bootstrapSuite) TestFindControllerNamespace(c *gc.C) {
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
			context.TODO(),
			&test.Namespace,
			v1.CreateOptions{},
		)
		c.Assert(err, jc.ErrorIsNil)
		ns, err := provider.FindControllerNamespace(
			client, test.ControllerUUID)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ns, jc.DeepEquals, &test.Namespace)
	}
}

type svcSpecTC struct {
	cloudType string
	spec      *provider.ControllerServiceSpec
	errStr    string
	cfg       *podcfg.BootstrapConfig
}

func (s *bootstrapSuite) TestGetControllerSvcSpec(c *gc.C) {
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
			spec: &provider.ControllerServiceSpec{
				ServiceType: core.ServiceTypeLoadBalancer,
			},
		},
		{
			cloudType: "ec2",
			spec: &provider.ControllerServiceSpec{
				ServiceType: core.ServiceTypeLoadBalancer,
				Annotations: k8sannotations.New(nil).
					Add("service.beta.kubernetes.io/aws-load-balancer-backend-protocol", "tcp"),
			},
		},
		{
			cloudType: "gce",
			spec: &provider.ControllerServiceSpec{
				ServiceType: core.ServiceTypeLoadBalancer,
			},
		},
		{
			cloudType: "microk8s",
			spec: &provider.ControllerServiceSpec{
				ServiceType: core.ServiceTypeClusterIP,
			},
		},
		{
			cloudType: "openstack",
			spec: &provider.ControllerServiceSpec{
				ServiceType: core.ServiceTypeLoadBalancer,
			},
		},
		{
			cloudType: "maas",
			spec: &provider.ControllerServiceSpec{
				ServiceType: core.ServiceTypeLoadBalancer,
			},
		},
		{
			cloudType: "lxd",
			spec: &provider.ControllerServiceSpec{
				ServiceType: core.ServiceTypeClusterIP,
			},
		},
		{
			cloudType: "unknown-cloud",
			spec: &provider.ControllerServiceSpec{
				ServiceType: core.ServiceTypeClusterIP,
			},
		},
		{
			cloudType: "microk8s",
			spec: &provider.ControllerServiceSpec{
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
			spec: &provider.ControllerServiceSpec{
				ServiceType:  core.ServiceTypeExternalName,
				ExternalName: "example.com",
				ExternalIPs:  []string{"1.1.1.1"},
			},
			cfg: getCfg("example.com", "external", []string{"1.1.1.1"}),
		},
		{
			cloudType: "microk8s",
			spec: &provider.ControllerServiceSpec{
				ServiceType:  core.ServiceTypeExternalName,
				ExternalName: "example.com",
			},
			cfg: getCfg("example.com", "external", nil),
		},
	} {
		c.Logf("testing %d %q", i, t.cloudType)
		spec, err := s.controllerStackerGetter().GetControllerSvcSpec(t.cloudType, t.cfg)
		if len(t.errStr) == 0 {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, t.errStr)
		}
		c.Check(spec, jc.DeepEquals, t.spec)
	}
}

func int64Ptr(a int64) *int64 {
	return &a
}

func (s *bootstrapSuite) TestBootstrap(c *gc.C) {
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
	_, err := s.mockNamespaces.Get(context.TODO(), s.namespace, v1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)

	var bootstrapWatchers []k8swatcher.KubernetesNotifyWatcher
	s.setupBroker(c, newK8sClientFunc, newK8sRestClientFunc, randomPrefixFunc, &bootstrapWatchers)

	// Broker's namespace is "controller" now - controllerModelConfig.Name()
	c.Assert(s.broker.Namespace(), jc.DeepEquals, s.namespace)
	c.Assert(
		s.broker.GetAnnotations().ToMap(), jc.DeepEquals,
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

	APIPort := s.controllerCfg.APIPort()
	SSHServerPort := s.controllerCfg.SSHServerPort()
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
					TargetPort: intstr.FromInt(APIPort),
					Port:       int32(APIPort),
				},
				{
					Name:       "ssh-server",
					TargetPort: intstr.FromInt(SSHServerPort),
					Port:       int32(SSHServerPort),
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
					TargetPort: intstr.FromInt(APIPort),
					Port:       int32(APIPort),
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
	c.Assert(err, jc.ErrorIsNil)
	secretCAASImageRepoData, err := repoDetails.SecretData()
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)

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
			"--tls",
			"--tlsAllowInvalidHostnames",
			"--tlsAllowInvalidCertificates",
			"--tlsCertificateKeyFile=/var/lib/juju/server.pem",
			"--eval",
			"db.adminCommand('ping')",
		},
	}
	statefulSetSpec.Spec.Template.Spec.Containers = []core.Container{
		{
			Name:            "charm",
			ImagePullPolicy: core.PullIfNotPresent,
			Image:           "ghcr.io/juju/charm-base:ubuntu-22.04",
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
			Image:           "test-account/juju-db:4.4",
			Command: []string{
				"/bin/sh",
			},
			Args: []string{
				"-c",
				`printf 'args="--dbpath=/var/lib/juju/db --tlsCertificateKeyFile=/var/lib/juju/server.pem --tlsCertificateKeyFilePassword=ignored --tlsMode=requireTLS --port=1234 --journal --replSet=juju --quiet --oplogSize=1024 --auth --keyFile=/var/lib/juju/shared-secret --storageEngine=wiredTiger --bind_ip_all"\nipv6Disabled=$(sysctl net.ipv6.conf.all.disable_ipv6 -n)\nif [ $ipv6Disabled -eq 0 ]; then\n  args="${args} --ipv6"\nfi\nSHARED_SECRET_SRC="/var/lib/juju/shared-secret.temp"\nSHARED_SECRET_DST="/var/lib/juju/shared-secret"\nrm "${SHARED_SECRET_DST}" || true\ncp "${SHARED_SECRET_SRC}" "${SHARED_SECRET_DST}"\nchown 170:170 "${SHARED_SECRET_DST}"\nchmod 600 "${SHARED_SECRET_DST}"\nls -lah "${SHARED_SECRET_DST}"\nwhile [ ! -f "/var/lib/juju/server.pem" ]; do\n  echo "Waiting for /var/lib/juju/server.pem to be created..."\n  sleep 1\ndone\nexec mongod ${args}\n'>/tmp/mongo.sh && chmod a+x /tmp/mongo.sh && exec /tmp/mongo.sh`,
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
				FailureThreshold:    60,
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
			Image:           "test-account/jujud-operator:" + expectedVersion.String(),
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

test -e $JUJU_DATA_DIR/agents/controller-0/agent.conf || JUJU_DEV_FEATURE_FLAGS=developer-mode $JUJU_TOOLS_DIR/jujud bootstrap-state $JUJU_DATA_DIR/bootstrap-params --data-dir $JUJU_DATA_DIR --debug --timeout 10m0s

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
				FailureThreshold:    100,
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
		Image:           "test-account/jujud-operator:" + expectedVersion.String(),
		WorkingDir:      "/var/lib/juju",
		Command:         []string{"/opt/containeragent"},
		Args:            []string{"init", "--containeragent-pebble-dir", "/containeragent/pebble", "--charm-modified-version", "0", "--data-dir", "/var/lib/juju", "--bin-dir", "/charm/bin", "--profile-dir", "/containeragent/etc/profile.d", "--controller"},
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
				Name:      "juju-controller-test-agent-conf",
				MountPath: "/var/lib/juju/template-agent.conf",
				SubPath:   "controller-unit-agent.conf",
			},
		},
		SecurityContext: &core.SecurityContext{
			RunAsUser:              int64Ptr(170),
			RunAsGroup:             int64Ptr(170),
			ReadOnlyRootFilesystem: pointer.Bool(true),
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
				"model.juju.is/name": "controller",
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
	s.AddCleanup(func(c *gc.C) {
		close(done)
	})

	// Ensure storage class is inplace.
	_, err = s.mockStorageClass.Create(context.Background(), &sc, v1.CreateOptions{})
	c.Assert(err, jc.ErrorIsNil)

	serviceWatcher, err := s.mockServices.Watch(context.Background(), v1.ListOptions{LabelSelector: "app.kubernetes.io/name=juju-controller-test"})
	c.Assert(err, jc.ErrorIsNil)
	defer serviceWatcher.Stop()
	serviceChanges := serviceWatcher.ResultChan()

	statefulsetWatcher, err := s.mockStatefulSets.Watch(context.Background(), v1.ListOptions{LabelSelector: "app.kubernetes.io/name=juju-controller-test"})
	c.Assert(err, jc.ErrorIsNil)
	defer statefulsetWatcher.Stop()
	statefulsetChanges := statefulsetWatcher.ResultChan()

	go func() {
		errChan <- controllerStacker.Deploy()
	}()
	go func(clk *testclock.Clock) {
		for {
			select {
			case <-done:
				return
			case <-serviceChanges:
				// Ensure service address is available.
				svc, err := s.mockServices.Get(context.Background(), "juju-controller-test-service", v1.GetOptions{})
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(svc, gc.DeepEquals, svcNotFullyProvisioned)

				svc.Spec.ClusterIP = svcPublicIP
				svc, err = s.mockServices.Update(context.Background(), svc, v1.UpdateOptions{})
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(svc, gc.DeepEquals, svcProvisioned)
				err = clk.WaitAdvance(3*time.Second, coretesting.ShortWait, 1)
				c.Assert(err, jc.ErrorIsNil)
				serviceChanges = nil
			case <-statefulsetChanges:
				// Ensure pod created - the fake client does not automatically create pods for the statefulset.
				podName := s.pcfg.GetPodName()
				ss, err := s.mockStatefulSets.Get(context.Background(), `juju-controller-test`, v1.GetOptions{})
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(ss, gc.DeepEquals, statefulSetSpec)
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

				pp, err := s.mockPods.Create(context.Background(), p, v1.CreateOptions{})
				c.Assert(err, jc.ErrorIsNil)

				_, err = s.broker.GetPod(podName)
				c.Assert(err, jc.ErrorIsNil)
				podFirer()
				pp.Status.Phase = core.PodRunning
				_, err = s.mockPods.Update(context.Background(), pp, v1.UpdateOptions{})
				c.Assert(err, jc.ErrorIsNil)
				podFirer()
				statefulsetChanges = nil
			}
		}
	}(s.clock)

	select {
	case err := <-errChan:
		c.Assert(err, jc.ErrorIsNil)

		ss, err := s.mockStatefulSets.Get(context.Background(), `juju-controller-test`, v1.GetOptions{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ss, gc.DeepEquals, statefulSetSpec)

		svc, err := s.mockServices.Get(context.Background(), `juju-controller-test-service`, v1.GetOptions{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(svc, gc.DeepEquals, svcProvisioned)

		secret, err := s.mockSecrets.Get(context.Background(), "juju-controller-test-secret", v1.GetOptions{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(secret, gc.DeepEquals, secretWithServerPEMAdded)

		secret, err = s.mockSecrets.Get(context.Background(), "juju-image-pull-secret", v1.GetOptions{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(secret, gc.DeepEquals, secretCAASImageRepo)

		secret, err = s.mockSecrets.Get(context.Background(), "juju-controller-test-application-config", v1.GetOptions{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(secret, gc.DeepEquals, secretControllerAppConfig)

		configmap, err := s.mockConfigMaps.Get(context.Background(), "juju-controller-test-configmap", v1.GetOptions{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(configmap, gc.DeepEquals, configMapWithAgentConfAdded)

		crb, err := s.mockClusterRoleBindings.Get(context.Background(), `controller-1`, v1.GetOptions{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(crb, gc.DeepEquals, controllerServiceCRB)

		c.Assert(bootstrapWatchers, gc.HasLen, 2)
		c.Assert(workertest.CheckKilled(c, bootstrapWatchers[0]), jc.ErrorIsNil)
		c.Assert(workertest.CheckKilled(c, bootstrapWatchers[1]), jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for deploy return")
	}
}

func (s *bootstrapSuite) TestBootstrapFailedTimeout(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Eventually the namespace wil be set to controllerName.
	// So we have to specify the final namespace(controllerName) for later use.
	newK8sClientFunc, newK8sRestClientFunc := s.setupK8sRestClient(c, s.pcfg.ControllerName)
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}
	_, err := s.mockNamespaces.Get(context.TODO(), s.namespace, v1.GetOptions{})
	c.Assert(err, jc.Satisfies, k8serrors.IsNotFound)

	var watchers []k8swatcher.KubernetesNotifyWatcher
	s.setupBroker(c, newK8sClientFunc, newK8sRestClientFunc, randomPrefixFunc, &watchers)

	// Broker's namespace is "controller" now - controllerModelConfig.Name()
	c.Assert(s.broker.Namespace(), jc.DeepEquals, s.namespace)
	c.Assert(
		s.broker.GetAnnotations().ToMap(), jc.DeepEquals,
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
	ctx := modelcmd.BootstrapContext(mockStdCtx, cmdtesting.Context(c))
	controllerStacker.SetContext(ctx)

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
		errChan <- controllerStacker.Deploy()
	}()

	select {
	case err := <-errChan:
		c.Assert(err, gc.ErrorMatches, `creating service for controller: waiting for controller service address fully provisioned timeout`)
		c.Assert(watchers, gc.HasLen, 0)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for deploy return")
	}
}
