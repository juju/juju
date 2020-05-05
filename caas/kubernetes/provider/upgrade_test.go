// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"encoding/json"
	"time"

	"github.com/golang/mock/gomock"
	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/testing"
)

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
			RevisionHistoryLimit: int32Ptr(0),
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
	updated.Spec.Template.Spec.Containers[1].Image = "jujud-operator:6.6.6"
	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("controller", v1.GetOptions{}).
			Return(&ss, nil),
		s.mockStatefulSets.EXPECT().Update(&updated).
			Return(nil, nil),
	)

	err := s.broker.Upgrade("controller", version.MustParse("6.6.6"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *K8sBrokerSuite) assertUpgradeOperator(c *gc.C, shouldTimeout bool, adjustClock func(), assertCalls ...*gomock.Call) {
	operatorSS := apps.StatefulSet{
		ObjectMeta: v1.ObjectMeta{
			Name: "app-name-operator",
			Annotations: map[string]string{
				"juju-version":       "1.1.1",
				"juju.io/controller": testing.ControllerTag.Id(),
			},
			Labels: map[string]string{"juju-app": "app-name"},
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
						{Name: "juju-operator", Image: "jujud-operator:1.1.1"},
					},
				},
			},
		},
	}

	opPodPending := core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-operator",
		},
		Status: core.PodStatus{
			Phase:   core.PodPending,
			Message: "test message.",
		},
	}
	opPodRuning := core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-operator",
		},
		Status: core.PodStatus{
			Phase:   core.PodRunning,
			Message: "test message.",
		},
	}
	opCm := core.ConfigMap{
		Data: map[string]string{
			"test-agent.conf": "agent-conf-data",
			"operator.yaml":   "operator-info-data",
		},
	}

	watcherOperator, operatorNotifier := newKubernetesTestWatcher()
	s.k8sWatcherFn = func(_ cache.SharedIndexInformer, _ string, _ jujuclock.Clock) (provider.KubernetesNotifyWatcher, error) {
		return watcherOperator, nil
	}

	updatedOperatorSS := operatorSS
	updatedOperatorSS.Annotations["juju-version"] = "6.6.6"
	updatedOperatorSS.Spec.Template.Annotations["juju-version"] = "6.6.6"
	updatedOperatorSS.Spec.Template.Spec.Containers[1].Image = "jujud-operator:6.6.6"

	preAssertCalls := []*gomock.Call{
		// handle legacy operator name.
		s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("app-name-operator", v1.GetOptions{}).
			Return(&operatorSS, nil),
	}
	if shouldTimeout {
		preAssertCalls = append(preAssertCalls,
			s.mockStatefulSets.EXPECT().Update(&updatedOperatorSS).
				DoAndReturn(func(in *apps.StatefulSet) (*apps.StatefulSet, error) {
					return in, nil
				}),
			// check operator status.
			s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
			s.mockStatefulSets.EXPECT().Get("app-name-operator", v1.GetOptions{}).
				Return(&updatedOperatorSS, nil),
			s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=app-name"}).
				Return(&core.PodList{Items: []core.Pod{opPodPending}}, nil),
			s.mockConfigMaps.EXPECT().Get("app-name-operator-config", v1.GetOptions{}).
				Return(&opCm, nil),
		)
	} else {
		preAssertCalls = append(preAssertCalls,
			s.mockStatefulSets.EXPECT().Update(&updatedOperatorSS).
				DoAndReturn(func(in *apps.StatefulSet) (*apps.StatefulSet, error) {
					operatorNotifier()
					return in, nil
				}),
			// check operator status.
			s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
			s.mockStatefulSets.EXPECT().Get("app-name-operator", v1.GetOptions{}).
				Return(&updatedOperatorSS, nil),
			s.mockPods.EXPECT().List(v1.ListOptions{LabelSelector: "juju-operator=app-name"}).
				Return(&core.PodList{Items: []core.Pod{opPodRuning}}, nil),
			s.mockConfigMaps.EXPECT().Get("app-name-operator-config", v1.GetOptions{}).
				Return(&opCm, nil),

			// upgrade Juju init container in workload pod.
			s.mockStatefulSets.EXPECT().Get("juju-operator-app-name", v1.GetOptions{}).
				Return(nil, s.k8sNotFoundError()),
		)
	}
	gomock.InOrder(
		append(
			preAssertCalls,
			assertCalls...,
		)...,
	)

	errChan := make(chan error)
	go func() {
		errChan <- s.broker.Upgrade("app-name", version.MustParse("6.6.6"))
	}()

	adjustClock()
	select {
	case err := <-errChan:
		if shouldTimeout {
			c.Assert(err, jc.Satisfies, errors.IsTimeout)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for Upgrade return")
	}
}

func (s *K8sBrokerSuite) TestUpgradeOperatorTimeoutFailed(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	s.assertUpgradeOperator(c, true,
		func() {
			err := s.clock.WaitAdvance(30*time.Second, testing.ShortWait, 1)
			c.Assert(err, jc.ErrorIsNil)
		},
	)
}

