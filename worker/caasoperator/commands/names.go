// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"sort"
)

// CmdSuffix is the filename suffix to use for executables.
const cmdSuffix = ""

type command interface{}

var registeredCommands = map[string]command{}

// baseCommands maps Command names to creators.
var baseCommands = map[string]command{
	"config-get" + cmdSuffix:              nil,
	"juju-log" + cmdSuffix:                nil,
	"status-get" + cmdSuffix:              nil,
	"status-set" + cmdSuffix:              nil,
	"application-version-set" + cmdSuffix: nil,
	"relation-ids" + cmdSuffix:            nil,
	"relation-list" + cmdSuffix:           nil,
	"relation-set" + cmdSuffix:            nil,
	"relation-get" + cmdSuffix:            nil,
	"container-spec-set" + cmdSuffix:      nil,
}

func allEnabledCommands() map[string]command {
	all := map[string]command{}
	add := func(m map[string]command) {
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
