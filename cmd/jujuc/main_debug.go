// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build debug

package main

import (
	"github.com/juju/juju/internal/dlv"

	"github.com/juju/juju/cmd/juju/commands"
)

func init() {
	commands.Main = dlv.Wrap(dlv.WithDefault(),
		dlv.WithPort(10121),
	)(commands.Main)
}
