// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
)

type quayContainerRegistry struct {
	*baseClient
}

func newQuayContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, DefaultTransport)
	return &quayContainerRegistry{c}
}

// Match checks if the repository details matches current provider format.
func (c *quayContainerRegistry) Match() bool {
	c.prepare()
	return strings.Contains(c.repoDetails.ServerAddress, "quay.io")
}

func (c *quayContainerRegistry) WrapTransport() error {
	return errors.NotSupportedf("quay.io container registry")
}
