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
	RotatePolicy *secrets.RotatePolicy `json:"rotate-policy,omitempty"`
	// ExpireTime is when a secret should expire.
	ExpireTime *time.Time `json:"expire-time,omitempty"`
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

	// ScopeTag is defines the entity to which the secret life is scoped.
	ScopeTag string `json:"scope-tag"`
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

// SecretURIArgs holds args for identifying secrets.
type SecretURIArgs struct {
	Args []SecretURIArg `json:"args"`
}

// SecretURIArg holds the args for identifying a secret.
type SecretURIArg struct {
	// URI identifies the secret.
	URI string `json:"uri"`
}

// SecretIdResult is the result of getting secret ID data.
type SecretIdResult struct {
	Label string `json:"label"`
}

// SecretIdResults holds results for getting secret IDs.
type SecretIdResults struct {
	Result map[string]SecretIdResult `json:"result"`
	Error  *Error                    `json:"error,omitempty"`
}

// GetSecretConsumerInfoArgs holds the args for getting secret
// consumer metadata.
type GetSecretConsumerInfoArgs struct {
	ConsumerTag string   `json:"consumer-tag"`
	URIs        []string `json:"uris"`
}

// SecretConsumerInfoResults holds secret value results.
type SecretConsumerInfoResults struct {
	Results []SecretConsumerInfoResult `json:"results"`
}

// SecretConsumerInfoResult is the result of getting a secret value.
type SecretConsumerInfoResult struct {
	Revision int    `json:"revision"`
	Label    string `json:"label"`
	Error    *Error `json:"error,omitempty"`
}

// GetSecretValueArgs holds the args for getting secret values.
type GetSecretValueArgs struct {
	Args []GetSecretValueArg `json:"args"`
}

// GetSecretValueArg holds the args for getting a secret value.
type GetSecretValueArg struct {
	URI    string `json:"uri"`
	Label  string `json:"label,omitempty"`
	Update bool   `json:"update,omitempty"`
	Peek   bool   `json:"peek,omitempty"`
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

// SecretsFilter is used when querying secrets.
type SecretsFilter struct {
	URI      *string `json:"uri,omitempty"`
	Revision *int    `json:"revision,omitempty"`
	OwnerTag *string `json:"owner-tag,omitempty"`
}

// ListSecretsArgs holds the args for listing secrets.
type ListSecretsArgs struct {
	ShowSecrets bool          `json:"show-secrets"`
	Filter      SecretsFilter `json:"filter"`
}

// ListSecretResults holds secret metadata results.
type ListSecretResults struct {
	Results []ListSecretResult `json:"results"`
}

type SecretRevision struct {
	Revision   int        `json:"revision"`
	CreateTime time.Time  `json:"create-time"`
	UpdateTime time.Time  `json:"update-time"`
	ExpireTime *time.Time `json:"expire-time,omitempty"`
}

// ListSecretResult is the result of getting secret metadata.
type ListSecretResult struct {
	URI              string             `json:"uri"`
	Version          int                `json:"version"`
	OwnerTag         string             `json:"owner-tag"`
	ScopeTag         string             `json:"scope-tag"`
	Provider         string             `json:"provider"`
	ProviderID       string             `json:"provider-id,omitempty"`
	RotatePolicy     string             `json:"rotate-policy,omitempty"`
	NextRotateTime   *time.Time         `json:"next-rotate-time,omitempty"`
	Description      string             `json:"description,omitempty"`
	Label            string             `json:"label,omitempty"`
	LatestRevision   int                `json:"latest-revision"`
	LatestExpireTime *time.Time         `json:"latest-expire-time,omitempty"`
	CreateTime       time.Time          `json:"create-time"`
	UpdateTime       time.Time          `json:"update-time"`
	Revisions        []SecretRevision   `json:"revisions"`
	Value            *SecretValueResult `json:"value,omitempty"`
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

// GrantRevokeSecretArgs holds args for changing access to secrets.
type GrantRevokeSecretArgs struct {
	Args []GrantRevokeSecretArg `json:"args"`
}

// GrantRevokeSecretArg holds the args for changing access to a secret.
type GrantRevokeSecretArg struct {
	// URI identifies the secret to grant.
	URI string `json:"uri"`

	// ScopeTag is defines the entity to which the access is scoped.
	ScopeTag string `json:"scope-tag"`

	// OwnerTag is the owner of the secret.
	SubjectTags []string `json:"subject-tags"`

	// Role is the role being granted.
	Role string `json:"role"`
}
