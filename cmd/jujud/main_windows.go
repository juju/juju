// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"

	"code.google.com/p/winsvc/svc"

	"github.com/juju/juju/service/windows"
)

func main() {
	isInteractive, err := svc.IsAnInteractiveSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if isInteractive {
		Main(os.Args)
	}

	s := windows.NewSystemService("jujud", Main, os.Args)
	if err := s.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
