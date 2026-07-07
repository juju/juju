// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !debug

package main

import "os"

func main() {
	os.Exit(Main(os.Args))
}
