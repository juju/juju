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

type acr struct {
	*baseClient
}

func newACR(repoDetails docker.ImageRepoDetails, transport http.RoundTripper) RegistryInternal {
	c := newBase(repoDetails, DefaultTransport)
	return &acr{c}
}

func (c *acr) Match() bool {
	return strings.Contains(c.repoDetails.ServerAddress, "azurecr.io")
}

func getUserNameFromAuthForACR(auth string) (string, error) {
	content, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return "", errors.Trace(err)
	}
	parts := strings.Split(string(content), ":")
	if len(parts) < 1 {
		return "", errors.NotValidf("auth %q", auth)
	}
	return parts[0], nil
}

func (c *acr) WrapTransport() error {
	if !c.repoDetails.IsPrivate() {
		return nil
	}
	transport := c.client.Transport
	if !c.repoDetails.TokenAuthConfig.Empty() {
		username := c.repoDetails.Username
		if username == "" {
			var err error
			username, err = getUserNameFromAuthForACR(c.repoDetails.Auth)
			if err != nil {
				return errors.Trace(err)
			}
		}
		password := c.repoDetails.Password
		if password == "" {
			password = c.repoDetails.IdentityToken
		}
		transport = newTokenTransport(
			transport,
			username, password,
			"", "",
		)
	}
	c.client.Transport = errorTransport{transport}
	return nil
}
