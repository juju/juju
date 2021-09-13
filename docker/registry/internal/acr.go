// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/tools"
)

// TODO(ycliuhw): test and verify azureContainerRegistry integration further.
type azureContainerRegistry struct {
	*baseClient
}

func newAzureContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, DefaultTransport)
	return &azureContainerRegistry{c}
}

// Match checks if the repository details matches current provider format.
func (c *azureContainerRegistry) Match() bool {
	c.prepare()
	return strings.Contains(c.repoDetails.ServerAddress, "azurecr.io")
}

func getUserNameFromAuth(auth string) (string, error) {
	content, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return "", errors.Trace(err)
	}
	parts := strings.Split(string(content), ":")
	if len(parts) < 1 {
		return "", errors.NotValidf("auth %q", auth)
	}
	return parts[0], nil
}

func (c *azureContainerRegistry) WrapTransport() error {
	transport := c.client.Transport
	if c.repoDetails.IsPrivate() && !c.repoDetails.TokenAuthConfig.Empty() {
		username := c.repoDetails.Username
		if username == "" {
			var err error
			username, err = getUserNameFromAuth(c.repoDetails.Auth)
			if err != nil {
				return errors.Trace(err)
			}
		}
		password := c.repoDetails.Password
		if password == "" {
			password = c.repoDetails.IdentityToken
		}
		transport = newTokenTransport(
			transport,
			username, password,
			"", "", false,
		)
	}
	c.client.Transport = newErrorTransport(transport)
	return nil
}

// Tags fetches tags for an OCI image.
func (c azureContainerRegistry) Tags(imageName string) (versions tools.Versions, err error) {
	apiVersion := c.APIVersion()

	if apiVersion == APIVersionV1 {
		url := c.url("/repositories/%s/tags", imageName)
		var response tagsResponseV1
		return c.fetchTags(url, &response)
	}
	if apiVersion == APIVersionV2 {
		url := c.url("/%s/tags/list", imageName)
		var response tagsResponseV2
		return c.fetchTags(url, &response)
	}
	// This should never happen.
	return nil, nil
}
