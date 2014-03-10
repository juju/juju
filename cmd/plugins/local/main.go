// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"os"

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
	plugin := jujuLocalPlugin()
	os.Exit(cmd.Main(plugin, cmd.DefaultContext(), args[1:]))
}
