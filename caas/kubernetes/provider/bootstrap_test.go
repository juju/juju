// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3/workertest"
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
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
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
	s.controllerCfg["juju-db-snap-channel"] = "4.0/stable"
	s.controllerCfg[controller.CAASImageRepo] = `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}`[1:]
	pcfg, err := podcfg.NewBootstrapControllerPodConfig(
		s.controllerCfg, controllerName, "bionic", constraints.MustParse("root-disk=10000M mem=4000M"))
	c.Assert(err, jc.ErrorIsNil)

	pcfg.JujuVersion = jujuversion.Current
	pcfg.OfficialBuild = 666
	pcfg.APIInfo = &api.Info{
		Password: "password",
		CACert:   coretesting.CACert,
		ModelTag: coretesting.ModelTag,
	}
	pcfg.Bootstrap.ControllerModelConfig = s.cfg
	pcfg.Bootstrap.BootstrapMachineInstanceId = "instance-id"
	pcfg.Bootstrap.HostedModelConfig = map[string]interface{}{
		"name": "hosted-model",
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

	s.setupBroker(c, coretesting.ControllerTag.Id(), newK8sClientFunc, newK8sRestClientFunc, randomPrefixFunc)

	// Broker's namespace is "controller" now - controllerModelConfig.Name()
	c.Assert(s.broker.GetCurrentNamespace(), jc.DeepEquals, s.namespace)
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
	s.pcfg.Bootstrap.GUI = &tools.GUIArchive{
		URL:     "http://gui-url",
		Version: version.MustParse("6.6.6"),
		SHA256:  "deadbeef",
		Size:    999,
	}
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

	secretCAASImageRepoData, err := s.controllerCfg.CAASImageRepo().SecretData()
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
			"bootstrap-params": string(bootstrapParamsContent),
			"agent.conf":       controllerStacker.GetAgentConfigContent(c),
		},
	}

	numberOfPods := int32(1)
	fileMode := int32(256)
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
						Resources: core.ResourceRequirements{
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
					RestartPolicy:                core.RestartPolicyAlways,
					ServiceAccountName:           "controller",
					AutomountServiceAccountToken: pointer.BoolPtr(true),
					Volumes: []core.Volume{
						{
							Name: "juju-controller-test-server-pem",
							VolumeSource: core.VolumeSource{
								Secret: &core.SecretVolumeSource{
									SecretName:  "juju-controller-test-secret",
									DefaultMode: &fileMode,
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
									DefaultMode: &fileMode,
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
						Key:  "agent.conf",
						Path: "template-agent.conf",
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

	probCmds := &core.ExecAction{
		Command: []string{
			"mongo",
			fmt.Sprintf("--port=%d", s.controllerCfg.StatePort()),
			"--ssl",
			"--sslAllowInvalidHostnames",
			"--sslAllowInvalidCertificates",
			"--sslPEMKeyFile=/var/lib/juju/server.pem",
			"--eval",
			"db.adminCommand('ping')",
		},
	}
	expectedArgs := []string{
		`printf 'args="--dbpath=/var/lib/juju/db --sslPEMKeyFile=/var/lib/juju/server.pem --sslPEMKeyPassword=ignored --sslMode=requireSSL --port=1234 --journal --replSet=juju --quiet --oplogSize=1024 --auth --keyFile=/var/lib/juju/shared-secret --storageEngine=wiredTiger --bind_ip_all"`,
		`ipv6Disabled=$(sysctl net.ipv6.conf.all.disable_ipv6 -n)`,
		`if [ $ipv6Disabled -eq 0 ]; then`,
		`  args="${args} --ipv6"`,
		`fi`,
		`$(mongod ${args})`,
		`'>/root/mongo.sh && chmod a+x /root/mongo.sh && /root/mongo.sh`,
	}
	statefulSetSpec.Spec.Template.Spec.Containers = []core.Container{
		{
			Name:            "mongodb",
			ImagePullPolicy: core.PullIfNotPresent,
			Image:           "test-account/juju-db:4.0",
			Command: []string{
				"/bin/sh",
			},
			Args: []string{
				"-c",
				strings.Join(expectedArgs, "\\n"),
			},
			Ports: []core.ContainerPort{
				{
					Name:          "mongodb",
					ContainerPort: int32(s.controllerCfg.StatePort()),
					Protocol:      "TCP",
				},
			},
			ReadinessProbe: &core.Probe{
				Handler: core.Handler{
					Exec: probCmds,
				},
				FailureThreshold:    3,
				InitialDelaySeconds: 5,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				TimeoutSeconds:      1,
			},
			LivenessProbe: &core.Probe{
				Handler: core.Handler{
					Exec: probCmds,
				},
				FailureThreshold:    3,
				InitialDelaySeconds: 30,
				PeriodSeconds:       10,
				SuccessThreshold:    1,
				TimeoutSeconds:      5,
			},
			Resources: core.ResourceRequirements{
				Limits: core.ResourceList{
					core.ResourceMemory: resource.MustParse("4000Mi"),
				},
			},
			VolumeMounts: []core.VolumeMount{
				{
					Name:      "storage",
					MountPath: "/var/lib/juju",
				},
				{
					Name:      "storage",
					MountPath: "/var/lib/juju/db",
					SubPath:   "db",
				},
				{
					Name:      "juju-controller-test-server-pem",
					MountPath: "/var/lib/juju/template-server.pem",
					SubPath:   "template-server.pem",
					ReadOnly:  true,
				},
				{
					Name:      "juju-controller-test-shared-secret",
					MountPath: "/var/lib/juju/shared-secret",
					SubPath:   "shared-secret",
					ReadOnly:  true,
				},
			},
		},
		{
			Name:            "api-server",
			ImagePullPolicy: core.PullIfNotPresent,
			Image:           "test-account/jujud-operator:" + jujuversion.Current.String() + ".666",
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

echo Installing Dashboard...
export gui='/var/lib/juju/gui'
mkdir -p $gui
curl -sSf -o $gui/gui.tar.bz2 --retry 10 'http://gui-url' || echo Unable to retrieve Juju Dashboard
[ -f $gui/gui.tar.bz2 ] && sha256sum $gui/gui.tar.bz2 > $gui/jujugui.sha256
[ -f $gui/jujugui.sha256 ] && (grep 'deadbeef' $gui/jujugui.sha256 && printf %s '{"version":"6.6.6","url":"http://gui-url","sha256":"deadbeef","size":999}' > $gui/downloaded-gui.txt || echo Juju GUI checksum mismatch)
test -e $JUJU_DATA_DIR/agents/controller-0/agent.conf || JUJU_DEV_FEATURE_FLAGS=developer-mode $JUJU_TOOLS_DIR/jujud bootstrap-state $JUJU_DATA_DIR/bootstrap-params --data-dir $JUJU_DATA_DIR --debug --timeout 10m0s
JUJU_DEV_FEATURE_FLAGS=developer-mode $JUJU_TOOLS_DIR/jujud machine --data-dir $JUJU_DATA_DIR --controller-id 0 --log-to-stderr --debug
`[1:],
			},
			WorkingDir: "/var/lib/juju",
			Resources: core.ResourceRequirements{
				Limits: core.ResourceList{
					core.ResourceMemory: resource.MustParse("4000Mi"),
				},
			},
			VolumeMounts: []core.VolumeMount{
				{
					Name:      "storage",
					MountPath: "/var/lib/juju",
				},
				{
					Name:      "juju-controller-test-agent-conf",
					MountPath: "/var/lib/juju/agents/controller-0/template-agent.conf",
					SubPath:   "template-agent.conf",
				},
				{
					Name:      "juju-controller-test-server-pem",
					MountPath: "/var/lib/juju/template-server.pem",
					SubPath:   "template-server.pem",
					ReadOnly:  true,
				},
				{
					Name:      "juju-controller-test-shared-secret",
					MountPath: "/var/lib/juju/shared-secret",
					SubPath:   "shared-secret",
					ReadOnly:  true,
				},
				{
					Name:      "juju-controller-test-bootstrap-params",
					MountPath: "/var/lib/juju/bootstrap-params",
					SubPath:   "bootstrap-params",
					ReadOnly:  true,
				},
			},
		},
	}

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

		configmap, err := s.mockConfigMaps.Get(context.Background(), "juju-controller-test-configmap", v1.GetOptions{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(configmap, gc.DeepEquals, configMapWithAgentConfAdded)

		crb, err := s.mockClusterRoleBindings.Get(context.Background(), `controller-1`, v1.GetOptions{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(crb, gc.DeepEquals, controllerServiceCRB)

		c.Assert(s.watchers, gc.HasLen, 2)
		c.Assert(workertest.CheckKilled(c, s.watchers[0]), jc.ErrorIsNil)
		c.Assert(workertest.CheckKilled(c, s.watchers[1]), jc.ErrorIsNil)
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

	s.setupBroker(c, coretesting.ControllerTag.Id(), newK8sClientFunc, newK8sRestClientFunc, randomPrefixFunc)

	// Broker's namespace is "controller" now - controllerModelConfig.Name()
	c.Assert(s.broker.GetCurrentNamespace(), jc.DeepEquals, s.namespace)
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
	s.pcfg.Bootstrap.GUI = &tools.GUIArchive{
		URL:     "http://gui-url",
		Version: version.MustParse("6.6.6"),
		SHA256:  "deadbeef",
		Size:    999,
	}
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
		mockStdCtx.EXPECT().Done().Return(ctxDoneChan),
		mockStdCtx.EXPECT().Done().DoAndReturn(func() <-chan struct{} {
			ctxDoneChan <- struct{}{}
			return ctxDoneChan
		}),
		mockStdCtx.EXPECT().Err().Return(context.DeadlineExceeded),
	)

	errChan := make(chan error)
	go func() {
		errChan <- controllerStacker.Deploy()
	}()

	select {
	case err := <-errChan:
		c.Assert(err, gc.ErrorMatches, `creating service for controller: waiting for controller service address fully provisioned timeout`)
		c.Assert(s.watchers, gc.HasLen, 0)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for deploy return")
	}
}
