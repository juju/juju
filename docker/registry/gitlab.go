// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"net/http"
	"strings"

	"github.com/juju/juju/docker"
)

type gitlab struct {
	*baseClient
}

func newGitlab(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, DefaultTransport)
	return &gitlab{c}
}

func (c *gitlab) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "registry.gitlab.com")
}

func (c *gitlab) WrapTransport() error {
	if !c.repoDetails.IsPrivate() {
		return nil
	}
	transport := c.client.Transport
	if !c.repoDetails.BasicAuthConfig.Empty() {
		transport = newTokenTransport(
			transport, c.repoDetails.Username, c.repoDetails.Password, c.repoDetails.Auth, "",
		)
	}
	c.client.Transport = errorTransport{transport}
	return nil
}
