// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

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
	c := newBase(repoDetails, DefaultTransport)
	return &elasticContainerRegistry{c}
}

func (c *elasticContainerRegistry) Match() bool {
	c.prepare()
	return strings.Contains(c.repoDetails.ServerAddress, "ecr.aws")
}

func (c *elasticContainerRegistry) WrapTransport() error {
	return errors.NotSupportedf("AWS elastic container registry")
}
