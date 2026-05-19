// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package secret manages the lifecycle and access control of secrets.
//
// See the sections below for details about this package.
// See github.com/juju/juju/domain/secretbackend for backend configuration.
// See github.com/juju/juju/core/secrets for core secret types.
//
// # How this package works
//
// **Secrets**: Secrets are sensitive data (like passwords, tokens, or
// certificates) that are stored in versioned revisions and managed through
// backends -- such as Vault, Kubernetes, or the built-in controller backend --
// to ensure security and persistence. Each secret is addressed by a unique URI
// and can be owned by a model (user secrets), an application, or a unit (charm
// secrets).
//
// **Secret access and grants**: Access to a secret is managed through grants,
// which define the role (view or manage) and the scope (unit, application,
// model, or relation) of the permissions. This allows for fine-grained control
// over which entities can read or update a secret's revisions.
//
//	Grant Request           Validation              Stored Grant
//	+---------------+       +---------------+       +---------------+
//	| Subject: unit |       | Check owner   |       | Secret URI    |
//	| Role: view    | ----> | and context   | ----> | Subject UUID  |
//	| Scope: rel    |       |               |       | Role/Scope    |
//	+---------------+       +---------------+       +---------------+
//
// **Secret rotation and expiry**: Secrets can be configured with rotation policies
// (hourly, daily, weekly, etc.) that determine when a secret should be updated.
// The service tracks rotation metadata, including the next rotation time, and
// notifies watchers when a rotation event is required. Similarly, secret
// revisions can have an expiry time, after which they are considered obsolete
// and may be pruned.
//
//	Policy Set              Monitoring              Rotation Event
//	+---------------+       +---------------+       +---------------+
//	| Set policy:   |       | Watch next    |       | Trigger       |
//	| daily         | ----> | rotate time   | ----> | rotation      |
//	|               |       |               |       | worker        |
//	+---------------+       +---------------+       +---------------+
//
// # How to use this package correctly
//
// **Architecture**: This package MUST NOT import from apiserver or
// internal/worker packages. Domain logic must remain transport-agnostic. Only
// core/* and domain/* packages may be imported for type definitions.
//
// >> Is this layer boundary enforcement a formal contract for all domain
// >> packages?
//
// >> **Concurrency**: Are there any concurrency guarantees this package
// >> provides? Thread-safety for reads? Required serialization for writes?
// >> If not verifiable, this category will be omitted.
//
// **Error handling**: NotFound errors MUST use secreterrors.SecretNotFound or
// secreterrors.SecretRevisionNotFound. Permission errors MUST use
// secreterrors.PermissionDenied. Security-sensitive errors MUST NOT expose
// secret content in messages.
//
// >> Are these error type requirements part of the package contract, or
// >> implementation choices?
//
// **Immutability**: Secret URIs MUST NOT be modified after creation. Secret
// revisions MUST NOT be modified or deleted after creation -- they MAY only be
// marked as obsolete through expiry.
//
// >> Is this append-only behavior a guaranteed contract, or just current
// >> implementation?
//
// **Lifecycle**: SecretService MUST be initialized with valid State and
// SecretBackendState implementations before use. Backend providers MUST be
// initialized before content operations.
//
// >> Is initialization order a documented requirement or enforced by
// >> panic/error?
//
// **Security**: Secret values MUST NOT be accessed without verifying read
// permissions. Secret values MUST NOT be logged or included in error messages.
//
// >> Should this be stated as "service layer enforces grant checks" (current
// >> implementation) or as caller obligation? Is the logging prohibition a
// >> contract or best practice?
//
// **Transactions**: Callers MUST NOT hold transactions across multiple service
// calls to avoid deadlocks. Write operations MUST occur within database
// transactions.
//
// >> Is the transaction boundary guidance (no holding across calls) a
// >> documented contract or tribal knowledge?
//
// **Validation**: Secret URIs MUST be validated before storage operations.
// Rotation policies MUST use enumerated constants from secrets.RotatePolicy.
// Grant scopes MUST match the secret's ownership scope.
//
// >> Are these validation requirements enforced by the package, or must callers
// >> ensure validity?
package secret
