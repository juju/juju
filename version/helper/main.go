// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"github.com/juju/juju/version"
)

// main quick and dirty utility for printing out the Juju version for use in
// Makefiles and scripts.
func main() {
	fmt.Println(version.Current)
}
