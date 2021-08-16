// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"regexp"
	"strings"

	"github.com/juju/errors"
)

// Scope represents the scope of a secret.
type Scope string

const (
	ScopeApplication = Scope("application")
)

var validScopes = map[Scope]bool{
	ScopeApplication: true,
}

// SecretType is the type of a secret.
// This is used when creating a secret.
type SecretType string

const (
	TypeBlob     = SecretType("blob")
	TypePassword = SecretType("password")
)

var validTypes = map[SecretType]bool{
	TypeBlob:     true,
	TypePassword: true,
}

// SecretConfig is used when cresting a secret.
type SecretConfig struct {
	Type   SecretType
	Path   string
	Scope  Scope
	Params map[string]interface{}
}

// NewSecretConfig is used to create an application scoped blob secret.
func NewSecretConfig(nameParts ...string) *SecretConfig {
	return &SecretConfig{
		Type:  TypeBlob,
		Scope: ScopeApplication,
		Path:  strings.Join(nameParts, "."),
	}
}

// TODO(wallyworld) - use a schema to describe the config
const (
	PasswordLength       = "password-length"
	PasswordSpecialChars = "password-special-chars"
)

// NewPasswordSecretConfig is used to create an application scoped password secret.
func NewPasswordSecretConfig(length int, specialChars bool, nameParts ...string) *SecretConfig {
	return &SecretConfig{
		Type:  TypePassword,
		Scope: ScopeApplication,
		Path:  strings.Join(nameParts, "."),
		Params: map[string]interface{}{
			PasswordLength:       length,
			PasswordSpecialChars: specialChars,
		},
	}
}

var pathRegexp = regexp.MustCompile(`^[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*$`)

// Validate returns an error if the config is not valid.
func (c *SecretConfig) Validate() error {
	if _, ok := validTypes[c.Type]; !ok {
		return errors.NotValidf("secret type %q", c.Type)
	}
	if _, ok := validScopes[c.Scope]; !ok {
		return errors.NotValidf("secret scope %q", c.Scope)
	}
	if !pathRegexp.MatchString(c.Path) {
		return errors.NotValidf("secret path %q", c.Path)
	}
	return nil
}
