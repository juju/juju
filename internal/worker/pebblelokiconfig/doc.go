// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package pebblelokiconfig reconciles a Pebble log-forwarding layer for
// the containeragent service. It watches the controller-wide Loki
// configuration via the agent logger API and, on every change, pushes a
// dedicated layer (label juju-loki-log-forwarding) to the local Pebble
// daemon using AddLayer(combine=true). The layer contains a single
// log-targets entry of type loki pointing at the controller Loki push URL
// and forwarding logs for the container-agent service only.
//
// The worker is scoped to the containeragent service: it does not touch
// workload sidecars or couple to the model operator. Transient Pebble
// errors (socket not found, temporary connection failures) are retried
// with exponential backoff. Permanent Pebble incompatibility — for example
// a 400 response indicating an unknown "log-targets" section — causes the
// worker to stay on the direct fallback path and stop attempting further
// updates.
package pebblelokiconfig