func (s *K8sBrokerSuite) TestUpgradeOperatorForStatefulApp(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec, "operator/image-path")
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)
	podSpec.Containers[0].VolumeMounts = append(dataVolumeMounts(), core.VolumeMount{
		Name:      "database-appuuid",
		MountPath: "path/to/here",
	})
	workloadStatefulSet := unitStatefulSetArg(2, "workload-storage", podSpec)
	expectedPatchSS := apps.StatefulSet{Spec: unitStatefulSetArg(2, "workload-storage", podSpec).Spec}
	upgradedInitContainer := initContainers()[0]
	upgradedInitContainer.Image = "jujud-operator:6.6.6"
	expectedPatchSS.Spec.Template.Spec.InitContainers = []core.Container{upgradedInitContainer}
	expectedPatchSSData, err := json.Marshal(expectedPatchSS)
	c.Assert(err, jc.ErrorIsNil)

	s.assertUpgradeOperator(c, false, func() {},
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{}).
			Return(workloadStatefulSet, nil),
		s.mockStatefulSets.EXPECT().Patch("app-name", k8stypes.StrategicMergePatchType, expectedPatchSSData).
			Return(nil, nil),
	)
}

func (s *K8sBrokerSuite) TestUpgradeOperatorForStatelessApp(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec, "operator/image-path")
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)

	workloadDeployment := &apps.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"juju-app": "app-name"},
			Annotations: map[string]string{
				"fred":               "mary",
				"juju.io/controller": testing.ControllerTag.Id(),
				"juju-app-uuid":      "appuuid",
			}},
		Spec: apps.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			RevisionHistoryLimit: int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						"juju-app": "app-name",
					},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"fred":               "mary",
						"juju.io/controller": testing.ControllerTag.Id(),
					},
				},
				Spec: podSpec,
			},
		},
	}
	expectedPatchDeployment := apps.Deployment{Spec: workloadDeployment.Spec}
	upgradedInitContainer := initContainers()[0]
	upgradedInitContainer.Image = "jujud-operator:6.6.6"
	expectedPatchDeployment.Spec.Template.Spec.InitContainers = []core.Container{upgradedInitContainer}
	expectedPatchDeploymentData, err := json.Marshal(expectedPatchDeployment)
	c.Assert(err, jc.ErrorIsNil)

	s.assertUpgradeOperator(c, false, func() {},
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get("app-name", v1.GetOptions{}).
			Return(workloadDeployment, nil),
		s.mockDeployments.EXPECT().Patch("app-name", k8stypes.StrategicMergePatchType, expectedPatchDeploymentData).
			Return(nil, nil),
	)
}

func (s *K8sBrokerSuite) TestUpgradeOperatorForDaemonApp(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	basicPodSpec := getBasicPodspec()
	workloadSpec, err := provider.PrepareWorkloadSpec("app-name", "app-name", basicPodSpec, "operator/image-path")
	c.Assert(err, jc.ErrorIsNil)
	podSpec := provider.PodSpec(workloadSpec)

	workloadDaemonSet := &apps.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   "app-name",
			Labels: map[string]string{"juju-app": "app-name"},
			Annotations: map[string]string{
				"juju.io/controller": testing.ControllerTag.Id(),
				"juju-app-uuid":      "appuuid",
			}},
		Spec: apps.DaemonSetSpec{
			Selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"juju-app": "app-name"},
			},
			RevisionHistoryLimit: int32Ptr(0),
			Template: core.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: "app-name-",
					Labels:       map[string]string{"juju-app": "app-name"},
					Annotations: map[string]string{
						"apparmor.security.beta.kubernetes.io/pod": "runtime/default",
						"seccomp.security.beta.kubernetes.io/pod":  "docker/default",
						"juju.io/controller":                       testing.ControllerTag.Id(),
					},
				},
				Spec: podSpec,
			},
		},
	}
	expectedPatchDaemonSet := apps.DaemonSet{Spec: workloadDaemonSet.Spec}
	upgradedInitContainer := initContainers()[0]
	upgradedInitContainer.Image = "jujud-operator:6.6.6"
	expectedPatchDaemonSet.Spec.Template.Spec.InitContainers = []core.Container{upgradedInitContainer}
	expectedPatchDaemonSetData, err := json.Marshal(expectedPatchDaemonSet)
	c.Assert(err, jc.ErrorIsNil)

	s.assertUpgradeOperator(c, false, func() {},
		s.mockStatefulSets.EXPECT().Get("app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDeployments.EXPECT().Get("app-name", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockDaemonSets.EXPECT().Get("app-name", v1.GetOptions{}).
			Return(workloadDaemonSet, nil),
		s.mockDaemonSets.EXPECT().Patch("app-name", k8stypes.StrategicMergePatchType, expectedPatchDaemonSetData).
			Return(nil, nil),
	)
}

func (s *K8sBrokerSuite) TestUpgradeNothingToUpgrade(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockStatefulSets.EXPECT().Get("juju-operator-test-app", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockStatefulSets.EXPECT().Get("test-app-operator", v1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
	)

	err := s.broker.Upgrade("test-app", version.MustParse("6.6.6"))
	c.Assert(err, gc.ErrorMatches, `getting the existing statefulset "test-app-operator" to upgrade:  "test" not found`)
}
