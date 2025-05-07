// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"bytes"
	"fmt"
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

type aksSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&aksSuite{})

func (s *aksSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	err := os.Setenv("PATH", "/path/to/here")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *aksSuite) TestGetKubeConfig(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	configFile := filepath.Join(c.MkDir(), "config")
	err := os.Setenv("KUBECONFIG", configFile)
	c.Assert(err, jc.ErrorIsNil)
	aks := &aks{
		CommandRunner: mockRunner,
	}
	err = os.WriteFile(configFile, []byte("data"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "az aks get-credentials --name mycluster --resource-group resourceGroup --overwrite-existing -f " + configFile,
			Environment: mergeEnv(os.Environ(), []string{"KUBECONFIG=" + configFile}),
		}).
			Return(&exec.ExecResponse{
				Code: 0,
			}, nil),
	)

	rdr, clusterName, err := aks.getKubeConfig(&clusterParams{
		openFile:      osFilesystem{}.Open,
		name:          "mycluster",
		resourceGroup: "resourceGroup",
	})
	c.Check(err, jc.ErrorIsNil)
	defer rdr.Close()

	c.Assert(clusterName, tc.Equals, "mycluster")
	data, err := io.ReadAll(rdr)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), tc.DeepEquals, "data")
}

func (s *aksSuite) TestInteractiveParam(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	aks := &aks{
		CommandRunner: mockRunner,
	}
	resourceGroup := "testRG"
	clusterName := "mycluster"
	clusterJSONResp := fmt.Sprintf(`
[
  {
    "name": "%s",
    "resourceGroup": "%s"
  },
  {
    "name": "notThisCluster",
    "resourceGroup": "notThisRG"
  }
]
`, clusterName, resourceGroup)

	resourcegroupJSONResp := fmt.Sprintf(`
[
  {
    "location": "westus2",
    "name": "%s"
  }
]`, resourceGroup)

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "az aks list --output json",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(clusterJSONResp),
			}, nil),
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands: fmt.Sprintf(
				`az group list --output json --query "[?properties.provisioningState=='Succeeded'] | [?name=='%s']"`,
				resourceGroup),
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(resourcegroupJSONResp),
			}, nil),
	)
	stdin := strings.NewReader("mycluster in resource group testRG\n")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: io.Discard,
		Stdin:  stdin,
	}
	expected := `
Available Clusters
  mycluster in resource group testRG
  notThisCluster in resource group notThisRG

Select cluster [mycluster in resource group testRG]: 
`[1:]

	outParams, err := aks.interactiveParams(ctx, &clusterParams{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		name:          clusterName,
		resourceGroup: resourceGroup,
		region:        "westus2",
	})
}

func (s *aksSuite) TestInteractiveParamResourceGroupDefined(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	aks := &aks{
		CommandRunner: mockRunner,
	}
	resourceGroup := "testRG"
	clusterName := "mycluster"
	clusterJSONResp := fmt.Sprintf(`
[
  {
    "name": "%s",
    "resourceGroup": "%s"
  }
]
`, clusterName, resourceGroup)

	resourcegroupJSONResp := fmt.Sprintf(`
[
  {
    "location": "westus2",
    "name": "%s"
  }
]`, resourceGroup)

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "az aks list --output json --resource-group " + resourceGroup,
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(clusterJSONResp),
			}, nil),
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands: fmt.Sprintf(
				`az group list --output json --query "[?properties.provisioningState=='Succeeded'] | [?name=='%s']"`,
				resourceGroup),
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(resourcegroupJSONResp),
			}, nil),
	)
	stdin := strings.NewReader("mycluster\n")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: io.Discard,
		Stdin:  stdin,
	}
	expected := `
Available Clusters
  mycluster

Select cluster [mycluster]: 
`[1:]

	outParams, err := aks.interactiveParams(ctx, &clusterParams{
		resourceGroup: resourceGroup,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		name:          clusterName,
		resourceGroup: resourceGroup,
		region:        "westus2",
	})
}

