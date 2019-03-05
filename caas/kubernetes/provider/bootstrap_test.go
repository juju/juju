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
	intstr "k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/controller"
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
	s.BaseSuite.SetUpTest(c)

	s.controllerCfg = testing.FakeControllerConfig()
	pcfg, err := podcfg.NewBootstrapControllerPodConfig(s.controllerCfg, "bionic")
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
		controllerStacker, err := provider.NewcontrollerStackForTest("juju-controller-test", s.broker, s.pcfg)
		c.Assert(err, jc.ErrorIsNil)
		return controllerStacker
	}
}

func (s *bootstrapSuite) TestBootstrap(c *gc.C) {
	ctrl := s.setupBroker(c)
	defer ctrl.Finish()

	controllerStacker := s.controllerStackerGetter()

	scName := "default-storageclass"
	sc := k8sstorage.StorageClass{
		ObjectMeta: v1.ObjectMeta{
			Name: scName,
			Annotations: map[string]string{
				"storageclass.kubernetes.io/is-default-class": "true",
			},
		},
	}
	scs := &k8sstorage.StorageClassList{Items: []k8sstorage.StorageClass{sc}}

	ns := &core.Namespace{ObjectMeta: v1.ObjectMeta{Name: s.namespace}}
	svc := &core.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-service",
			Labels:    map[string]string{"juju-application": "juju-controller-test"},
			Namespace: s.namespace,
		},
		Spec: core.ServiceSpec{
			Selector: map[string]string{"juju-application": "juju-controller-test"},
			Type:     core.ServiceType("NodePort"), // TODO(caas): NodePort works for single node only like microk8s.
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
			Labels:    map[string]string{"juju-application": "juju-controller-test"},
			Namespace: s.namespace,
		},
		Type: core.SecretTypeOpaque,
	}
	secretWithSharedSecretAdded := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-secret",
			Labels:    map[string]string{"juju-application": "juju-controller-test"},
			Namespace: s.namespace,
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"shared-secret": []byte(provider.TmpControllerSharedSecret),
		},
	}
	secretWithServerPEMAdded := &core.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-secret",
			Labels:    map[string]string{"juju-application": "juju-controller-test"},
			Namespace: s.namespace,
		},
		Type: core.SecretTypeOpaque,
		Data: map[string][]byte{
			"shared-secret": []byte(provider.TmpControllerSharedSecret),
			"server.pem":    []byte(provider.TmpControllerServerPem),
		},
	}

	emptyConfigMap := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-configmap",
			Labels:    map[string]string{"juju-application": "juju-controller-test"},
			Namespace: s.namespace,
		},
	}
	bootstrapParamsContent, err := s.pcfg.Bootstrap.StateInitializationParams.Marshal()
	c.Assert(err, jc.ErrorIsNil)
	configMapWithBootstrapParamsAdded := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-configmap",
			Labels:    map[string]string{"juju-application": "juju-controller-test"},
			Namespace: s.namespace,
		},
		Data: map[string]string{
			"bootstrap-params": string(bootstrapParamsContent),
		},
	}
	configMapWithAgentConfAdded := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:      "juju-controller-test-configmap",
			Labels:    map[string]string{"juju-application": "juju-controller-test"},
			Namespace: s.namespace,
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
			Labels:    map[string]string{"juju-application": "juju-controller-test"},
			Namespace: s.namespace,
		},
		Spec: apps.StatefulSetSpec{
			ServiceName: "juju-controller-test-service",
			Replicas:    &numberOfPods,
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-application": "juju-controller-test"},
			},
			VolumeClaimTemplates: []core.PersistentVolumeClaim{
				{
					ObjectMeta: v1.ObjectMeta{
						Name:   "juju-controller-test-pod-storage",
						Labels: map[string]string{"juju-application": "juju-controller-test"},
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
					Labels:    map[string]string{"juju-application": "juju-controller-test"},
					Namespace: s.namespace,
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
											Path: "server.pem",
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
			Image:           "mongo:3.6.6",
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
				"--wiredTigerCacheSizeGB=0.25",
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
					Name:      "juju-controller-test-pod-storage",
					MountPath: "/var/lib/juju/db",
					SubPath:   "db",
				},
				{
					Name:      "juju-controller-test-server-pem",
					MountPath: "/var/lib/juju/server.pem",
					SubPath:   "server.pem",
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
			ImagePullPolicy: core.PullAlways,
			Image:           s.pcfg.GetControllerImagePath(),
			VolumeMounts: []core.VolumeMount{
				{
					Name:      "juju-controller-test-pod-storage",
					MountPath: "/var/lib/juju",
				},
				{
					Name:      "juju-controller-test-agent-conf",
					MountPath: "/var/lib/juju/agents/machine-0/template-agent.conf",
					SubPath:   "template-agent.conf",
				},
				{
					Name:      "juju-controller-test-server-pem",
					MountPath: "/var/lib/juju/server.pem",
					SubPath:   "server.pem",
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

		// find storageclass to use.
		s.mockStorageClass.EXPECT().List(v1.ListOptions{}).Times(1).
			Return(scs, nil),

		// ensure statefulset.
		s.mockStatefulSets.EXPECT().Create(statefulSetSpec).Times(1).
			Return(statefulSetSpec, nil),
	)
	c.Assert(controllerStacker.Deploy(), jc.ErrorIsNil)
}
