// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry/internal"
)

var logger = loggo.GetLogger("juju.docker.registry")

// Registry provides APIs to interact with the OCI provider client.
type Registry = internal.Registry

var (
	// Override for testing.
	DefaultTransport = http.DefaultTransport
)

// New returns a Registry interface providing methods for interacting with registry APIs.
func New(repoDetails docker.ImageRepoDetails) (Registry, error) {
	var provider internal.RegistryInternal = internal.NewBase(repoDetails, DefaultTransport)
	for _, providerNewer := range internal.Providers() {
		p := providerNewer(repoDetails, DefaultTransport)
		if p.Match() {
			logger.Tracef("found registry client %#v for %s", p, repoDetails)
			provider = p
			break
		}
	}
	if err := internal.InitProvider(provider); err != nil {
		return nil, errors.Trace(err)
	}
	return provider, nil
}
