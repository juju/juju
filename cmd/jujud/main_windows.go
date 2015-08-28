// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gabriel-samfira/sys/windows/svc"
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/cmd/service"
	"github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/osenv"
)

func init() {
	featureflag.SetFlagsFromRegistry(osenv.JujuRegistryKey, osenv.JujuFeatureFlagEnvKey)
}

func main() {
	isInteractive, err := svc.IsAnInteractiveSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	commandName := filepath.Base(os.Args[0])
	if isInteractive || commandName != names.Jujud {
		os.Exit(Main(os.Args))
	} else {
		s := service.SystemService{
			Name: "jujud",
			Cmd:  Main,
			Args: os.Args,
		}
		if err := s.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}