func (s *aksSuite) TestInteractiveParamsNoResourceGroupSpecifiedSingleResourceGroupInUse(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	aks := &aks{
		CommandRunner: mockRunner,
	}
	resourceGroup := "testRG"
	clusterName := "mycluster"
	clusterJSONResp := fmt.Sprintf(`
[
  {
    "name": "%s",
    "resourceGroup": "%s"
  },
  {
    "name": "notMeSir",
    "resourceGroup": "%s"
  }
]
`, clusterName, resourceGroup, resourceGroup)

	resourcegroupJSONResp := fmt.Sprintf(`
[
  {
    "location": "westus2",
    "name": "%s"
  }
]`, resourceGroup)
	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "az aks list --output json",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(clusterJSONResp),
			}, nil),
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands: fmt.Sprintf(
				`az group list --output json --query "[?properties.provisioningState=='Succeeded'] | [?name=='%s']"`,
				resourceGroup),
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(resourcegroupJSONResp),
			}, nil),
	)
	stdin := strings.NewReader("mycluster\n")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: io.Discard,
		Stdin:  stdin,
	}
	expected := `
Available Clusters In Resource Group TestRG
  mycluster
  notMeSir

Select cluster [mycluster]: 
`[1:]

	outParams, err := aks.interactiveParams(ctx, &clusterParams{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		name:          clusterName,
		resourceGroup: resourceGroup,
		region:        "westus2",
	})
}

func (s *aksSuite) TestInteractiveParamsNoResourceGroupSpecifiedMultiResourceGroupsInUse(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	aks := &aks{
		CommandRunner: mockRunner,
	}
	resourceGroup := "testRG"
	clusterName := "mycluster"
	clusterJSONResp := fmt.Sprintf(`
[
  {
    "name": "%s",
    "resourceGroup": "%s"
  },
  {
    "name": "notMeSir",
    "resourceGroup": "MonsterResourceGroup"
  }
]
`, clusterName, resourceGroup)

	resourcegroupJSONResp := fmt.Sprintf(`
[
  {
    "location": "westus2",
    "name": "%s"
  }
]`, resourceGroup)

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "az aks list --output json",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(clusterJSONResp),
			}, nil),
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands: fmt.Sprintf(
				`az group list --output json --query "[?properties.provisioningState=='Succeeded'] | [?name=='%s']"`,
				resourceGroup),
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(resourcegroupJSONResp),
			}, nil),
	)
	stdin := strings.NewReader("mycluster in resource group testRG\n")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: io.Discard,
		Stdin:  stdin,
	}
	expected := `
Available Clusters
  mycluster in resource group testRG
  notMeSir in resource group MonsterResourceGroup

Select cluster [mycluster in resource group testRG]: 
`[1:]

	outParams, err := aks.interactiveParams(ctx, &clusterParams{})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		name:          clusterName,
		resourceGroup: resourceGroup,
		region:        "westus2",
	})
}

func (s *aksSuite) TestInteractiveParamsClusterSpecifiedNoResourceGroupSpecifiedSingleGroupInUse(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	aks := &aks{
		CommandRunner: mockRunner,
	}
	resourceGroup := "testRG"
	clusterName := "mycluster"
	clusterJSONResp := fmt.Sprintf(`
[
  {
    "name": "%s",
    "resourceGroup": "%s"
  },
  {
    "name": "notMeCluster",
    "resourceGroup": "%s"
  }
]`, clusterName, resourceGroup, resourceGroup)

	namedResourcegroupJSONResp := fmt.Sprintf(`
[
  {
    "location": "westus2",
    "name": "%s"
  }
]`, resourceGroup)

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "az aks list --output json",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(clusterJSONResp),
			}, nil),
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands: fmt.Sprintf(
				`az group list --output json --query "[?properties.provisioningState=='Succeeded'] | [?name=='%s']"`,
				resourceGroup),
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(namedResourcegroupJSONResp),
			}, nil),
	)
	stdin := strings.NewReader("")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: io.Discard,
		Stdin:  stdin,
	}
	expected := ""
	outParams, err := aks.interactiveParams(ctx, &clusterParams{
		name: clusterName,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		name:          clusterName,
		resourceGroup: resourceGroup,
		region:        "westus2",
	})
}

