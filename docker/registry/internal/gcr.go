// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
)

type googleContainerRegistry struct {
	*baseClient
}

func newGoogleContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, DefaultTransport)
	return &googleContainerRegistry{c}
}

// Match checks if the repository details matches current provider format.
func (c *googleContainerRegistry) Match() bool {
	c.prepare()
	return strings.Contains(c.repoDetails.ServerAddress, "gcr.io")
}

func (c *googleContainerRegistry) WrapTransport() error {
	return errors.NotSupportedf("google container registry")
}
