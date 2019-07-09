// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"bytes"
	"crypto/rand"
	"time"

	"github.com/golang/mock/gomock"
	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/workertest"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	watch "k8s.io/apimachinery/pkg/watch"

	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/testing"
)

type imageSuite struct {
	BaseSuite
}

var _ = gc.Suite(&imageSuite{})

func (s *imageSuite) setupController(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	newK8sRestClientFunc := s.setupK8sRestClient(c, ctrl, "test")
	newK8sWatcherForTest := func(wi watch.Interface, name string, clock jujuclock.Clock) (*provider.KubernetesWatcher, error) {
		w, err := provider.NewKubernetesWatcher(wi, name, clock)
		c.Assert(err, jc.ErrorIsNil)
		<-w.Changes() // Consume initial event for testing.
		s.watchers = append(s.watchers, w)
		return w, err
	}
	s.setupBroker(c, ctrl, newK8sRestClientFunc, newK8sWatcherForTest)
	c.Assert(s.broker.GetCurrentNamespace(), jc.DeepEquals, "test")
	s.PatchValue(&rand.Reader, bytes.NewReader([]byte{
		0xf0, 0x0d, 0xba, 0xad,
	}))
	return ctrl
}

func (s *imageSuite) TestPullFail(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
	podSpec := core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Namespace: s.getNamespace(),
			Name:      "operator-image-prepull-f00dbaad",
		},
		Spec: core.PodSpec{
			RestartPolicy: core.RestartPolicyNever,
			Containers: []core.Container{
				core.Container{
					Name:            "jujud",
					Image:           "test/image/check",
					ImagePullPolicy: core.PullIfNotPresent,
					Command:         []string{"/opt/jujud"},
					Args:            []string{"version"},
				},
			},
		},
	}
	pod := podSpec
	pod.Status = core.PodStatus{
		Phase: core.PodPending,
		ContainerStatuses: []core.ContainerStatus{
			core.ContainerStatus{
				Name: "jujud",
				State: core.ContainerState{
					Waiting: &core.ContainerStateWaiting{
						Reason: "ImagePullBackOff",
					},
				},
			},
		},
	}
	podWatcher := s.k8sNewFakeWatcher()

	gomock.InOrder(
		s.mockPods.EXPECT().Watch(
			v1.ListOptions{
				FieldSelector:        "metadata.name=operator-image-prepull-f00dbaad",
				Watch:                true,
				IncludeUninitialized: true,
			},
		).
			Return(podWatcher, nil).Times(1),
		s.mockPods.EXPECT().Create(&podSpec).
			Return(&pod, nil).Times(1),
		s.mockPods.EXPECT().Delete("operator-image-prepull-f00dbaad", &v1.DeleteOptions{}).
			Return(nil).Times(1),
	)

	err := provider.OperatorImagePrepullCheck(s.broker, "test/image/check")
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(s.watchers, gc.HasLen, 1)
	c.Assert(workertest.CheckKilled(c, s.watchers[0]), jc.ErrorIsNil)
	c.Assert(podWatcher.IsStopped(), jc.IsTrue)
}

func (s *imageSuite) TestPullSuccess(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
	podSpec := core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Namespace: s.getNamespace(),
			Name:      "operator-image-prepull-f00dbaad",
		},
		Spec: core.PodSpec{
			RestartPolicy: core.RestartPolicyNever,
			Containers: []core.Container{
				core.Container{
					Name:            "jujud",
					Image:           "test/image/check",
					ImagePullPolicy: core.PullIfNotPresent,
					Command:         []string{"/opt/jujud"},
					Args:            []string{"version"},
				},
			},
		},
	}
	pod := podSpec
	pod.Status = core.PodStatus{
		Phase: core.PodSucceeded,
	}
	podWatcher := s.k8sNewFakeWatcher()

	gomock.InOrder(
		s.mockPods.EXPECT().Watch(
			v1.ListOptions{
				FieldSelector:        "metadata.name=operator-image-prepull-f00dbaad",
				Watch:                true,
				IncludeUninitialized: true,
			},
		).
			Return(podWatcher, nil).Times(1),
		s.mockPods.EXPECT().Create(&podSpec).
			Return(&pod, nil).Times(1),
		s.mockPods.EXPECT().Delete("operator-image-prepull-f00dbaad", &v1.DeleteOptions{}).
			Return(nil).Times(1),
	)

	err := provider.OperatorImagePrepullCheck(s.broker, "test/image/check")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.watchers, gc.HasLen, 1)
	c.Assert(workertest.CheckKilled(c, s.watchers[0]), jc.ErrorIsNil)
	c.Assert(podWatcher.IsStopped(), jc.IsTrue)
}

