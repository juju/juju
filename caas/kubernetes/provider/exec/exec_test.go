// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec_test

import (
	"bytes"
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/juju/juju/caas/kubernetes/provider/exec"
	coretesting "github.com/juju/juju/internal/testing"
)

type execSuite struct {
	BaseSuite
}

var _ = tc.Suite(&execSuite{})

func (s *execSuite) TestExecParamsValidateCommandsAndPodName(c *tc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	type testcase struct {
		Params  exec.ExecParams
		Err     string
		PodName string
	}

	for _, tc := range []testcase{
		{
			Params: exec.ExecParams{},
			Err:    "empty commands not valid",
		},
		{
			Params: exec.ExecParams{
				Commands: []string{"echo", "'hello world'"},
				PodName:  "",
			},
			Err: `podName "" not valid`,
		},
		{
			Params: exec.ExecParams{
				Commands: []string{"echo", "'hello world'"},
				PodName:  "cm/gitlab-k8s-0",
			},
			Err: `podName "cm/gitlab-k8s-0" not valid`,
		},
		{
			Params: exec.ExecParams{
				Commands: []string{"echo", "'hello world'"},
				PodName:  "cm/",
			},
			Err: `podName "cm/" not valid`,
		},
		{
			Params: exec.ExecParams{
				Commands: []string{"echo", "'hello world'"},
				PodName:  "pod/",
			},
			Err: `podName "pod/" not valid`,
		},
	} {
		c.Check(tc.Params.Validate(context.Background(), s.mockPodGetter), tc.ErrorMatches, tc.Err)
	}

}

