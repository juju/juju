// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/factory_mock.go github.com/juju/juju/internal/worker/uniter/runner Factory,Runner
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/context_mock.go github.com/juju/juju/internal/worker/uniter/runner/context Context

func TestPackage(t *stdtesting.T) {
	// TODO(fwereade): there's no good reason for this test to use mongo.
	tc.TestingT(t)
}
