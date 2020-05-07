// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"time"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/testing"
)

var operatorAnnotations = map[string]string{
	"fred":               "mary",
	"juju-version":       "2.99.0",
	"juju.io/controller": testing.ControllerTag.Id(),
}

var operatorServiceArg = &core.Service{
	ObjectMeta: v1.ObjectMeta{
		Name:   "test-operator",
		Labels: map[string]string{"juju-operator": "test"},
		Annotations: map[string]string{
			"fred":               "mary",
			"juju-version":       "2.99.0",
			"juju.io/controller": testing.ControllerTag.Id(),
		},
	},
	Spec: core.ServiceSpec{
		Selector: map[string]string{"juju-operator": "test"},
		Type:     "ClusterIP",
		Ports: []core.ServicePort{
			{Port: 30666, TargetPort: intstr.FromInt(30666), Protocol: "TCP"},
		},
	},
}

func operatorPodSpec(serviceAccountName string, withStorage bool) core.PodSpec {
	spec := core.PodSpec{
		ServiceAccountName:           serviceAccountName,
		AutomountServiceAccountToken: boolPtr(true),
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
	if withStorage {
		spec.Containers[0].VolumeMounts = append(spec.Containers[0].VolumeMounts, core.VolumeMount{
			Name:      "charm",
			MountPath: "path/to/agent/agents",
		})
	}
	return spec
}

func operatorStatefulSetArg(numUnits int32, scName, serviceAccountName string, withStorage bool) *appsv1.StatefulSet {
	ss := &appsv1.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-operator",
			Labels: map[string]string{
				"juju-operator": "test",
			},
			Annotations: operatorAnnotations,
		},
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
						"fred":               "mary",
						"juju-version":       "2.99.0",
						"juju.io/controller": testing.ControllerTag.Id(),
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
					},
				},
				Spec: operatorPodSpec(serviceAccountName, withStorage),
			},
			PodManagementPolicy: apps.ParallelPodManagement,
		},
	}
	if withStorage {
		ss.Spec.VolumeClaimTemplates = []core.PersistentVolumeClaim{{
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
		}}
	}
	return ss
}

