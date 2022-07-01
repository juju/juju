// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/juju/juju/v3/caas/kubernetes/provider/exec"
)

type statusSuite struct {
	BaseSuite
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) TestStatus(c *gc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	t := time.Now()
	params := exec.StatusParams{
		PodName: "gitlab-k8s-uid",
	}
	pod := core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{Name: "gitlab-container"},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
			ContainerStatuses: []core.ContainerStatus{
				{Name: "gitlab-container", State: core.ContainerState{Running: &core.ContainerStateRunning{StartedAt: metav1.Time{t}}}},
			},
		},
	}
	pod.SetUID("gitlab-k8s-uid")
	pod.SetName("gitlab-k8s-0")

	gomock.InOrder(
		s.mockPodGetter.EXPECT().Get(gomock.Any(), "gitlab-k8s-uid", metav1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockPodGetter.EXPECT().List(gomock.Any(), metav1.ListOptions{}).
			Return(&core.PodList{Items: []core.Pod{pod}}, nil),
	)

	status, err := s.execClient.Status(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.DeepEquals, &exec.Status{
		PodName: "gitlab-k8s-0",
		ContainerStatus: []exec.ContainerStatus{
			{
				Name:               "gitlab-container",
				Running:            true,
				StartedAt:          t,
				InitContainer:      false,
				EphemeralContainer: false,
			},
		},
	})
}

func (s *statusSuite) TestStatusInit(c *gc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	t := time.Now()
	params := exec.StatusParams{
		PodName: "gitlab-k8s-uid",
	}
	pod := core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{Name: "gitlab-container"},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
			InitContainerStatuses: []core.ContainerStatus{
				{Name: "gitlab-container", State: core.ContainerState{Running: &core.ContainerStateRunning{StartedAt: metav1.Time{t}}}},
			},
		},
	}
	pod.SetUID("gitlab-k8s-uid")
	pod.SetName("gitlab-k8s-0")

	gomock.InOrder(
		s.mockPodGetter.EXPECT().Get(gomock.Any(), "gitlab-k8s-uid", metav1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockPodGetter.EXPECT().List(gomock.Any(), metav1.ListOptions{}).
			Return(&core.PodList{Items: []core.Pod{pod}}, nil),
	)

	status, err := s.execClient.Status(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.DeepEquals, &exec.Status{
		PodName: "gitlab-k8s-0",
		ContainerStatus: []exec.ContainerStatus{
			{
				Name:               "gitlab-container",
				Running:            true,
				StartedAt:          t,
				InitContainer:      true,
				EphemeralContainer: false,
			},
		},
	})
}

func (s *statusSuite) TestStatusEphemeral(c *gc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	t := time.Now()
	params := exec.StatusParams{
		PodName: "gitlab-k8s-uid",
	}
	pod := core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{Name: "gitlab-container"},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
			EphemeralContainerStatuses: []core.ContainerStatus{
				{Name: "gitlab-container", State: core.ContainerState{Running: &core.ContainerStateRunning{StartedAt: metav1.Time{t}}}},
			},
		},
	}
	pod.SetUID("gitlab-k8s-uid")
	pod.SetName("gitlab-k8s-0")

	gomock.InOrder(
		s.mockPodGetter.EXPECT().Get(gomock.Any(), "gitlab-k8s-uid", metav1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockPodGetter.EXPECT().List(gomock.Any(), metav1.ListOptions{}).
			Return(&core.PodList{Items: []core.Pod{pod}}, nil),
	)

	status, err := s.execClient.Status(params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.DeepEquals, &exec.Status{
		PodName: "gitlab-k8s-0",
		ContainerStatus: []exec.ContainerStatus{
			{
				Name:               "gitlab-container",
				Running:            true,
				StartedAt:          t,
				InitContainer:      false,
				EphemeralContainer: true,
			},
		},
	})
}

func (s *statusSuite) TestStatusPodNotFound(c *gc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	params := exec.StatusParams{
		PodName: "gitlab-k8s-uid",
	}

	gomock.InOrder(
		s.mockPodGetter.EXPECT().Get(gomock.Any(), "gitlab-k8s-uid", metav1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockPodGetter.EXPECT().List(gomock.Any(), metav1.ListOptions{}).
			Return(&core.PodList{}, nil),
	)

	status, err := s.execClient.Status(params)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(status, gc.IsNil)
}
