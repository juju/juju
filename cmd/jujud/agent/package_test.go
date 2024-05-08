// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent // not agent_test for no good reason

import (
	stdtesting "testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/machine_mock.go github.com/juju/juju/cmd/jujud/agent CommandRunner

func TestPackage(t *stdtesting.T) {
	// TODO(waigani) 2014-03-19 bug 1294458
	// Refactor to use base suites
	coretesting.MgoSSLTestPackage(t)
}
