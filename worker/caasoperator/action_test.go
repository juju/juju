// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	utilexec "github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	k8sexec "k8s.io/client-go/util/exec"

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

func (s *actionSuite) symlinkJujudCommand(out *bytes.Buffer, baseDir string, file string) exec.ExecParams {
	cmd := fmt.Sprintf("test -f %s || ln -s "+baseDir+"/tools/unit-gitlab-k8s-0/jujud %s", file, file)
	return exec.ExecParams{
		PodName:  "gitlab-xxxx",
		Commands: strings.Split(cmd, " "),
		Stdout:   out,
		Stderr:   out,
	}
}

func (s *actionSuite) TestRunnerExecFunc(c *gc.C) {
	s.assertRunnerExecFunc(c, "")
}

func (s *actionSuite) TestRunnerExecFuncWithError(c *gc.C) {
	s.assertRunnerExecFunc(c, "boom")
}

func (s *actionSuite) assertRunnerExecFunc(c *gc.C, errMsg string) {
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

	calls := []*gomock.Call{
		s.unitAPI.EXPECT().Refresh().Times(1).Return(nil),
		s.unitAPI.EXPECT().ProviderID().Times(1).Return("gitlab-xxxx"),
		s.unitAPI.EXPECT().Name().Times(1).Return("gitlab-k8s/0"),

		s.executor.EXPECT().Exec(
			exec.ExecParams{
				PodName:  "gitlab-xxxx",
				Commands: []string{"test", "-f", baseDir + "/agents/unit-gitlab-k8s-0/operator.yaml", "||", "echo notfound"},
				Stdout:   out,
				Stderr:   out,
			}, cancel,
		).Times(1).DoAndReturn(func(...interface{}) error {
			out.WriteString("notfound")
			return nil
		}),

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
					Path: baseDir + "/agents/application-gitlab-k8s/charm",
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
				Commands: []string{"test", "-d", baseDir + "/tools/unit-gitlab-k8s-0", "||", "mkdir", "-p", baseDir + "/tools/unit-gitlab-k8s-0"},
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
					Path:    baseDir + "/tools/unit-gitlab-k8s-0",
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
					Path: filepath.Join(os.TempDir(), "operator.yaml"),
				},
				Dest: exec.FileResource{
					Path:    baseDir + "/agents/unit-gitlab-k8s-0/operator.yaml",
					PodName: "gitlab-xxxx",
				},
			}, cancel,
		).Times(1).Return(nil),
	}
	calls = append(calls,
		s.executor.EXPECT().Exec(s.symlinkJujudCommand(out, baseDir, "/usr/bin/juju-run"),
			cancel).Times(1).Return(nil))
	for _, cmdName := range jujuc.CommandNames() {
		s.executor.EXPECT().Exec(s.symlinkJujudCommand(out, baseDir, baseDir+"/tools/unit-gitlab-k8s-0/"+cmdName),
			cancel).Times(1).Return(nil)
	}

	expectedCode := 0
	var exitErr error
	if errMsg != "" {
		exitErr = errors.Trace(k8sexec.CodeExitError{Code: 3, Err: errors.New(errMsg)})
		expectedCode = 3
	}
	calls = append(calls,
		s.executor.EXPECT().Exec(
			exec.ExecParams{
				PodName:  "gitlab-xxxx",
				Commands: []string{"storage-list"},
				Env:      []string{"AAAA=1111"},
				Stdout:   out,
				Stderr:   out,
			}, cancel,
		).Times(1).DoAndReturn(func(...interface{}) error {
			out.WriteString("some message")
			return exitErr
		}),
	)

	gomock.InOrder(calls...)

	result, err := runnerExecFunc(
		runner.ExecParams{
			Commands: []string{"storage-list"},
			Env:      []string{"AAAA=1111"},
			Stdout:   out,
			Stderr:   out,
			Cancel:   cancel,
		},
	)
	c.Assert(result, jc.DeepEquals, &utilexec.ExecResponse{
		Code:   expectedCode,
		Stdout: []byte("some message"),
	})
	if exitErr == nil {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.ErrorMatches, "boom")
	}
}
