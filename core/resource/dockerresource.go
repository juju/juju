// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	_ "crypto/sha256"
	_ "crypto/sha512"
	"time"
)

// Token defines a token value with expiration time.
type Token struct {
	// Value is the value of the token.
	Value string

	// ExpiresAt is the unix time in seconds and milliseconds when the authorization token expires.
	ExpiresAt *time.Time
}

// NewToken creates a Token.
func NewToken(value string) *Token {
	if value == "" {
		return nil
	}
	return &Token{Value: value}
}

// Empty checks if the auth information is empty.
func (t *Token) Empty() bool {
	return t == nil || t.Value == ""
}

// Content returns the raw content of the token.
func (t *Token) Content() string {
	if t.Empty() {
		return ""
	}
	return t.Value
}

// BasicAuthConfig contains authorization information for basic auth.
type BasicAuthConfig struct {
	// Auth is the base64 encoded "username:password" string.
	Auth *Token

	// Username holds the username used to gain access to a non-public image.
	Username string

	// Password holds the password used to gain access to a non-public image.
	Password string
}

// Empty checks if the auth information is empty.
func (ba BasicAuthConfig) Empty() bool {
	return ba.Auth.Empty() && ba.Username == "" && ba.Password == ""
}

// TokenAuthConfig contains authorization information for token auth.
// Juju does not support the docker credential helper because k8s does not support it either.
// https://kubernetes.io/docs/concepts/containers/images/#configuring-nodes-to-authenticate-to-a-private-registry
type TokenAuthConfig struct {
	Email string

	// IdentityToken is used to authenticate the user and get
	// an access token for the registry.
	IdentityToken *Token

	// RegistryToken is a bearer token to be sent to a registry
	RegistryToken *Token
}

// Empty checks if the auth information is empty.
func (ac TokenAuthConfig) Empty() bool {
	return ac.RegistryToken.Empty() && ac.IdentityToken.Empty()
}

// ImageRepoDetails contains authorization information for connecting to a Registry.
type ImageRepoDetails struct {
	BasicAuthConfig
	TokenAuthConfig

	// Repository is the namespace of the image repo.
	Repository string

	// ServerAddress is the auth server address.
	ServerAddress string

	// Region is the cloud region.
	Region string
}

// IsPrivate checks if the repository detail is private.
func (rid ImageRepoDetails) IsPrivate() bool {
	return !rid.BasicAuthConfig.Empty() || !rid.TokenAuthConfig.Empty()
}

// DockerImageDetails holds the details for a Docker resource type.
type DockerImageDetails struct {
	// RegistryPath holds the path of the Docker image (including host and sha256) in a docker registry.
	RegistryPath string

	ImageRepoDetails
}

// IsPrivate shows if the image repo is private or not.
func (did DockerImageDetails) IsPrivate() bool {
	return did.ImageRepoDetails.IsPrivate()
}
