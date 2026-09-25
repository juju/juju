// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package testhelpers provides the common building blocks for Juju
// test suites: CleanupSuite (patching of environment and ordinary
// variables for the duration of a test, plus a clean-up stack called
// when the test teardown happens), LoggingSuite (logger redirect so
// test logging does not pollute the output), and FakeHomeSuite (which
// isolates the user's home directory from accidental modification by
// installing a fake home with fake ssh keys). The canonical base that
// composes these is internal/testing.BaseSuite.
package testhelpers
