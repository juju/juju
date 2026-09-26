// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package testing provides test fixtures that simulate a Juju
// environment: ApiServerSuite spins up an apiserver on top of a
// controller model (with the domain services that implies), so tests
// can exercise a running Juju API. It is heavyweight — prefer one of
// the base suites in internal/testing unless a real API server is
// needed.
package testing
