// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/tools"
)

type azureContainerRegistry struct {
	*baseClient
}

func newAzureContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, transport)
	c.repoDetails.ServerAddress = c.repoDetails.Repository
	return &azureContainerRegistry{c}
}

// Match checks if the repository details matches current provider format.
func (c *azureContainerRegistry) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "azurecr.io")
}

func (c *azureContainerRegistry) WrapTransport(...TransportWrapper) error {
	if c.repoDetails.BasicAuthConfig.Empty() {
		return errors.NewNotValid(nil, fmt.Sprintf(`username and password are required for registry %q`, c.repoDetails.Repository))
	}
	return c.baseClient.WrapTransport()
}

// Tags fetches tags for an OCI image.
func (c azureContainerRegistry) Tags(imageName string) (versions tools.Versions, err error) {
	apiVersion := c.APIVersion()

	// acr puts the namespace under subdomain.
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
