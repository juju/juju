// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"bytes"
	"os"
	"path/filepath"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/caasoperator/mocks"
)

type UnitInitializerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&UnitInitializerSuite{})

func (s *UnitInitializerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *UnitInitializerSuite) TestInitialize(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockExecClient := mocks.NewMockExecutor(ctrl)

	params := caasoperator.InitializeUnitParams{
		ReTrier: func(f func() error, _ func(error) bool, _ caasoperator.Logger, _ jujuclock.Clock, _ <-chan struct{}) error {
			return f()
		},
		UnitTag: names.NewUnitTag("gitlab/0"),
		Logger:  loggo.GetLogger("test"),
		Paths: caasoperator.Paths{
			State: caasoperator.StatePaths{
				CharmDir: "dir/charm",
			},
		},
		ExecClient: mockExecClient,
		OperatorInfo: caas.OperatorInfo{
			CACert: "ca-cert",
		},
		ProviderID: "gitlab-ffff",
		TempDir: func(dir string, prefix string) (string, error) {
			return filepath.Join(dir, prefix+"-random"), nil
		},
		WriteFile: func(path string, data []byte, perm os.FileMode) error {
			return nil
		},
	}

	gomock.InOrder(
		mockExecClient.EXPECT().Exec(exec.ExecParams{
			Commands:      []string{"mkdir", "-p", filepath.Join(os.TempDir(), "unit-gitlab-0-random")},
			PodName:       "gitlab-ffff",
			ContainerName: "juju-pod-init",
			Stdout:        &bytes.Buffer{},
			Stderr:        &bytes.Buffer{},
		}, gomock.Any()).Return(nil),
		mockExecClient.EXPECT().Copy(exec.CopyParams{
			Src: exec.FileResource{
				Path: "dir/charm",
			},
			Dest: exec.FileResource{
				Path:          filepath.Join(os.TempDir(), "unit-gitlab-0-random"),
				PodName:       "gitlab-ffff",
				ContainerName: "juju-pod-init",
			},
		}, gomock.Any()).Return(nil),
		mockExecClient.EXPECT().Copy(exec.CopyParams{
			Src: exec.FileResource{
				Path: filepath.Join(os.TempDir(), "unit-gitlab-0-random/ca.crt"),
			},
			Dest: exec.FileResource{
				Path:          filepath.Join(os.TempDir(), "unit-gitlab-0-random/ca.crt"),
				PodName:       "gitlab-ffff",
				ContainerName: "juju-pod-init",
			},
		}, gomock.Any()).Return(nil),
		mockExecClient.EXPECT().Copy(exec.CopyParams{
			Src: exec.FileResource{
				Path: "/var/lib/juju/agents/unit-gitlab-0/operator-client-cache.yaml",
			},
			Dest: exec.FileResource{
				Path:          filepath.Join(os.TempDir(), "unit-gitlab-0-random/operator-client-cache.yaml"),
				PodName:       "gitlab-ffff",
				ContainerName: "juju-pod-init",
			},
		}, gomock.Any()).Return(nil),
		mockExecClient.EXPECT().Exec(exec.ExecParams{
			Commands: []string{"/var/lib/juju/tools/jujud", "caas-unit-init",
				"--unit", "unit-gitlab-0",
				"--charm-dir",
				filepath.Join(os.TempDir(), "unit-gitlab-0-random/charm"),
				"--send",
				"--operator-file",
				filepath.Join(os.TempDir(), "unit-gitlab-0-random/operator-client-cache.yaml"),
				"--operator-ca-cert-file",
				filepath.Join(os.TempDir(), "unit-gitlab-0-random/ca.crt"),
			},
			WorkingDir:    "/var/lib/juju",
			PodName:       "gitlab-ffff",
			ContainerName: "juju-pod-init",
			Stdout:        &bytes.Buffer{},
			Stderr:        &bytes.Buffer{},
		}, gomock.Any()).Return(nil),
	)

	cancel := make(chan struct{})
	err := caasoperator.InitializeUnit(params, cancel)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitInitializerSuite) TestInitializeUnitMissingProviderID(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockExecClient := mocks.NewMockExecutor(ctrl)

	params := caasoperator.InitializeUnitParams{
		ReTrier: func(f func() error, _ func(error) bool, _ caasoperator.Logger, _ jujuclock.Clock, _ <-chan struct{}) error {
			return f()
		},
		UnitTag: names.NewUnitTag("gitlab/0"),
		Logger:  loggo.GetLogger("test"),
		Paths: caasoperator.Paths{
			State: caasoperator.StatePaths{
				CharmDir: "dir/charm",
			},
		},
		ExecClient: mockExecClient,
		OperatorInfo: caas.OperatorInfo{
			CACert: "ca-cert",
		},
		ProviderID: "",
		TempDir: func(dir string, prefix string) (string, error) {
			return filepath.Join(dir, prefix+"-random"), nil
		},
		WriteFile: func(path string, data []byte, perm os.FileMode) error {
			return nil
		},
	}

	gomock.InOrder()

	cancel := make(chan struct{})
	err := caasoperator.InitializeUnit(params, cancel)
	c.Assert(err, gc.ErrorMatches, "missing ProviderID not valid")
}

