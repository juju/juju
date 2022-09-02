// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	stdtesting "testing"

	_ "github.com/juju/juju/secrets/provider/all"
	coretesting "github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/hookunit_mock.go github.com/juju/juju/worker/uniter/runner/context HookUnit
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/worker/uniter/runner/context LeadershipContext

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
