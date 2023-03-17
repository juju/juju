// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/core/secrets"
)

// SecretBackendConfigResultsV1 holds config info for creating
// secret backend clients for a specific model.
type SecretBackendConfigResultsV1 struct {
	ControllerUUID string                         `json:"model-controller"`
	ModelUUID      string                         `json:"model-uuid"`
	ModelName      string                         `json:"model-name"`
	ActiveID       string                         `json:"active-id"`
	Configs        map[string]SecretBackendConfig `json:"configs,omitempty"`
}

// SecretBackendArgs holds args for querying secret backends.
type SecretBackendArgs struct {
	BackendIDs []string `json:"backend-ids"`
}

// SecretBackendConfigResults holds config info for creating
// secret backend clients for a specific model.
type SecretBackendConfigResults struct {
	ActiveID string                               `json:"active-id"`
	Results  map[string]SecretBackendConfigResult `json:"results,omitempty"`
}

// SecretBackendConfigResult holds config info for creating
// secret backend clients for a specific model.
type SecretBackendConfigResult struct {
	ControllerUUID string              `json:"model-controller"`
	ModelUUID      string              `json:"model-uuid"`
	ModelName      string              `json:"model-name"`
	Draining       bool                `json:"draining"`
	Config         SecretBackendConfig `json:"config,omitempty"`
}

// SecretBackendConfig holds config for creating a secret backend client.
type SecretBackendConfig struct {
	BackendType string                 `json:"type"`
	Params      map[string]interface{} `json:"params,omitempty"`
}

// SecretContentParams holds params for representing the content of a secret.
type SecretContentParams struct {
	// Data is the key values of the secret value itself.
	Data map[string]string `json:"data,omitempty"`
	// ValueRef is the content reference for when a secret
	// backend like vault is used.
	ValueRef *SecretValueRef `json:"value-ref,omitempty"`
}

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
	Content SecretContentParams `json:"content,omitempty"`
}

// CreateSecretURIsArg holds args for creating secret URIs.
type CreateSecretURIsArg struct {
	Count int `json:"count"`
}

// CreateSecretArgs holds args for creating secrets.
type CreateSecretArgs struct {
	Args []CreateSecretArg `json:"args"`
}

// CreateSecretArg holds the args for creating a secret.
type CreateSecretArg struct {
	UpsertSecretArg

	// URI identifies the secret to create.
	// If empty, the controller generates a URI.
	URI *string `json:"uri,omitempty"`
	// OwnerTag is the owner of the secret.
	OwnerTag string `json:"owner-tag"`
}

// UpdateSecretArgs holds args for updating secrets.
type UpdateSecretArgs struct {
	Args []UpdateSecretArg `json:"args"`
}

// UpdateSecretArg holds the args for updating a secret.
type UpdateSecretArg struct {
	UpsertSecretArg

	// URI identifies the secret to update.
	URI string `json:"uri"`
}

// DeleteSecretArgs holds args for deleting secrets.
type DeleteSecretArgs struct {
	Args []DeleteSecretArg `json:"args"`
}

// DeleteSecretArg holds the args for deleting a secret.
type DeleteSecretArg struct {
	URI       string `json:"uri"`
	Revisions []int  `json:"revisions,omitempty"`
}

