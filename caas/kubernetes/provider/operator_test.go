// Copyright 2019 Canonical Ltd.
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
			{Name: "JUJU_OPERATOR_SERVICE_IP", Value: "10.1.2.3"},
			{
				Name: "JUJU_OPERATOR_POD_IP",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
			{
				Name: "JUJU_OPERATOR_NAMESPACE",
				ValueFrom: &core.EnvVarSource{
					FieldRef: &core.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
		},
		VolumeMounts: []core.VolumeMount{{
			Name:      "test-operator-config",
			MountPath: "path/to/agent/agents/application-test/template-agent.conf",
			SubPath:   "template-agent.conf",
		}, {
			Name:      "test-operator-config",
			MountPath: "path/to/agent/agents/application-test/operator.yaml",
			SubPath:   "operator.yaml",
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
				}, {
					Key:  "operator.yaml",
					Path: "operator.yaml",
				}},
			},
		},
	}},
}

var operatorServiceArg = &core.Service{
	ObjectMeta: v1.ObjectMeta{
		Name:   "test-operator",
		Labels: map[string]string{"juju-operator": "test"},
	},
	Spec: core.ServiceSpec{
		Selector: map[string]string{"juju-operator": "test"},
		Type:     "ClusterIP",
		Ports: []core.ServicePort{
			{Port: 30666, TargetPort: intstr.FromInt(30666), Protocol: "TCP"},
		},
	},
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

func (s *K8sSuite) TestOperatorPodConfig(c *gc.C) {
	tags := map[string]string{
		"fred": "mary",
	}
	pod, err := provider.OperatorPod("gitlab", "gitlab", "10666", "/var/lib/juju", "jujusolutions/jujud-operator", "2.99.0", tags)
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
	c.Assert(pod.Spec.Containers[0].VolumeMounts, gc.HasLen, 2)
	c.Assert(pod.Spec.Containers[0].VolumeMounts[0].MountPath, gc.Equals, "/var/lib/juju/agents/application-gitlab/template-agent.conf")
	c.Assert(pod.Spec.Containers[0].VolumeMounts[1].MountPath, gc.Equals, "/var/lib/juju/agents/application-gitlab/operator.yaml")

	podEnv := make(map[string]string)
	for _, env := range pod.Spec.Containers[0].Env {
		podEnv[env.Name] = env.Value
	}
	c.Assert(podEnv["JUJU_OPERATOR_SERVICE_IP"], gc.Equals, "10666")
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
		s.mockServices.EXPECT().Delete("test-operator", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
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

func (s *K8sBrokerSuite) TestEnsureOperatorCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	configMapArg := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:   "test-operator-config",
			Labels: map[string]string{"juju-app": "test", "juju-model": "test"},
		},
		Data: map[string]string{
			"test-agent.conf": "agent-conf-data",
			"operator.yaml":   "operator-info-data",
		},
	}
	statefulSetArg := operatorStatefulSetArg(1, "test-operator-storage")

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(operatorServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(operatorServiceArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&core.Service{Spec: core.ServiceSpec{ClusterIP: "10.1.2.3"}}, nil),
		s.mockConfigMaps.EXPECT().Create(configMapArg).Times(1).
			Return(configMapArg, nil),
		s.mockStorageClass.EXPECT().Get("test-operator-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "test-operator-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(statefulSetArg, nil),
	)

	err := s.broker.EnsureOperator("test", "path/to/agent", &caas.OperatorConfig{
		OperatorImagePath: "/path/to/image",
		Version:           version.MustParse("2.99.0"),
		AgentConf:         []byte("agent-conf-data"),
		OperatorInfo:      []byte("operator-info-data"),
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

func (s *K8sBrokerSuite) TestEnsureOperatorUpdate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	configMapArg := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:   "test-operator-config",
			Labels: map[string]string{"juju-app": "test", "juju-model": "test"},
		},
		Data: map[string]string{
			"test-agent.conf": "agent-conf-data",
			"operator.yaml":   "operator-info-data",
		},
	}
	statefulSetArg := operatorStatefulSetArg(1, "test-operator-storage")

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(operatorServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(operatorServiceArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&core.Service{Spec: core.ServiceSpec{ClusterIP: "10.1.2.3"}}, nil),
		s.mockConfigMaps.EXPECT().Create(configMapArg).Times(1).
			Return(configMapArg, nil),
		s.mockStorageClass.EXPECT().Get("test-operator-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "test-operator-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(statefulSetArg, nil),
		s.mockStatefulSets.EXPECT().Get("test-operator", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(statefulSetArg, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(statefulSetArg, nil),
	)

	err := s.broker.EnsureOperator("test", "path/to/agent", &caas.OperatorConfig{
		OperatorImagePath: "/path/to/image",
		Version:           version.MustParse("2.99.0"),
		AgentConf:         []byte("agent-conf-data"),
		OperatorInfo:      []byte("operator-info-data"),
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
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(operatorServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(operatorServiceArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&core.Service{Spec: core.ServiceSpec{ClusterIP: "10.1.2.3"}}, nil),
		s.mockConfigMaps.EXPECT().Get("test-operator-config", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-operator-storage", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "test-operator-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).Times(1).
			Return(statefulSetArg, nil),
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
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(operatorServiceArg).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(operatorServiceArg).Times(1).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{IncludeUninitialized: false}).Times(1).
			Return(&core.Service{Spec: core.ServiceSpec{ClusterIP: "10.1.2.3"}}, nil),
		s.mockConfigMaps.EXPECT().Get("test-operator-config", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Delete("test-operator", s.deleteOptions(v1.DeletePropagationForeground, nil)).Times(1).
			Return(s.k8sNotFoundError()),
	)

	err := s.broker.EnsureOperator("test", "path/to/agent", &caas.OperatorConfig{
		OperatorImagePath: "/path/to/image",
		Version:           version.MustParse("2.99.0"),
		CharmStorage: caas.CharmStorageParams{
			Size:     uint64(10),
			Provider: "kubernetes",
		},
	})
	c.Assert(err, gc.ErrorMatches, `config map for "test" should already exist: configmap "test-operator-config" not found`)
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
	ss := apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Annotations: map[string]string{
				"juju-version": "2.99.0",
			},
		},
		Spec: apps.StatefulSetSpec{
			Template: core.PodTemplateSpec{
				Spec: core.PodSpec{
					Containers: []core.Container{{
						Name:  "juju-operator",
						Image: "test-image",
					}},
				},
			},
		},
	}
	cm := core.ConfigMap{
		Data: map[string]string{
			"test-agent.conf": "agent-conf-data",
			"operator.yaml":   "operator-info-data",
		},
	}
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("test-operator", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&ss, nil),
		s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator==test"}).Times(1).
			Return(&core.PodList{Items: []core.Pod{opPod}}, nil),
		s.mockConfigMaps.EXPECT().Get("test-operator-config", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&cm, nil),
	)

	operator, err := s.broker.Operator("test")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(operator.Status.Status, gc.Equals, status.Allocating)
	c.Assert(operator.Status.Message, gc.Equals, "test message.")
	c.Assert(operator.Config.Version, gc.Equals, version.MustParse("2.99.0"))
	c.Assert(operator.Config.OperatorImagePath, gc.Equals, "test-image")
	c.Assert(operator.Config.AgentConf, gc.DeepEquals, []byte("agent-conf-data"))
	c.Assert(operator.Config.OperatorInfo, gc.DeepEquals, []byte("operator-info-data"))
}

func (s *K8sBrokerSuite) TestOperatorNoPodFound(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	ss := apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Annotations: map[string]string{
				"juju-version": "2.99.0",
			},
		},
		Spec: apps.StatefulSetSpec{
			Template: core.PodTemplateSpec{
				Spec: core.PodSpec{
					Containers: []core.Container{{
						Name:  "juju-operator",
						Image: "test-image",
					}},
				},
			},
		},
	}
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("test-operator", v1.GetOptions{IncludeUninitialized: true}).Times(1).
			Return(&ss, nil),
		s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator==test"}).Times(1).
			Return(&core.PodList{Items: []core.Pod{}}, nil),
	)

	_, err := s.broker.Operator("test")
	c.Assert(err, gc.ErrorMatches, "operator pod for application \"test\" not found")
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
