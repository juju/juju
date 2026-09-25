// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package testing provides base suites and helpers for writing tests
// in Juju. A suite is a struct that provides specific setup and
// teardown behavior (and useful variables and functions) for a test
// suite written with the tc package.
//
// The suites in this package are:
//
//   - BaseSuite (base.go): the base that all test suites should embed.
//     It redirects the logger, isolates the user's home directory
//     against accidental modification, scrubs the Juju environment
//     variables (via JujuOSEnvSuite) and blocks outgoing network
//     access. It is composed of the LoggingSuite and the CleanupSuite
//     from internal/testhelpers; the CleanupSuite provides the
//     patching of environment and ordinary variables for the duration
//     of a test, and a clean-up stack called at teardown.
//   - FakeJujuXDGDataHomeSuite (environ.go): a BaseSuite that also
//     sets up a fake Juju home directory ($JUJU_DATA/$XDG_DATA_HOME
//     pointing at a FakeHome with a fake ~/.config/juju), for tests
//     that need a fake client-side Juju environment.
//
// Suites providing a fake home directory live in
// internal/testhelpers (FakeHomeSuite and the FakeHome type it
// installs); the API-server-backed suite for tests that need a
// controller lives in juju/testing (ApiServerSuite).
package testing
