// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"os"
	"testing"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/clock_mock.go github.com/juju/clock Clock

func TestMain(m *testing.M) {
	testhelpers.ExecHelperProcess()
	os.Exit(m.Run())
}
