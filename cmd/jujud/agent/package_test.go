// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent // not agent_test for no good reason

import (
	"os"
	"testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/machine_mock.go github.com/juju/juju/cmd/jujud-controller/agent CommandRunner

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
