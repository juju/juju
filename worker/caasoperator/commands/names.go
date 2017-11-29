// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"sort"

	"github.com/juju/juju/worker/common/hookcommands"
)

// CmdSuffix is the filename suffix to use for executables.
const cmdSuffix = ""

var registeredCommands = map[string]hookcommands.NewCommandFunc{}

// baseCommands maps Command names to creators.
var baseCommands = map[string]hookcommands.NewCommandFunc{
	"config-get" + cmdSuffix:              nil,
	"juju-log" + cmdSuffix:                nil,
	"status-get" + cmdSuffix:              hookcommands.NewStatusGetCommand,
	"status-set" + cmdSuffix:              hookcommands.NewStatusSetCommand,
	"application-version-set" + cmdSuffix: nil,
	"relation-ids" + cmdSuffix:            nil,
	"relation-list" + cmdSuffix:           nil,
	"relation-set" + cmdSuffix:            nil,
	"relation-get" + cmdSuffix:            nil,
	"container-spec-set" + cmdSuffix:      nil,
}

func allEnabledCommands() map[string]hookcommands.NewCommandFunc {
	all := map[string]hookcommands.NewCommandFunc{}
	add := func(m map[string]hookcommands.NewCommandFunc) {
		for k, v := range m {
			all[k] = v
		}
	}
	add(baseCommands)
	add(registeredCommands)
	return all
}

// CommandNames returns the names of all hook commands.
func CommandNames() (names []string) {
	for name := range allEnabledCommands() {
		names = append(names, name)
	}
	sort.Strings(names)
	return
}
