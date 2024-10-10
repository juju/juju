package dlv

import (
	"fmt"
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

// Wrap wraps a MainWithArgs function to enable debugging.
//
// It works in two phases:
// At first call, it launches the delve command `exec` with the current binary. However, before the
// call it set a environment variable DELVE_ANYTHING_NO_DEBUG.
// At the second call, DELVE_ANYTHING_NO_DEBUG is set, so it just return the "normal" main.
func Wrap(opts ...Option) func(main MainWithArgs) MainWithArgs {
	return func(main MainWithArgs) MainWithArgs {
		if _, exists := os.LookupEnv(envNoDebug); exists {
			return main
		}
		if err := os.Setenv(envNoDebug, "1"); err != nil {
			fmt.Printf("Failed to set env %q: %v\n", envNoDebug, err)
			fmt.Println("Starting without debug mode...")
			return main
		}

		config := Config{}
		config.apply(opts...)

		// Run with delve
		return func(args []string) int {
			command := args[0]
			dlvArgs := append(config.args(), "exec", command, "--")
			dlvArgs = append(dlvArgs, args[1:]...)
			fmt.Printf("Starting dlv with %v\n", dlvArgs)

			dlvCmd := cmds.New(false)
			dlvCmd.SetArgs(dlvArgs)

			defer fmt.Println("dlv has stopped")

			fmt.Println("Running in debug mode")
			if err := dlvCmd.Execute(); err != nil {
				fmt.Printf("Failed to run dlv: %v\n", err)
				return 1
			}
			fmt.Printf("End debug mode\n")
			return 0
		}
	}
}
