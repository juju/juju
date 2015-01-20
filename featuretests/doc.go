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

To run tests excluding the functional tests in this package, specify
the "--featuretests=false" option:
	go test github.com/juju/juju/... --featuretests=false
*/
package featuretests
