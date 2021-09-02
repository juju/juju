// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
)

const (
	dockerServerAddress = "index.docker.io"
)

type dockerhub struct {
	*baseClient
}

func newDockerhub(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, DefaultTransport)
	return &dockerhub{c}
}

func (c *dockerhub) Match() bool {
	return c.repoDetails.ServerAddress == "" || strings.Contains(c.repoDetails.ServerAddress, "docker")
}

func (c *dockerhub) WrapTransport() error {
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

func (c *dockerhub) DecideBaseURL() error {
	if c.repoDetails.ServerAddress == "" {
		c.repoDetails.ServerAddress = dockerServerAddress
	}
	if err := c.baseClient.DecideBaseURL(); err != nil {
		return errors.Trace(err)
	}
	url, err := url.Parse(c.repoDetails.ServerAddress)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Criticalf("dockerhub.DecideBaseURL url.String(1) => %q", url.String())
	url.Scheme = "https"
	logger.Criticalf("dockerhub.DecideBaseURL url.String(2) => %q", url.String())
	addr := url.String()
	if !strings.HasSuffix(addr, "/") {
		// This "/" matters because docker uses url string for the credential key and expects the trailing slash.
		addr += "/"
	}
	c.repoDetails.ServerAddress = addr
	logger.Criticalf("dockerhub.DecideBaseURL c.repoDetails.ServerAddress => %q", c.repoDetails.ServerAddress)
	return nil
}
