// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"github.com/juju/juju/docker"
	"github.com/juju/juju/tools"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/http_mock.go net/http RoundTripper
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/registry_mock.go github.com/juju/juju/docker/registry Registry,RegistryInternal,Matcher,Initializer

// Registry provides APIs to interact with the OCI provider client.
type Registry interface {
	Tags(string) (tools.Versions, error)
	Close() error
	Ping() error
	ImageRepoDetails() docker.ImageRepoDetails
}

// RegistryInternal provides methods of registry clients for internal operations.
// It's exported for generating mocks for tests.
type RegistryInternal interface {
	Matcher
	Registry
	Initializer
}

// Matcher provides a method for selecting which registry client to use.
type Matcher interface {
	Match() bool
}

// Initializer provides methods for initializing the registry client.
type Initializer interface {
	WrapTransport() error
	DecideBaseURL() error
	Ping() error
}
