// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package testing provides test fixtures that run a Juju API server.
//
// ApiServerSuite spins up an API server on top of a controller model, so
// that tests can exercise the Juju API against real domain services.
// MacaroonSuite wraps it with macaroon authentication enabled.
//
// See github.com/juju/juju/internal/testing for the base suites for tests
// that do not need an API server.
package testing
