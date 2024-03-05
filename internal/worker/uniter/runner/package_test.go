// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/factory_mock.go github.com/juju/juju/internal/worker/uniter/runner Factory,Runner
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/internal/worker/uniter/runner/context Context

func TestPackage(t *stdtesting.T) {
	// TODO(fwereade): there's no good reason for this test to use mongo.
	gc.TestingT(t)
}
