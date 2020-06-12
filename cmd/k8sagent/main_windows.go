// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/featureflag"
	"golang.org/x/sys/windows/svc"

	"github.com/juju/juju/cmd/service"
	"github.com/juju/juju/juju/names"
	"github.com/juju/juju/juju/osenv"
)

func init() {
	// If feature flags have been set on env, use them instead.
	if os.Getenv(osenv.JujuFeatureFlagEnvKey) != "" {
		featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	} else {
		featureflag.SetFlagsFromRegistry(osenv.JujuRegistryKey, osenv.JujuFeatureFlagEnvKey)
	}
}

func main() {
	isInteractive, err := svc.IsAnInteractiveSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	commandName := filepath.Base(os.Args[0])
	if isInteractive || commandName != names.K8sagent {
		os.Exit(mainWrapper(os.Args))
	} else {
		s := service.SystemService{
			Name: names.K8sagent,
			Cmd:  mainWrapper,
			Args: os.Args,
		}
		if err := s.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}
