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
	c := newBase(repoDetails, transport)
	return &azureContainerRegistry{c}
}

// Match checks if the repository details matches current provider format.
func (c *azureContainerRegistry) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "azurecr.io")
}

func unpackAuthToken(auth string) (string, string, error) {
	content, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return "", "", errors.Annotate(err, "doing base64 decode on the auth token")
	}
	parts := strings.Split(string(content), ":")
	if len(parts) < 2 {
		return "", "", errors.NotValidf("registry auth token")
	}
	return parts[0], parts[1], nil
}

func azureContainerRegistryTransport(
	transport http.RoundTripper, repoDetails *docker.ImageRepoDetails,
) (http.RoundTripper, error) {
	if repoDetails.IsPrivate() && !repoDetails.TokenAuthConfig.Empty() {
		username := repoDetails.Username
		if username == "" {
			var err error
			username, _, err = unpackAuthToken(repoDetails.Auth)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		password := repoDetails.Password
		if password == "" {
			password = repoDetails.IdentityToken
		}
		transport = newTokenTransport(
			transport,
			username, password,
			"", "", false,
		)
	}
	return transport, nil
}

func (c *azureContainerRegistry) WrapTransport(wrappers ...TransportWrapper) (err error) {
	if c.client.Transport, err = mergeTransportWrappers(
		c.client.Transport, c.repoDetails, azureContainerRegistryTransport, wrapErrorTransport,
	); err != nil {
		return errors.Trace(err)
	}
	return nil
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