func (s *aksSuite) TestInteractiveParamsClusterSpecifiedNoResourceGroupSpecifiedMultiClusterInUse(c *tc.C) {
	// If a cluster name is given but there are multiple clusters of that
	// name in different resource groups the user must be prompted to choose
	// which of those resource groups is the correct one.
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	aks := &aks{
		CommandRunner: mockRunner,
	}
	resourceGroup := "testRG"
	clusterName := "mycluster"
	clusterJSONResp := fmt.Sprintf(`
[
  {
    "name": "%s",
    "resourceGroup": "%s"
  },
  {
    "name": "%s",
    "resourceGroup": "notMeGroup"
  },
  {
    "name": "notMeCluster",
    "resourceGroup": "differentRG"
  }
]`, clusterName, resourceGroup, clusterName)

	resourcegroupJSONResp := fmt.Sprintf(`
[
  {
    "location": "westus2",
    "name": "%s"
  },
  {
    "location": "westus2",
    "name": "notMeGroup"
  }
]`, resourceGroup)

	namedResourcegroupJSONResp := fmt.Sprintf(`
[
  {
    "location": "westus2",
    "name": "%s"
  }
]`, resourceGroup)

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "az aks list --output json",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(clusterJSONResp),
			}, nil),
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    `az group list --output json --query "[?properties.provisioningState=='Succeeded']"`,
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(resourcegroupJSONResp),
			}, nil),
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands: fmt.Sprintf(
				`az group list --output json --query "[?properties.provisioningState=='Succeeded'] | [?name=='%s']"`,
				resourceGroup),
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(namedResourcegroupJSONResp),
			}, nil),
	)
	stdin := strings.NewReader("testRG in westus2\n")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: io.Discard,
		Stdin:  stdin,
	}
	expected := `
Available Resource Groups
  testRG in westus2
  notMeGroup in westus2

Select resource group [testRG in westus2]: 
`[1:]
	outParams, err := aks.interactiveParams(ctx, &clusterParams{
		name: clusterName,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		name:          clusterName,
		resourceGroup: resourceGroup,
		region:        "westus2",
	})
}

func (s *aksSuite) TestInteractiveParamsClusterSpecifiedResourceGroupSpecified(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	aks := &aks{
		CommandRunner: mockRunner,
	}
	resourceGroup := "testRG"
	clusterName := "mycluster"
	namedResourcegroupJSONResp := fmt.Sprintf(`
[
  {
    "location": "westus2",
    "name": "%s"
  }
]`, resourceGroup)

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands: fmt.Sprintf(
				`az group list --output json --query "[?properties.provisioningState=='Succeeded'] | [?name=='%s']"`,
				resourceGroup),
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(namedResourcegroupJSONResp),
			}, nil),
	)

	stdin := strings.NewReader("")
	out := &bytes.Buffer{}
	ctx := &cmd.Context{
		Dir:    c.MkDir(),
		Stdout: out,
		Stderr: io.Discard,
		Stdin:  stdin,
	}
	expected := ""
	outParams, err := aks.interactiveParams(ctx, &clusterParams{
		name:          clusterName,
		resourceGroup: resourceGroup,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(outParams, jc.DeepEquals, &clusterParams{
		name:          clusterName,
		resourceGroup: resourceGroup,
		region:        "westus2",
	})
}

func (s *aksSuite) TestEnsureExecutablePicksAZ(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	aks := &aks{CommandRunner: mockRunner}

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "which az",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(""),
			}, nil),
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "az account show",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code:   0,
				Stdout: []byte(""),
			}, nil),
	)
	err := aks.ensureExecutable()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *aksSuite) TestEnsureExecutableNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockRunner := mocks.NewMockCommandRunner(ctrl)
	aks := &aks{CommandRunner: mockRunner}

	gomock.InOrder(
		mockRunner.EXPECT().RunCommands(exec.RunParams{
			Commands:    "which az",
			Environment: os.Environ(),
		}).
			Return(&exec.ExecResponse{
				Code: 1,
			}, nil),
	)
	err := aks.ensureExecutable()
	c.Assert(err, tc.ErrorMatches, `az not found. Please 'apt install az' \(see: .*\).*`)
}
