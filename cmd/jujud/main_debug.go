// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build debug

package main

import (
	"github.com/juju/juju/internal/dlv"
)

func init() {
	Main = dlv.Wrap(dlv.WithDefault(),
		dlv.WithPort(10122),
	)(Main)
}
