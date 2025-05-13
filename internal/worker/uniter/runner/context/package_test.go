// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/internal/worker/uniter/runner/context LeadershipContext

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
