// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry/internal"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.docker.registry")

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/http_mock.go net/http RoundTripper
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/registry_mock.go github.com/juju/juju/internal/docker/registry/internal Registry

// Registry provides APIs to interact with the OCI provider client.
type Registry = internal.Registry

var (
	// Override for testing.
	DefaultTransport = http.DefaultTransport
)

// New returns a Registry interface providing methods for interacting with registry APIs.
func New(repoDetails docker.ImageRepoDetails) (Registry, error) {
	var provider internal.RegistryInternal
	for _, providerNewer := range internal.Providers() {
		p, err := providerNewer(repoDetails, DefaultTransport)
		if err != nil {
			logger.Tracef("error matching registry client %#v for %s: %s", p, repoDetails, err.Error())
			continue
		}
		if p.Match() {
			logger.Tracef("found registry client %#v for %s", p, repoDetails)
			provider = p
			break
		}
	}
	var err error
	if provider == nil {
		provider, err = internal.NewBase(repoDetails, DefaultTransport)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	if err := internal.InitProvider(provider); err != nil {
		return nil, errors.Trace(err)
	}
	return provider, nil
}
