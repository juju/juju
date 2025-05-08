// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec_test

import (
	"archive/tar"
	"bytes"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

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

func (s *execSuite) TestFileResourceValidate(c *tc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()
	c.Assert((&exec.FileResource{}).Validate(), tc.ErrorMatches, `path was missing`)
}

func (s *execSuite) TestCopyParamsValidate(c *tc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	s.suiteMocks.EXPECT().RemoteCmdExecutorGetter(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(s.mockRemoteCmdExecutor, nil)

	type testcase struct {
		Params exec.CopyParams
		Err    string
	}
	for _, testCase := range []testcase{
		{
			Params: exec.CopyParams{},
			Err:    "path was missing",
		},
		{
			Params: exec.CopyParams{
				Src: exec.FileResource{
					Path:    "",
					PodName: "",
				},
			},
			Err: "path was missing",
		},
		{
			Params: exec.CopyParams{
				Src: exec.FileResource{
					Path:    "/var/lib/juju/tools",
					PodName: "",
				},
				Dest: exec.FileResource{
					Path:    "",
					PodName: "",
				},
			},
			Err: "path was missing",
		},
		{
			Params: exec.CopyParams{
				Src: exec.FileResource{
					Path:    "/var/lib/juju/tools",
					PodName: "",
				},
				Dest: exec.FileResource{
					Path:    "/var/lib/juju/tools",
					PodName: "",
				},
			},
			Err: "copy either from pod to host or from host to pod",
		},
	} {
		c.Check(testCase.Params.Validate(), tc.ErrorMatches, testCase.Err)
	}

	// failed: can not copy from a pod to another pod.
	params := exec.CopyParams{
		Src: exec.FileResource{
			Path:    "/var/lib/juju/tools",
			PodName: "gitlab-k8s-0",
		},
		Dest: exec.FileResource{
			Path:    "/var/lib/juju/tools",
			PodName: "mariadb-k8s-0",
		},
	}
	c.Assert(params.Validate(), tc.ErrorMatches, "cross pods copy is not supported")
}

func (s *execSuite) TestCopyToPod(c *tc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	s.suiteMocks.EXPECT().RemoteCmdExecutorGetter(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(s.mockRemoteCmdExecutor, nil)

	srcPath, err := os.CreateTemp(c.MkDir(), "testfile")
	c.Assert(err, tc.ErrorIsNil)
	defer srcPath.Close()
	defer os.Remove(srcPath.Name())

	params := exec.CopyParams{
		Src: exec.FileResource{
			Path:    srcPath.Name(),
			PodName: "",
		},
		Dest: exec.FileResource{
			Path:    "/testdir",
			PodName: "gitlab-k8s-0",
		},
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
				{Name: "gitlab-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			},
		},
	}
	pod.SetName("gitlab-k8s-0")

	checkRemotePathRequest := rest.NewRequestWithClient(
		&url.URL{Path: "/path/"},
		"",
		rest.ClientContentConfig{GroupVersion: core.SchemeGroupVersion},
		nil,
	).Resource("pods").Name("gitlab-k8s-0").Namespace("test").
		SubResource("exec").Param("container", "gitlab-container").VersionedParams(
		&core.PodExecOptions{
			Container: "gitlab-container",
			Command:   []string{"test", "-d", srcPath.Name()},
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	copyRequest := rest.NewRequestWithClient(
		&url.URL{Path: "/path/"},
		"",
		rest.ClientContentConfig{GroupVersion: core.SchemeGroupVersion},
		nil,
	).Resource("pods").Name("gitlab-k8s-0").Namespace("test").
		SubResource("exec").Param("container", "gitlab-container").VersionedParams(
		&core.PodExecOptions{
			Container: "gitlab-container",
			Command:   []string{"tar", "-xmf", "-", "/testdir"},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	gomock.InOrder(
		// check remote path is dir or not.
		s.mockPodGetter.EXPECT().Get(gomock.Any(), "gitlab-k8s-0", metav1.GetOptions{}).Return(&pod, nil),
		s.restClient.EXPECT().Post().Return(checkRemotePathRequest),
		s.mockRemoteCmdExecutor.EXPECT().Stream(
			remotecommand.StreamOptions{
				Stdout: &stdout,
				Stderr: &stderr,
				Tty:    false,
			},
		).Return(nil),

		// copy files.
		s.mockPodGetter.EXPECT().Get(gomock.Any(), "gitlab-k8s-0", metav1.GetOptions{}).Return(&pod, nil),
		s.restClient.EXPECT().Post().Return(copyRequest),
		s.mockRemoteCmdExecutor.EXPECT().Stream(
			remotecommand.StreamOptions{
				Stdin:  s.pipReader,
				Stdout: &stdout,
				Stderr: &stderr,
				Tty:    false,
			},
		).Return(nil),
	)

	cancel := make(<-chan struct{}, 1)
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.execClient.Copy(context.Background(), params, cancel)
	}()
	select {
	case err := <-errChan:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for Copy return")
	}
}

func (s *execSuite) TestCopyFromPod(c *tc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	s.suiteMocks.EXPECT().RemoteCmdExecutorGetter(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(s.mockRemoteCmdExecutor, nil)

	srcPath, err := os.CreateTemp(c.MkDir(), "testfile")
	c.Assert(err, tc.ErrorIsNil)
	fileContent := `test data`
	_, err = srcPath.WriteString(fileContent)
	c.Assert(err, tc.ErrorIsNil)
	defer srcPath.Close()
	defer os.Remove(srcPath.Name())

	destPath := filepath.Join(c.MkDir(), "destFile")
	params := exec.CopyParams{
		Src: exec.FileResource{
			Path:    srcPath.Name(),
			PodName: "gitlab-k8s-0",
		},
		Dest: exec.FileResource{
			Path:    destPath,
			PodName: "",
		},
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
				{Name: "gitlab-container", State: core.ContainerState{Running: &core.ContainerStateRunning{}}},
			},
		},
	}
	pod.SetName("gitlab-k8s-0")

	copyRequest := rest.NewRequestWithClient(
		&url.URL{Path: "/path/"},
		"",
		rest.ClientContentConfig{GroupVersion: core.SchemeGroupVersion},
		nil,
	).Resource("pods").Name("gitlab-k8s-0").Namespace("test").
		SubResource("exec").Param("container", "gitlab-container").VersionedParams(
		&core.PodExecOptions{
			Container: "gitlab-container",
			Command:   []string{"tar", "cf", "-", srcPath.Name()},
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	var stderr bytes.Buffer
	gomock.InOrder(
		// copy files.
		s.mockPodGetter.EXPECT().Get(gomock.Any(), "gitlab-k8s-0", metav1.GetOptions{}).Return(&pod, nil),
		s.restClient.EXPECT().Post().Return(copyRequest),
		s.mockRemoteCmdExecutor.EXPECT().Stream(
			remotecommand.StreamOptions{
				Stdin:  nil,
				Stdout: s.pipWriter,
				Stderr: &stderr,
				Tty:    false,
			},
		).DoAndReturn(
			func(ops remotecommand.StreamOptions) error {
				tarWriter := tar.NewWriter(ops.Stdout)
				err = tarWriter.WriteHeader(&tar.Header{
					// tar strips the leading '/' if it's there.
					Name: strings.TrimLeft(srcPath.Name(), "/"),
					Mode: 0600,
					Size: int64(len(fileContent)),
				})
				c.Assert(err, tc.ErrorIsNil)
				_, err = tarWriter.Write([]byte(fileContent))
				c.Assert(err, tc.ErrorIsNil)
				err = tarWriter.Close()
				c.Assert(err, tc.ErrorIsNil)
				return nil
			},
		),
	)

	cancel := make(<-chan struct{})
	errChan := make(chan error)
	go func() {
		errChan <- s.execClient.Copy(context.Background(), params, cancel)
	}()
	select {
	case err := <-errChan:
		c.Assert(err, tc.ErrorIsNil)
		data, err := os.ReadFile(destPath)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(string(data), tc.DeepEquals, fileContent)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for Copy return")
	}
}
