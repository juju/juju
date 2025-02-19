// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package resources contains API calls for functionality related to listing
// and indicating which resource an application should use.
//
// AddPendingResource handles 2 main scenarios:
//  1. A resource is provided to juju before the application exists. With the
//     current juju cli, this can happen during bundle or local charm
//     deployment.
//  2. A resource needs to be updated by running the `juju refresh` command.
//
// In juju 4.x, AddPendingResource returns the resource UUID for use by the
// juju client to optionally provide to the Upload method. In prior juju
// versions, a pending resource UUID was return and later reconciled in
// mongo when the application was created or during upload.
//
// The actual upload of a resource from the juju cli for deploy, refresh or
// attach-storage is done via the apiserver/internal/handlers/resources
// package.
package resources
