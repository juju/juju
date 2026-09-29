// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package testing provides the base suites and helpers for writing Juju
// tests.
//
// A suite is a struct that provides specific setup and teardown behaviour
// (and useful variables and functions) for tests written with the tc
// package. The suites in this package are:
//
//   - BaseSuite (base.go), the base that test suites should embed. It
//     redirects the logger, protects the user's home directory, scrubs the
//     Juju environment variables and blocks outgoing network access. It is
//     composed of the CleanupSuite and LoggingSuite from
//     github.com/juju/juju/internal/testhelpers and the JujuOSEnvSuite
//     defined here;
//   - FakeJujuXDGDataHomeSuite (environ.go), which isolates the user's home
//     directory and sets up a Juju home with a sample environment and
//     certificate, for tests that need a fake client-side Juju environment.
//
// See github.com/juju/juju/internal/testhelpers for the lower-level suites
// these are built from, and github.com/juju/juju/juju/testing for the suite
// that runs an API server on top of a controller model.
package testing
