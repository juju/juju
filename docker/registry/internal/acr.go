// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"net/http"
	"strings"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/tools"
)

// TODO(ycliuhw): test and verify azureContainerRegistry integration further.
type azureContainerRegistry struct {
	*baseClient
}

func newAzureContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, transport)
	return &azureContainerRegistry{c}
}

// Match checks if the repository details matches current provider format.
func (c *azureContainerRegistry) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "azurecr.io")
}

func (c *azureContainerRegistry) WrapTransport(...TransportWrapper) error {
	return c.baseClient.WrapTransport(newPrivateOnlyTransport)
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
