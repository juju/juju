//go:build debug
// +build debug

package main

import (
	"github.com/juju/juju/internal/dlv"
	"github.com/juju/juju/internal/dlv/config"
)

func init() {
	runMain = dlv.Wrap(config.Default(),
		dlv.WithPort(1122),
		dlv.WaitDebugger())(runMain)
}
