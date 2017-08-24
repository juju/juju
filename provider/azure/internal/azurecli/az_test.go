// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azurecli_test

import (
	"os/exec"
	"strings"

	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/juju/errors"
	"github.com/juju/juju/provider/azure/internal/azurecli"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type azSuite struct{}

var _ = gc.Suite(&azSuite{})

func (s *azSuite) TestGetAccessToken(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account get-access-token -o json": result{
					stdout: []byte(`
{
  "accessToken": "ACCESSTOKEN",
  "expiresOn": "2017-06-07 09:27:58.063743",
  "subscription": "5544b9a5-0000-0000-0000-fedceb5d3696",
  "tenant": "a52afd7f-0000-0000-0000-e47a54b982da",
  "tokenType": "Bearer"
}
`[1:]),
				},
			},
		}.Exec,
	}
	tok, err := azcli.GetAccessToken("", "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tok, jc.DeepEquals, &azurecli.AccessToken{
		AccessToken:  "ACCESSTOKEN",
		ExpiresOn:    "2017-06-07 09:27:58.063743",
		Subscription: "5544b9a5-0000-0000-0000-fedceb5d3696",
		Tenant:       "a52afd7f-0000-0000-0000-e47a54b982da",
		TokenType:    "Bearer",
	})
}

func (s *azSuite) TestGetAccessTokenWithSubscriptionAndResource(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account get-access-token --subscription 5544b9a5-0000-0000-0000-fedceb5d3696 --resource resid -o json": result{
					stdout: []byte(`
{
  "accessToken": "ACCESSTOKEN",
  "expiresOn": "2017-06-07 09:27:58.063743",
  "subscription": "5544b9a5-0000-0000-0000-fedceb5d3696",
  "tenant": "a52afd7f-0000-0000-0000-e47a54b982da",
  "tokenType": "Bearer"
}
`[1:]),
				},
			},
		}.Exec,
	}
	tok, err := azcli.GetAccessToken("5544b9a5-0000-0000-0000-fedceb5d3696", "resid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tok, jc.DeepEquals, &azurecli.AccessToken{
		AccessToken:  "ACCESSTOKEN",
		ExpiresOn:    "2017-06-07 09:27:58.063743",
		Subscription: "5544b9a5-0000-0000-0000-fedceb5d3696",
		Tenant:       "a52afd7f-0000-0000-0000-e47a54b982da",
		TokenType:    "Bearer",
	})
}

func (s *azSuite) TestGetAccessTokenError(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account get-access-token -o json": result{
					error: &exec.ExitError{Stderr: []byte("test error")},
				},
			},
		}.Exec,
	}
	tok, err := azcli.GetAccessToken("", "")
	c.Assert(err, gc.ErrorMatches, `execution failure: test error`)
	c.Assert(tok, gc.IsNil)
}

func (s *azSuite) TestGetAccessTokenJSONError(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account get-access-token -o json": result{
					stdout: []byte(`}`),
				},
			},
		}.Exec,
	}
	tok, err := azcli.GetAccessToken("", "")
	c.Assert(err, gc.ErrorMatches, `cannot unmarshal output: invalid character '}' looking for beginning of value`)
	c.Assert(tok, gc.IsNil)
}

func (s *azSuite) TestAzureTokenFromAccessToken(c *gc.C) {
	tok := azurecli.AccessToken{
		AccessToken:  "0123456789",
		ExpiresOn:    "2017-06-05 10:20:43.752534",
		Subscription: "00000000-0000-0000-0000-00000001",
		Tenant:       "00000000-0000-0000-0000-00000002",
		TokenType:    "Bearer",
	}
	tok1 := tok.Token()
	c.Assert(tok1, jc.DeepEquals, &adal.Token{
		AccessToken: "0123456789",
		Type:        "Bearer",
	})
}

func (s *azSuite) TestShowAccount(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account show -o json": result{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(acc, jc.DeepEquals, &azurecli.Account{
		CloudName: "AzureCloud",
		ID:        "5544b9a5-0000-0000-0000-fedceb5d3696",
		IsDefault: true,
		Name:      "AccountName",
		State:     "Enabled",
		TenantId:  "a52afd7f-0000-0000-0000-e47a54b982da",
	})
}

func (s *azSuite) TestShowAccountWithSubscription(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account show --subscription 5544b9a5-0000-0000-0000-fedceb5d3696 -o json": result{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(acc, jc.DeepEquals, &azurecli.Account{
		CloudName: "AzureCloud",
		ID:        "5544b9a5-0000-0000-0000-fedceb5d3696",
		IsDefault: true,
		Name:      "AccountName",
		State:     "Enabled",
		TenantId:  "a52afd7f-0000-0000-0000-e47a54b982da",
	})
}

