// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package externalcontrollerupdater implements a worker that keeps
// the local controller's cached records of peer controllers current.
//
// Cross-model relations operate across models that may belong to different
// controllers. When a local model consumes a remote offer, the local
// controller stores a record -- an external controller record -- containing
// the remote controller's UUID, alias, API addresses, and CA certificate.
// These records let the local controller route cross-model traffic to the
// correct peer without requiring the operator to re-enter connection details
// after addresses change.
//
// The external-controller-updater worker watches local external controller
// records for additions and removals, then connects to each peer controller
// directly and watches for published address changes. When a peer reports new
// addresses, the worker updates the local cached record so that future
// cross-model API calls use the current addresses. Local state is accessed
// via the controller domain external-controller service (consumed through the
// domain-services manifold); peer contact uses direct API connections.
//
// Address changes can only be discovered automatically while some working path
// to the peer controller still exists -- either a live watch connection or a
// reachable redirect address. If all cached addresses become stale with no
// surviving path, the worker cannot recover on its own and operator
// intervention is required to update the stale record.
//
// See github.com/juju/juju/domain/externalcontroller for the external
// controller domain service that owns the persisted records. See
// github.com/juju/juju/core/crossmodel for the ControllerInfo type that
// represents an external controller record.
package externalcontrollerupdater
