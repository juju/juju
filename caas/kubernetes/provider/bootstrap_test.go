// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8sstorage "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"

	"github.com/juju/juju/api"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/controller"
	k8sannotations "github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type bootstrapSuite struct {
	BaseSuite

	controllerCfg controller.Config
	pcfg          *podcfg.ControllerPodConfig

	controllerStackerGetter func() provider.ControllerStackerForTest
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {

	controllerName := "controller-1"

	s.BaseSuite.SetUpTest(c)

	cfg, err := config.New(config.UseDefaults, testing.FakeConfig().Merge(testing.Attrs{
		config.NameKey:              "controller",
		provider.OperatorStorageKey: "",
		provider.WorkloadStorageKey: "",
	}))
	c.Assert(err, jc.ErrorIsNil)
	s.cfg = cfg

	s.controllerCfg = testing.FakeControllerConfig()
	pcfg, err := podcfg.NewBootstrapControllerPodConfig(
		s.controllerCfg, controllerName, "bionic", constraints.MustParse("root-disk=10000M mem=4000M"))
	c.Assert(err, jc.ErrorIsNil)

	pcfg.JujuVersion = jujuversion.Current
	pcfg.OfficialBuild = 666
	pcfg.APIInfo = &api.Info{
		Password: "password",
		CACert:   testing.CACert,
		ModelTag: testing.ModelTag,
	}
	pcfg.Controller.MongoInfo = &mongo.MongoInfo{
		Password: "password", Info: mongo.Info{CACert: testing.CACert},
	}
	pcfg.Bootstrap.ControllerModelConfig = s.cfg
	pcfg.Bootstrap.BootstrapMachineInstanceId = "instance-id"
	pcfg.Bootstrap.HostedModelConfig = map[string]interface{}{
		"name": "hosted-model",
	}
	pcfg.Bootstrap.StateServingInfo = controller.StateServingInfo{
		Cert:         testing.ServerCert,
		PrivateKey:   testing.ServerKey,
		CAPrivateKey: testing.CAKey,
		StatePort:    123,
		APIPort:      456,
	}
	pcfg.Bootstrap.StateServingInfo = controller.StateServingInfo{
		Cert:         testing.ServerCert,
		PrivateKey:   testing.ServerKey,
		CAPrivateKey: testing.CAKey,
		StatePort:    123,
		APIPort:      456,
	}
	pcfg.Bootstrap.ControllerConfig = s.controllerCfg
	s.pcfg = pcfg
	s.controllerStackerGetter = func() provider.ControllerStackerForTest {
		controllerStacker, err := provider.NewcontrollerStackForTest(
			envtesting.BootstrapContext(c), "juju-controller-test", "some-storage", s.broker, s.pcfg,
		)
		c.Assert(err, jc.ErrorIsNil)
		return controllerStacker
	}
}

func (s *bootstrapSuite) TestControllerCorelation(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	existingNs := core.Namespace{}
	existingNs.SetName("controller-1")
	existingNs.SetAnnotations(map[string]string{
		"juju.io/model":         s.cfg.UUID(),
		"juju.io/controller":    testing.ControllerTag.Id(),
		"juju.io/is-controller": "true",
	})

	c.Assert(s.broker.GetCurrentNamespace(), jc.DeepEquals, s.getNamespace())
	c.Assert(s.broker.GetAnnotations().ToMap(), jc.DeepEquals, map[string]string{
		"juju.io/model":      s.cfg.UUID(),
		"juju.io/controller": testing.ControllerTag.Id(),
	})

	gomock.InOrder(
		s.mockNamespaces.EXPECT().List(v1.ListOptions{}).
			Return(&core.NamespaceList{Items: []core.Namespace{existingNs}}, nil),
	)
	ns, err := provider.ControllerCorelation(s.broker)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(
		// "is-controller" is set as well.
		s.broker.GetAnnotations().ToMap(), jc.DeepEquals,
		map[string]string{
			"juju.io/model":         s.cfg.UUID(),
			"juju.io/controller":    testing.ControllerTag.Id(),
			"juju.io/is-controller": "true",
		},
	)
	// controller namespace linked back(changed from 'controller' to 'controller-1')
	c.Assert(ns, jc.DeepEquals, "controller-1")
}