func (s *UnitInitializerSuite) TestInitializeContainerMissing(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockExecClient := mocks.NewMockExecutor(ctrl)

	params := caasoperator.InitializeUnitParams{
		ReTrier: func(f func() error, _ func(error) bool, _ caasoperator.Logger, _ jujuclock.Clock, _ <-chan struct{}) error {
			return f()
		},
		UnitTag: names.NewUnitTag("gitlab/0"),
		Logger:  loggo.GetLogger("test"),
		Paths: caasoperator.Paths{
			State: caasoperator.StatePaths{
				CharmDir: "dir/charm",
			},
		},
		ExecClient: mockExecClient,
		OperatorInfo: caas.OperatorInfo{
			CACert: "ca-cert",
		},
		ProviderID: "gitlab-ffff",
		TempDir: func(dir string, prefix string) (string, error) {
			return filepath.Join(dir, prefix+"-random"), nil
		},
		WriteFile: func(path string, data []byte, perm os.FileMode) error {
			return nil
		},
	}

	gomock.InOrder(
		mockExecClient.EXPECT().Exec(exec.ExecParams{
			Commands:      []string{"mkdir", "-p", filepath.Join(os.TempDir(), "unit-gitlab-0-random")},
			PodName:       "gitlab-ffff",
			ContainerName: "juju-pod-init",
			Stdout:        &bytes.Buffer{},
			Stderr:        &bytes.Buffer{},
		}, gomock.Any()).Return(nil),
		mockExecClient.EXPECT().Copy(exec.CopyParams{
			Src: exec.FileResource{
				Path: "dir/charm",
			},
			Dest: exec.FileResource{
				Path:          filepath.Join(os.TempDir(), "unit-gitlab-0-random"),
				PodName:       "gitlab-ffff",
				ContainerName: "juju-pod-init",
			},
		}, gomock.Any()).Return(errors.NotFoundf("container")),
	)

	cancel := make(chan struct{})
	err := caasoperator.InitializeUnit(params, cancel)
	c.Assert(err, gc.ErrorMatches, "container not found")
}

func (s *UnitInitializerSuite) TestInitializePodNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockExecClient := mocks.NewMockExecutor(ctrl)

	params := caasoperator.InitializeUnitParams{
		ReTrier: func(f func() error, _ func(error) bool, _ caasoperator.Logger, _ jujuclock.Clock, _ <-chan struct{}) error {
			return f()
		},
		UnitTag: names.NewUnitTag("gitlab/0"),
		Logger:  loggo.GetLogger("test"),
		Paths: caasoperator.Paths{
			State: caasoperator.StatePaths{
				CharmDir: "dir/charm",
			},
		},
		ExecClient: mockExecClient,
		OperatorInfo: caas.OperatorInfo{
			CACert: "ca-cert",
		},
		ProviderID: "gitlab-ffff",
		TempDir: func(dir string, prefix string) (string, error) {
			return filepath.Join(dir, prefix+"-random"), nil
		},
		WriteFile: func(path string, data []byte, perm os.FileMode) error {
			return nil
		},
	}

	gomock.InOrder(
		mockExecClient.EXPECT().Exec(exec.ExecParams{
			Commands:      []string{"mkdir", "-p", filepath.Join(os.TempDir(), "unit-gitlab-0-random")},
			PodName:       "gitlab-ffff",
			ContainerName: "juju-pod-init",
			Stdout:        &bytes.Buffer{},
			Stderr:        &bytes.Buffer{},
		}, gomock.Any()).Return(nil),
		mockExecClient.EXPECT().Copy(exec.CopyParams{
			Src: exec.FileResource{
				Path: "dir/charm",
			},
			Dest: exec.FileResource{
				Path:          filepath.Join(os.TempDir(), "unit-gitlab-0-random"),
				PodName:       "gitlab-ffff",
				ContainerName: "juju-pod-init",
			},
		}, gomock.Any()).Return(errors.NotFoundf("container")),
	)

	cancel := make(chan struct{})
	err := caasoperator.InitializeUnit(params, cancel)
	c.Assert(err, gc.ErrorMatches, "container not found")
}

func (s *UnitInitializerSuite) TestRunnerWithRetry(c *gc.C) {
	cancel := make(chan struct{})
	clk := testclock.NewClock(time.Time{})
	called := 0
	execRequest := func() error {
		called++
		if called < 3 {
			return exec.NewExecRetryableError(errors.New("fake testing 137"))
		}
		return nil
	}

	errChan := make(chan error)
	go func() {
		errChan <- caasoperator.RunnerWithRetry(execRequest, func(err error) bool {
			return err != nil && !exec.IsExecRetryableError(err)
		}, loggo.GetLogger("test"), clk, cancel)
	}()
	err := clk.WaitAdvance(2*time.Second, testing.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)
	err = clk.WaitAdvance(2*time.Second, testing.ShortWait, 1)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case err := <-errChan:
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(called, gc.DeepEquals, 3)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for RunnerWithRetry return")
	}
}
