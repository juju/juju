// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/docker/registry/internal"
)

var logger = loggo.GetLogger("juju.docker.registry")

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/registry_mock.go github.com/juju/juju/docker/registry Registry

// Registry provides APIs to interact with the OCI provider client.
type Registry = internal.Registry

// New returns a Registry interface providing methods for interacting with registry APIs.
func New(repoDetails docker.ImageRepoDetails) (Registry, error) {
	var provider internal.RegistryInternal = internal.NewBase(repoDetails, internal.DefaultTransport)
	for _, providerNewer := range internal.Providers() {
		p := providerNewer(repoDetails, internal.DefaultTransport)
		if p.Match() {
			logger.Tracef("found registry client %#v for %#v", p, repoDetails)
			provider = p
			break
		}
	}
	if err := internal.InitProvider(provider); err != nil {
		return nil, errors.Trace(err)
	}
	return provider, nil
}
