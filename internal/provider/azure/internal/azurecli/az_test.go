// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azurecli_test

import (
	"os/exec"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/provider/azure/internal/azurecli"
)

type azSuite struct{}

var _ = tc.Suite(&azSuite{})

func (s *azSuite) TestShowAccount(c *tc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account show -o json": {
					stdout: []byte(`
{
  "environmentName": "AzureCloud",
  "id": "5544b9a5-0000-0000-0000-fedceb5d3696",
  "isDefault": true,
  "name": "AccountName",
  "state": "Enabled",
  "tenantId": "a52afd7f-0000-0000-0000-e47a54b982da",
  "user": {
    "name": "user@example.com",
    "type": "user"
  }
}

`[1:]),
				},
			},
		}.Exec,
	}
	acc, err := azcli.ShowAccount("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(acc, tc.DeepEquals, &azurecli.Account{
		CloudName: "AzureCloud",
		ID:        "5544b9a5-0000-0000-0000-fedceb5d3696",
		IsDefault: true,
		Name:      "AccountName",
		State:     "Enabled",
		TenantId:  "a52afd7f-0000-0000-0000-e47a54b982da",
	})
}

func (s *azSuite) TestShowAccountWithSubscription(c *tc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account show --subscription 5544b9a5-0000-0000-0000-fedceb5d3696 -o json": {
					stdout: []byte(`
{
  "environmentName": "AzureCloud",
  "id": "5544b9a5-0000-0000-0000-fedceb5d3696",
  "isDefault": true,
  "name": "AccountName",
  "state": "Enabled",
  "tenantId": "a52afd7f-0000-0000-0000-e47a54b982da",
  "user": {
    "name": "user@example.com",
    "type": "user"
  }
}

`[1:]),
				},
			},
		}.Exec,
	}
	acc, err := azcli.ShowAccount("5544b9a5-0000-0000-0000-fedceb5d3696")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(acc, tc.DeepEquals, &azurecli.Account{
		CloudName: "AzureCloud",
		ID:        "5544b9a5-0000-0000-0000-fedceb5d3696",
		IsDefault: true,
		Name:      "AccountName",
		State:     "Enabled",
		TenantId:  "a52afd7f-0000-0000-0000-e47a54b982da",
	})
}

func (s *azSuite) TestShowAccountError(c *tc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account show -o json": {
					error: &exec.ExitError{Stderr: []byte("test error\nusage ...")},
				},
			},
		}.Exec,
	}
	acc, err := azcli.ShowAccount("")
	c.Assert(err, tc.ErrorMatches, `execution failure: test error`)
	c.Assert(acc, tc.IsNil)
}

func (s *azSuite) TestListAccounts(c *tc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account list -o json": {
					stdout: []byte(`
[
  {
    "cloudName": "AzureCloud",
    "id": "d7ad3057-0000-0000-0000-513d7136eec5",
    "isDefault": false,
    "name": "Free Trial",
    "state": "Enabled",
    "tenantId": "b7bb0664-0000-0000-0000-4d5f1481ef22",
    "user": {
      "name": "user@example.com",
      "type": "user"
    }
  },
  {
    "cloudName": "AzureCloud",
    "id": "5af17b7d-0000-0000-0000-5cd99887fdf7",
    "isDefault": true,
    "name": "Pay-As-You-Go",
    "state": "Enabled",
    "tenantId": "2da419a9-0000-0000-0000-ac7c24bbe2e7",
    "user": {
      "name": "user@example.com",
      "type": "user"
    }
  }
]
`[1:]),
				},
			},
		}.Exec,
	}
	accs, err := azcli.ListAccounts()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(accs, tc.DeepEquals, []azurecli.Account{{
		CloudName: "AzureCloud",
		ID:        "d7ad3057-0000-0000-0000-513d7136eec5",
		IsDefault: false,
		Name:      "Free Trial",
		State:     "Enabled",
		TenantId:  "b7bb0664-0000-0000-0000-4d5f1481ef22",
	}, {
		CloudName: "AzureCloud",
		ID:        "5af17b7d-0000-0000-0000-5cd99887fdf7",
		IsDefault: true,
		Name:      "Pay-As-You-Go",
		State:     "Enabled",
		TenantId:  "2da419a9-0000-0000-0000-ac7c24bbe2e7",
	}})
}

func (s *azSuite) TestListAccountsError(c *tc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account list -o json": {
					error: errors.New("test error"),
				},
			},
		}.Exec,
	}
	accs, err := azcli.ListAccounts()
	c.Assert(err, tc.ErrorMatches, `execution failure: test error`)
	c.Assert(accs, tc.IsNil)
}

