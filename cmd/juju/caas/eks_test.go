// Copyright 2020 Canonical Ltd.
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
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4/exec"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/caas/mocks"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

type eksSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&eksSuite{})

func (s *eksSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	err := os.Setenv("PATH", "/path/to/here")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *eksSuite) TestGetKubeConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	configFile := filepath.Join(c.MkDir(), "config")
	err := os.Setenv("KUBECONFIG", configFile)
	c.Assert(err, jc.ErrorIsNil)
	eksCMD := &eks{
		tool:          "eksctl",
		CommandRunner: mockRunner,
	}
	err = os.WriteFile(configFile, []byte("data"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "eksctl utils write-kubeconfig --cluster mycluster --kubeconfig " + configFile + " --region ap-southeast-2",
			Environment: mergeEnv(os.Environ(), []string{"KUBECONFIG=" + configFile}),
		}).
			Return(&exec.ExecResponse{
				Code: 0,
			}, nil),
	)

	rdr, clusterName, err := eksCMD.getKubeConfig(&clusterParams{
		openFile: osFilesystem{}.Open,
		name:     "mycluster",
		region:   "ap-southeast-2",
	})
	c.Check(err, jc.ErrorIsNil)
	defer rdr.Close()

	c.Assert(clusterName, tc.Equals, "mycluster.ap-southeast-2.eksctl.io")
	data, err := io.ReadAll(rdr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), tc.DeepEquals, "data")
}

func (s *eksSuite) TestInteractiveParam(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	eksCMD := &eks{
		tool:          "eksctl",
		CommandRunner: mockRunner,
	}
	clusterJSONResp := `
[
    {
        "name": "mycluster",
        "region": "ap-southeast-2"
    }
]`

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "eksctl get cluster --region ap-southeast-2 -o json",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(clusterJSONResp),
			}, nil),
	)
	stdin := strings.NewReader("ap-southeast-2\n")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: io.Discard,
		Stdin:  stdin,
	}
	expected := `
Enter region: 
`[1:]

	outParams, err := eksCMD.interactiveParams(ctx, &clusterParams{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		name:   "mycluster",
		region: "ap-southeast-2",
	})
}

func (s *eksSuite) TestInteractiveParamNoClusterFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	eksCMD := &eks{
		tool:          "eksctl",
		CommandRunner: mockRunner,
	}
	clusterJSONResp := `
[]`

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "eksctl get cluster --region ap-southeast-2 -o json",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(clusterJSONResp),
			}, nil),
	)
	stdin := strings.NewReader("ap-southeast-2\n")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: io.Discard,
		Stdin:  stdin,
	}
	expected := `
Enter region: 
`[1:]

	_, err := eksCMD.interactiveParams(ctx, &clusterParams{})
	c.Check(err, tc.ErrorMatches, `no cluster found in region "ap-southeast-2"`)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
}

func (s *eksSuite) TestInteractiveParamMultiClustersLegacyCLI(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	eksCMD := &eks{
		tool:          "eksctl",
		CommandRunner: mockRunner,
	}
	clusterJSONResp := `
[
    {
        "name": "mycluster",
        "region": "ap-southeast-2"
	},
	{
        "name": "mycluster2",
        "region": "ap-southeast-2"
    }
]`

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "eksctl get cluster --region ap-southeast-2 -o json",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(clusterJSONResp),
			}, nil),
	)
	stdin := strings.NewReader("ap-southeast-2\nmycluster\n")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: io.Discard,
		Stdin:  stdin,
	}
	expected := `
Enter region: 
Available Clusters In Ap-Southeast-2
  mycluster
  mycluster2

Select cluster [mycluster]: 
`[1:]

	outParams, err := eksCMD.interactiveParams(ctx, &clusterParams{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		name:   "mycluster",
		region: "ap-southeast-2",
	})
}

func (s *eksSuite) TestInteractiveParamMultiClusters(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	eksCMD := &eks{
		tool:          "eksctl",
		CommandRunner: mockRunner,
	}
	clusterJSONResp := `
[
    {
        "metadata": {
            "name": "nw-deploy-kubeflow-1272",
            "region": "ap-southeast-2"
        },
        "status": {
            "eksctlCreated": "True"
        }
    },
    {
        "metadata": {
            "name": "k1",
            "region": "ap-southeast-2"
        },
        "status": {
            "eksctlCreated": "True"
        }
    }
]`

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "eksctl get cluster --region ap-southeast-2 -o json",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(clusterJSONResp),
			}, nil),
	)
	stdin := strings.NewReader("ap-southeast-2\nk1\n")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: io.Discard,
		Stdin:  stdin,
	}
	expected := `
Enter region: 
Available Clusters In Ap-Southeast-2
  nw-deploy-kubeflow-1272
  k1

Select cluster [nw-deploy-kubeflow-1272]: 
`[1:]

	outParams, err := eksCMD.interactiveParams(ctx, &clusterParams{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		name:   "k1",
		region: "ap-southeast-2",
	})
}

func (s *eksSuite) TestEnsureExecutable(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	eksCMD := &eks{tool: "eksctl", CommandRunner: mockRunner}

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "which eksctl",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(""),
			}, nil),
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "eksctl get cluster",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(""),
			}, nil),
	)
	err := eksCMD.ensureExecutable()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *eksSuite) TestEnsureExecutableNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	eksCMD := &eks{tool: "eksctl", CommandRunner: mockRunner}

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "which eksctl",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code: 1,
			}, nil),
	)
	err := eksCMD.ensureExecutable()
	c.Assert(err, tc.ErrorMatches, `"eksctl" not found. Please install "eksctl" \(see: .*\).*`)
}
