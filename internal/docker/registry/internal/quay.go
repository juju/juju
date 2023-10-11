// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/docker"
)

type quayContainerRegistry struct {
	*baseClient
}

func newQuayContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) (RegistryInternal, error) {
	c, err := newBase(repoDetails, transport, normalizeRepoDetailsCommon)
	if err != nil {
		return nil, err
	}
	return &quayContainerRegistry{c}, nil
}

// Match checks if the repository details matches current provider format.
func (c *quayContainerRegistry) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "quay.io")
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
	return errors.Trace(unwrapNetError(err))
}