func (s *azSuite) TestFindAccountsWithCloudName(c *tc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account list --query [?cloudName=='AzureCloud'] -o json": {
					stdout: []byte(`
[
  {
    "cloudName": "AzureCloud",
    "id": "d7ad3057-0000-0000-0000-513d7136eec5",
    "isDefault": false,
    "name": "Free Trial",
    "state": "Enabled",
    "tenantId": "b7bb0664-0000-0000-0000-4d5f1481ef22",
    "homeTenantId": "b7bb0664-0000-0000-0000-4d5f1481ef66",
    "user": {
      "name": "user@example.com",
      "type": "user"
    }
  },
  {
    "cloudName": "AzureCloud",
    "id": "5af17b7d-0000-0000-0000-5cd99887fdf7",
    "isDefault": true,
    "name": "Pay-As-You-Go",
    "state": "Enabled",
    "tenantId": "2da419a9-0000-0000-0000-ac7c24bbe2e7",
    "user": {
      "name": "user@example.com",
      "type": "user"
    }
  }
]
`[1:]),
				},
			},
		}.Exec,
	}
	accs, err := azcli.FindAccountsWithCloudName("AzureCloud")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(accs, tc.DeepEquals, []azurecli.Account{{
		CloudName:    "AzureCloud",
		ID:           "d7ad3057-0000-0000-0000-513d7136eec5",
		IsDefault:    false,
		Name:         "Free Trial",
		State:        "Enabled",
		TenantId:     "b7bb0664-0000-0000-0000-4d5f1481ef22",
		HomeTenantId: "b7bb0664-0000-0000-0000-4d5f1481ef66",
	}, {
		CloudName: "AzureCloud",
		ID:        "5af17b7d-0000-0000-0000-5cd99887fdf7",
		IsDefault: true,
		Name:      "Pay-As-You-Go",
		State:     "Enabled",
		TenantId:  "2da419a9-0000-0000-0000-ac7c24bbe2e7",
	}})
	c.Assert(accs[0].AuthTenantId(), tc.Equals, accs[0].HomeTenantId)
	c.Assert(accs[1].AuthTenantId(), tc.Equals, accs[1].TenantId)
}

func (s *azSuite) TestFindAccountsWithCloudNameError(c *tc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account list --query [?cloudName=='AzureCloud'] -o json": {
					error: errors.New("test error"),
				},
			},
		}.Exec,
	}
	accs, err := azcli.FindAccountsWithCloudName("AzureCloud")
	c.Assert(err, tc.ErrorMatches, `execution failure: test error`)
	c.Assert(accs, tc.IsNil)
}

func (s *azSuite) TestShowCloud(c *tc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az cloud show -o json": {
					stdout: []byte(`
{
  "isActive": true,
  "name": "AzureCloud",
  "profile": "latest"
}
`[1:]),
				},
			},
		}.Exec,
	}
	cloud, err := azcli.ShowCloud("")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloud, tc.DeepEquals, &azurecli.Cloud{
		IsActive: true,
		Name:     "AzureCloud",
		Profile:  "latest",
	})
}

func (s *azSuite) TestShowCloudWithName(c *tc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az cloud show --name AzureUSGovernment -o json": {
					stdout: []byte(`
{
  "isActive": false,
  "name": "AzureUSGovernment",
  "profile": "latest"
}
`[1:]),
				},
			},
		}.Exec,
	}
	cloud, err := azcli.ShowCloud("AzureUSGovernment")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloud, tc.DeepEquals, &azurecli.Cloud{
		IsActive: false,
		Name:     "AzureUSGovernment",
		Profile:  "latest",
	})
}

func (s *azSuite) TestShowCloudError(c *tc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az cloud show -o json": {
					error: errors.New("test error"),
				},
			},
		}.Exec,
	}
	cloud, err := azcli.ShowCloud("")
	c.Assert(err, tc.ErrorMatches, `execution failure: test error`)
	c.Assert(cloud, tc.IsNil)
}

func (s *azSuite) TestListClouds(c *tc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az cloud list -o json": {
					stdout: []byte(`
[
  {
    "isActive": true,
    "name": "AzureCloud",
    "profile": "latest"
  },
  {
    "isActive": false,
    "name": "AzureChinaCloud",
    "profile": "latest"
  },
  {
    "isActive": false,
    "name": "AzureUSGovernment",
    "profile": "latest"
  },
  {
    "isActive": false,
    "name": "AzureGermanCloud",
    "profile": "latest"
  }
]

`[1:]),
				},
			},
		}.Exec,
	}
	clouds, err := azcli.ListClouds()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.DeepEquals, []azurecli.Cloud{{
		IsActive: true,
		Name:     "AzureCloud",
		Profile:  "latest",
	}, {
		IsActive: false,
		Name:     "AzureChinaCloud",
		Profile:  "latest",
	}, {
		IsActive: false,
		Name:     "AzureUSGovernment",
		Profile:  "latest",
	}, {
		IsActive: false,
		Name:     "AzureGermanCloud",
		Profile:  "latest",
	}})
}

func (s *azSuite) TestListCloudsError(c *tc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az cloud list -o json": {
					error: errors.New("test error"),
				},
			},
		}.Exec,
	}
	cloud, err := azcli.ListClouds()
	c.Assert(err, tc.ErrorMatches, `execution failure: test error`)
	c.Assert(cloud, tc.IsNil)
}

type result struct {
	stdout []byte
	error  error
}

type testExecutor struct {
	commands map[string]result
}

func (e testExecutor) Exec(cmd string, args []string) ([]byte, error) {
	c := strings.Join(append([]string{cmd}, args...), " ")
	r, ok := e.commands[c]
	if !ok {
		return nil, errors.Errorf("unexpected command '%s'", c)
	}
	return r.stdout, r.error
}