// SecretRevisionArg holds the args for secret revisions.
type SecretRevisionArg struct {
	URI           string `json:"uri"`
	Revisions     []int  `json:"revisions"`
	PendingDelete bool   `json:"pending-delete"`
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

// GetSecretContentArgs holds the args for getting secret values.
type GetSecretContentArgs struct {
	Args []GetSecretContentArg `json:"args"`
}

// GetSecretContentArg holds the args for getting a secret value.
type GetSecretContentArg struct {
	URI     string `json:"uri"`
	Label   string `json:"label,omitempty"`
	Refresh bool   `json:"refresh,omitempty"`
	Peek    bool   `json:"peek,omitempty"`
}

// SecretContentResults holds secret value results.
type SecretContentResults struct {
	Results []SecretContentResult `json:"results"`
}

// SecretContentResult is the result of getting secret content.
type SecretContentResult struct {
	Content SecretContentParams `json:"content"`
	Error   *Error              `json:"error,omitempty"`
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

// SecretValueRef holds a reference to a secret
// value in a secret backend.
type SecretValueRef struct {
	BackendID  string `json:"backend-id"`
	RevisionID string `json:"revision-id"`
}

// SecretRevision holds secret revision metadata.
type SecretRevision struct {
	Revision    int             `json:"revision"`
	ValueRef    *SecretValueRef `json:"value-ref,omitempty"`
	BackendName *string         `json:"backend-name,omitempty"`
	CreateTime  time.Time       `json:"create-time,omitempty"`
	UpdateTime  time.Time       `json:"update-time,omitempty"`
	ExpireTime  *time.Time      `json:"expire-time,omitempty"`
}

// ListSecretResult is the result of getting secret metadata.
type ListSecretResult struct {
	URI              string             `json:"uri"`
	Version          int                `json:"version"`
	OwnerTag         string             `json:"owner-tag"`
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

// SecretTriggerChange describes a change to a secret trigger.
type SecretTriggerChange struct {
	URI             string    `json:"uri"`
	Revision        int       `json:"revision,omitempty"`
	NextTriggerTime time.Time `json:"next-trigger-time"`
}

// SecretTriggerWatchResult holds secret trigger change events.
type SecretTriggerWatchResult struct {
	WatcherId string                `json:"watcher-id"`
	Changes   []SecretTriggerChange `json:"changes"`
	Error     *Error                `json:"error,omitempty"`
}

// SecretRotatedArgs holds the args for updating rotated secret info.
type SecretRotatedArgs struct {
	Args []SecretRotatedArg `json:"args"`
}

// SecretRotatedArg holds the args for updating rotated secret info.
type SecretRotatedArg struct {
	URI              string `json:"uri"`
	OriginalRevision int    `json:"original-revision"`
	Skip             bool   `json:"skip"`
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

// ListSecretBackendsResults holds secret backend results.
type ListSecretBackendsResults struct {
	Results []SecretBackendResult `json:"results"`
}

// SecretBackendResult holds a secret backend and related info.
type SecretBackendResult struct {
	Result SecretBackend `json:"result"`
	// Include the ID so we can report on backends with errors.
	ID         string `json:"id"`
	NumSecrets int    `json:"num-secrets"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	Error      *Error `json:"error,omitempty"`
}

// AddSecretBackendArgs holds args for adding secret backends.
type AddSecretBackendArgs struct {
	Args []AddSecretBackendArg `json:"args"`
}

// AddSecretBackendArg holds args for adding a secret backend.
type AddSecretBackendArg struct {
	SecretBackend
	// Include the ID so we can optionally
	// import existing backend metadata.
	ID string `json:"id,omitempty"`
}

// UpdateSecretBackendArgs holds args for updating secret backends.
type UpdateSecretBackendArgs struct {
	Args []UpdateSecretBackendArg `json:"args"`
}

// UpdateSecretBackendArg holds args for updating a secret backend.
type UpdateSecretBackendArg struct {
	// Name is the name of the backend to update.
	Name string `json:"name"`

	// NameChange if set, renames the backend.
	NameChange *string `json:"name-change,omitempty"`

	// TokenRotateInterval is the interval to rotate
	// the backend master access token.
	TokenRotateInterval *time.Duration `json:"token-rotate-interval"`

	// Config are the backend's updated configuration attributes.
	Config map[string]interface{} `json:"config"`

	// Reset contains attributes to clear or reset.
	Reset []string `json:"reset"`

	// Force means to update the backend even if a ping fails.
	Force bool `json:"force,omitempty"`
}

// ListSecretBackendsArgs holds the args for listing secret backends.
type ListSecretBackendsArgs struct {
	Names  []string `json:"names"`
	Reveal bool     `json:"reveal"`
}

// SecretBackend holds secret backend details.
type SecretBackend struct {
	// Name is the name of the backend.
	Name string `json:"name"`

	// Backend is the backend provider, eg "vault".
	BackendType string `json:"backend-type"`

	// TokenRotateInterval is the interval to rotate
	// the backend master access token.
	TokenRotateInterval *time.Duration `json:"token-rotate-interval,omitempty"`

	// Config are the backend's configuration attributes.
	Config map[string]interface{} `json:"config"`
}

// RemoveSecretBackendArgs holds args for removing secret backends.
type RemoveSecretBackendArgs struct {
	Args []RemoveSecretBackendArg `json:"args"`
}

// RemoveSecretBackendArg holds args for removing a secret backend.
type RemoveSecretBackendArg struct {
	Name  string `json:"name"`
	Force bool   `json:"force,omitempty"`
}

// RotateSecretBackendArgs holds the args for updating rotated secret backend info.
type RotateSecretBackendArgs struct {
	BackendIDs []string `json:"backend-ids"`
}

// SecretBackendRotateChange describes a change to a secret backend rotation.
type SecretBackendRotateChange struct {
	ID              string    `json:"id"`
	Name            string    `json:"backend-name"`
	NextTriggerTime time.Time `json:"next-trigger-time"`
}

// SecretBackendRotateWatchResult holds secret backend rotate change events.
type SecretBackendRotateWatchResult struct {
	WatcherId string                      `json:"watcher-id"`
	Changes   []SecretBackendRotateChange `json:"changes"`
	Error     *Error                      `json:"error,omitempty"`
}
