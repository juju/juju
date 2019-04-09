// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	k8sstorage "k8s.io/api/storage/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
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

	s.controllerUUID = "9bec388c-d264-4cde-8b29-3e675959157a"

	s.controllerCfg = testing.FakeControllerConfig()
	pcfg, err := podcfg.NewBootstrapControllerPodConfig(s.controllerCfg, controllerName, "bionic")
	c.Assert(err, jc.ErrorIsNil)

	pcfg.JujuVersion = jujuversion.Current
	pcfg.APIInfo = &api.Info{
		Password: "password",
		CACert:   coretesting.CACert,
		ModelTag: coretesting.ModelTag,
	}
	pcfg.Controller.MongoInfo = &mongo.MongoInfo{
		Password: "password", Info: mongo.Info{CACert: coretesting.CACert},
	}
	pcfg.Bootstrap.ControllerModelConfig = s.cfg
	pcfg.Bootstrap.BootstrapMachineInstanceId = "instance-id"
	pcfg.Bootstrap.HostedModelConfig = map[string]interface{}{
		"name": "hosted-model",
	}
	pcfg.Bootstrap.StateServingInfo = params.StateServingInfo{
		Cert:         coretesting.ServerCert,
		PrivateKey:   coretesting.ServerKey,
		CAPrivateKey: coretesting.CAKey,
		StatePort:    123,
		APIPort:      456,
	}
	pcfg.Bootstrap.StateServingInfo = params.StateServingInfo{
		Cert:         coretesting.ServerCert,
		PrivateKey:   coretesting.ServerKey,
		CAPrivateKey: coretesting.CAKey,
		StatePort:    123,
		APIPort:      456,
	}
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
		"juju.io/controller":    s.controllerUUID,
		"juju.io/is-controller": "true",
	})

	c.Assert(s.broker.GetCurrentNamespace(), jc.DeepEquals, "controller")
	c.Assert(s.broker.GetAnnotations().ToMap(), jc.DeepEquals, map[string]string{
		"juju.io/model":      s.cfg.UUID(),
		"juju.io/controller": s.controllerUUID,
	})

	gomock.InOrder(
		s.mockNamespaces.EXPECT().List(v1.ListOptions{IncludeUninitialized: true}).Times(1).
			Return(&core.NamespaceList{Items: []core.Namespace{existingNs}}, nil),
	)
	var err error
	s.broker, err = provider.ControllerCorelation(s.broker)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(
		// "is-controller" is set as well.
		s.broker.GetAnnotations().ToMap(), jc.DeepEquals,
		map[string]string{
			"juju.io/model":         s.cfg.UUID(),
			"juju.io/controller":    s.controllerUUID,
			"juju.io/is-controller": "true",
		},
	)
	// controller namespace linked back(changed from 'controller' to 'controller-1')
	c.Assert(s.broker.GetCurrentNamespace(), jc.DeepEquals, "controller-1")
}

