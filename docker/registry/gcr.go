// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

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

func (c *googleContainerRegistry) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "gcr.io")
}

func (c *googleContainerRegistry) WrapTransport() error {
	return errors.NotSupportedf("googleContainerRegistry")
}