func (s *azSuite) TestShowAccountError(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account show -o json": result{
					error: &exec.ExitError{Stderr: []byte("test error\nusage ...")},
				},
			},
		}.Exec,
	}
	acc, err := azcli.ShowAccount("")
	c.Assert(err, gc.ErrorMatches, `execution failure: test error`)
	c.Assert(acc, gc.IsNil)
}

func (s *azSuite) TestListAccounts(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account list -o json": result{
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accs, jc.DeepEquals, []azurecli.Account{{
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

func (s *azSuite) TestListAccountsError(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account list -o json": result{
					error: errors.New("test error"),
				},
			},
		}.Exec,
	}
	accs, err := azcli.ListAccounts()
	c.Assert(err, gc.ErrorMatches, `execution failure: test error`)
	c.Assert(accs, gc.IsNil)
}

func (s *azSuite) TestFindAccountsWithCloudName(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account list --query [?cloudName=='AzureCloud'] -o json": result{
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
	accs, err := azcli.FindAccountsWithCloudName("AzureCloud")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accs, jc.DeepEquals, []azurecli.Account{{
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

func (s *azSuite) TestFindAccountsWithCloudNameError(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az account list --query [?cloudName=='AzureCloud'] -o json": result{
					error: errors.New("test error"),
				},
			},
		}.Exec,
	}
	accs, err := azcli.FindAccountsWithCloudName("AzureCloud")
	c.Assert(err, gc.ErrorMatches, `execution failure: test error`)
	c.Assert(accs, gc.IsNil)
}

func (s *azSuite) TestShowCloud(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az cloud show -o json": result{
					stdout: []byte(`
{
  "endpoints": {
    "activeDirectory": "https://login.microsoftonline.com",
    "activeDirectoryGraphResourceId": "https://graph.windows.net/",
    "activeDirectoryResourceId": "https://management.core.windows.net/",
    "batchResourceId": "https://batch.core.windows.net/",
    "gallery": "https://gallery.azure.com/",
    "management": "https://management.core.windows.net/",
    "resourceManager": "https://management.azure.com/",
    "sqlManagement": "https://management.core.windows.net:8443/"
  },
  "isActive": true,
  "name": "AzureCloud",
  "profile": "latest",
  "suffixes": {
    "azureDatalakeAnalyticsCatalogAndJobEndpoint": "azuredatalakeanalytics.net",
    "azureDatalakeStoreFileSystemEndpoint": "azuredatalakestore.net",
    "keyvaultDns": ".vault.azure.net",
    "sqlServerHostname": ".database.windows.net",
    "storageEndpoint": "core.windows.net"
  }
}
`[1:]),
				},
			},
		}.Exec,
	}
	cloud, err := azcli.ShowCloud("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloud, jc.DeepEquals, &azurecli.Cloud{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectory:                "https://login.microsoftonline.com",
			ActiveDirectoryGraphResourceID: "https://graph.windows.net/",
			ActiveDirectoryResourceID:      "https://management.core.windows.net/",
			BatchResourceID:                "https://batch.core.windows.net/",
			Management:                     "https://management.core.windows.net/",
			ResourceManager:                "https://management.azure.com/",
			SQLManagement:                  "https://management.core.windows.net:8443/",
		},
		IsActive: true,
		Name:     "AzureCloud",
		Profile:  "latest",
		Suffixes: azurecli.CloudSuffixes{
			AzureDatalakeAnalyticsCatalogAndJobEndpoint: "azuredatalakeanalytics.net",
			AzureDatalakeStoreFileSystemEndpoint:        "azuredatalakestore.net",
			KeyvaultDNS:                                 ".vault.azure.net",
			SQLServerHostname:                           ".database.windows.net",
			StorageEndpoint:                             "core.windows.net",
		},
	})
}

