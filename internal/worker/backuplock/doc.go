// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package backuplock owns the controller-wide backup creation lease.
//
// Each creation request has a distinct lease holder. The worker renews the lease
// while the request builds its archive and cancels its context on lease loss.
//
// The API server owns the worker lifecycle. Callers release the lease after
// creation stops, before streaming the completed archive to the client.
package backuplock
