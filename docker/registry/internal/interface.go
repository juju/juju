// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"time"

	"github.com/juju/juju/docker"
	"github.com/juju/juju/internal/tools"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/registry_mock.go github.com/juju/juju/docker/registry/internal Initializer

// Registry provides APIs to interact with the OCI provider client.
type Registry interface {
	Tags(string) (tools.Versions, error)
	GetArchitecture(imageName, tag string) (string, error)
	Close() error
	Ping() error
	ImageRepoDetails() docker.ImageRepoDetails
	ShouldRefreshAuth() (bool, *time.Duration)
	RefreshAuth() error
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
	WrapTransport(...TransportWrapper) error
	DecideBaseURL() error
}
