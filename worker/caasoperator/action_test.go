// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"bytes"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/caasoperator/mocks"
	"github.com/juju/juju/worker/uniter"
)

type actionSuite struct {
	testing.BaseSuite

	executor *mocks.MockExecutor
	client   *fakeClient
}

var _ = gc.Suite(&actionSuite{})

func (s *actionSuite) setupExecClient(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.executor = mocks.NewMockExecutor(ctrl)
	s.client = &fakeClient{
		life: life.Alive,
	}
	return ctrl
}

func (s *actionSuite) TestRunnerExecFunc(c *gc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	gitlabTag := names.NewUnitTag("gitlab-k8s/0")
	runnerExecFuncGetter := caasoperator.GetNewRunnerExecutor(s.executor, s.client, "/var/lib/juju")(gitlabTag, uniter.Paths{})
	cancel := make(<-chan struct{}, 1)
	var stdout, stderr bytes.Buffer
	gomock.InOrder(
		s.executor.EXPECT().Exec(exec.ExecParams{
			PodName:  "gitlab-xxxx",
			Commands: []string{"mkdir", "-p", "/var/lib/juju"},
			Stdout:   &stdout,
			Stderr:   &stderr,
		}, cancel).Times(1).Return(nil),
		s.executor.EXPECT().Copy(exec.CopyParam{
			Src: exec.FileResource{
				Path: "/var/lib/juju/agents",
			},
			Dest: exec.FileResource{
				Path:    "/var/lib/juju/agents",
				PodName: "gitlab-xxxx",
			},
		}, cancel).Times(1).Return(nil),
		s.executor.EXPECT().Copy(exec.CopyParam{
			Src: exec.FileResource{
				Path: "/var/lib/juju/tools",
			},
			Dest: exec.FileResource{
				Path:    "/var/lib/juju/tools",
				PodName: "gitlab-xxxx",
			},
		}, cancel).Times(1).Return(nil),
		s.executor.EXPECT().Exec(exec.ExecParams{
			PodName:  "gitlab-xxxx",
			Commands: []string{"storage-list"},
			Env:      []string{"AAAA=1111"},
			Stdout:   &stdout,
			Stderr:   &stderr,
		}, cancel).Times(1).Return(nil),
	)

	_, err := runnerExecFuncGetter(
		[]string{"storage-list"}, []string{"AAAA=1111"}, "", nil, nil, cancel,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.client.CheckCall(c, 0, "Units", []names.Tag{gitlabTag})
}
