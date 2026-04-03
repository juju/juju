// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package secret manages the lifecycle and access control of secrets.
//
// Secrets are sensitive data (like passwords, tokens, or certificates) that are
// stored in versioned revisions and managed through backends -- such as Vault,
// Kubernetes, or the built-in controller backend -- to ensure security and
// persistence. Each secret is addressed by a unique URI and can be owned by a
// model (user secrets), an application, or a unit (charm secrets).
//
// See github.com/juju/juju/domain/secretbackend for details on how different
// backend providers are configured and selected. See
// github.com/juju/juju/core/secrets for the core types used to represent secret
// URIs, metadata, and values across the codebase. See the sections below for
// package-level concerns that span multiple interfaces.
//
// # Secret Access and Grants
//
// Access to a secret is managed through grants, which define the role (view or
// manage) and the scope (unit, application, model, or relation) of the
// permissions. This allows for fine-grained control over which entities can
// read or update a secret's revisions.
//
//	Grant Request           Validation              Stored Grant
//	+---------------+       +---------------+       +---------------+
//	| Subject: unit |       | Check owner   |       | Secret URI    |
//	| Role: view    | ----> | and context   | ----> | Subject UUID  |
//	| Scope: rel    |       |               |       | Role/Scope    |
//	+---------------+       +---------------+       +---------------+
//
// # Rotation and Expiry
//
// Secrets can be configured with rotation policies (hourly, daily, weekly,
// etc.) that determine when a secret should be updated. The service tracks
// rotation metadata, including the next rotation time, and notifies watchers
// when a rotation event is required. Similarly, secret revisions can have an
// expiry time, after which they are considered obsolete and may be pruned.
//
//	Policy Set              Monitoring              Rotation Event
//	+---------------+       +---------------+       +---------------+
//	| Set policy:   |       | Watch next    |       | Trigger       |
//	| daily         | ----> | rotate time   | ----> | rotation      |
//	|               |       |               |       | worker        |
//	+---------------+       +---------------+       +---------------+
package secret