func (s *K8sSuite) TestOperatorPodConfig(c *gc.C) {
	tags := map[string]string{
		"fred":               "mary",
		"juju.io/controller": testing.ControllerTag.Id(),
	}
	pod, err := provider.OperatorPod("gitlab", "gitlab", "10666", "/var/lib/juju", "jujusolutions/jujud-operator", "2.99.0", tags, "operator-service-account")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pod.Name, gc.Equals, "gitlab")
	c.Assert(pod.Labels, jc.DeepEquals, map[string]string{
		"juju-operator": "gitlab",
	})
	c.Assert(pod.Annotations, jc.DeepEquals, map[string]string{
		"fred":               "mary",
		"juju-version":       "2.99.0",
		"juju.io/controller": testing.ControllerTag.Id(),
		"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
		"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
	})
	c.Assert(pod.Spec.ServiceAccountName, gc.Equals, "operator-service-account")
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
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		// delete RBAC resources.
		s.mockRoleBindings.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "juju-operator=test"},
		).Return(nil),
		s.mockRoles.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "juju-operator=test"},
		).Return(nil),
		s.mockServiceAccounts.EXPECT().DeleteCollection(
			s.deleteOptions(v1.DeletePropagationForeground, ""),
			v1.ListOptions{LabelSelector: "juju-operator=test"},
		).Return(nil),

		s.mockConfigMaps.EXPECT().Delete("test-operator-config", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Delete("test-configurations-config", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
		s.mockServices.EXPECT().Delete("test-operator", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Delete("test-operator", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
		s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=test"}).
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
		s.mockSecrets.EXPECT().Delete("test-jujud-secret", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
		s.mockPersistentVolumeClaims.EXPECT().Delete("test-operator-volume", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
		s.mockPersistentVolumes.EXPECT().Delete("test-operator-volume", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Delete("test-operator", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),
	)

	err := s.broker.DeleteOperator("test")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureOperatorNoAgentConfig(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		AutomountServiceAccountToken: boolPtr(true),
	}
	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs:     []string{"create"},
			},
		},
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		RoleRef: rbacv1.RoleRef{
			Name: "test-operator",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "test-operator",
				Namespace: "test",
			},
		},
	}
	statefulSetArg := operatorStatefulSetArg(1, "test-operator-storage", "test-operator", true)
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(operatorServiceArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(operatorServiceArg).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(&core.Service{Spec: core.ServiceSpec{ClusterIP: "10.1.2.3"}}, nil),

		// ensure RBAC resources.
		s.mockServiceAccounts.EXPECT().Create(svcAccount).Return(svcAccount, nil),
		s.mockRoles.EXPECT().Create(role).Return(role, nil),
		s.mockRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=test"}).
			Return(&rbacv1.RoleBindingList{Items: []rbacv1.RoleBinding{}}, nil),
		s.mockRoleBindings.EXPECT().Create(rb).Return(rb, nil),

		s.mockConfigMaps.EXPECT().Get("test-operator-config", v1.GetOptions{}).
			Return(nil, nil),
		s.mockStorageClass.EXPECT().Get("test-operator-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "test-operator-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).
			Return(statefulSetArg, nil),
	)

	err := s.broker.EnsureOperator("test", "path/to/agent", &caas.OperatorConfig{
		OperatorImagePath: "/path/to/image",
		Version:           version.MustParse("2.99.0"),
		ResourceTags: map[string]string{
			"fred":                 "mary",
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		CharmStorage: &caas.CharmStorageParams{
			Size:         uint64(10),
			Provider:     "kubernetes",
			Attributes:   map[string]interface{}{"storage-class": "operator-storage"},
			ResourceTags: map[string]string{"foo": "bar"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureOperatorCreate(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	configMapArg := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator-config",
			Labels:      map[string]string{"juju-app": "test"},
			Annotations: operatorAnnotations,
		},
		Data: map[string]string{
			"test-agent.conf": "agent-conf-data",
			"operator.yaml":   "operator-info-data",
		},
	}

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		AutomountServiceAccountToken: boolPtr(true),
	}
	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs:     []string{"create"},
			},
		},
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		RoleRef: rbacv1.RoleRef{
			Name: "test-operator",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "test-operator",
				Namespace: "test",
			},
		},
	}
	statefulSetArg := operatorStatefulSetArg(1, "test-operator-storage", "test-operator", true)

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(operatorServiceArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(operatorServiceArg).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(&core.Service{Spec: core.ServiceSpec{ClusterIP: "10.1.2.3"}}, nil),

		// ensure RBAC resources.
		s.mockServiceAccounts.EXPECT().Create(svcAccount).Return(svcAccount, nil),
		s.mockRoles.EXPECT().Create(role).Return(role, nil),
		s.mockRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=test"}).
			Return(&rbacv1.RoleBindingList{Items: []rbacv1.RoleBinding{}}, nil),
		s.mockRoleBindings.EXPECT().Create(rb).Return(rb, nil),

		s.mockConfigMaps.EXPECT().Update(configMapArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Create(configMapArg).
			Return(configMapArg, nil),
		s.mockStorageClass.EXPECT().Get("test-operator-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "test-operator-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).
			Return(statefulSetArg, nil),
	)

	err := s.broker.EnsureOperator("test", "path/to/agent", &caas.OperatorConfig{
		OperatorImagePath: "/path/to/image",
		Version:           version.MustParse("2.99.0"),
		AgentConf:         []byte("agent-conf-data"),
		OperatorInfo:      []byte("operator-info-data"),
		ResourceTags: map[string]string{
			"fred":                 "mary",
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		CharmStorage: &caas.CharmStorageParams{
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
			Name:        "test-operator-config",
			Labels:      map[string]string{"juju-app": "test"},
			Annotations: operatorAnnotations,
			Generation:  1234,
		},
		Data: map[string]string{
			"test-agent.conf": "agent-conf-data",
			"operator.yaml":   "operator-info-data",
		},
	}

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		AutomountServiceAccountToken: boolPtr(true),
	}
	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs:     []string{"create"},
			},
		},
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		RoleRef: rbacv1.RoleRef{
			Name: "test-operator",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "test-operator",
				Namespace: "test",
			},
		},
	}
	rbUID := rb.GetUID()

	statefulSetArg := operatorStatefulSetArg(1, "test-operator-storage", "test-operator", true)

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(operatorServiceArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(operatorServiceArg).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(&core.Service{Spec: core.ServiceSpec{ClusterIP: "10.1.2.3"}}, nil),

		// ensure RBAC resources.
		s.mockServiceAccounts.EXPECT().Create(svcAccount).Return(nil, s.k8sAlreadyExistsError()),
		s.mockServiceAccounts.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=test"}).
			Return(&core.ServiceAccountList{Items: []core.ServiceAccount{*svcAccount}}, nil),
		s.mockServiceAccounts.EXPECT().Update(svcAccount).Return(svcAccount, nil),
		s.mockRoles.EXPECT().Create(role).Return(nil, s.k8sAlreadyExistsError()),
		s.mockRoles.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=test"}).
			Return(&rbacv1.RoleList{Items: []rbacv1.Role{*role}}, nil),
		s.mockRoles.EXPECT().Update(role).Return(role, nil),
		s.mockRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=test"}).
			Return(&rbacv1.RoleBindingList{Items: []rbacv1.RoleBinding{*rb}}, nil),
		s.mockRoleBindings.EXPECT().Delete("test-operator", s.deleteOptions(v1.DeletePropagationForeground, rbUID)).Return(nil),
		s.mockRoleBindings.EXPECT().Get("test-operator", v1.GetOptions{}).Return(rb, nil),
		s.mockRoleBindings.EXPECT().Get("test-operator", v1.GetOptions{}).Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Create(rb).Return(rb, nil),

		s.mockConfigMaps.EXPECT().Update(configMapArg).
			Return(configMapArg, nil),
		s.mockStorageClass.EXPECT().Get("test-operator-storage", v1.GetOptions{}).
			Return(&storagev1.StorageClass{ObjectMeta: v1.ObjectMeta{Name: "test-operator-storage"}}, nil),
		s.mockStatefulSets.EXPECT().Create(statefulSetArg).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockStatefulSets.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(statefulSetArg, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).
			Return(nil, nil),
	)

	errChan := make(chan error)
	go func() {
		errChan <- s.broker.EnsureOperator("test", "path/to/agent", &caas.OperatorConfig{
			OperatorImagePath: "/path/to/image",
			Version:           version.MustParse("2.99.0"),
			AgentConf:         []byte("agent-conf-data"),
			OperatorInfo:      []byte("operator-info-data"),
			ResourceTags: map[string]string{
				"fred":                 "mary",
				"juju-controller-uuid": testing.ControllerTag.Id(),
			},
			CharmStorage: &caas.CharmStorageParams{
				Size:         uint64(10),
				Provider:     "kubernetes",
				Attributes:   map[string]interface{}{"storage-class": "operator-storage"},
				ResourceTags: map[string]string{"foo": "bar"},
			},
			ConfigMapGeneration: 1234,
		})
	}()
	err := s.clock.WaitAdvance(2*time.Second, testing.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case err := <-errChan:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for EnsureOperator return")
	}
}

