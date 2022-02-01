// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/caas/mocks"
	"github.com/juju/juju/cmd/modelcmd"
)

type gkeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&gkeSuite{})

func (s *gkeSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	err := os.Setenv("PATH", "/path/to/here")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *gkeSuite) TestInteractiveParams(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	gke := &gke{CommandRunner: mockRunner}

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "gcloud auth list --format value\\(account,status\\)",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte("mysecret\ndefaultSecret *"),
			}, nil),
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "gcloud projects list --account mysecret --filter lifecycleState:ACTIVE --format value\\(projectId\\)",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte("myproject"),
			}, nil),
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "gcloud container clusters list --filter status:RUNNING --account mysecret --project myproject --format value\\(name,zone\\)",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte("mycluster asia-southeast1-a"),
			}, nil),
	)

	stdin := strings.NewReader("mysecret\nmyproject\nmycluster in asia-southeast1\n")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: ioutil.Discard,
		Stdin:  stdin,
	}
	expected := `
Available Accounts
  mysecret
  defaultSecret

Select account [defaultSecret]: 
Available Projects
  myproject

Select project [myproject]: 
Available Clusters
  mycluster in asia-southeast1

Select cluster [mycluster in asia-southeast1]: 
`[1:]

	outParams, err := gke.interactiveParams(ctx, &clusterParams{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		project:    "myproject",
		name:       "mycluster",
		region:     "asia-southeast1",
		zone:       "asia-southeast1-a",
		credential: "mysecret",
	})
}

func (s *gkeSuite) TestInteractiveParamsProjectSpecified(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	gke := &gke{CommandRunner: mockRunner}

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "gcloud container clusters list --filter status:RUNNING --account mysecret --project myproject --format value\\(name,zone\\)",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte("mycluster asia-southeast1-a"),
			}, nil),
	)

	stdin := strings.NewReader("mycluster in asia-southeast1\n")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: ioutil.Discard,
		Stdin:  stdin,
	}
	expected := `
Available Clusters
  mycluster in asia-southeast1

Select cluster [mycluster in asia-southeast1]: 
`[1:]

	outParams, err := gke.interactiveParams(ctx, &clusterParams{
		project:    "myproject",
		credential: "mysecret",
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		project:    "myproject",
		name:       "mycluster",
		region:     "asia-southeast1",
		zone:       "asia-southeast1-a",
		credential: "mysecret",
	})
}

func (s *gkeSuite) TestInteractiveParamsProjectAndRegionSpecified(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	gke := &gke{CommandRunner: mockRunner}

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "gcloud container clusters list --filter status:RUNNING --account mysecret --project myproject --format value\\(name,zone\\)",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte("mycluster asia-southeast1-a"),
			}, nil),
	)

	stdin := strings.NewReader("mycluster in asia-southeast1\n")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: ioutil.Discard,
		Stdin:  stdin,
	}
	expected := `
Available Clusters
  mycluster in asia-southeast1

Select cluster [mycluster in asia-southeast1]: 
`[1:]

	outParams, err := gke.interactiveParams(ctx, &clusterParams{
		project:    "myproject",
		region:     "asia-southeast1",
		credential: "mysecret",
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		project:    "myproject",
		name:       "mycluster",
		region:     "asia-southeast1",
		zone:       "asia-southeast1-a",
		credential: "mysecret",
	})
}

type osFilesystem struct {
	modelcmd.Filesystem
}

func (osFilesystem) Open(name string) (modelcmd.ReadSeekCloser, error) {
	return os.Open(name)
}

func (s *gkeSuite) TestGetKubeConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	configFile := filepath.Join(c.MkDir(), "config")
	err := os.Setenv("KUBECONFIG", configFile)
	c.Assert(err, jc.ErrorIsNil)
	gke := &gke{CommandRunner: mockRunner}
	err = ioutil.WriteFile(configFile, []byte("data"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "gcloud container clusters get-credentials mycluster --account mysecret --project myproject --zone asia-southeast1-a",
			Environment: mergeEnv(os.Environ(), []string{"KUBECONFIG=" + configFile}),
		}).
			Return(&exec.ExecResponse{
				Code: 0,
			}, nil),
	)
	rdr, clusterName, err := gke.getKubeConfig(&clusterParams{
		openFile:   osFilesystem{}.Open,
		project:    "myproject",
		zone:       "asia-southeast1-a",
		region:     "asia-southeast1",
		name:       "mycluster",
		credential: "mysecret",
	})
	c.Check(err, jc.ErrorIsNil)
	defer rdr.Close()

	c.Assert(clusterName, gc.Equals, "gke_myproject_asia-southeast1-a_mycluster")
	data, err := ioutil.ReadAll(rdr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.DeepEquals, "data")
}

func (s *gkeSuite) TestEnsureExecutableGcloudFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	gke := &gke{CommandRunner: mockRunner}

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "which gcloud",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code: 0,
			}, nil),
	)
	err := gke.ensureExecutable()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *gkeSuite) TestEnsureExecutableGcloudNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	gke := &gke{CommandRunner: mockRunner}

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "which gcloud",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code: 1,
			}, nil),
	)
	err := gke.ensureExecutable()
	c.Assert(err, gc.ErrorMatches, "gcloud command not found, please 'snap install google-cloud-sdk --classic' then try again: ")
}
