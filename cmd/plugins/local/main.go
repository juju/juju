// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"os"
	"os/exec"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/cmd"
)

var logger = loggo.GetLogger("juju.plugins.local")

const localDoc = `

Juju local is used to provide extra commands that assist with the local
provider. 

See Also:
    juju help local-provider
`

func jujuLocalPlugin() cmd.Command {
	plugin := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "juju local",
		UsagePrefix: "juju",
		Doc:         localDoc,
		Purpose:     "local provider specific commands",
		Log:         &cmd.Log{},
	})

	return plugin
}

// Main registers subcommands for the juju-local executable.
func Main(args []string) {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		logger.Debugf("error: %v\n", err)
		os.Exit(2)
	}
	plugin := jujuLocalPlugin()
	os.Exit(cmd.Main(plugin, ctx, args[1:]))
}

var checkIfRoot = func() bool {
	return os.Getuid() == 0
}

// runAsRoot ensures that the executable is running as root.
// If checkIfRoot returns true, the call function is called,
// otherwise executable is executed using sudo and the extra args
// passed through.
func runAsRoot(executable string, args []string, context *cmd.Context, call func(*cmd.Context) error) error {
	if checkIfRoot() {
		logger.Debugf("running as root")
		return call(context)
	}

	logger.Debugf("running as user")

	fullpath, err := exec.LookPath(executable)
	if err != nil {
		return err
	}

	sudoArgs := []string{"--preserve-env", fullpath}
	sudoArgs = append(sudoArgs, args...)

	command := exec.Command("sudo", sudoArgs...)
	// Now hook up stdin, stdout, stderr
	command.Stdin = context.Stdin
	command.Stdout = context.Stdout
	command.Stderr = context.Stderr
	// And run it!
	return command.Run()
}
