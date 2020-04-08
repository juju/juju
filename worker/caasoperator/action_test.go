// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
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
	unitPaths := uniter.NewPaths(baseDir, names.NewUnitTag("gitlab-k8s/0"), &uniter.SocketConfig{})
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

	logger := loggo.GetLogger("test")
	runnerExecFunc := caasoperator.GetNewRunnerExecutor(logger, s.executor)(s.unitAPI, unitPaths)
	cancel := make(<-chan struct{}, 1)
	stdout := bytes.NewBufferString("")
	stderr := bytes.NewBufferString("")
	expectedCode := 0
	var exitErr error
	if errMsg != "" {
		exitErr = errors.Trace(k8sexec.CodeExitError{Code: 3, Err: errors.New(errMsg)})
		expectedCode = 3
	}
	gomock.InOrder(
		s.unitAPI.EXPECT().Refresh().Return(nil),
		s.unitAPI.EXPECT().ProviderID().Return("gitlab-xxxx"),
		s.unitAPI.EXPECT().Name().Return("gitlab-k8s/0"),
		s.executor.EXPECT().Exec(
			exec.ExecParams{
				PodName:  "gitlab-xxxx",
				Commands: []string{"storage-list"},
				Env:      []string{"AAAA=1111"},
				Stdout:   stdout,
				Stderr:   stderr,
			}, cancel,
		).DoAndReturn(func(...interface{}) error {
			stdout.WriteString("some message")
			stderr.WriteString("some err message")
			return exitErr
		}),
	)

	outLogger := &mockHookLogger{}
	errLogger := &mockHookLogger{}
	result, err := runnerExecFunc(
		runner.ExecParams{
			Commands:     []string{"storage-list"},
			Env:          []string{"AAAA=1111"},
			Stdout:       stdout,
			StdoutLogger: outLogger,
			Stderr:       stdout,
			StderrLogger: errLogger,
			Cancel:       cancel,
		},
	)
	c.Assert(outLogger.stopped, jc.IsTrue)
	c.Assert(errLogger.stopped, jc.IsTrue)
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

type exitError struct {
	code int
	err  string
}

var _ exec.ExitError = exitError{}

func (e exitError) String() string {
	return e.err
}

func (e exitError) Error() string {
	return e.err
}

func (e exitError) ExitStatus() int {
	return e.code
}
