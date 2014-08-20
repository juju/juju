// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.
// +build !windows

package main

import (
	"os"

	// this is here to make godeps output the same on all OSes.
	_ "bitbucket.org/kardianos/service"
)

func main() {
	Main(os.Args)
}
