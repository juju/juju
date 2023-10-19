// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package usersecretsdrain provides the backend
// implementation for the usersecretsdrain facade.
package usersecretsdrain

import (
	coresecrets "github.com/juju/juju/core/secrets"
)

// SecretsState is the interface for the state package.
type SecretsState interface {
	GetSecret(*coresecrets.URI) (*coresecrets.SecretMetadata, error)
	GetSecretValue(*coresecrets.URI, int) (coresecrets.SecretValue, *coresecrets.ValueRef, error)
}
