// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package tracing holds charm and workload tracing configuration. This
// configuration is used to configure OTEL collectors for Juju's distributed
// tracing. Charms and workloads get OTEL collector endpoint and certificate
// information through their respective integration paths.
//
// For more information about tracing in Juju see the following:
// - github.com/juju/juju/core/trace
// - github.com/juju/juju/internal/worker/trace
package tracing
