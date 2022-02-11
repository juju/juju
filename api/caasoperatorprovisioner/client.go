// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/base"
	charmscommon "github.com/juju/juju/api/common/charms"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

// Client allows access to the CAAS operator provisioner API endpoint.
type Client struct {
	facade base.FacadeCaller
	*charmscommon.CharmInfoClient
	*charmscommon.ApplicationCharmInfoClient
}

// NewClient returns a client used to access the CAAS Operator Provisioner API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASOperatorProvisioner")
	charmInfoClient := charmscommon.NewCharmInfoClient(facadeCaller)
	appCharmInfoClient := charmscommon.NewApplicationCharmInfoClient(facadeCaller)
	return &Client{
		facade:                     facadeCaller,
		CharmInfoClient:            charmInfoClient,
		ApplicationCharmInfoClient: appCharmInfoClient,
	}
}

// WatchApplications returns a StringsWatcher that notifies of
// changes to the lifecycles of CAAS applications in the current model.
func (c *Client) WatchApplications() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	if err := c.facade.FacadeCall("WatchApplications", nil, &result); err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// ApplicationPassword holds parameters for setting
// an application password.
type ApplicationPassword struct {
	Name     string
	Password string
}

// SetPasswords sets API passwords for the specified applications.
func (c *Client) SetPasswords(appPasswords []ApplicationPassword) (params.ErrorResults, error) {
	var result params.ErrorResults
	args := params.EntityPasswords{Changes: make([]params.EntityPassword, len(appPasswords))}
	for i, p := range appPasswords {
		args.Changes[i] = params.EntityPassword{
			Tag: names.NewApplicationTag(p.Name).String(), Password: p.Password,
		}
	}
	err := c.facade.FacadeCall("SetPasswords", args, &result)
	if err != nil {
		return params.ErrorResults{}, err
	}
	if len(result.Results) != len(args.Changes) {
		return params.ErrorResults{}, errors.Errorf("expected %d result(s), got %d", len(args.Changes), len(result.Results))
	}
	return result, nil
}

// maybeNotFound returns an error satisfying errors.IsNotFound
// if the supplied error has a CodeNotFound error.
func maybeNotFound(err *params.Error) error {
	if err == nil || !params.IsCodeNotFound(err) {
		return err
	}
	return errors.NewNotFound(err, "")
}

// Life returns the lifecycle state for the specified CAAS application
// or unit in the current model.
func (c *Client) Life(appName string) (life.Value, error) {
	if !names.IsValidApplication(appName) {
		return "", errors.NotValidf("application name %q", appName)
	}
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewApplicationTag(appName).String()}},
	}

	var results params.LifeResults
	if err := c.facade.FacadeCall("Life", args, &results); err != nil {
		return "", err
	}
	if n := len(results.Results); n != 1 {
		return "", errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return "", maybeNotFound(err)
	}
	return results.Results[0].Life, nil
}

// OperatorProvisioningInfo holds the info needed to provision an operator.
type OperatorProvisioningInfo struct {
	ImageDetails resources.DockerImageDetails
	Version      version.Number
	APIAddresses []string
	Tags         map[string]string
	CharmStorage *storage.KubernetesFilesystemParams
}

// OperatorProvisioningInfo returns the info needed to provision an operator for an application.
func (c *Client) OperatorProvisioningInfo(applicationName string) (OperatorProvisioningInfo, error) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: names.NewApplicationTag(applicationName).String()},
	}}
	var result params.OperatorProvisioningInfoResults
	if err := c.facade.FacadeCall("OperatorProvisioningInfo", args, &result); err != nil {
		return OperatorProvisioningInfo{}, err
	}
	if len(result.Results) != 1 {
		return OperatorProvisioningInfo{}, errors.Errorf("expected one result, got %d", len(result.Results))
	}
	info := result.Results[0]
	if err := info.Error; err != nil {
		return OperatorProvisioningInfo{}, errors.Trace(err)
	}
	imageRepo := resources.DockerImageDetails{
		RegistryPath: info.ImageDetails.RegistryPath,
		ImageRepoDetails: docker.ImageRepoDetails{
			Repository:    info.ImageDetails.Repository,
			ServerAddress: info.ImageDetails.ServerAddress,
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: info.ImageDetails.Username,
				Password: info.ImageDetails.Password,
				Auth:     docker.NewToken(info.ImageDetails.Auth),
			},
			TokenAuthConfig: docker.TokenAuthConfig{
				IdentityToken: docker.NewToken(info.ImageDetails.IdentityToken),
				RegistryToken: docker.NewToken(info.ImageDetails.RegistryToken),
				Email:         info.ImageDetails.Email,
			},
		},
	}
	return OperatorProvisioningInfo{
		ImageDetails: imageRepo,
		Version:      info.Version,
		APIAddresses: info.APIAddresses,
		Tags:         info.Tags,
		CharmStorage: filesystemFromParams(info.CharmStorage),
	}, nil
}

func filesystemFromParams(in *params.KubernetesFilesystemParams) *storage.KubernetesFilesystemParams {
	if in == nil {
		return nil
	}
	return &storage.KubernetesFilesystemParams{
		StorageName:  in.StorageName,
		Provider:     storage.ProviderType(in.Provider),
		Size:         in.Size,
		Attributes:   in.Attributes,
		ResourceTags: in.Tags,
	}
}

// OperatorCertificate provides all the information an operator needs to
// create a TLS listener.
type OperatorCertificate struct {
	CACert     string
	Cert       string
	PrivateKey string
}

// IssueOperatorCertificate issues an x509 certificate for use by the specified application operator.
func (c *Client) IssueOperatorCertificate(applicationName string) (OperatorCertificate, error) {
	if !names.IsValidApplication(applicationName) {
		return OperatorCertificate{}, errors.NotValidf("application name %q", applicationName)
	}
	args := params.Entities{Entities: []params.Entity{
		{Tag: names.NewApplicationTag(applicationName).String()},
	}}
	var result params.IssueOperatorCertificateResults
	if err := c.facade.FacadeCall("IssueOperatorCertificate", args, &result); err != nil {
		return OperatorCertificate{}, errors.Trace(err)
	}
	if len(result.Results) != 1 {
		return OperatorCertificate{}, errors.Errorf("expected one result, got %d", len(result.Results))
	}
	certInfo := result.Results[0]
	if err := certInfo.Error; err != nil {
		return OperatorCertificate{}, errors.Trace(err)
	}
	return OperatorCertificate{
		CACert:     certInfo.CACert,
		Cert:       certInfo.Cert,
		PrivateKey: certInfo.PrivateKey,
	}, nil
}
