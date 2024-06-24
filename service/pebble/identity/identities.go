// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package identity

type IdentitiesFile struct {
	Identities map[string]*Identity `json:"identities" yaml:"identities"`
}

// Identity holds the configuration of a single identity.
type Identity struct {
	Access IdentityAccess `json:"access" yaml:"access"`

	// One or more of the following type-specific configuration fields must be
	// non-nil (currently the only type is "local").
	Local *LocalIdentity `json:"local,omitempty" yaml:"local,omitempty"`
}

// IdentityAccess defines the access level for an identity.
type IdentityAccess string

const (
	AdminAccess     IdentityAccess = "admin"
	ReadAccess      IdentityAccess = "read"
	UntrustedAccess IdentityAccess = "untrusted"
)

// LocalIdentity holds identity configuration specific to the "local" type
// (for ucrednet/UID authentication).
type LocalIdentity struct {
	// This is a pointer so we can distinguish between not set and 0 (a valid
	// user-id meaning root).
	UserID *uint32 `json:"user-id" yaml:"user-id"`
}
