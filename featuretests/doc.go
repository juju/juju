// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*

This package contains functional tests only. Each feature-set of
Juju should have it's own file in which all the happy-paths are
tested.

Rules:

1) Do NOT mirror the architecture/namespaces of Juju. This should be a
very flat folder.

2) Whenever possible, do not mock anything. The goal is to test the
entire stack as well as can be done within a test suite.

3) Avoid writing tests that do not specifically address a user-facing
feature. The place for that is in unit tests within Juju Core.

4) If a feature test fails in CI, unit test coverage should be improved to
reproduce the failure WITH mocks and WITHOUT a full stack setup before landing
the branch.

These tests are run by the CI landing bot, but disabled by default, because
they are integration tests, not unit tests. They are an important safety net
that MUST be run by CI, but they are too long-running and resource intensive
for development.

To enable CI tests in your development environment, including the functional
tests in this package, set the environment variable:

	JUJU_FEATURE_TESTS=1 go test github.com/juju/juju/...

or, run `make featurecheck`.

*/
package featuretests
