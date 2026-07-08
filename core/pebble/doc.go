// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package pebble provides shared constants for Pebble-related paths and
// service names used across Juju layers (workers, commands, and providers).
//
// These constants live in core/ so that both cmd/ and internal/worker/ can
// reference them without creating cross-layer dependencies.
package pebble
