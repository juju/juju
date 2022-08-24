// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/factory_mock.go github.com/juju/juju/worker/uniter/runner Factory,Runner
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/worker/uniter/runner/context Context

func TestPackage(t *stdtesting.T) {
	// TODO(fwereade): there's no good reason for this test to use mongo.
	coretesting.MgoTestPackage(t)
}
