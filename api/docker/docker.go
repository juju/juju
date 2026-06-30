// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package docker provides the public API types for OCI image and registry
// credential representation. These types carry the JSON and YAML struct tags
// required for wire-level serialization so that external clients (e.g. the
// Terraform Juju provider) can marshal resources that the Juju controller can
// deserialize correctly.
//
// The internal representation lives in github.com/juju/juju/internal/docker;
// the [FromInternal] and [ToInternal] functions convert between the two.
package docker

import (
	"time"
)

// Token defines a token value with expiration time.
//
// The internal docker.Token type implements custom JSON marshalling (it
// marshals as a bare string), so this type mirrors that behaviour by also
// marshalling as a bare string value. This keeps YAML/JSON output identical
// to what the Juju controller expects.
type Token struct {
	// Value is the value of the token.
	Value string

	// ExpiresAt is the unix time in seconds and milliseconds when the
	// authorization token expires.
	ExpiresAt *time.Time
}

// Content returns the raw content of the token.
func (t *Token) Content() string {
	if t == nil || t.Value == "" {
		return ""
	}
	return t.Value
}

// NewToken creates a Token.
func NewToken(value string) *Token {
	if value == "" {
		return nil
	}
	return &Token{Value: value}
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

// Empty checks if the auth information is empty.
func (ba BasicAuthConfig) Empty() bool {
	return (ba.Auth == nil || ba.Auth.Value == "") && ba.Username == "" && ba.Password == ""
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

// Empty checks if the auth information is empty.
func (ac TokenAuthConfig) Empty() bool {
	return (ac.RegistryToken == nil || ac.RegistryToken.Value == "") &&
		(ac.IdentityToken == nil || ac.IdentityToken.Value == "")
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

// IsPrivate checks if the repository detail is private.
func (rid ImageRepoDetails) IsPrivate() bool {
	return !rid.BasicAuthConfig.Empty() || !rid.TokenAuthConfig.Empty()
}

// DockerImageDetails holds the details for a Docker resource type.
type DockerImageDetails struct {
	// RegistryPath holds the path of the Docker image (including host and
	// sha256) in a docker registry.
	RegistryPath string `json:"ImageName" yaml:"registrypath"`

	ImageRepoDetails `json:",inline" yaml:",inline"`
}

// IsPrivate shows if the image repo is private or not.
func (did DockerImageDetails) IsPrivate() bool {
	return did.ImageRepoDetails.IsPrivate()
}