func (s *azSuite) TestShowCloudWithName(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az cloud show --name AzureUSGovernment -o json": result{
					stdout: []byte(`
{
  "endpoints": {
    "activeDirectory": "https://login.microsoftonline.com",
    "activeDirectoryGraphResourceId": "https://graph.windows.net/",
    "activeDirectoryResourceId": "https://management.core.usgovcloudapi.net/",
    "batchResourceId": "https://batch.core.usgovcloudapi.net/",
    "gallery": "https://gallery.usgovcloudapi.net/",
    "management": "https://management.core.usgovcloudapi.net/",
    "resourceManager": "https://management.usgovcloudapi.net/",
    "sqlManagement": "https://management.core.usgovcloudapi.net:8443/"
  },
  "isActive": false,
  "name": "AzureUSGovernment",
  "profile": "latest",
  "suffixes": {
    "azureDatalakeAnalyticsCatalogAndJobEndpoint": null,
    "azureDatalakeStoreFileSystemEndpoint": null,
    "keyvaultDns": ".vault.usgovcloudapi.net",
    "sqlServerHostname": ".database.usgovcloudapi.net",
    "storageEndpoint": "core.usgovcloudapi.net"
  }
}
`[1:]),
				},
			},
		}.Exec,
	}
	cloud, err := azcli.ShowCloud("AzureUSGovernment")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloud, jc.DeepEquals, &azurecli.Cloud{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectory:                "https://login.microsoftonline.com",
			ActiveDirectoryGraphResourceID: "https://graph.windows.net/",
			ActiveDirectoryResourceID:      "https://management.core.usgovcloudapi.net/",
			BatchResourceID:                "https://batch.core.usgovcloudapi.net/",
			Management:                     "https://management.core.usgovcloudapi.net/",
			ResourceManager:                "https://management.usgovcloudapi.net/",
			SQLManagement:                  "https://management.core.usgovcloudapi.net:8443/",
		},
		IsActive: false,
		Name:     "AzureUSGovernment",
		Profile:  "latest",
		Suffixes: azurecli.CloudSuffixes{
			KeyvaultDNS:       ".vault.usgovcloudapi.net",
			SQLServerHostname: ".database.usgovcloudapi.net",
			StorageEndpoint:   "core.usgovcloudapi.net",
		},
	})
}

func (s *azSuite) TestShowCloudError(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az cloud show -o json": result{
					error: errors.New("test error"),
				},
			},
		}.Exec,
	}
	cloud, err := azcli.ShowCloud("")
	c.Assert(err, gc.ErrorMatches, `execution failure: test error`)
	c.Assert(cloud, gc.IsNil)
}

func (s *azSuite) TestFindCloudsWithResourceManagerEndpoint(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az cloud list --query [?endpoints.resourceManager=='https://management.azure.com/'] -o json": result{
					stdout: []byte(`
[
  {
    "endpoints": {
      "activeDirectory": "https://login.microsoftonline.com",
      "activeDirectoryGraphResourceId": "https://graph.windows.net/",
      "activeDirectoryResourceId": "https://management.core.windows.net/",
      "batchResourceId": "https://batch.core.windows.net/",
      "gallery": "https://gallery.azure.com/",
      "management": "https://management.core.windows.net/",
      "resourceManager": "https://management.azure.com/",
      "sqlManagement": "https://management.core.windows.net:8443/"
    },
    "isActive": true,
    "name": "AzureCloud",
    "profile": "latest",
    "suffixes": {
      "azureDatalakeAnalyticsCatalogAndJobEndpoint": "azuredatalakeanalytics.net",
      "azureDatalakeStoreFileSystemEndpoint": "azuredatalakestore.net",
      "keyvaultDns": ".vault.azure.net",
      "sqlServerHostname": ".database.windows.net",
      "storageEndpoint": "core.windows.net"
    }
  }
]
`[1:]),
				},
			},
		}.Exec,
	}
	cloud, err := azcli.FindCloudsWithResourceManagerEndpoint("https://management.azure.com/")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloud, jc.DeepEquals, []azurecli.Cloud{{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectory:                "https://login.microsoftonline.com",
			ActiveDirectoryGraphResourceID: "https://graph.windows.net/",
			ActiveDirectoryResourceID:      "https://management.core.windows.net/",
			BatchResourceID:                "https://batch.core.windows.net/",
			Management:                     "https://management.core.windows.net/",
			ResourceManager:                "https://management.azure.com/",
			SQLManagement:                  "https://management.core.windows.net:8443/",
		},
		IsActive: true,
		Name:     "AzureCloud",
		Profile:  "latest",
		Suffixes: azurecli.CloudSuffixes{
			AzureDatalakeAnalyticsCatalogAndJobEndpoint: "azuredatalakeanalytics.net",
			AzureDatalakeStoreFileSystemEndpoint:        "azuredatalakestore.net",
			KeyvaultDNS:                                 ".vault.azure.net",
			SQLServerHostname:                           ".database.windows.net",
			StorageEndpoint:                             "core.windows.net",
		},
	}})
}