func (s *imageSuite) TestPullWatcherSuccess(c *gc.C) {
	ctrl := s.setupController(c)
	defer ctrl.Finish()
	podSpec := core.Pod{
		ObjectMeta: v1.ObjectMeta{
			Namespace: s.getNamespace(),
			Name:      "operator-image-prepull-f00dbaad",
		},
		Spec: core.PodSpec{
			RestartPolicy: core.RestartPolicyNever,
			Containers: []core.Container{
				core.Container{
					Name:            "jujud",
					Image:           "test/image/check",
					ImagePullPolicy: core.PullIfNotPresent,
					Command:         []string{"/opt/jujud"},
					Args:            []string{"version"},
				},
			},
		},
	}
	pod0 := podSpec
	pod0.Status = core.PodStatus{
		Phase: core.PodPending,
	}
	pod1 := podSpec
	pod1.Status = core.PodStatus{
		Phase: core.PodPending,
		ContainerStatuses: []core.ContainerStatus{
			core.ContainerStatus{
				Name: "jujud",
				State: core.ContainerState{
					Waiting: &core.ContainerStateWaiting{
						Reason: "ErrImagePull",
					},
				},
			},
		},
	}
	pod2 := podSpec
	pod2.Status = core.PodStatus{
		Phase: core.PodSucceeded,
	}

	podWatcher := s.k8sNewFakeWatcher()
	gomock.InOrder(
		s.mockPods.EXPECT().Watch(
			v1.ListOptions{
				FieldSelector:        "metadata.name=operator-image-prepull-f00dbaad",
				Watch:                true,
				IncludeUninitialized: true,
			},
		).
			Return(podWatcher, nil).Times(1),
		s.mockPods.EXPECT().Create(&podSpec).
			DoAndReturn(func(_ *core.Pod) (*core.Pod, error) {
				podWatcher.Action("PodCreated", nil)
				s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
				return &pod0, nil
			}).Times(1),
		s.mockPods.EXPECT().Get("operator-image-prepull-f00dbaad", metav1.GetOptions{IncludeUninitialized: true}).
			DoAndReturn(func(_ string, _ metav1.GetOptions) (*core.Pod, error) {
				podWatcher.Action("PodPending", nil)
				s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
				return &pod1, nil
			}).Times(1),
		s.mockPods.EXPECT().Get("operator-image-prepull-f00dbaad", metav1.GetOptions{IncludeUninitialized: true}).
			DoAndReturn(func(_ string, _ metav1.GetOptions) (*core.Pod, error) {
				podWatcher.Action("PodSucceeded", nil)
				s.clock.WaitAdvance(time.Second, testing.ShortWait, 1)
				return &pod2, nil
			}).Times(1),
		s.mockPods.EXPECT().Delete("operator-image-prepull-f00dbaad", &v1.DeleteOptions{}).
			Return(nil).Times(1),
	)

	err := provider.OperatorImagePrepullCheck(s.broker, "test/image/check")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.watchers, gc.HasLen, 1)
	c.Assert(workertest.CheckKilled(c, s.watchers[0]), jc.ErrorIsNil)
	c.Assert(podWatcher.IsStopped(), jc.IsTrue)
}
