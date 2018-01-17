// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"sort"

	"github.com/juju/cmd"
)

type creator func(Context) (cmd.Command, error)

var registeredCommands = map[string]creator{}

// baseCommands maps Command names to creators.
var baseCommands = map[string]creator{
	"config-get":              NewConfigGetCommand,
	"juju-log":                NewJujuLogCommand,
	"status-get":              nil,
	"status-set":              NewStatusSetCommand,
	"application-version-set": nil,
	"relation-ids":            nil,
	"relation-list":           nil,
	"relation-set":            nil,
	"relation-get":            nil,
	"container-spec-set":      NewContainerspecSetCommand,
}

func allEnabledCommands() map[string]creator {
	all := map[string]creator{}
	add := func(m map[string]creator) {
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
