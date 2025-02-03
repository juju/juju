// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dlv

import (
	"context"
	"os"

	"github.com/go-delve/delve/cmd/dlv/cmds"
)

// MainWithArgs is a type representing a function that takes command-line arguments as input and returns an exit code.
type MainWithArgs func(args []string) int

const (
	// envNoDebug is an environment variable used to redirect the control flow to a new delve instance
	// or to the program. It is set as environment variable the first time the flow pass in the function
	envNoDebug = "JUJU_NO_DLV_DEBUG"
)

// NewDlvRunner wraps a MainWithArgs function to enable debugging.
//
// It works in two phases:
// At first call, it launches the delve command `exec` with the current binary. However, before the
// call it sets an environment variable DELVE_ANYTHING_NO_DEBUG.
// At the second call, DELVE_ANYTHING_NO_DEBUG is set, so it just returns the "normal" main.
func NewDlvRunner(opts ...Option) func(main MainWithArgs) MainWithArgs {
	config := Config{}
	config.apply(opts...)

	logger := config.logger()

	return func(main MainWithArgs) MainWithArgs {
		if _, exists := os.LookupEnv(envNoDebug); exists {
			return main
		}
		if err := os.Setenv(envNoDebug, "1"); err != nil {
			logger.Warningf(context.TODO(), "Failed to set env %q: %v", envNoDebug, err)
			logger.Warningf(context.TODO(), "Starting without debug mode...")
			return main
		}

		// Run with delve
		return func(args []string) int {

			// Prepare dlv commands with expected options and command
			command := args[0]
			dlvArgs := append(config.args(), "exec", command, "--")
			dlvArgs = append(dlvArgs, args[1:]...)
			dlvCmd := cmds.New(false)
			dlvCmd.SetArgs(dlvArgs)

			// Start "sidecars" (go routine required to make some commands works... Like fixing permissions on listening
			// socket
			config.runSidecars()

			logger.Infof(context.TODO(), "Starting dlv with %v", dlvArgs)
			logger.Infof(context.TODO(), "Running in debug mode")
			defer logger.Infof(context.TODO(), "dlv has stopped")

			// Execute delve
			if err := dlvCmd.Execute(); err != nil {
				logger.Errorf(context.TODO(), "Failed to run dlv: %v\n", err)
				return 1
			}
			return 0
		}
	}
}
