// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
)

type gcr struct {
	*baseClient
}

func newGCR(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, DefaultTransport)
	return &gcr{c}
}

func (c *gcr) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "gcr.io")
}

func (c *gcr) WrapTransport() error {
	return errors.NotSupportedf("GCR")
}
