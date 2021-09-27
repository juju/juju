// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
)

type elasticContainerRegistry struct {
	*baseClient
}

func newElasticContainerRegistry(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, transport)
	return &elasticContainerRegistry{c}
}

// Match checks if the repository details matches current provider format.
func (c *elasticContainerRegistry) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "ecr.aws")
}

func (c *elasticContainerRegistry) WrapTransport(...TransportWrapper) error {
	return errors.NotSupportedf("AWS elastic container registry")
}