func (s *bootstrapSuite) TestGetControllerSvcSpec(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	for cloudType, out := range map[string]*provider.ControllerServiceSpec{
		"azure": {
			ServiceType: core.ServiceTypeLoadBalancer,
		},
		"ec2": {
			ServiceType: core.ServiceTypeLoadBalancer,
			Annotations: k8sannotations.New(nil).
				Add("service.beta.kubernetes.io/aws-load-balancer-backend-protocol", "tcp"),
		},
		"gce": {
			ServiceType: core.ServiceTypeLoadBalancer,
		},
		"microk8s": {
			ServiceType: core.ServiceTypeClusterIP,
		},
		"openstack": {
			ServiceType: core.ServiceTypeLoadBalancer,
		},
		"maas": {
			ServiceType: core.ServiceTypeLoadBalancer,
		},
		"lxd": {
			ServiceType: core.ServiceTypeClusterIP,
		},
		"unknown-cloud": {
			ServiceType: core.ServiceTypeClusterIP,
		},
	} {
		spec, _ := s.controllerStackerGetter().GetControllerSvcSpec(cloudType, nil)
		c.Check(spec, jc.DeepEquals, out)
	}
}

func (s *bootstrapSuite) TestBootstrap(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	// Eventually the namespace wil be set to controllerName.
	// So we have to specify the final namespace(controllerName) for later use.
	newK8sClientFunc, newK8sRestClientFunc := s.setupK8sRestClient(c, ctrl, s.pcfg.ControllerName)
	randomPrefixFunc := func() (string, error) {
		return "appuuid", nil
	}
	s.namespace = "controller-1"
	s.setupBroker(c, ctrl, newK8sClientFunc, newK8sRestClientFunc, randomPrefixFunc)

	// Broker's namespace is "controller" now - controllerModelConfig.Name()
	c.Assert(s.broker.GetCurrentNamespace(), jc.DeepEquals, s.getNamespace())
	c.Assert(
		s.broker.GetAnnotations().ToMap(), jc.DeepEquals,
		map[string]string{
			"juju.io/model":      s.cfg.UUID(),
			"juju.io/controller": testing.ControllerTag.Id(),
		},
	)

	// Done in broker.Bootstrap method actually.
	s.broker.GetAnnotations().Add("juju.io/is-controller", "true")

	s.pcfg.Bootstrap.Timeout = 10 * time.Minute
	s.pcfg.Bootstrap.ControllerExternalIPs = []string{"10.0.0.1"}

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
			Name:   s.getNamespace(),
			Labels: provider.LabelsForModel("controller-1"),
		},
	}
	ns.Name = s.getNamespace()
	s.ensureJujuNamespaceAnnotations(true, ns)
	svcNotProvisioned := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-controller-test-service",
			Namespace:   s.getNamespace(),
			Labels:      map[string]string{"juju-app": "juju-controller-test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
		},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"juju-app": "juju-controller-test"},
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
			Namespace:   s.getNamespace(),
			Labels:      map[string]string{"juju-app": "juju-controller-test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
		},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"juju-app": "juju-controller-test"},
			Type:     core.ServiceType("LoadBalancer"),
			Ports: []core.ServicePort{
				{
					Name:       "api-server",
					TargetPort: intstr.FromInt(APIPort),
					Port:       int32(APIPort),
				},
			},
			LoadBalancerIP: svcPublicIP,
		},
	}

	emptySecret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-controller-test-secret",
			Namespace:   s.getNamespace(),
			Labels:      map[string]string{"juju-app": "juju-controller-test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
		},
		Type: core.SecretTypeOpaque,
	}
	secretWithSharedSecretAdded := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-controller-test-secret",
			Namespace:   s.getNamespace(),
			Labels:      map[string]string{"juju-app": "juju-controller-test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"shared-secret": []byte(sharedSecret),
		},
	}
	secretWithServerPEMAdded := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-controller-test-secret",
			Namespace:   s.getNamespace(),
			Labels:      map[string]string{"juju-app": "juju-controller-test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"shared-secret": []byte(sharedSecret),
			"server.pem":    []byte(sslKey),
		},
	}

	emptyConfigMap := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-controller-test-configmap",
			Namespace:   s.getNamespace(),
			Labels:      map[string]string{"juju-app": "juju-controller-test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
		},
	}
	bootstrapParamsContent, err := s.pcfg.Bootstrap.StateInitializationParams.Marshal()
	c.Assert(err, jc.ErrorIsNil)

	configMapWithBootstrapParamsAdded := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-controller-test-configmap",
			Namespace:   s.getNamespace(),
			Labels:      map[string]string{"juju-app": "juju-controller-test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
		},
		Data: map[string]string{
			"bootstrap-params": string(bootstrapParamsContent),
		},
	}
	configMapWithAgentConfAdded := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:        "juju-controller-test-configmap",
			Namespace:   s.getNamespace(),
			Labels:      map[string]string{"juju-app": "juju-controller-test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
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
			Name:        "juju-controller-test",
			Namespace:   s.getNamespace(),
			Labels:      map[string]string{"juju-app": "juju-controller-test"},
			Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
		},
		Spec: apps.StatefulSetSpec{
			ServiceName: "juju-controller-test-service",
			Replicas:    &numberOfPods,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "juju-controller-test"},
			},
			VolumeClaimTemplates: []core.PersistentVolumeClaim{
				{
					ObjectMeta: v1.ObjectMeta{
						Name:        "storage",
						Labels:      map[string]string{"juju-app": "juju-controller-test"},
						Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
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
					Name:        "controller-0",
					Namespace:   s.getNamespace(),
					Labels:      map[string]string{"juju-app": "juju-controller-test"},
					Annotations: map[string]string{"juju.io/controller": testing.ControllerTag.Id()},
				},
				Spec: core.PodSpec{
					RestartPolicy: core.RestartPolicyAlways,
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
	statefulSetSpec.Spec.Template.Spec.Containers = []core.Container{
		{
			Name:            "mongodb",
			ImagePullPolicy: core.PullIfNotPresent,
			Image:           "jujusolutions/juju-db:4.0",
			Command: []string{
				"mongod",
			},
			Args: []string{
				"--dbpath=/var/lib/juju/db",
				"--sslPEMKeyFile=/var/lib/juju/server.pem",
				"--sslPEMKeyPassword=ignored",
				"--sslMode=requireSSL",
				fmt.Sprintf("--port=%d", s.controllerCfg.StatePort()),
				"--journal",
				"--replSet=juju",
				"--quiet",
				"--oplogSize=1024",
				"--ipv6",
				"--auth",
				"--keyFile=/var/lib/juju/shared-secret",
				"--storageEngine=wiredTiger",
				"--bind_ip_all",
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
			Image:           "jujusolutions/jujud-operator:" + jujuversion.Current.String() + ".666",
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

test -e $JUJU_DATA_DIR/agents/controller-0/agent.conf || $JUJU_TOOLS_DIR/jujud bootstrap-state $JUJU_DATA_DIR/bootstrap-params --data-dir $JUJU_DATA_DIR --debug --timeout 10m0s
$JUJU_TOOLS_DIR/jujud machine --data-dir $JUJU_DATA_DIR --controller-id 0 --log-to-stderr --debug
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

	eventsPartial := &core.EventList{
		Items: []core.Event{
			{
				Type:   core.EventTypeNormal,
				Reason: provider.PullingImage,
			},
			{
				Type:   core.EventTypeNormal,
				Reason: provider.PulledImage,
			},
			{
				InvolvedObject: core.ObjectReference{FieldPath: "spec.containers{mongodb}"},
				Type:           core.EventTypeNormal,
				Reason:         provider.StartedContainer,
				Message:        "Started container mongodb",
			},
		},
	}

	eventsDone := &core.EventList{
		Items: []core.Event{
			{
				Type:   core.EventTypeNormal,
				Reason: provider.PullingImage,
			},
			{
				Type:   core.EventTypeNormal,
				Reason: provider.PulledImage,
			},
			{
				InvolvedObject: core.ObjectReference{FieldPath: "spec.containers{mongodb}"},
				Type:           core.EventTypeNormal,
				Reason:         provider.StartedContainer,
				Message:        "Started container mongodb",
			},
			{
				InvolvedObject: core.ObjectReference{FieldPath: "spec.containers{api-server}"},
				Type:           core.EventTypeNormal,
				Reason:         provider.StartedContainer,
				Message:        "Started container api-server",
			},
		},
	}

	podReady := &core.Pod{
		Status: core.PodStatus{
			Phase: core.PodRunning,
		},
	}

	podWatcher, podFirer := newKubernetesTestWatcher()
	eventWatcher, eventFirer := newKubernetesTestWatcher()
	<-podWatcher.Changes()
	<-eventWatcher.Changes()
	watchers := []provider.KubernetesNotifyWatcher{podWatcher, eventWatcher}
	watchCallCount := 0

	s.k8sWatcherFn = provider.NewK8sWatcherFunc(func(_ cache.SharedIndexInformer, n string, _ jujuclock.Clock) (provider.KubernetesNotifyWatcher, error) {
		if watchCallCount >= len(watchers) {
			return nil, errors.NotFoundf("no watcher available for index %d", watchCallCount)
		}
		w := watchers[watchCallCount]
		watchCallCount++
		return w, nil
	})

	gomock.InOrder(
		// create namespace.
		s.mockNamespaces.EXPECT().Create(ns).
			Return(ns, nil),

		// ensure service
		s.mockServices.EXPECT().Get("juju-controller-test-service", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(svcNotProvisioned).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(svcNotProvisioned).
			Return(svcNotProvisioned, nil),

		// below calls are for GetService - 1st address no provisioned yet.
		s.mockServices.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app=juju-controller-test"}).
			Return(&core.ServiceList{Items: []core.Service{*svcNotProvisioned}}, nil),
		s.mockStatefulSets.EXPECT().Get("juju-operator-juju-controller-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("juju-controller-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get("juju-controller-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDaemonSets.EXPECT().Get("juju-controller-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		// below calls are for GetService - 2nd address is ready.
		s.mockServices.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app=juju-controller-test"}).
			Return(&core.ServiceList{Items: []core.Service{*svcProvisioned}}, nil),
		s.mockStatefulSets.EXPECT().Get("juju-operator-juju-controller-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("juju-controller-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get("juju-controller-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDaemonSets.EXPECT().Get("juju-controller-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		// ensure shared-secret secret.
		s.mockSecrets.EXPECT().Get("juju-controller-test-secret", v1.GetOptions{}).AnyTimes().
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(emptySecret).AnyTimes().
			Return(emptySecret, nil),
		s.mockSecrets.EXPECT().Get("juju-controller-test-secret", v1.GetOptions{}).AnyTimes().
			Return(emptySecret, nil),
		s.mockSecrets.EXPECT().Update(secretWithSharedSecretAdded).AnyTimes().
			Return(secretWithSharedSecretAdded, nil),

		// ensure server.pem secret.
		s.mockSecrets.EXPECT().Get("juju-controller-test-secret", v1.GetOptions{}).AnyTimes().
			Return(secretWithSharedSecretAdded, nil),
		s.mockSecrets.EXPECT().Update(secretWithServerPEMAdded).AnyTimes().
			Return(secretWithServerPEMAdded, nil),

		// initialize the empty configmap.
		s.mockConfigMaps.EXPECT().Get("juju-controller-test-configmap", v1.GetOptions{}).AnyTimes().
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Create(emptyConfigMap).AnyTimes().
			Return(emptyConfigMap, nil),

		// ensure bootstrap-params configmap.
		s.mockConfigMaps.EXPECT().Get("juju-controller-test-configmap", v1.GetOptions{}).AnyTimes().
			Return(emptyConfigMap, nil),
		s.mockConfigMaps.EXPECT().Create(configMapWithBootstrapParamsAdded).AnyTimes().
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockConfigMaps.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app=juju-controller-test"}).
			Return(&core.ConfigMapList{Items: []core.ConfigMap{*emptyConfigMap}}, nil),
		s.mockConfigMaps.EXPECT().Update(configMapWithBootstrapParamsAdded).AnyTimes().
			Return(configMapWithBootstrapParamsAdded, nil),

		// ensure agent.conf configmap.
		s.mockConfigMaps.EXPECT().Get("juju-controller-test-configmap", v1.GetOptions{}).AnyTimes().
			Return(configMapWithBootstrapParamsAdded, nil),
		s.mockConfigMaps.EXPECT().Create(configMapWithAgentConfAdded).AnyTimes().
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockConfigMaps.EXPECT().List(v1.ListOptions{LabelSelector: "juju-app=juju-controller-test"}).
			Return(&core.ConfigMapList{Items: []core.ConfigMap{*configMapWithBootstrapParamsAdded}}, nil),
		s.mockConfigMaps.EXPECT().Update(configMapWithAgentConfAdded).AnyTimes().
			Return(configMapWithAgentConfAdded, nil),

		// Check the operator storage exists.
		// first check if <namespace>-<storage-class> exist or not.
		s.mockStorageClass.EXPECT().Get("controller-1-some-storage", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		// not found, fallback to <storage-class>.
		s.mockStorageClass.EXPECT().Get("some-storage", v1.GetOptions{}).
			Return(&sc, nil),

		s.mockStatefulSets.EXPECT().Create(statefulSetSpec).
			DoAndReturn(func(_ interface{}) (*apps.StatefulSet, error) {
				eventFirer()
				return statefulSetSpec, nil
			}),

		s.mockEvents.EXPECT().List(
			listOptionsFieldSelectorMatcher("involvedObject.name=controller-0,involvedObject.kind=Pod"),
		).Return(&core.EventList{}, nil),

		s.mockEvents.EXPECT().List(
			listOptionsFieldSelectorMatcher("involvedObject.name=controller-0,involvedObject.kind=Pod"),
		).DoAndReturn(func(...interface{}) (*core.EventList, error) {
			podFirer()
			return eventsPartial, nil
		}),

		s.mockEvents.EXPECT().List(
			listOptionsFieldSelectorMatcher("involvedObject.name=controller-0,involvedObject.kind=Pod"),
		).Return(eventsDone, nil),

		s.mockPods.EXPECT().Get("controller-0", v1.GetOptions{}).
			Return(podReady, nil),
	)

	errChan := make(chan error)
	go func() {
		errChan <- controllerStacker.Deploy()
	}()

	err = s.clock.WaitAdvance(3*time.Second, testing.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case err := <-errChan:
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(s.watchers, gc.HasLen, 2)
		c.Assert(workertest.CheckKilled(c, s.watchers[0]), jc.ErrorIsNil)
		c.Assert(workertest.CheckKilled(c, s.watchers[1]), jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for deploy return")
	}
}
