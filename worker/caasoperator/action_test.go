// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"bytes"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/caasoperator/mocks"
	"github.com/juju/juju/worker/uniter/runner"
)

type actionSuite struct {
	testing.BaseSuite

	executor *mocks.MockExecutor
	unitAPI  *mocks.MockProviderIDGetter
}

var _ = gc.Suite(&actionSuite{})

func (s *actionSuite) setupExecClient(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.executor = mocks.NewMockExecutor(ctrl)
	s.unitAPI = mocks.NewMockProviderIDGetter(ctrl)
	return ctrl
}

func (s *actionSuite) TestRunnerExecFunc(c *gc.C) {
	ctrl := s.setupExecClient(c)
	defer ctrl.Finish()

	runnerExecFunc := caasoperator.GetNewRunnerExecutor(s.executor, "/var/lib/juju")(s.unitAPI)
	cancel := make(<-chan struct{}, 1)
	out := bytes.NewBufferString("")
	gomock.InOrder(
		s.unitAPI.EXPECT().Refresh().Times(1).Return(nil),
		s.unitAPI.EXPECT().ProviderID().Times(1).Return("gitlab-xxxx"),
		s.unitAPI.EXPECT().Name().Times(1).Return("gitlab-k8s/0"),

		s.executor.EXPECT().Exec(
			exec.ExecParams{
				PodName:  "gitlab-xxxx",
				Commands: []string{"mkdir", "-p", "/var/lib/juju"},
				Stdout:   out,
				Stderr:   out,
			}, cancel,
		).Times(1).Return(nil),
		s.executor.EXPECT().Copy(
			exec.CopyParam{
				Src: exec.FileResource{
					Path: "/var/lib/juju/agents",
				},
				Dest: exec.FileResource{
					Path:    "/var/lib/juju/agents",
					PodName: "gitlab-xxxx",
				},
			}, cancel,
		).Times(1).Return(nil),
		s.executor.EXPECT().Copy(
			exec.CopyParam{
				Src: exec.FileResource{
					Path: "/var/lib/juju/tools",
				},
				Dest: exec.FileResource{
					Path:    "/var/lib/juju/tools",
					PodName: "gitlab-xxxx",
				},
			}, cancel,
		).Times(1).Return(nil),
		s.executor.EXPECT().Exec(
			exec.ExecParams{
				PodName:  "gitlab-xxxx",
				Commands: []string{"storage-list"},
				Env:      []string{"AAAA=1111"},
				Stdout:   out,
				Stderr:   out,
			}, cancel,
		).Times(1).Return(nil),
	)

	_, err := runnerExecFunc(
		runner.ExecParams{
			Commands: []string{"storage-list"},
			Env:      []string{"AAAA=1111"},
			Stdout:   out,
			Stderr:   out,
			Cancel:   cancel,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}