func (s *execSuite) TestProcessEnv(c *tc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	res, err := exec.ProcessEnv(
		[]string{
			"AAA=1", "BBB=1 2", "CCC=1\n2", "DDD=1='2'", "EEE=1;2;\"foo\"",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.Equals, "export AAA=1; export BBB='1 2'; export CCC='1\n2'; export DDD=1=\\'2\\'; export EEE=1\\;2\\;\\\"foo\\\"; ")
}

func (s *execSuite) TestExecParamsValidatePodContainerExistence(c *tc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	s.suiteMocks.EXPECT().RemoteCmdExecutorGetter(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(s.mockRemoteCmdExecutor, nil)

	// failed - completed pod.
	params := exec.ExecParams{
		Commands: []string{"echo", "'hello world'"},
		PodName:  "gitlab-k8s-uid",
	}
	pod := core.Pod{
		Spec: core.PodSpec{
			InitContainers: []core.Container{
				{Name: "gitlab-container"},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodSucceeded,
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
	c.Assert(params.Validate(context.Background(), s.mockPodGetter), tc.ErrorMatches, `cannot exec into a container within the "Succeeded" pod "gitlab-k8s-0"`)

	// failed - failed pod
	params = exec.ExecParams{
		Commands: []string{"echo", "'hello world'"},
		PodName:  "gitlab-k8s-uid",
	}
	pod = core.Pod{
		Spec: core.PodSpec{
			InitContainers: []core.Container{
				{Name: "gitlab-container"},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodFailed,
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
	c.Assert(params.Validate(context.Background(), s.mockPodGetter), tc.ErrorMatches, `cannot exec into a container within the "Failed" pod "gitlab-k8s-0"`)

	// failed - containerName not found
	params = exec.ExecParams{
		Commands:      []string{"echo", "'hello world'"},
		PodName:       "gitlab-k8s-uid",
		ContainerName: "non-existing-container-name",
	}
	pod = core.Pod{
		Spec: core.PodSpec{
			InitContainers: []core.Container{
				{Name: "gitlab-container"},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
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
	c.Assert(params.Validate(context.Background(), s.mockPodGetter), tc.ErrorMatches, `container "non-existing-container-name" not found`)

	// all good - container name specified for init container
	params = exec.ExecParams{
		Commands:      []string{"echo", "'hello world'"},
		PodName:       "gitlab-k8s-uid",
		ContainerName: "gitlab-container",
	}
	pod = core.Pod{
		Spec: core.PodSpec{
			InitContainers: []core.Container{
				{Name: "gitlab-container"},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodPending,
			InitContainerStatuses: []core.ContainerStatus{
				{Name: "gitlab-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
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
	c.Assert(params.Validate(context.Background(), s.mockPodGetter), tc.ErrorIsNil)

	// all good - container name specified.
	params = exec.ExecParams{
		Commands:      []string{"echo", "'hello world'"},
		PodName:       "gitlab-k8s-uid",
		ContainerName: "gitlab-container",
	}
	pod = core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{Name: "gitlab-container"},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
			ContainerStatuses: []core.ContainerStatus{
				{Name: "gitlab-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
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
	c.Assert(params.Validate(context.Background(), s.mockPodGetter), tc.ErrorIsNil)

	// non fatal error - container not running - container name specified.
	params = exec.ExecParams{
		Commands:      []string{"echo", "'hello world'"},
		PodName:       "gitlab-k8s-uid",
		ContainerName: "gitlab-container",
	}
	pod = core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{Name: "gitlab-container"},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
			ContainerStatuses: []core.ContainerStatus{
				{Name: "gitlab-container", State: core.ContainerState{Waiting: &core.ContainerStateWaiting{}}},
			},
		},
	}
	pod.SetUID("gitlab-k8s-uid")
	pod.SetName("gitlab-k8s-0")
	gomock.InOrder(
		s.mockPodGetter.EXPECT().Get(gomock.Any(), "gitlab-k8s-uid", metav1.GetOptions{}).Times(1).
			Return(nil, s.k8sNotFoundError()),
		s.mockPodGetter.EXPECT().List(gomock.Any(), metav1.ListOptions{}).Times(1).
			Return(&core.PodList{Items: []core.Pod{pod}}, nil),
	)
	c.Assert(params.Validate(context.Background(), s.mockPodGetter), tc.ErrorMatches, `container \"gitlab-container\" not running`)

	// all good - no container name specified, pick the 1st container.
	params = exec.ExecParams{
		Commands: []string{"echo", "'hello world'"},
		PodName:  "gitlab-k8s-uid",
	}
	c.Assert(params.ContainerName, tc.Equals, "")
	pod = core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{Name: "gitlab-container"},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
			ContainerStatuses: []core.ContainerStatus{
				{Name: "gitlab-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
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
	c.Assert(params.Validate(context.Background(), s.mockPodGetter), tc.ErrorIsNil)
	c.Assert(params.ContainerName, tc.Equals, "gitlab-container")
}

func (s *execSuite) TestExec(c *tc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	s.suiteMocks.EXPECT().RemoteCmdExecutorGetter(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(s.mockRemoteCmdExecutor, nil)

	var stdin, stdout, stderr bytes.Buffer
	params := exec.ExecParams{
		Commands: []string{"echo", "'hello world'"},
		PodName:  "gitlab-k8s-uid",
		Stdout:   &stdout,
		Stderr:   &stderr,
		Stdin:    &stdin,
	}
	c.Assert(params.ContainerName, tc.Equals, "")
	pod := core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{Name: "gitlab-container"},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
			ContainerStatuses: []core.ContainerStatus{
				{Name: "gitlab-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			},
		},
	}
	pod.SetUID("gitlab-k8s-uid")
	pod.SetName("gitlab-k8s-0")

	request := rest.NewRequestWithClient(
		&url.URL{Path: "/path/"},
		"",
		rest.ClientContentConfig{GroupVersion: core.SchemeGroupVersion},
		nil,
	).Resource("pods").Name("gitlab-k8s-0").Namespace("test").
		SubResource("exec").Param("container", "gitlab-container").VersionedParams(
		&core.PodExecOptions{
			Container: "gitlab-container",
			Command:   []string{""},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)
	gomock.InOrder(
		s.mockPodGetter.EXPECT().Get(gomock.Any(), "gitlab-k8s-uid", metav1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockPodGetter.EXPECT().List(gomock.Any(), metav1.ListOptions{}).
			Return(&core.PodList{Items: []core.Pod{pod}}, nil),

		s.restClient.EXPECT().Post().Return(request),
		s.mockRemoteCmdExecutor.EXPECT().Stream(
			remotecommand.StreamOptions{
				Stdin:  &stdin,
				Stdout: &stdout,
				Stderr: &stderr,
				Tty:    false,
			},
		).Return(nil),
	)

	cancel := make(<-chan struct{}, 1)
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.execClient.Exec(context.Background(), params, cancel)
	}()

	select {
	case err := <-errChan:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for Exec return")
	}
}

func (s *execSuite) TestExecCancel(c *tc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	s.PatchValue(exec.RandomString, func(n int, validRunes []rune) string {
		return "random"
	})

	var stdin, stdout, stderr bytes.Buffer
	params := exec.ExecParams{
		Commands: []string{"echo", "'hello world'"},
		PodName:  "gitlab-k8s-uid",
		Stdout:   &stdout,
		Stderr:   &stderr,
		Stdin:    &stdin,
	}
	c.Assert(params.ContainerName, tc.Equals, "")
	pod := core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{Name: "gitlab-container"},
			},
		},
		Status: core.PodStatus{
			Phase: core.PodRunning,
			ContainerStatuses: []core.ContainerStatus{
				{Name: "gitlab-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			},
		},
	}
	pod.SetUID("gitlab-k8s-uid")
	pod.SetName("gitlab-k8s-0")

	cancel := make(chan struct{})
	wait := make(chan struct{})
	mut := sync.Mutex{}
	callNum := 0

	urls := []string{
		"/path/namespaces/test/pods/gitlab-k8s-0/exec?command=sh&command=-c&command=mkdir+-p+%2Ftmp%3B+echo+%24%24+%3E+%2Ftmp%2Frandom.pid%3B+exec+sh+-c+%27echo+%27%5C%27%27hello+world%27%5C%27%3B+&container=gitlab-container&container=gitlab-container&stderr=true&stdin=true&stdout=true",
		"/path/namespaces/test/pods/gitlab-k8s-0/exec?command=sh&command=-c&command=kill+-15+%24%28cat+%2Ftmp%2Frandom.pid%29&container=gitlab-container&container=gitlab-container&stderr=true&stdout=true",
		"/path/namespaces/test/pods/gitlab-k8s-0/exec?command=sh&command=-c&command=kill+-9+-%24%28cat+%2Ftmp%2Frandom.pid%29&container=gitlab-container&container=gitlab-container&stderr=true&stdout=true",
		"/path/namespaces/test/pods/gitlab-k8s-0/exec?command=sh&command=-c&command=kill+-9+-%24%28cat+%2Ftmp%2Frandom.pid%29&container=gitlab-container&container=gitlab-container&stderr=true&stdout=true",
	}
	waitTime := 11 * time.Second
	requests := []*rest.Request{}
	for i := 0; i < len(urls); i++ {
		request := rest.NewRequestWithClient(
			&url.URL{Path: "/path/"},
			"",
			rest.ClientContentConfig{GroupVersion: core.SchemeGroupVersion},
			nil,
		)
		requests = append(requests, request)
	}

	gomock.InOrder(
		s.mockPodGetter.EXPECT().Get(gomock.Any(), "gitlab-k8s-uid", metav1.GetOptions{}).
			Return(nil, s.k8sNotFoundError()),
		s.mockPodGetter.EXPECT().List(gomock.Any(), metav1.ListOptions{}).
			Return(&core.PodList{Items: []core.Pod{pod}}, nil),
	)

	s.restClient.EXPECT().Post().AnyTimes().DoAndReturn(func() *rest.Request {
		mut.Lock()
		defer mut.Unlock()
		c.Assert(callNum, tc.LessThan, len(requests))
		return requests[callNum]
	})
	s.suiteMocks.EXPECT().RemoteCmdExecutorGetter(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(config *rest.Config, method string, url *url.URL) (remotecommand.Executor, error) {
			mut.Lock()
			defer mut.Unlock()
			c.Assert(callNum, tc.LessThan, len(urls))
			c.Check(url.String(), tc.Equals, urls[callNum])
			return s.mockRemoteCmdExecutor, nil
		})
	s.mockRemoteCmdExecutor.EXPECT().Stream(gomock.Any()).AnyTimes().DoAndReturn(func(opts remotecommand.StreamOptions) error {
		mut.Lock()
		currentCall := callNum
		callNum++
		mut.Unlock()
		switch currentCall {
		case 0:
			close(cancel)
			err := s.clock.WaitAdvance(waitTime, coretesting.LongWait, 1)
			c.Assert(err, tc.ErrorIsNil)
		case 1:
		case 2:
		case 3:
			close(wait)
		}
		return nil
	})

	errChan := make(chan error, 1)
	go func() {
		errChan <- s.execClient.Exec(context.Background(), params, cancel)
	}()

	select {
	case err := <-errChan:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for Exec return")
	}
}

func (s *execSuite) TestErrorHandling(c *tc.C) {
	err := exec.HandleContainerNotFoundError(errors.New(`unable to upgrade connection: container not found ("mariadb-k8s")`))
	c.Assert(err, tc.FitsTypeOf, &exec.ContainerNotRunningError{})
	err = exec.HandleContainerNotFoundError(errors.New(`wow`))
	c.Assert(err, tc.ErrorMatches, "wow")
}