func (s *azSuite) TestFindCloudsWithResourceManagerEndpointError(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az cloud list --query [?endpoints.resourceManager=='https://management.azure.com/'] -o json": result{
					error: errors.New("test error"),
				},
			},
		}.Exec,
	}
	cloud, err := azcli.FindCloudsWithResourceManagerEndpoint("https://management.azure.com/")
	c.Assert(err, gc.ErrorMatches, `execution failure: test error`)
	c.Assert(cloud, gc.IsNil)
}

func (s *azSuite) TestListClouds(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az cloud list -o json": result{
					stdout: []byte(`
[
  {
    "endpoints": {
      "activeDirectory": "https://login.microsoftonline.com",
      "activeDirectoryGraphResourceId": "https://graph.windows.net/",
      "activeDirectoryResourceId": "https://management.core.windows.net/",
      "batchResourceId": "https://batch.core.windows.net/",
      "gallery": "https://gallery.azure.com/",
      "management": "https://management.core.windows.net/",
      "resourceManager": "https://management.azure.com/",
      "sqlManagement": "https://management.core.windows.net:8443/"
    },
    "isActive": true,
    "name": "AzureCloud",
    "profile": "latest",
    "suffixes": {
      "azureDatalakeAnalyticsCatalogAndJobEndpoint": "azuredatalakeanalytics.net",
      "azureDatalakeStoreFileSystemEndpoint": "azuredatalakestore.net",
      "keyvaultDns": ".vault.azure.net",
      "sqlServerHostname": ".database.windows.net",
      "storageEndpoint": "core.windows.net"
    }
  },
  {
    "endpoints": {
      "activeDirectory": "https://login.chinacloudapi.cn",
      "activeDirectoryGraphResourceId": "https://graph.chinacloudapi.cn/",
      "activeDirectoryResourceId": "https://management.core.chinacloudapi.cn/",
      "batchResourceId": "https://batch.chinacloudapi.cn/",
      "gallery": "https://gallery.chinacloudapi.cn/",
      "management": "https://management.core.chinacloudapi.cn/",
      "resourceManager": "https://management.chinacloudapi.cn",
      "sqlManagement": "https://management.core.chinacloudapi.cn:8443/"
    },
    "isActive": false,
    "name": "AzureChinaCloud",
    "profile": "latest",
    "suffixes": {
      "azureDatalakeAnalyticsCatalogAndJobEndpoint": null,
      "azureDatalakeStoreFileSystemEndpoint": null,
      "keyvaultDns": ".vault.azure.cn",
      "sqlServerHostname": ".database.chinacloudapi.cn",
      "storageEndpoint": "core.chinacloudapi.cn"
    }
  },
  {
    "endpoints": {
      "activeDirectory": "https://login.microsoftonline.com",
      "activeDirectoryGraphResourceId": "https://graph.windows.net/",
      "activeDirectoryResourceId": "https://management.core.usgovcloudapi.net/",
      "batchResourceId": "https://batch.core.usgovcloudapi.net/",
      "gallery": "https://gallery.usgovcloudapi.net/",
      "management": "https://management.core.usgovcloudapi.net/",
      "resourceManager": "https://management.usgovcloudapi.net/",
      "sqlManagement": "https://management.core.usgovcloudapi.net:8443/"
    },
    "isActive": false,
    "name": "AzureUSGovernment",
    "profile": "latest",
    "suffixes": {
      "azureDatalakeAnalyticsCatalogAndJobEndpoint": null,
      "azureDatalakeStoreFileSystemEndpoint": null,
      "keyvaultDns": ".vault.usgovcloudapi.net",
      "sqlServerHostname": ".database.usgovcloudapi.net",
      "storageEndpoint": "core.usgovcloudapi.net"
    }
  },
  {
    "endpoints": {
      "activeDirectory": "https://login.microsoftonline.de",
      "activeDirectoryGraphResourceId": "https://graph.cloudapi.de/",
      "activeDirectoryResourceId": "https://management.core.cloudapi.de/",
      "batchResourceId": "https://batch.cloudapi.de/",
      "gallery": "https://gallery.cloudapi.de/",
      "management": "https://management.core.cloudapi.de/",
      "resourceManager": "https://management.microsoftazure.de",
      "sqlManagement": "https://management.core.cloudapi.de:8443/"
    },
    "isActive": false,
    "name": "AzureGermanCloud",
    "profile": "latest",
    "suffixes": {
      "azureDatalakeAnalyticsCatalogAndJobEndpoint": null,
      "azureDatalakeStoreFileSystemEndpoint": null,
      "keyvaultDns": ".vault.microsoftazure.de",
      "sqlServerHostname": ".database.cloudapi.de",
      "storageEndpoint": "core.cloudapi.de"
    }
  }
]

`[1:]),
				},
			},
		}.Exec,
	}
	clouds, err := azcli.ListClouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, jc.DeepEquals, []azurecli.Cloud{{
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectory:                "https://login.microsoftonline.com",
			ActiveDirectoryGraphResourceID: "https://graph.windows.net/",
			ActiveDirectoryResourceID:      "https://management.core.windows.net/",
			BatchResourceID:                "https://batch.core.windows.net/",
			Management:                     "https://management.core.windows.net/",
			ResourceManager:                "https://management.azure.com/",
			SQLManagement:                  "https://management.core.windows.net:8443/",
		},
		IsActive: true,
		Name:     "AzureCloud",
		Profile:  "latest",
		Suffixes: azurecli.CloudSuffixes{
			AzureDatalakeAnalyticsCatalogAndJobEndpoint: "azuredatalakeanalytics.net",
			AzureDatalakeStoreFileSystemEndpoint:        "azuredatalakestore.net",
			KeyvaultDNS:                                 ".vault.azure.net",
			SQLServerHostname:                           ".database.windows.net",
			StorageEndpoint:                             "core.windows.net",
		},
	}, {
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectory:                "https://login.chinacloudapi.cn",
			ActiveDirectoryGraphResourceID: "https://graph.chinacloudapi.cn/",
			ActiveDirectoryResourceID:      "https://management.core.chinacloudapi.cn/",
			BatchResourceID:                "https://batch.chinacloudapi.cn/",
			Management:                     "https://management.core.chinacloudapi.cn/",
			ResourceManager:                "https://management.chinacloudapi.cn",
			SQLManagement:                  "https://management.core.chinacloudapi.cn:8443/",
		},
		IsActive: false,
		Name:     "AzureChinaCloud",
		Profile:  "latest",
		Suffixes: azurecli.CloudSuffixes{
			KeyvaultDNS:       ".vault.azure.cn",
			SQLServerHostname: ".database.chinacloudapi.cn",
			StorageEndpoint:   "core.chinacloudapi.cn",
		},
	}, {
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectory:                "https://login.microsoftonline.com",
			ActiveDirectoryGraphResourceID: "https://graph.windows.net/",
			ActiveDirectoryResourceID:      "https://management.core.usgovcloudapi.net/",
			BatchResourceID:                "https://batch.core.usgovcloudapi.net/",
			Management:                     "https://management.core.usgovcloudapi.net/",
			ResourceManager:                "https://management.usgovcloudapi.net/",
			SQLManagement:                  "https://management.core.usgovcloudapi.net:8443/",
		},
		IsActive: false,
		Name:     "AzureUSGovernment",
		Profile:  "latest",
		Suffixes: azurecli.CloudSuffixes{
			KeyvaultDNS:       ".vault.usgovcloudapi.net",
			SQLServerHostname: ".database.usgovcloudapi.net",
			StorageEndpoint:   "core.usgovcloudapi.net",
		},
	}, {
		Endpoints: azurecli.CloudEndpoints{
			ActiveDirectory:                "https://login.microsoftonline.de",
			ActiveDirectoryGraphResourceID: "https://graph.cloudapi.de/",
			ActiveDirectoryResourceID:      "https://management.core.cloudapi.de/",
			BatchResourceID:                "https://batch.cloudapi.de/",
			Management:                     "https://management.core.cloudapi.de/",
			ResourceManager:                "https://management.microsoftazure.de",
			SQLManagement:                  "https://management.core.cloudapi.de:8443/",
		},
		IsActive: false,
		Name:     "AzureGermanCloud",
		Profile:  "latest",
		Suffixes: azurecli.CloudSuffixes{
			KeyvaultDNS:       ".vault.microsoftazure.de",
			SQLServerHostname: ".database.cloudapi.de",
			StorageEndpoint:   "core.cloudapi.de",
		},
	}})
}

func (s *azSuite) TestListCloudsError(c *gc.C) {
	azcli := azurecli.AzureCLI{
		Exec: testExecutor{
			commands: map[string]result{
				"az cloud list -o json": result{
					error: errors.New("test error"),
				},
			},
		}.Exec,
	}
	cloud, err := azcli.ListClouds()
	c.Assert(err, gc.ErrorMatches, `execution failure: test error`)
	c.Assert(cloud, gc.IsNil)
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
