// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package secret provides the domain for managing secrets in Juju.
//
// Secrets in Juju allow applications to securely store and retrieve sensitive
// information such as passwords, API keys, and certificates. The secret domain
// manages secret lifecycle, access control, and rotation policies.
//
// # Key Concepts
//
// Secrets have:
//   - Owners: applications or units that own the secret
//   - Consumers: applications or units granted access to the secret
//   - Revisions: versioned content with rotation support
//   - Backends: storage locations (Juju internal or external providers)
//   - Rotation policies: automatic or manual rotation schedules
//
// # Secret Lifecycle
//
// Secrets can be:
//   - Created by applications or users
//   - Granted to specific consumers with read or manage permissions
//   - Rotated manually or according to a policy
//   - Revoked when no longer needed
//   - Migrated during model migration
package secret
