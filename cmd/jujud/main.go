// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !debug

package main

import "os"

func main() {
	MainWrapper(os.Args)
}

// MainWrapper exists to preserve test functionality.
func MainWrapper(args []string) {
	os.Exit(Main(args))
}
