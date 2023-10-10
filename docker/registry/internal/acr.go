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
	c := newBase(repoDetails, transport, normalizeRepoDetailsAzure)
	return &azureContainerRegistry{c}
}

func normalizeRepoDetailsAzure(repoDetails *docker.ImageRepoDetails) {
	if repoDetails.ServerAddress == "" {
		repoDetails.ServerAddress = repoDetails.Repository
	}
}

func (c *azureContainerRegistry) String() string {
	return "azurecr.io"
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
	// acr puts the namespace under subdomain.
	url := c.url("/%s/tags/list", imageName)
	var response tagsResponseV2
	return c.fetchTags(url, &response)
}

// GetArchitecture returns the archtecture of the image for the specified tag.
func (c azureContainerRegistry) GetArchitecture(imageName, tag string) (string, error) {
	return getArchitecture(imageName, tag, c)
}

// GetManifests returns the manifests of the image for the specified tag.
func (c azureContainerRegistry) GetManifests(imageName, tag string) (*ManifestsResult, error) {
	url := c.url("/%s/manifests/%s", imageName, tag)
	return c.GetManifestsCommon(url)
}

// GetBlobs gets the archtecture of the image for the specified tag via blobs API.
func (c azureContainerRegistry) GetBlobs(imageName, digest string) (*BlobsResponse, error) {
	url := c.url("/%s/blobs/%s", imageName, digest)
	return c.GetBlobsCommon(url)
}
