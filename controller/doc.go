// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controller provides controller configuration for Juju.
//
// This configuration defines operational settings for a Juju controller (API
// configuration, auditing, rate limiting, logging, etc.). Configuration is
// stored as key-value pairs validated against a schema, with keys defined as
// constants (APIPort, AuditingEnabled, AgentRateLimitMax, etc.) and values
// coerced to appropriate types (integers, durations, strings, etc.). Controllers
// use this configuration to control runtime behavior across all models they
// manage.
//
// See github.com/juju/juju/apiserver for how controllers use this
// configuration. See github.com/juju/juju/cmd/juju/controller for CLI commands
// that manipulate controller configuration.
package controller
