// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package credential provides the domain for managing cloud credentials in Juju.
//
// Credentials are authentication information required to access cloud providers.
// The credential domain manages credential storage, validation, and lifecycle,
// including handling invalid or revoked credentials.
//
// # Key Concepts
//
// Credentials contain:
//   - Auth type: authentication method (e.g., oauth2, userpass, access-key)
//   - Attributes: provider-specific authentication data
//   - Owner: the Juju user who owns the credential
//   - Cloud: the cloud the credential authenticates to
//   - Validity state: valid, invalid, or revoked
//
// # Credential Lifecycle
//
// Credentials are:
//   - Added by users with cloud-specific authentication data
//   - Associated with models during model creation
//   - Validated periodically by the credential validator
//   - Marked invalid when authentication fails
//   - Revoked when compromised or no longer needed
package credential
