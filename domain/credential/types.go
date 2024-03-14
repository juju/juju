// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credential

// CloudCredentialInfo represents a credential.
type CloudCredentialInfo struct {
	// AuthType is the credential auth type.
	AuthType string

	// Attributes are the credential attributes.
	Attributes map[string]string

	// Revoked is true if the credential has been revoked.
	Revoked bool

	// Label is optionally set to describe the credentials to a user.
	Label string

	// Invalid is true if the credential is invalid.
	Invalid bool

	// InvalidReason contains the reason why a credential was flagged as invalid.
	// It is expected that this string will be empty when a credential is valid.
	InvalidReason string
}

// CloudCredentialResult represents a credential and the cloud it belongs to.
type CloudCredentialResult struct {
	CloudCredentialInfo

	// CloudName is the cloud the credential belongs to.
	CloudName string
}
