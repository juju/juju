// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build debug

package main

import (
	"github.com/juju/juju/internal/dlv"
)

func init() {
	runMain = dlv.NewDlvRunner(
		dlv.Headless(),
		dlv.WithApiVersion(2),
		dlv.WithPort(1122),
		dlv.WaitDebugger())(runMain)
}
