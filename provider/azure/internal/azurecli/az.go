// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azurecli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
)

// Logger for the Azure provider.
var logger = loggo.GetLogger("juju.provider.azure.internal.azurecli")

// AzureCLI
type AzureCLI struct {
	// Exec is a function that executes system commands and returns
	// the output. If this is nil then a default implementation using
	// os.exec will be used.
	Exec func(cmd string, args []string) (stdout []byte, err error)
}

// Error represents an error returned from the Azure CLI.
type Error struct {
	exec.ExitError
}

// Error implements the error interface.
func (e *Error) Error() string {
	if len(e.Stderr) == 0 {
		return e.ExitError.Error()
	}
	n := bytes.IndexByte(e.Stderr, '\n')
	if n < 0 {
		return string(e.Stderr)
	}
	return string(e.Stderr[:n])
}

// exec runs the given command using Exec if specified, or
// os.exec.Command.
func (a AzureCLI) exec(cmd string, args []string) ([]byte, error) {
	var out []byte
	var err error
	if a.Exec != nil {
		out, err = a.Exec(cmd, args)
	} else {
		out, err = exec.Command(cmd, args...).Output()
	}
	if exitError, ok := errors.Cause(err).(*exec.ExitError); ok {
		err = &Error{
			ExitError: *exitError,
		}
	}
	return out, err
}

// run attempts to execute "az" with the given arguments. Unmarshalling
// the json output into v.
func (a AzureCLI) run(v interface{}, args ...string) error {
	args = append(args, "-o", "json")
	logger.Debugf("running az %s", strings.Join(args, " "))
	b, err := a.exec("az", args)
	if err != nil {
		return errors.Annotate(err, "execution failure")
	}
	if err := json.Unmarshal(b, v); err != nil {
		return errors.Annotate(err, "cannot unmarshal output")
	}
	return nil
}

// AccessToken contains the result of the GetAccessToken function.
type AccessToken struct {
	AccessToken  string `json:"accessToken"`
	ExpiresOn    string `json:"expiresOn"`
	Subscription string `json:"subscription"`
	Tenant       string `json:"tenant"`
	TokenType    string `json:"tokenType"`
}

// Token creates an adal.Token from the AccessToken. This token can be
// used with go-autorest to access azure endpoints.
func (t AccessToken) Token() *adal.Token {
	return &adal.Token{
		AccessToken: t.AccessToken,
		Type:        t.TokenType,
	}
}

// GetAccessToken gets an access token from the Azure CLI to access the
// given resource using the given subscription. Either subscription or
// resource may be empty in which case the default from the az
// application are used.
func (a AzureCLI) GetAccessToken(subscription, resource string) (*AccessToken, error) {
	cmd := []string{"account", "get-access-token"}
	if subscription != "" {
		cmd = append(cmd, "--subscription", subscription)
	}
	if resource != "" {
		cmd = append(cmd, "--resource", resource)
	}
	var tok AccessToken
	if err := a.run(&tok, cmd...); err != nil {
		return nil, errors.Trace(err)
	}
	return &tok, nil
}

// Account contains details of an azure account (subscription).
type Account struct {
	CloudName string `json:"cloudName"`
	ID        string `json:"id"`
	IsDefault bool   `json:"isDefault"`
	Name      string `json:"name"`
	State     string `json:"state"`
	TenantId  string `json:"tenantId"`
}

// showAccount is a version of Account, but that can handle the subtle
// difference in output from az account show.
type showAccount struct {
	Account
	EnvironmentName string `json:"environmentName"`
}

// ShowAccount returns the account details for the account with the given
// subscription ID. If the subscription is empty then the default Azure
// CLI account is returned.
func (a AzureCLI) ShowAccount(subscription string) (*Account, error) {
	cmd := []string{"account", "show"}
	if subscription != "" {
		cmd = append(cmd, "--subscription", subscription)
	}
	var acc showAccount
	if err := a.run(&acc, cmd...); err != nil {
		return nil, errors.Trace(err)
	}
	if acc.Account.CloudName == "" {
		acc.Account.CloudName = acc.EnvironmentName
	}
	return &acc.Account, nil
}

// ListAccounts returns the details for all accounts available in the
// Azure CLI.
func (a AzureCLI) ListAccounts() ([]Account, error) {
	var accounts []Account
	if err := a.run(&accounts, "account", "list"); err != nil {
		return nil, errors.Trace(err)
	}
	return accounts, nil
}

// FindAccountsWithCloudName returns the details for all accounts with
// the given cloud name..
func (a AzureCLI) FindAccountsWithCloudName(name string) ([]Account, error) {
	var accounts []Account
	cmd := []string{
		"account",
		"list",
		"--query", fmt.Sprintf("[?cloudName=='%s']", name),
	}
	if err := a.run(&accounts, cmd...); err != nil {
		return nil, errors.Trace(err)
	}
	return accounts, nil
}

// Cloud contains details of a cloud configured in the Azure CLI.
type Cloud struct {
	Endpoints CloudEndpoints `json:"endpoints"`
	IsActive  bool           `json:"isActive"`
	Name      string         `json:"name"`
	Profile   string         `json:"profile"`
	Suffixes  CloudSuffixes  `json:"suffixes"`
}

// CloudEndpoints contains the endpoints used by a cloud.
type CloudEndpoints struct {
	ActiveDirectory                string `json:"activeDirectory"`
	ActiveDirectoryGraphResourceID string `json:"activeDirectoryGraphResourceId"`
	ActiveDirectoryResourceID      string `json:"activeDirectoryResourceId"`
	BatchResourceID                string `json:"batchResourceId"`
	Management                     string `json:"management"`
	ResourceManager                string `json:"resourceManager"`
	SQLManagement                  string `json:"sqlManagement"`
}

// CloudSuffixes contains the suffixes used with a cloud.
type CloudSuffixes struct {
	AzureDatalakeAnalyticsCatalogAndJobEndpoint string `json:"azureDatalakeAnalyticsCatalogAndJobEndpoint"`
	AzureDatalakeStoreFileSystemEndpoint        string `json:"azureDatalakeStoreFileSystemEndpoint"`
	KeyvaultDNS                                 string `json:"keyvaultDns"`
	SQLServerHostname                           string `json:"sqlServerHostname"`
	StorageEndpoint                             string `json:"storageEndpoint"`
}

// ShowCloud returns the details of the cloud with the given name. If the
// name is empty then the details of the default cloud will be returned.
func (a AzureCLI) ShowCloud(name string) (*Cloud, error) {
	cmd := []string{"cloud", "show"}
	if name != "" {
		cmd = append(cmd, "--name", name)
	}
	var cloud Cloud
	if err := a.run(&cloud, cmd...); err != nil {
		return nil, err
	}
	return &cloud, nil
}

// FindCloudsWithResourceManagerEndpoint returns a list of clouds which
// use the given url for it's resource manager endpoint.
func (a AzureCLI) FindCloudsWithResourceManagerEndpoint(url string) ([]Cloud, error) {
	var clouds []Cloud
	cmd := []string{
		"cloud",
		"list",
		"--query",
		fmt.Sprintf("[?endpoints.resourceManager=='%s']", url),
	}
	if err := a.run(&clouds, cmd...); err != nil {
		return nil, err
	}
	return clouds, nil
}

// ListClouds returns the details for all clouds available in the Azure
// CLI.
func (a AzureCLI) ListClouds() ([]Cloud, error) {
	var clouds []Cloud
	if err := a.run(&clouds, "cloud", "list"); err != nil {
		return nil, errors.Trace(err)
	}
	return clouds, nil
}
