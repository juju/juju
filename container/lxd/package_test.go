// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"os"
	"testing"

	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/clock_mock.go github.com/juju/clock Clock

func TestMain(m *testing.M) {
	jujutesting.ExecHelperProcess()
	os.Exit(m.Run())
}
