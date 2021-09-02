// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
)

type quay struct {
	*baseClient
}

func newQuay(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, DefaultTransport)
	return &quay{c}
}

func (c *quay) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "quay.io")
}

func (c *quay) WrapTransport() error {
	return errors.NotSupportedf("quay.io container registry")
}
