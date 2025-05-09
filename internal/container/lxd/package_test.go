// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"os"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

func Test(t *testing.T) {
	tc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/clock_mock.go github.com/juju/clock Clock

func TestMain(m *testing.M) {
	testhelpers.ExecHelperProcess()
	os.Exit(m.Run())
}
