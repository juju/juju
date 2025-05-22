// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent // not agent_test for no good reason

import (
	"os"
	"testing"

	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/machine_mock.go github.com/juju/juju/cmd/jujud/agent CommandRunner

func TestMain(m *testing.M) {
	os.Exit(func() int {
		defer coretesting.MgoSSLTestMain()()
		return m.Run()
	}())
}
