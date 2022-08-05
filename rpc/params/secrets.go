// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/core/secrets"
)

// UpsertSecretArg holds the args for creating or updating a secret.
type UpsertSecretArg struct {
	// RotatePolicy is how often a secret should be rotated.
	RotatePolicy *secrets.RotatePolicy `json:"rotate-policy"`
	// Expiry is when a secret should expire.
	Expiry *time.Time `json:"expiry"`
	// Description represents the secret's description.
	Description *string `json:"description,omitempty"`
	// Tags are the secret tags.
	Label *string `json:"label,omitempty"`
	// Params are used when generating secrets server side.
	// See core/secrets/secret.go.
	Params map[string]interface{} `json:"params,omitempty"`
	// Data is the key values of the secret value itself.
	Data map[string]string `json:"data,omitempty"`
}

// CreateSecretArgs holds args for creating secrets.
type CreateSecretArgs struct {
	Args []CreateSecretArg `json:"args"`
}

// CreateSecretArg holds the args for creating a secret.
type CreateSecretArg struct {
	UpsertSecretArg

	// OwnerTag is the owner of the secret.
	OwnerTag string `json:"owner-tag"`
}

// UpdateSecretArgs holds args for creating secrets.
type UpdateSecretArgs struct {
	Args []UpdateSecretArg `json:"args"`
}

// UpdateSecretArg holds the args for creating a secret.
type UpdateSecretArg struct {
	UpsertSecretArg

	// URI identifies the secret to update.
	URI string `json:"uri"`
}

// GetSecretArgs holds the args for getting secrets.
type GetSecretArgs struct {
	Args []GetSecretArg `json:"args"`
}

// GetSecretArg holds the args for getting a secret.
type GetSecretArg struct {
	URI string `json:"uri"`
}

// SecretValueResults holds secret value results.
type SecretValueResults struct {
	Results []SecretValueResult `json:"results"`
}

// SecretValueResult is the result of getting a secret value.
type SecretValueResult struct {
	Data  map[string]string `json:"data,omitempty"`
	Error *Error            `json:"error,omitempty"`
}

// ListSecretsArgs holds the args for listing secrets.
type ListSecretsArgs struct {
	ShowSecrets bool `json:"show-secrets"`
}

// ListSecretResults holds secret metadata results.
type ListSecretResults struct {
	Results []ListSecretResult `json:"results"`
}

// ListSecretResult is the result of getting secret metadata.
type ListSecretResult struct {
	URI            string             `json:"uri"`
	Version        int                `json:"version"`
	RotateInterval time.Duration      `json:"rotate-interval"`
	Description    string             `json:"description,omitempty"`
	OwnerTag       string             `json:"owner-tag"`
	Tags           map[string]string  `json:"tags,omitempty"`
	Provider       string             `json:"provider"`
	ProviderID     string             `json:"provider-id,omitempty"`
	Revision       int                `json:"revision"`
	CreateTime     time.Time          `json:"create-time"`
	UpdateTime     time.Time          `json:"update-time"`
	Value          *SecretValueResult `json:"value,omitempty"`
}

// SecretRotationChange describes a change to a secret rotation config.
type SecretRotationChange struct {
	URI            string        `json:"uri"`
	RotateInterval time.Duration `json:"rotate-interval"`
	LastRotateTime time.Time     `json:"last-rotate-time"`
}

// SecretRotationWatchResult holds secret rotation change events.
type SecretRotationWatchResult struct {
	SecretRotationWatcherId string                 `json:"watcher-id"`
	Changes                 []SecretRotationChange `json:"changes"`
	Error                   *Error                 `json:"error,omitempty"`
}

// SecretRotationWatchResults holds the results for any API call which ends up
// returning a list of SecretRotationWatchResult.
type SecretRotationWatchResults struct {
	Results []SecretRotationWatchResult `json:"results"`
}

// SecretRotatedArgs holds the args for updating rotated secret info.
type SecretRotatedArgs struct {
	Args []SecretRotatedArg `json:"args"`
}

// SecretRotatedArg holds the args for updating rotated secret info.
type SecretRotatedArg struct {
	URI  string    `json:"uri"`
	When time.Time `json:"when"`
}