func (s *K8sBrokerSuite) TestEnsureOperatorNoStorageExistingPVC(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	configMapArg := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator-config",
			Labels:      map[string]string{"juju-app": "test"},
			Annotations: operatorAnnotations,
		},
		Data: map[string]string{
			"test-agent.conf": "agent-conf-data",
			"operator.yaml":   "operator-info-data",
		},
	}

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		AutomountServiceAccountToken: boolPtr(true),
	}
	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs:     []string{"create"},
			},
		},
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		RoleRef: rbacv1.RoleRef{
			Name: "test-operator",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "test-operator",
				Namespace: "test",
			},
		},
	}
	scName := "test-operator-storage"
	statefulSetArg := operatorStatefulSetArg(1, scName, "test-operator", true)

	existingCharmPvc := &core.PersistentVolumeClaim{
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
	}

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(operatorServiceArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(operatorServiceArg).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(&core.Service{Spec: core.ServiceSpec{ClusterIP: "10.1.2.3"}}, nil),

		// ensure RBAC resources.
		s.mockServiceAccounts.EXPECT().Create(svcAccount).Return(svcAccount, nil),
		s.mockRoles.EXPECT().Create(role).Return(role, nil),
		s.mockRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=test"}).
			Return(&rbacv1.RoleBindingList{Items: []rbacv1.RoleBinding{}}, nil),
		s.mockRoleBindings.EXPECT().Create(rb).Return(rb, nil),
		s.mockConfigMaps.EXPECT().Update(configMapArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Create(configMapArg).
			Return(configMapArg, nil),

		// check for existing PVC in case of charm upgrade
		s.mockPersistentVolumeClaims.EXPECT().Get("charm", v1.GetOptions{}).
			Return(existingCharmPvc, nil),

		s.mockStatefulSets.EXPECT().Create(statefulSetArg).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockStatefulSets.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(statefulSetArg, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).
			Return(nil, nil),
	)

	err := s.broker.EnsureOperator("test", "path/to/agent", &caas.OperatorConfig{
		OperatorImagePath: "/path/to/image",
		Version:           version.MustParse("2.99.0"),
		AgentConf:         []byte("agent-conf-data"),
		OperatorInfo:      []byte("operator-info-data"),
		ResourceTags: map[string]string{
			"fred":                 "mary",
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureOperatorNoStorage(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	configMapArg := &core.ConfigMap{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator-config",
			Labels:      map[string]string{"juju-app": "test"},
			Annotations: operatorAnnotations,
		},
		Data: map[string]string{
			"test-agent.conf": "agent-conf-data",
			"operator.yaml":   "operator-info-data",
		},
	}

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		AutomountServiceAccountToken: boolPtr(true),
	}
	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs:     []string{"create"},
			},
		},
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		RoleRef: rbacv1.RoleRef{
			Name: "test-operator",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "test-operator",
				Namespace: "test",
			},
		},
	}

	statefulSetArg := operatorStatefulSetArg(1, "test-operator-storage", "test-operator", false)

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(operatorServiceArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(operatorServiceArg).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(&core.Service{Spec: core.ServiceSpec{ClusterIP: "10.1.2.3"}}, nil),

		// ensure RBAC resources.
		s.mockServiceAccounts.EXPECT().Create(svcAccount).Return(svcAccount, nil),
		s.mockRoles.EXPECT().Create(role).Return(role, nil),
		s.mockRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=test"}).
			Return(&rbacv1.RoleBindingList{Items: []rbacv1.RoleBinding{}}, nil),
		s.mockRoleBindings.EXPECT().Create(rb).Return(rb, nil),
		s.mockConfigMaps.EXPECT().Update(configMapArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Create(configMapArg).
			Return(configMapArg, nil),

		// check for existing PVC in case of charm upgrade
		s.mockPersistentVolumeClaims.EXPECT().Get("charm", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		s.mockStatefulSets.EXPECT().Create(statefulSetArg).
			Return(nil, s.k8sAlreadyExistsError()),
		s.mockStatefulSets.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(statefulSetArg, nil),
		s.mockStatefulSets.EXPECT().Update(statefulSetArg).
			Return(nil, nil),
	)

	err := s.broker.EnsureOperator("test", "path/to/agent", &caas.OperatorConfig{
		OperatorImagePath: "/path/to/image",
		Version:           version.MustParse("2.99.0"),
		AgentConf:         []byte("agent-conf-data"),
		OperatorInfo:      []byte("operator-info-data"),
		ResourceTags: map[string]string{
			"fred":                 "mary",
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) TestEnsureOperatorNoAgentConfigMissingConfigMap(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	svcAccount := &core.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		AutomountServiceAccountToken: boolPtr(true),
	}
	svcAccountUID := svcAccount.GetUID()
	role := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods/exec"},
				Verbs:     []string{"create"},
			},
		},
	}
	roleUID := role.GetUID()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: v1.ObjectMeta{
			Name:        "test-operator",
			Namespace:   "test",
			Labels:      map[string]string{"juju-operator": "test"},
			Annotations: operatorAnnotations,
		},
		RoleRef: rbacv1.RoleRef{
			Name: "test-operator",
			Kind: "Role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      "test-operator",
				Namespace: "test",
			},
		},
	}
	rbUID := rb.GetUID()
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Update(operatorServiceArg).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Create(operatorServiceArg).
			Return(nil, nil),
		s.mockServices.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(&core.Service{Spec: core.ServiceSpec{ClusterIP: "10.1.2.3"}}, nil),

		// ensure RBAC resources.
		s.mockServiceAccounts.EXPECT().Create(svcAccount).Return(svcAccount, nil),
		s.mockRoles.EXPECT().Create(role).Return(role, nil),
		s.mockRoleBindings.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=test"}).
			Return(&rbacv1.RoleBindingList{Items: []rbacv1.RoleBinding{}}, nil),
		s.mockRoleBindings.EXPECT().Create(rb).Return(rb, nil),

		s.mockConfigMaps.EXPECT().Get("test-operator-config", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),

		// clean up steps.
		s.mockServices.EXPECT().Delete("test-operator", s.deleteOptions(v1.DeletePropagationForeground, "")).
			Return(s.k8sNotFoundError()),

		// delete RBAC resources.
		s.mockRoleBindings.EXPECT().Delete("test-operator", s.deleteOptions(v1.DeletePropagationForeground, rbUID)).Return(nil),
		s.mockRoles.EXPECT().Delete("test-operator", s.deleteOptions(v1.DeletePropagationForeground, roleUID)).Return(nil),
		s.mockServiceAccounts.EXPECT().Delete("test-operator", s.deleteOptions(v1.DeletePropagationForeground, svcAccountUID)).Return(nil),
	)

	err := s.broker.EnsureOperator("test", "path/to/agent", &caas.OperatorConfig{
		OperatorImagePath: "/path/to/image",
		Version:           version.MustParse("2.99.0"),
		ResourceTags: map[string]string{
			"fred":                 "mary",
			"juju-controller-uuid": testing.ControllerTag.Id(),
		},
		CharmStorage: &caas.CharmStorageParams{
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
			Annotations: map[string]string{
				"juju-version":       "2.99.0",
				"juju.io/controller": testing.ControllerTag.Id(),
			},
		},
		Spec: core.PodSpec{
			Containers: []core.Container{{
				Name:  "juju-operator",
				Image: "test-image",
			}},
		},
		Status: core.PodStatus{
			Phase:   core.PodPending,
			Message: "test message.",
		},
	}
	ss := apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Annotations: map[string]string{
				"juju-version":       "2.99.0",
				"juju.io/controller": testing.ControllerTag.Id(),
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
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(&ss, nil),
		s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=test"}).
			Return(&core.PodList{Items: []core.Pod{opPod}}, nil),
		s.mockConfigMaps.EXPECT().Get("test-operator-config", v1.GetOptions{}).
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
				"juju-version":       "2.99.0",
				"juju.io/controller": testing.ControllerTag.Id(),
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
		s.mockStatefulSets.EXPECT().Get("juju-operator-test", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("test-operator", v1.GetOptions{}).
			Return(&ss, nil),
		s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=test"}).
			Return(&core.PodList{Items: []core.Pod{}}, nil),
	)

	_, err := s.broker.Operator("test")
	c.Assert(err, gc.ErrorMatches, "operator pod for application \"test\" not found")
}

func (s *K8sBrokerSuite) TestOperatorExists(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test-app", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(&apps.StatefulSet{}, nil),
	)

	exists, err := s.broker.OperatorExists("test-app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.DeepEquals, caas.OperatorState{
		Exists:      true,
		Terminating: false,
	})
}

