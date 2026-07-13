// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker

import (
	"time"
)

// Token defines a token value with expiration time.
type Token struct {
	// Value is the value of the token.
	Value string

	// ExpiresAt is the unix time in seconds and milliseconds when the
	// authorization token expires.
	ExpiresAt *time.Time
}

// BasicAuthConfig contains authorization information for basic auth.
type BasicAuthConfig struct {
	// Auth is the base64 encoded "username:password" string.
	Auth *Token `json:"auth,omitempty" yaml:"auth,omitempty"`

	// Username holds the username used to gain access to a non-public image.
	Username string `json:"username" yaml:"username"`

	// Password holds the password used to gain access to a non-public image.
	Password string `json:"password" yaml:"password"`
}

// TokenAuthConfig contains authorization information for token auth.
// Juju does not support the docker credential helper because k8s does not
// support it either.
// https://kubernetes.io/docs/concepts/containers/images/#configuring-nodes-to-authenticate-to-a-private-registry
type TokenAuthConfig struct {
	Email string `json:"email,omitempty" yaml:"email,omitempty"`

	// IdentityToken is used to authenticate the user and get an access token
	// for the registry.
	IdentityToken *Token `json:"identitytoken,omitempty" yaml:"identitytoken,omitempty"`

	// RegistryToken is a bearer token to be sent to a registry.
	RegistryToken *Token `json:"registrytoken,omitempty" yaml:"registrytoken,omitempty"`
}

// ImageRepoDetails contains authorization information for connecting to a
// Registry.
type ImageRepoDetails struct {
	BasicAuthConfig `json:",inline" yaml:",inline"`
	TokenAuthConfig `json:",inline" yaml:",inline"`

	// Repository is the namespace of the image repo.
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`

	// ServerAddress is the auth server address.
	ServerAddress string `json:"serveraddress,omitempty" yaml:"serveraddress,omitempty"`

	// Region is the cloud region.
	Region string `json:"region,omitempty" yaml:"region,omitempty"`
}

// DockerImageDetails holds the details for a Docker resource type.
type DockerImageDetails struct {
	// RegistryPath holds the path of the Docker image (including host and
	// sha256) in a docker registry.
	RegistryPath string `json:"ImageName" yaml:"registrypath"`

	ImageRepoDetails `json:",inline" yaml:",inline"`
}
