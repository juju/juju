// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package testhelpers provides the general-purpose building blocks for Juju
// test suites.
//
// A suite is a struct that provides specific setup and teardown behaviour
// (and useful variables and functions) for tests written with the tc
// package. The suites in this package include:
//
//   - CleanupSuite, which adds the ability to register cleanup functions
//     that are called at test or suite teardown, and provides the patching
//     of variables and environment variables on top of that;
//   - LoggingSuite, which redirects the Juju logger to the test logger;
//   - OsEnvSuite, which resets the environment variables in SetUpTest and
//     restores them in TearDownTest;
//   - FakeHomeSuite, which composes CleanupSuite and LoggingSuite and sets
//     up a fake home directory before running tests.
//
// See github.com/juju/juju/internal/testing for the base suite that composes
// these with Juju-specific isolation, and github.com/juju/juju/juju/testing
// for suites that run an API server.
package testhelpers