func (s *K8sBrokerSuite) TestOperatorExistsTerminating(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test-app", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(&apps.StatefulSet{
				ObjectMeta: v1.ObjectMeta{
					DeletionTimestamp: &v1.Time{time.Now()},
				},
			}, nil),
	)

	exists, err := s.broker.OperatorExists("test-app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.DeepEquals, caas.OperatorState{
		Exists:      true,
		Terminating: true,
	})
}

func (s *K8sBrokerSuite) TestOperatorExistsTerminated(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test-app", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockRoles.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Get("test-app-operator-config", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Get("test-app-configurations-config", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Get("test-app-juju-operator-secret", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockPods.EXPECT().List(v1.ListOptions{
			LabelSelector: "juju-operator=test-app",
		}).
			Return(&core.PodList{}, nil),
	)

	exists, err := s.broker.OperatorExists("test-app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.DeepEquals, caas.OperatorState{
		Exists:      false,
		Terminating: false,
	})
}

func (s *K8sBrokerSuite) TestOperatorExistsTerminatedMostly(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test-app", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServiceAccounts.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockRoles.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockRoleBindings.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Get("test-app-operator-config", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockConfigMaps.EXPECT().Get("test-app-configurations-config", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockServices.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockSecrets.EXPECT().Get("test-app-juju-operator-secret", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(&appsv1.Deployment{}, nil),
	)

	exists, err := s.broker.OperatorExists("test-app")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.DeepEquals, caas.OperatorState{
		Exists:      true,
		Terminating: true,
	})
}
