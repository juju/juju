// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"sort"

	"github.com/juju/juju/worker/common/hooks"
)

// CmdSuffix is the filename suffix to use for executables.
const cmdSuffix = ""

var registeredCommands = map[string]hooks.NewCommandFunc{}

// baseCommands maps Command names to creators.
var baseCommands = map[string]hooks.NewCommandFunc{
	"config-get" + cmdSuffix:              nil,
	"juju-log" + cmdSuffix:                nil,
	"status-get" + cmdSuffix:              hooks.NewStatusGetCommand,
	"status-set" + cmdSuffix:              hooks.NewStatusSetCommand,
	"application-version-set" + cmdSuffix: nil,
	"relation-ids" + cmdSuffix:            nil,
	"relation-list" + cmdSuffix:           nil,
	"relation-set" + cmdSuffix:            nil,
	"relation-get" + cmdSuffix:            nil,
	"container-spec-set" + cmdSuffix:      nil,
}

func allEnabledCommands() map[string]hooks.NewCommandFunc {
	all := map[string]hooks.NewCommandFunc{}
	add := func(m map[string]hooks.NewCommandFunc) {
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
