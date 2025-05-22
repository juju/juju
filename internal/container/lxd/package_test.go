// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"os"
	stdtesting "testing"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/clock_mock.go github.com/juju/clock Clock

func TestMain(m *stdtesting.M) {
	testhelpers.ExecHelperProcess()
	os.Exit(m.Run())
}