func (s *bootstrapSuite) TestBootstrap(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	// Eventually the namespace wil be set to controllerName.
	// So we have to specify the final namespace(controllerName) for later use.
	newK8sRestClientFunc := s.setupK8sRestClient(c, ctrl, s.pcfg.ControllerName)

	s.setupBroker(c, ctrl, newK8sRestClientFunc)
	// Broker's namespace is "controller" now - controllerModelConfig.Name()
	c.Assert(s.broker.GetCurrentNamespace(), jc.DeepEquals, "controller")
	c.Assert(
		s.broker.GetAnnotations().ToMap(), jc.DeepEquals,
		map[string]string{
			"juju.io/model":      s.cfg.UUID(),
			"juju.io/controller": s.controllerUUID,
		},
	)

	// These two are done in broker.Bootstrap method actually.
	s.broker.SetNamespace("controller-1")
	s.broker.GetAnnotations().Add("juju.io/is-controller", "true")

	controllerStacker := s.controllerStackerGetter()
	// Broker's namespace should be set to controller name now.
	c.Assert(s.broker.GetCurrentNamespace(), jc.DeepEquals, "controller-1")
	c.Assert(
		// "is-controller" is set as well.
		s.broker.GetAnnotations().ToMap(), jc.DeepEquals,
		map[string]string{
			"juju.io/model":         s.cfg.UUID(),
			"juju.io/controller":    s.controllerUUID,
			"juju.io/is-controller": "true",
		},
	)

	sharedSecret, sslKey := controllerStacker.GetSharedSecretAndSSLKey(c)

	scName := "some-storage"
	sc := k8sstorage.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: scName,
		},
	}

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: s.getNamespace()}}
	ns.Name = s.getNamespace()
	s.ensureJujuNamespaceAnnotations(true, ns)
	svc := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-service",
			Labels:    map[string]string{"juju-app": "juju-controller-test"},
			Namespace: s.getNamespace(),
		},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"juju-app": "juju-controller-test"},
			Type:     core.ServiceType("ClusterIP"),
			Ports: []core.ServicePort{
				{
					Name:       "mongodb",
					TargetPort: intstr.FromInt(37017),
					Port:       37017,
					Protocol:   "TCP",
				},
				{
					Name:       "api-server",
					TargetPort: intstr.FromInt(17070),
					Port:       17070,
				},
			},
		},
	}

	emptySecret := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-secret",
			Labels:    map[string]string{"juju-app": "juju-controller-test"},
			Namespace: s.getNamespace(),
		},
		Type: core.SecretTypeOpaque,
	}
	secretWithSharedSecretAdded := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-secret",
			Labels:    map[string]string{"juju-app": "juju-controller-test"},
			Namespace: s.getNamespace(),
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"shared-secret": []byte(sharedSecret),
		},
	}
	secretWithServerPEMAdded := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-secret",
			Labels:    map[string]string{"juju-app": "juju-controller-test"},
			Namespace: s.getNamespace(),
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"shared-secret": []byte(sharedSecret),
			"server.pem":    []byte(sslKey),
		},
	}

	emptyConfigMap := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-configmap",
			Labels:    map[string]string{"juju-app": "juju-controller-test"},
			Namespace: s.getNamespace(),
		},
	}
	bootstrapParamsContent, err := s.pcfg.Bootstrap.StateInitializationParams.Marshal()
	c.Assert(err, jc.ErrorIsNil)
	configMapWithBootstrapParamsAdded := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-configmap",
			Labels:    map[string]string{"juju-app": "juju-controller-test"},
			Namespace: s.getNamespace(),
		},
		Data: map[string]string{
			"bootstrap-params": string(bootstrapParamsContent),
		},
	}
	configMapWithAgentConfAdded := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-configmap",
			Labels:    map[string]string{"juju-app": "juju-controller-test"},
			Namespace: s.getNamespace(),
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
			Labels:    map[string]string{"juju-app": "juju-controller-test"},
			Namespace: s.getNamespace(),
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
						Name:   "storage",
						Labels: map[string]string{"juju-app": "juju-controller-test"},
					},
					Spec: core.PersistentVolumeClaimSpec{
						StorageClassName: &scName,
						AccessModes:      []core.PersistentVolumeAccessMode{core.ReadWriteOnce},
						Resources: core.ResourceRequirements{
							Requests: core.ResourceList{
								core.ResourceStorage: controllerStacker.GetStorageSize(),
							},
						},
					},
				},
			},
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels:    map[string]string{"juju-app": "juju-controller-test"},
					Namespace: s.getNamespace(),
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
			"--port=37017",
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
				"--port=37017",
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
					ContainerPort: int32(37017),
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
			Image:           "jujusolutions/jujud-operator:" + jujuversion.Current.String(),
			Command: []string{
				"/bin/sh",
			},
			Args: []string{
				"-c",
				`
cp /opt/jujud $(pwd)/jujud

test -e /var/lib/juju/agents/machine-0/agent.conf || ./jujud bootstrap-state /var/lib/juju/bootstrap-params --data-dir /var/lib/juju --debug --timeout 0s
./jujud machine --data-dir /var/lib/juju --machine-id 0 --debug
`[1:],
			},
			WorkingDir: "/var/lib/juju/tools",
			VolumeMounts: []core.VolumeMount{
				{
					Name:      "storage",
					MountPath: "/var/lib/juju",
				},
				{
					Name:      "juju-controller-test-agent-conf",
					MountPath: "/var/lib/juju/agents/machine-0/template-agent.conf",
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

	gomock.InOrder(
		// create namespace.
		s.mockNamespaces.EXPECT().Create(ns).Times(1).
			Return(ns, nil),

		// ensure service
		s.mockServices.EXPECT().Get("juju-controller-test-service", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(svc).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(svc).Times(1).
			Return(svc, nil),

		// ensure shared-secret secret.
		s.mockSecrets.EXPECT().Get("juju-controller-test-secret", v1.GetOptions{IncludeUninitialized: true}).AnyTimes().
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Create(emptySecret).AnyTimes().
			Return(emptySecret, nil),
		s.mockSecrets.EXPECT().Get("juju-controller-test-secret", v1.GetOptions{IncludeUninitialized: true}).AnyTimes().
			Return(emptySecret, nil),
		s.mockSecrets.EXPECT().Update(secretWithSharedSecretAdded).AnyTimes().
			Return(secretWithSharedSecretAdded, nil),

		// ensure server.pem secret.
		s.mockSecrets.EXPECT().Get("juju-controller-test-secret", v1.GetOptions{IncludeUninitialized: true}).AnyTimes().
			Return(secretWithSharedSecretAdded, nil),
		s.mockSecrets.EXPECT().Update(secretWithServerPEMAdded).AnyTimes().
			Return(secretWithServerPEMAdded, nil),

		// ensure bootstrap-params configmap.
		s.mockConfigMaps.EXPECT().Get("juju-controller-test-configmap", v1.GetOptions{IncludeUninitialized: true}).AnyTimes().
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Create(emptyConfigMap).AnyTimes().
			Return(emptyConfigMap, nil),
		s.mockConfigMaps.EXPECT().Get("juju-controller-test-configmap", v1.GetOptions{IncludeUninitialized: true}).AnyTimes().
			Return(emptyConfigMap, nil),
		s.mockConfigMaps.EXPECT().Update(configMapWithBootstrapParamsAdded).AnyTimes().
			Return(configMapWithBootstrapParamsAdded, nil),

		// ensure agent.conf configmap.
		s.mockConfigMaps.EXPECT().Get("juju-controller-test-configmap", v1.GetOptions{IncludeUninitialized: true}).AnyTimes().
			Return(configMapWithBootstrapParamsAdded, nil),
		s.mockConfigMaps.EXPECT().Update(configMapWithAgentConfAdded).AnyTimes().
			Return(configMapWithAgentConfAdded, nil),

		// Check the operator storage exists.
		// first check if <namespace>-<storage-class> exist or not.
		s.mockStorageClass.EXPECT().Get("controller-1-some-storage", v1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		// not found, fallback to <storage-class>.
		s.mockStorageClass.EXPECT().Get("some-storage", v1.GetOptions{}).Times(1).
			Return(&sc, nil),

		// ensure statefulset.
		s.mockStatefulSets.EXPECT().Create(statefulSetSpec).Times(1).
			Return(statefulSetSpec, nil),
	)
	c.Assert(controllerStacker.Deploy(), jc.ErrorIsNil)
}
