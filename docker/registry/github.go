// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/docker"
)

type github struct {
	*baseClient
}

func newGithub(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, DefaultTransport)
	return &github{c}
}

func (c *github) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "ghcr.io")
}

func getBearerTokenForGithub(auth string) (string, error) {
	if auth == "" {
		return "", errors.NotValidf("empty github container registry auth token")
	}
	content, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return "", errors.Trace(err)
	}
	parts := strings.Split(string(content), ":")
	if len(parts) < 2 {
		return "", errors.NotValidf("github container registry auth token %q", auth)
	}
	token := parts[1]
	return base64.StdEncoding.EncodeToString([]byte(token)), nil
}

func (c *github) WrapTransport() error {
	if !c.repoDetails.IsPrivate() {
		return nil
	}
	transport := c.client.Transport
	if !c.repoDetails.BasicAuthConfig.Empty() {
		bearerToken, err := getBearerTokenForGithub(c.repoDetails.Auth)
		if err != nil {
			return errors.Trace(err)
		}
		transport = newTokenTransport(
			transport, "", "", "", bearerToken,
		)
	}
	c.client.Transport = errorTransport{transport}
	return nil
}

// Ping pings the github endpoint.
func (c github) Ping() error {
	url := c.url("/")
	if !strings.HasSuffix(url, "/") {
		// github v2 root endpoint requires the trailing slash(otherwise 404 returns).
		url += "/"
	}
	logger.Debugf("github ping %q", url)
	resp, err := c.client.Get(url)
	if resp != nil {
		defer resp.Body.Close()
	}
	return errors.Trace(err)
}
