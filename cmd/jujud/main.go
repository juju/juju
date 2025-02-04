// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !debug

package main

import (
	"os"

	"github.com/juju/juju/internal/debug/coveruploader"
)

func main() {
	coveruploader.Enable()
	os.Exit(Main(os.Args))
}
