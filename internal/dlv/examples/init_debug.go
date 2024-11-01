// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build debug

package main

import (
	"github.com/juju/juju/internal/dlv"
	"github.com/juju/juju/internal/dlv/config"
)

func init() {
	runMain = dlv.NewDlvRunner(config.Default(),
		dlv.WithPort(1122),
		dlv.WaitDebugger())(runMain)
}
