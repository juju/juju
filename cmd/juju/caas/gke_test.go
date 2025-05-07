// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/utils/v4/exec"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/caas/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

type gkeSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&gkeSuite{})

func (s *gkeSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	err := os.Setenv("PATH", "/path/to/here")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *gkeSuite) TestInteractiveParams(c *tc.C) {
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
		Stderr: io.Discard,
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
	c.Check(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, tc.DeepEquals, &clusterParams{
		project:    "myproject",
		name:       "mycluster",
		region:     "asia-southeast1",
		zone:       "asia-southeast1-a",
		credential: "mysecret",
	})
}

func (s *gkeSuite) TestInteractiveParamsProjectSpecified(c *tc.C) {
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
		Stderr: io.Discard,
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
	c.Check(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, tc.DeepEquals, &clusterParams{
		project:    "myproject",
		name:       "mycluster",
		region:     "asia-southeast1",
		zone:       "asia-southeast1-a",
		credential: "mysecret",
	})
}

func (s *gkeSuite) TestInteractiveParamsProjectAndRegionSpecified(c *tc.C) {
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
		Stderr: io.Discard,
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
	c.Check(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, tc.DeepEquals, &clusterParams{
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

func (s *gkeSuite) TestGetKubeConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	configFile := filepath.Join(c.MkDir(), "config")
	err := os.Setenv("KUBECONFIG", configFile)
	c.Assert(err, tc.ErrorIsNil)
	gke := &gke{CommandRunner: mockRunner}
	err = os.WriteFile(configFile, []byte("data"), 0644)
	c.Assert(err, tc.ErrorIsNil)

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
	c.Check(err, tc.ErrorIsNil)
	defer rdr.Close()

	c.Assert(clusterName, tc.Equals, "gke_myproject_asia-southeast1-a_mycluster")
	data, err := io.ReadAll(rdr)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.DeepEquals, "data")
}

func (s *gkeSuite) TestEnsureExecutableGcloudFound(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
}

func (s *gkeSuite) TestEnsureExecutableGcloudNotFound(c *tc.C) {
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
	c.Assert(err, tc.ErrorMatches, "gcloud command not found, please 'snap install google-cloud-sdk --classic' then try again: ")
}
