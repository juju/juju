// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"launchpad.net/juju-core/names"
	"regexp"
)

const (
	ContainerSnippet     = "(/[a-z]+/" + names.NumberSnippet + ")"
	ContainerSpecSnippet = "([a-z]+:)?"
)

var (
	validMachineOrNewContainer = regexp.MustCompile("^" + ContainerSpecSnippet + names.MachineSnippet + "$")
)

// IsMachineOrNewContainer returns whether spec is a valid machine id
// or new container definition.
func IsMachineOrNewContainer(spec string) bool {
	return validMachineOrNewContainer.MatchString(spec)
}
