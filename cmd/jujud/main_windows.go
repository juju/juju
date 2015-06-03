// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"code.google.com/p/winsvc/svc"

	"github.com/juju/juju/cmd/service"
	"github.com/juju/juju/juju/names"
)

func main() {
	isInteractive, err := svc.IsAnInteractiveSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	commandName := filepath.Base(os.Args[0])
	if isInteractive || commandName != names.Jujud {
		Main(os.Args)
	} else {
		s := service.NewSystemService("jujud", Main, os.Args)
		if err := s.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}
