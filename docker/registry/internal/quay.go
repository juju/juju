// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/tools"
)

type quayContainerRegistry struct {
	*baseClient
}

func newQuayContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, transport)
	return &quayContainerRegistry{c}
}

// Match checks if the repository details matches current provider format.
func (c *quayContainerRegistry) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "quay.io")
}

// APIVersion returns the registry API version to use.
func (c *quayContainerRegistry) APIVersion() APIVersion {
	// quay container registry always uses v2.
	return APIVersionV2
}

func (c quayContainerRegistry) url(pathTemplate string, args ...interface{}) string {
	return commonURLGetter(c.APIVersion(), *c.baseURL, pathTemplate, args...)
}

// DecideBaseURL decides the API url to use.
func (c *quayContainerRegistry) DecideBaseURL() error {
	return errors.Trace(decideBaseURLCommon(c.APIVersion(), c.repoDetails, c.baseURL))
}

// Tags fetches tags for an OCI image.
func (c quayContainerRegistry) Tags(imageName string) (versions tools.Versions, err error) {
	// google container registry always uses v2.
	return fetchTagsV2(c, imageName)
}

// Ping pings the quay endpoint.
func (c quayContainerRegistry) Ping() error {
	if !c.repoDetails.IsPrivate() {
		// quay.io root path requires authentication.
		// So skip ping for public repositories.
		return nil
	}
	url := c.url("/")
	logger.Debugf("quay container registry ping %q", url)
	resp, err := c.client.Get(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	return errors.Trace(err)
}
