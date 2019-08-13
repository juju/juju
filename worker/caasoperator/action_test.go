// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/caasoperator/mocks"
	"github.com/juju/juju/worker/uniter"
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

	baseDir := c.MkDir()
	operatorPaths := caasoperator.NewPaths(baseDir, names.NewApplicationTag("gitlab-k8s"))
	unitPaths := uniter.NewPaths(baseDir, names.NewUnitTag("gitlab-k8s/0"), true)
	for _, p := range []string{
		operatorPaths.GetCharmDir(),
		unitPaths.GetCharmDir(),

		operatorPaths.GetToolsDir(),
		unitPaths.GetToolsDir(),
	} {
		err := os.MkdirAll(p, 0700)
		c.Check(err, jc.ErrorIsNil)
	}
	err := utils.AtomicWriteFile(filepath.Join(operatorPaths.GetToolsDir(), "jujud"), []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)

	runnerExecFunc := caasoperator.GetNewRunnerExecutor(s.executor, operatorPaths)(s.unitAPI, unitPaths)
	cancel := make(<-chan struct{}, 1)
	out := bytes.NewBufferString("")
	gomock.InOrder(
		s.unitAPI.EXPECT().Refresh().Times(1).Return(nil),
		s.unitAPI.EXPECT().ProviderID().Times(1).Return("gitlab-xxxx"),
		s.unitAPI.EXPECT().Name().Times(1).Return("gitlab-k8s/0"),

		s.executor.EXPECT().Exec(
			exec.ExecParams{
				PodName:  "gitlab-xxxx",
				Commands: []string{"test", "-d", baseDir + "/agents/application-gitlab-k8s", "||", "mkdir", "-p", baseDir + "/agents/application-gitlab-k8s"},
				Stdout:   out,
				Stderr:   out,
			}, cancel,
		).Times(1).Return(nil),
		s.executor.EXPECT().Copy(
			exec.CopyParam{
				Src: exec.FileResource{
					Path: baseDir + "/agents/application-gitlab-k8s/charm",
				},
				Dest: exec.FileResource{
					Path:    baseDir + "/agents/application-gitlab-k8s",
					PodName: "gitlab-xxxx",
				},
			}, cancel,
		).Times(1).Return(nil),

		s.executor.EXPECT().Exec(
			exec.ExecParams{
				PodName:  "gitlab-xxxx",
				Commands: []string{"test", "-d", baseDir + "/agents/unit-gitlab-k8s-0", "||", "mkdir", "-p", baseDir + "/agents/unit-gitlab-k8s-0"},
				Stdout:   out,
				Stderr:   out,
			}, cancel,
		).Times(1).Return(nil),
		s.executor.EXPECT().Copy(
			exec.CopyParam{
				Src: exec.FileResource{
					Path: baseDir + "/agents/unit-gitlab-k8s-0/charm",
				},
				Dest: exec.FileResource{
					Path:    baseDir + "/agents/unit-gitlab-k8s-0",
					PodName: "gitlab-xxxx",
				},
			}, cancel,
		).Times(1).Return(nil),

		s.executor.EXPECT().Exec(
			exec.ExecParams{
				PodName:  "gitlab-xxxx",
				Commands: []string{"test", "-d", baseDir + "/tools", "||", "mkdir", "-p", baseDir + "/tools"},
				Stdout:   out,
				Stderr:   out,
			}, cancel,
		).Times(1).Return(nil),
		s.executor.EXPECT().Copy(
			exec.CopyParam{
				Src: exec.FileResource{
					Path: baseDir + "/tools/jujud",
				},
				Dest: exec.FileResource{
					Path:    baseDir + "/tools",
					PodName: "gitlab-xxxx",
				},
			}, cancel,
		).Times(1).Return(nil),

		s.executor.EXPECT().Exec(
			exec.ExecParams{
				PodName:  "gitlab-xxxx",
				Commands: []string{"test", "-d", baseDir + "/tools", "||", "mkdir", "-p", baseDir + "/tools"},
				Stdout:   out,
				Stderr:   out,
			}, cancel,
		).Times(1).Return(nil),
		s.executor.EXPECT().Copy(
			exec.CopyParam{
				Src: exec.FileResource{
					Path: baseDir + "/tools/unit-gitlab-k8s-0",
				},
				Dest: exec.FileResource{
					Path:    baseDir + "/tools",
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

	_, err = runnerExecFunc(
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
